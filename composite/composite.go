// Package composite 做多层半透明叠合，并给出定点舍入误差的累积式、
// 每层误差预算的反解、反解方案与等分方案的对照。
//
// 颜色以 [0,1] 归一；误差一律以“档”计：一档 = 8 位输出相邻两个可表示值
// 之间的间隔，即 1/255（Level）。
//
// # 累积式
//
// 叠合从底到顶按 over 递推。第 i 层精确值为
//
//	C_i = (1−α_i)·C_{i−1} + α_i·c_i
//
// 但每层都把累积色舍到定点格点上，存下来的是 Ĉ_i = C_i + e_i，于是
//
//	e_0 = 0
//	e_i = (1−α_i)·e_{i−1} + δ_i          （δ_i 为第 i 层的舍入误差）
//
// 展开后得到最终误差的累积式：
//
//	e_N = Σ_{i=1..N} w_i·δ_i ,   w_i = ∏_{j=i+1..N}(1−α_j) ,   w_N = 1
//
// 后层按 (1−α) 衰减前层的误差；顶层不衰减，直接落到最终颜色上。
// 若每层舍入误差有上界 |δ_i| ≤ ε_i，则
//
//	|e_N| ≤ Σ w_i·ε_i  ≤  一档
//
// 反解即在这条不等式下给每层分配 ε_i。
package composite

import (
	"errors"
	"fmt"
	"math"
)

// Level 是一档灰度在颜色空间中的宽度（8 位输出的相邻间隔）。
// 注意：误差量（ε_i、δ_i、Budget）都以“档”为单位，一档 = 1.0；把颜色差
// 折算成档要乘 255。
const Level = 1.0 / 255.0

// Budget 是“不超一档”的预算，单位为档。
const Budget = 1.0

// MaxGuard 是定点格能用的最大保护小数位数。
const MaxGuard = 20

var (
	// ErrAlpha 表示 α 不在 [0,1]。
	ErrAlpha = errors.New("透明度超出 [0,1]")
	// ErrColor 表示颜色不在 [0,1]。
	ErrColor = errors.New("颜色超出 [0,1]")
	// ErrBudget 表示误差预算非正或非有限。
	ErrBudget = errors.New("误差预算非法")
	// ErrNoLayers 表示层数为零。
	ErrNoLayers = errors.New("层数为零")
	// ErrShape 表示图层数与方案数不一致。
	ErrShape = errors.New("层数与方案长度不一致")
	// ErrTight 表示该误差预算在定点阶梯里无解。
	ErrTight = errors.New("误差预算过紧，定点阶梯内无解")
	// ErrRoundMode 表示舍入方式或保护位非法。
	ErrRoundMode = errors.New("未知舍入方式")
)

// Mode 是一层的定点舍入方式。
type Mode int

const (
	// Truncate 向零（向下）截断，即 uint8(x) 的语义；单层误差 |δ| ≤ 1 个格距。
	Truncate Mode = iota
	// Nearest 就近舍入，恰为半格时向下；单层误差 |δ| ≤ 半个格距。
	Nearest
)

func (m Mode) String() string {
	switch m {
	case Truncate:
		return "截断"
	case Nearest:
		return "就近"
	}
	return "?"
}

// Scheme 是一层的定点格式：格距 2^-Guard 档（Guard 为保护小数位数），
// 按 Mode 舍入。Guard=0 即落在 8 位输出格上。
type Scheme struct {
	Mode  Mode
	Guard int
}

// Quantum 返回格距（档）。
func (s Scheme) Quantum() float64 { return math.Pow(2, -float64(s.Guard)) }

// Err 返回该格式下单层最大舍入误差（档）。
func (s Scheme) Err() float64 {
	q := s.Quantum()
	if s.Mode == Truncate {
		return q
	}
	return q / 2
}

// Valid 报告格式是否合法。
func (s Scheme) Valid() bool {
	return (s.Mode == Truncate || s.Mode == Nearest) && s.Guard >= 0 && s.Guard <= MaxGuard
}

func (s Scheme) String() string { return fmt.Sprintf("%s g%d", s.Mode, s.Guard) }

// Weights 返回衰减系数 w_i = ∏_{j>i}(1−α_j)；最后一层为 1。
// 顶层（最后一层）的误差不经任何衰减，直接体现在最终颜色上。
func Weights(alphas []float64) []float64 {
	w := make([]float64, len(alphas))
	acc := 1.0
	for i := len(alphas) - 1; i >= 0; i-- {
		w[i] = acc
		acc *= 1 - alphas[i]
	}
	return w
}

