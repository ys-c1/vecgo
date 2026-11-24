package light

import (
	"errors"
	"math"
)

// 默认收敛参数。判据是“残差 + 步长”双条件：
//
//	|F(w_k)| ≤ TolResidual
//	|w_{k+1}-w_k| ≤ TolStep*(1+|w_k|)（或 Newton 步在浮点上已归零）
//
// 两者同时满足才判收敛；最多迭代 MaxIter 次。
const (
	DefaultTolResidual = 1e-13
	DefaultTolStep     = 1e-14
	DefaultMaxIter     = 50
)

// Options 控制感知守恒权重的非线性求解。零值会被 WithDefaults 补全。
type Options struct {
	TolResidual float64
	TolStep     float64
	MaxIter     int
	Trace       bool // 为 true 时在 Result.Trace 里保留逐轮迭代记录
}

// Iteration 是一轮 Newton（或二分保护步）的可核查记录。
type Iteration struct {
	K       int     `json:"k"`
	W       float64 `json:"w"`
	AbsF    float64 `json:"abs_f"` // 该轮判定点上的 |F(w)|
	Step    float64 `json:"step"`  // 本轮将采取的 |w_next-w|
	Guarded bool    `json:"guarded"`
}

// WithDefaults 把零值字段替换为默认收敛参数。
func (o Options) WithDefaults() Options {
	if o.TolResidual <= 0 {
		o.TolResidual = DefaultTolResidual
	}
	if o.TolStep <= 0 {
		o.TolStep = DefaultTolStep
	}
	if o.MaxIter <= 0 {
		o.MaxIter = DefaultMaxIter
	}
	return o
}

// Result 是一组光源强度在两种归一口径下的完整对照与求解记录。
type Result struct {
	N         int       `json:"n"`
	Intensity []float64 `json:"intensity"`

	// 线性强度和 S=ΣI_i，以及感知守恒目标 T=ΣP(I_i)。
	SumLinear       float64 `json:"sum_linear"`
	TargetPerceived float64 `json:"target_perceived"`
	TargetLstar     float64 `json:"target_lstar"`

	// 朴素线性归一：权重和为 1，n 灯各取 1/n（两灯即 1/2），输出 S/n。
	LinearWeight        float64   `json:"linear_weight"`
	LinearOut           float64   `json:"linear_out"`
	BrightnessLinear    float64   `json:"brightness_linear"` // P(S/n)，0..1
	LstarLinear         float64   `json:"lstar_linear"`      // L*(S/n)
	WeightsProportional []float64 `json:"weights_proportional"`

	// 感知守恒非线性解：公共增益 Weight，输出 Weight*S，满足 P(Out)=T。
	Weight           float64     `json:"perceptual_weight"`
	Out              float64     `json:"perceptual_out_linear"`
	Brightness       float64     `json:"brightness_perceptual"` // P(Out)=T（未裁切时）
	Lstar            float64     `json:"lstar_perceptual"`
	Iterations       int         `json:"iterations"`
	Residual         float64     `json:"residual"` // |P(Weight*S)-实际目标|
	Converged        bool        `json:"converged"`
	Clipped          bool        `json:"clipped"` // ΣP(I_i)>P(1)=1，目标被压到参考白
	Solver           string      `json:"solver"`
	TolResidual      float64     `json:"tol_residual"`
	TolStep          float64     `json:"tol_step"`
	ClosedFormWeight float64     `json:"closed_form_weight"` // P^{-1}(min(T,1))/S，交叉核对用
	Trace            []Iteration `json:"trace,omitempty"`
}

// ErrNegativeIntensity 表示传入了负的或非有限的光源强度。
var ErrNegativeIntensity = errors.New("light: 强度必须是有限非负数")

