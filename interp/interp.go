// Package interp 实现一维三次 Hermite（Catmull-Rom）插值，以及
// Fritsch–Carlson 保单调限制器。
//
// 限制器只修正控制点处的切线，不改控制点的函数值，因此插值曲线仍然
// 经过全部控制点。数学推导见同目录 monotone.md。
package interp

import (
	"errors"
	"math"
	"sort"
)

// Spline 是一条分段三次 Hermite 插值曲线。
// X 必须严格递增，Tangents 与 X 等长，为各控制点处的一阶导。
type Spline struct {
	X, Y, Tangents []float64
}

// New 构造三次插值曲线。limited 为 false 时只做 Catmull-Rom 初始切线
// （不施加限制器，保留原始过冲行为）；为 true 时施加保单调限制器。
func New(x, y []float64, limited bool) (*Spline, error) {
	n := len(x)
	if n != len(y) || n < 2 {
		return nil, errors.New("x/y 长度不一致或点数不足 2")
	}
	if sort.Float64sAreSorted(x) == false {
		return nil, errors.New("x 必须严格递增")
	}
	for i := 1; i < n; i++ {
		if x[i] <= x[i-1] {
			return nil, errors.New("x 必须严格递增")
		}
	}
	m := initialTangents(x, y)
	if limited {
		LimitMonotone(x, y, m)
	}
	return &Spline{X: append([]float64(nil), x...), Y: append([]float64(nil), y...), Tangents: m}, nil
}

// h00/h10/h01/h11 为三次 Hermite 基函数，t∈[0,1]。
func h00(t float64) float64 { return 2*t*t*t - 3*t*t + 1 }
func h10(t float64) float64 { return t*t*t - 2*t*t + t }
func h01(t float64) float64 { return -2*t*t*t + 3*t*t }
func h11(t float64) float64 { return t*t*t - t*t }

// Eval 在位置 z 处取值，超出定义域按端点外推（端点切线线性外推）。
func (s *Spline) Eval(z float64) float64 {
	n := len(s.X)
	if z <= s.X[0] {
		return s.Y[0] + s.Tangents[0]*(z-s.X[0])
	}
	if z >= s.X[n-1] {
		return s.Y[n-1] + s.Tangents[n-1]*(z-s.X[n-1])
	}
	k := sort.Search(n, func(i int) bool { return s.X[i] > z }) - 1
	h := s.X[k+1] - s.X[k]
	t := (z - s.X[k]) / h
	return h00(t)*s.Y[k] + h10(t)*h*s.Tangents[k] +
		h01(t)*s.Y[k+1] + h11(t)*h*s.Tangents[k+1]
}

// Secants 返回各区间割线斜率 Δ_k=(y_{k+1}-y_k)/(x_{k+1}-x_k)。
func Secants(x, y []float64) []float64 {
	d := make([]float64, len(x)-1)
	for k := range d {
		d[k] = (y[k+1] - y[k]) / (x[k+1] - x[k])
	}
	return d
}

// initialTangents 用三点加权（非均匀 Catmull-Rom）公式计算初始切线，
// 端点用单侧割线斜率。
func initialTangents(x, y []float64) []float64 {
	n := len(x)
	d := Secants(x, y)
	m := make([]float64, n)
	m[0], m[n-1] = d[0], d[n-2]
	for k := 1; k < n-1; k++ {
		hl, hr := x[k]-x[k-1], x[k+1]-x[k]
		m[k] = (hl*d[k] + hr*d[k-1]) / (hl + hr)
	}
	return m
}

// SameSign 是限制器生效判据：相邻割线同号。
//   - Δ_{k-1}*Δ_k > 0：同号（均正或均负），控制点位于单调段内部，限制器生效；
//   - 乘积 == 0：一侧为平台，切线必须置 0；
//   - 乘积 < 0：异号，控制点为局部极值，切线必须置 0。
//
// 检查方式即对相邻割线斜率的乘积取符号（与 sign(Δ_{k-1})==sign(Δ_k)
// 等价，但乘积形式可同时覆盖平台情形）。
func SameSign(a, b float64) bool { return a*b > 0 }

// LimitMonotone 原地施加 Fritsch–Carlson 保单调限制器：
//  1. 相邻割线异号或含平台（乘积 ≤ 0）的控制点，切线置 0；
//  2. 对每个单调区间按保守圆约束 α²+β²≤9 缩放两端切线
//     （α=m_k/Δ_k, β=m_{k+1}/Δ_k），迭代至收敛。
//
// Δ_k=0 的平台区间两端切线直接置 0（平台上不允许任何斜率）。
func LimitMonotone(x, y, m []float64) {
	n := len(x)
	d := Secants(x, y)

	// 第一步：逐控制点做同号检查。
	for k := 1; k < n-1; k++ {
		if !SameSign(d[k-1], d[k]) {
			m[k] = 0
		}
	}

	// 第二步：逐区间施加 α²+β²≤9。迭代是因为同一条切线被左右两个
	// 区间共享，缩放后可能使相邻区间重新越界；切线在迭代中只减不增，
	// 故必然收敛。
	const maxIter = 64
	for iter := 0; iter < maxIter; iter++ {
		changed := false
		for k := 0; k < n-1; k++ {
			delta := d[k]
			if delta == 0 {
				if m[k] != 0 || m[k+1] != 0 {
					m[k], m[k+1] = 0, 0
					changed = true
				}
				continue
			}
			alpha := m[k] / delta
			beta := m[k+1] / delta
			sum := alpha*alpha + beta*beta
			if sum > 9 {
				tau := 3 / math.Sqrt(sum)
				m[k] *= tau
				m[k+1] *= tau
				changed = true
			}
		}
		if !changed {
			break
		}
	}
}

// ExactBetaMax 返回精确单调区域内、给定 α=m_k/Δ_k 时允许的
// β=m_{k+1}/Δ_k 上界（允许区间为 [0, βmax]）；ok=false 表示 α<0 或 α>4，
// 不存在任何可行 β。
//
// 精确区域 R = {α,β≥0 且 [2α+β≤3 或 α+2β≤3 或 E(α,β)≤0]}，
// 两条直线只决定区域的下腹部，外边界恒为椭圆外弧的上根，故
//
//	βmax(α) = (6-α+√(3α(4-α)))/2，0≤α≤4。
//
// 推导见 monotone.md。
func ExactBetaMax(alpha float64) (betaMax float64, ok bool) {
	if alpha < 0 || alpha > 4 {
		return 0, false
	}
	return (6 - alpha + math.Sqrt(3*alpha*(4-alpha))) / 2, true
}