func checkAlphas(alphas []float64) error {
	if len(alphas) == 0 {
		return ErrNoLayers
	}
	for i, a := range alphas {
		if math.IsNaN(a) || a < 0 || a > 1 {
			return fmt.Errorf("%w: 第 %d 层 α=%v", ErrAlpha, i+1, a)
		}
	}
	return nil
}

// Allowed 反解每层可分配的最大误差（档）。
//
// 把总预算 B 先按层数等额切成终值份额：每层占终值 B/N；再按该层的衰减
// 把终值份额换算回它自己的存储误差，得到
//
//	ε_i* = B / (N · w_i)
//
// 各层取满时 Σ w_i·ε_i* = B，即预算恰好用尽。被上面多层盖住的层（w_i 小）
// 的误差会被大幅衰减，因而可以粗得多；顶层 w_N=1，拿到最紧的 B/N。
// w_i=0 的层被完全遮住，不参与终值，上界为 +∞。
func Allowed(alphas []float64, budget float64) ([]float64, error) {
	if err := checkAlphas(alphas); err != nil {
		return nil, err
	}
	if math.IsNaN(budget) || math.IsInf(budget, 0) || budget <= 0 {
		return nil, fmt.Errorf("%w: %v", ErrBudget, budget)
	}
	w := Weights(alphas)
	out := make([]float64, len(alphas))
	for i := range out {
		if w[i] <= 0 {
			out[i] = math.Inf(1)
			continue
		}
		out[i] = budget / (float64(len(alphas)) * w[i])
	}
	return out, nil
}

// Realize 从定点阶梯里挑最省的一档：误差不超过 budget，且格距尽量粗。
// 阶梯含两族（截断与就近），保护位从 0 到 MaxGuard。
func Realize(budget float64) (Scheme, error) {
	if math.IsNaN(budget) || budget <= 0 {
		return Scheme{}, fmt.Errorf("%w: %v", ErrBudget, budget)
	}
	best := Scheme{}
	bestErr := -1.0
	found := false
	for g := 0; g <= MaxGuard; g++ {
		for _, m := range []Mode{Nearest, Truncate} {
			s := Scheme{Mode: m, Guard: g}
			e := s.Err()
			if e > budget*(1+1e-12) {
				continue
			}
			// 误差更大者更省；误差相同时取格距更粗（保护位更少）者。
			if !found || e > bestErr || (e == bestErr && s.Quantum() > best.Quantum()) {
				best, bestErr, found = s, e, true
			}
		}
	}
	if !found {
		return Scheme{}, fmt.Errorf("%w: 需要单层误差 < %g 档", ErrTight, math.Pow(2, -float64(MaxGuard)))
	}
	return best, nil
}

// Plan 对每个 α 反解误差上界并兑现成具体格式，返回逐层方案。
func Plan(alphas []float64, budget float64) ([]Scheme, error) {
	eps, err := Allowed(alphas, budget)
	if err != nil {
		return nil, err
	}
	out := make([]Scheme, len(alphas))
	for i, e := range eps {
		if math.IsInf(e, 1) {
			// 完全被遮住的层对终值无贡献，用最粗格式即可。
			out[i] = Scheme{Mode: Truncate, Guard: 0}
			continue
		}
		s, err := Realize(e)
		if err != nil {
			return nil, fmt.Errorf("第 %d 层: %w", i+1, err)
		}
		out[i] = s
	}
	return out, nil
}

// Bound 返回保证的终值误差上界 Σ w_i·Err(s_i)（档）。
func Bound(alphas []float64, schemes []Scheme) (float64, error) {
	if err := checkAlphas(alphas); err != nil {
		return 0, err
	}
	if len(schemes) != len(alphas) {
		return 0, fmt.Errorf("%w: %d 层 vs %d 方案", ErrShape, len(alphas), len(schemes))
	}
	w := Weights(alphas)
	b := 0.0
	for i, s := range schemes {
		if !s.Valid() {
			return 0, fmt.Errorf("%w: 第 %d 层 %v", ErrRoundMode, i+1, s)
		}
		b += w[i] * s.Err()
	}
	return b, nil
}