// Compare 对一组（任意盏）线性光源强度同时给出：
//   - 朴素线性归一（权重和为 1，各灯 1/n，输出 S/n）的亮度；
//   - 感知亮度守恒权重 w（Newton 非线性求解）及其迭代次数/残差。
//
// n==1 时走精确平凡路径：Weight=1、Out=I[0]、Iterations=0，输入值原样返回，
// 以保证“单光源路径不变”。n==0 返回全零确定结果。
func Compare(intensities []float64, opts Options) (Result, error) {
	o := opts.WithDefaults()
	r := Result{
		N:            len(intensities),
		Intensity:    append([]float64(nil), intensities...),
		LinearWeight: math.NaN(),
		TolResidual:  o.TolResidual,
		TolStep:      o.TolStep,
		Solver:       "newton+bisection-guard",
	}

	var s float64
	for _, v := range intensities {
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
			return r, ErrNegativeIntensity
		}
		s += v
	}
	r.SumLinear = s

	// 线性比例归一权重 I_i/S（S=0 时全 0），作为“直接归一”的另一种常见口径。
	r.WeightsProportional = make([]float64, len(intensities))
	if s > 0 {
		for i, v := range intensities {
			r.WeightsProportional[i] = v / s
		}
	}

	// 感知目标 T=ΣP(I_i)。
	var t float64
	for _, v := range intensities {
		t += Perceived(v)
	}
	r.TargetPerceived = t
	r.TargetLstar = 100 * t

	// 朴素线性归一：n 灯均分权重（Σw=1），线性输出 S/n。
	if n := len(intensities); n > 0 {
		r.LinearWeight = 1.0 / float64(n)
		r.LinearOut = s / float64(n)
		r.BrightnessLinear = Perceived(r.LinearOut)
		r.LstarLinear = Lightness(r.LinearOut)
	}

	// n==0：确定的全零结果。
	if len(intensities) == 0 {
		r.Converged = true
		r.Weight = 1
		r.Out = 0
		r.Brightness = 0
		r.Lstar = 0
		r.ClosedFormWeight = 1
		r.Iterations = 0
		r.Residual = 0
		r.Solver = "identity-zero"
		return r, nil
	}

	// n==1：精确恒等路径，绝不迭代/缩放——单光源路径不变。
	// 即使该灯强度为 0 也走这里，保证单灯语义在边界值上仍一致。
	if len(intensities) == 1 {
		r.Weight = 1
		r.Out = intensities[0]
		r.Brightness = Perceived(intensities[0])
		r.Lstar = Lightness(intensities[0])
		r.Iterations = 0
		r.Residual = 0
		r.Converged = true
		r.ClosedFormWeight = 1
		r.Solver = "single-light-identity"
		return r, nil
	}

	// n≥2 但强度全为 0：确定的全零结果（守恒权重取 1）。
	if s == 0 {
		r.Converged = true
		r.Weight = 1
		r.Out = 0
		r.Brightness = 0
		r.Lstar = 0
		r.ClosedFormWeight = 1
		r.Iterations = 0
		r.Residual = 0
		r.Solver = "identity-zero"
		return r, nil
	}

	// 目标超过参考白 P(1)=1 时，守恒目标无法在量程内实现；按参考白裁切，
	// 并置 Clipped 如实报告。三组验收强度都在量程内，不会触发。
	target := t
	if target > 1.0 {
		r.Clipped = true
		target = 1.0
	}

	// 闭式解（L* 可逆）仅用于与迭代解交叉核对；主路径仍做非线性迭代，
	// 因为题目要求报告迭代次数与残差。
	r.ClosedFormWeight = InvPerceived(target) / s

	w, iters, res, converged, solver, trace := solveConservation(s, target, o)
	r.Weight = w
	r.Iterations = iters
	r.Residual = res
	r.Converged = converged
	if o.Trace {
		r.Trace = trace
	}
	if solver != "" {
		r.Solver = solver
	}
	r.Out = w * s
	r.Brightness = Perceived(r.Out)
	r.Lstar = Lightness(r.Out)
	return r, nil
}

// ConservationWeight 是两灯（或多灯和）守恒权重的便捷封装。
func ConservationWeight(i1, i2 float64, opts Options) (Result, error) {
	return Compare([]float64{i1, i2}, opts)
}

// finite 报告 f 是否既非 NaN 也非 ±Inf。
func finite(f float64) bool {
	return !math.IsNaN(f) && !math.IsInf(f, 0)
}

// residualAndDeriv 计算 F(w)=P(w*S)-target 及其解析导数 F'(w)=S*P'(w*S)。
func residualAndDeriv(w, s, target float64) (f, df float64) {
	y := w * s
	f = Perceived(y) - target
	df = s * dLightness(y) / 100.0
	return f, df
}

// solveConservation 用 Newton 迭代求单调方程 F(w)=0，并用二分区间保护。
//
// 维护区间 [lo,hi] 使 F(lo)≤0≤F(hi)：每步先按当前 w 的残差符号收窄区间，
// 再尝试解析 Newton 步；Newton 点不落在区间内或导数失效时，本步改取区间
// 中点（二分）。因此对任意有限非负输入都不会发散，同时验收三组暗部强度时
// 全程是纯 Newton 步，5~6 次即到浮点残差地板。
//
// 返回最后一个迭代点、收敛判定时所用的迭代序号、|F| 残差与是否收敛。
func solveConservation(s, target float64, o Options) (w float64, iters int, res float64, converged bool, solver string, trace []Iteration) {
	lo, hi := 0.0, 1.0
	trace = make([]Iteration, 0, 8)

	// 扩张右端直到 F(hi)≥0（P 无界递增，必然可达；Inf 时 F=+Inf）。
	for guard := 0; guard < 100; guard++ {
		fh, _ := residualAndDeriv(hi, s, target)
		if fh >= 0 {
			break
		}
		hi *= 2
	}

	w = 1.0 // 初值：原始线性和。凹 P、未裁切时 F(1)≤0，根在 w≥1 一侧。
	solver = "newton"

	for k := 1; k <= o.MaxIter; k++ {
		f, df := residualAndDeriv(w, s, target)
		absF := math.Abs(f)

		// 用当前点收窄二分区间。
		if f <= 0 {
			lo = w
		} else {
			hi = w
		}

		// 先算“如果现在停”的判据所需步长：Newton 候选，必要时二分。
		next := w
		guarded := false
		if df > 0 && finite(df) {
			cand := w - f/df
			if finite(cand) && cand >= lo && cand <= hi {
				next = cand
			} else {
				next = (lo + hi) / 2
				guarded = true
			}
		} else {
			next = (lo + hi) / 2
			guarded = true
		}
		if guarded {
			solver = "newton+bisection-guard"
		}
		step := math.Abs(next - w)
		trace = append(trace, Iteration{K: k, W: w, AbsF: absF, Step: step, Guarded: guarded})

		// 收敛判据：残差达标，且相对步长达标（或步长在浮点上归零）。
		// 接受这一步并报告最终点 next 上的实测残差。
		if absF <= o.TolResidual &&
			(step <= o.TolStep*(1+math.Abs(w)) || next == w) {
			finalF, _ := residualAndDeriv(next, s, target)
			return next, k, math.Abs(finalF), true, solver, trace
		}

		w = next
	}

	f, _ := residualAndDeriv(w, s, target)
	return w, o.MaxIter, math.Abs(f), false, solver, trace
}