// PrefixBound 返回在前 k 层（1..k）上、按最坏情况逐层传播得到的误差上界
// |e_k| ≤ ε_k + (1−α_k)·|e_{k−1}|。它是累积式 e_k = Σ_{i≤k} w_i^(k)·δ_i 的
// 等价递推写法（w_i^(k) = ∏_{i<j≤k}(1−α_j)），其中最后一层的权重恒为 1。
//
// 用途是核对“总误差的界要对最后一层验一次”：k=N 时 PrefixBound 必须
// 等于 Bound（闭式 Σ w_i·ε_i），也必须等于逐层实跑的 |e_k| 上界。
func PrefixBound(alphas []float64, schemes []Scheme, k int) (float64, error) {
	if err := checkAlphas(alphas); err != nil {
		return 0, err
	}
	if len(schemes) != len(alphas) {
		return 0, fmt.Errorf("%w: %d 层 vs %d 方案", ErrShape, len(alphas), len(schemes))
	}
	if k < 1 || k > len(alphas) {
		return 0, fmt.Errorf("层号 %d 超出 1..%d", k, len(alphas))
	}
	e := 0.0
	for i := 0; i < k; i++ {
		if !schemes[i].Valid() {
			return 0, fmt.Errorf("%w: 第 %d 层 %v", ErrRoundMode, i+1, schemes[i])
		}
		e = schemes[i].Err() + (1-alphas[i])*e
	}
	return e, nil
}

// Layer 是一层半透明色。
type Layer struct {
	Color float64 // 0..1
	Alpha float64 // 0..1
}

// Result 是一次定点叠合的产出。
type Result struct {
	Top       float64   // 顶层颜色（定点存储值）
	Exact     float64   // 不做舍入的参考值
	Deltas    []float64 // 每层舍入误差 δ_i（档），从底到顶
	Deviation float64   // (Top − Exact) 折算成档
}

// Exact 是不做任何定点舍入的参考叠合。
func Exact(layers []Layer) float64 {
	c := 0.0
	for _, l := range layers {
		c = (1-l.Alpha)*c + l.Alpha*l.Color
	}
	return c
}

func roundTo(x float64, s Scheme) float64 {
	q := math.Pow(2, -float64(s.Guard)) // 格距（档）
	y := x * 255.0 / q
	// 整数格点上的二进制浮点可能略低于整数，先吸附，避免假截断。
	if f := math.Round(y); math.Abs(y-f) < 1e-9 {
		y = f
	}
	k := math.Floor(y)
	if s.Mode == Nearest && y-k > 0.5 {
		k++
	}
	if k < 0 {
		k = 0
	}
	if hi := 255.0 / q; k > hi {
		k = hi
	}
	return k * q / 255.0
}

// OverFixed 按给定格式逐层叠合，返回顶层颜色与每层舍入误差 δ_i（档）。
// δ_i = (舍入后 − 舍入前)·255；上一层的误差已含在“舍入前”里，所以 δ_i 与
// 累积式中的 δ_i 一一对应，且 Deviation = Σ w_i·δ_i（由构造保证）。
func OverFixed(layers []Layer, schemes []Scheme) (Result, error) {
	if len(layers) != len(schemes) {
		return Result{}, fmt.Errorf("%w: %d 层 vs %d 方案", ErrShape, len(layers), len(schemes))
	}
	for i, l := range layers {
		if math.IsNaN(l.Alpha) || l.Alpha < 0 || l.Alpha > 1 {
			return Result{}, fmt.Errorf("%w: 第 %d 层 α=%v", ErrAlpha, i+1, l.Alpha)
		}
		if math.IsNaN(l.Color) || l.Color < 0 || l.Color > 1 {
			return Result{}, fmt.Errorf("%w: 第 %d 层颜色=%v", ErrColor, i+1, l.Color)
		}
		if !schemes[i].Valid() {
			return Result{}, fmt.Errorf("%w: 第 %d 层 %v", ErrRoundMode, i+1, schemes[i])
		}
	}
	c := 0.0
	res := Result{Deltas: make([]float64, len(layers))}
	for i, l := range layers {
		before := (1-l.Alpha)*c + l.Alpha*l.Color
		c = roundTo(before, schemes[i])
		res.Deltas[i] = (c - before) * 255.0
	}
	res.Top = c
	res.Exact = Exact(layers)
	res.Deviation = (c - res.Exact) * 255.0
	return res, nil
}
