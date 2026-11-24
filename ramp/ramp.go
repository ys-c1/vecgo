// Package ramp 实现渐变控制点的三次 Hermite 插值，以及“相邻切线同号 +
// Fritsch–Carlson 幅值界”约束下的单切线最优修复。
//
// 设计与推导见同目录 DESIGN.md。
package ramp

import (
	"errors"
	"math"

	"vecgo/raster"
)

// Stop 是一个渐变控制点：X 为横向（纵向渐变图里即行方向）归一化位置 [0,1]，
// Y 为该点的强度值 [0,1]。
type Stop struct {
	X, Y float64
}

// Curve 是由控制点张成的分段三次 Hermite 曲线。
type Curve struct {
	Stops []Stop
	X, Y  []float64
	H     []float64 // H[i] = X[i+1]-X[i]
	D     []float64 // D[i] = (Y[i+1]-Y[i])/H[i]，第 i 段弦斜率
	M     []float64 // M[i] = 节点 i 处的切线（dy/dx）
}

// New 校验控制点并构造曲线，切线初始化为朴素平均切线（会过冲的那版）。
func New(stops []Stop) (*Curve, error) {
	if len(stops) < 2 {
		return nil, errors.New("渐变至少需要两个控制点")
	}
	x := make([]float64, len(stops))
	y := make([]float64, len(stops))
	for i, s := range stops {
		if math.IsNaN(s.X) || math.IsNaN(s.Y) || math.IsInf(s.X, 0) || math.IsInf(s.Y, 0) {
			return nil, errors.New("控制点必须是有限数")
		}
		if s.X < 0 || s.X > 1 || s.Y < 0 || s.Y > 1 {
			return nil, errors.New("控制点坐标必须落在 [0,1]")
		}
		if i > 0 && s.X <= stops[i-1].X {
			return nil, errors.New("控制点 X 必须严格递增")
		}
		x[i], y[i] = s.X, s.Y
	}
	n := len(stops)
	c := &Curve{Stops: append([]Stop{}, stops...), X: x, Y: y}
	c.H = make([]float64, n-1)
	c.D = make([]float64, n-1)
	for i := 0; i+1 < n; i++ {
		c.H[i] = x[i+1] - x[i]
		c.D[i] = (y[i+1] - y[i]) / c.H[i]
	}
	c.M = c.NaiveTangents()
	return c, nil
}

// NaiveTangents 给出事故版本的切线：端点取弦斜率，内部取相邻弦斜率的算术平均
// （Catmull-Rom 风格）。坡度陡缓切换时它会把切线给大，产生往回勾的过冲。
func (c *Curve) NaiveTangents() []float64 {
	n := len(c.X)
	m := make([]float64, n)
	m[0] = c.D[0]
	m[n-1] = c.D[n-2]
	for k := 1; k+1 < n; k++ {
		m[k] = 0.5 * (c.D[k-1] + c.D[k])
	}
	return m
}

// SetTangents 替换当前切线。
func (c *Curve) SetTangents(m []float64) error {
	if len(m) != len(c.M) {
		return errors.New("切线数量与控制点不一致")
	}
	for _, v := range m {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return errors.New("切线必须是有限数")
		}
	}
	copy(c.M, m)
	return nil
}

// Hermite 计算第 i 段上 u=(x-X[i])/H[i]∈[0,1] 处的三次 Hermite 值。
func Hermite(y0, y1, m0, m1, h, u float64) float64 {
	u2 := u * u
	u3 := u2 * u
	return (2*u3-3*u2+1)*y0 +
		(u3-2*u2+u)*h*m0 +
		(-2*u3+3*u2)*y1 +
		(u3-u2)*h*m1
}

// Eval 在任意归一化位置 x∈[0,1] 处求值（使用当前切线）。
func (c *Curve) Eval(x float64) float64 {
	n := len(c.X)
	if x <= c.X[0] {
		return c.Y[0]
	}
	if x >= c.X[n-1] {
		return c.Y[n-1]
	}
	k := 0
	for k+1 < len(c.H) && c.X[k+1] <= x {
		k++
	}
	u := (x - c.X[k]) / c.H[k]
	return Hermite(c.Y[k], c.Y[k+1], c.M[k], c.M[k+1], c.H[k], u)
}

// 以下常数是 Hermite 基函数与“曲线−控制折线”之差 q(u) 在 [0,1] 上的精确积分，
// 见 DESIGN.md 第 3 节。
const (
	intH10Sq   = 1.0 / 105.0 // ∫ h10²
	intH11Sq   = 1.0 / 105.0 // ∫ h11²
	intH10H11  = -1.0 / 140.0
	intQSq     = 1.0 / 210.0 // ∫ q²
	intQH10    = 1.0 / 420.0
	intQH11    = 1.0 / 420.0
	FCcircleK2 = 9.0 // Fritsch–Carlson: α²+β² ≤ 9
)

// Quadratic 返回单切线目标函数
//
//	J(t) = ∫(C_t-L)² dx = A t² - 2B t + C
//
// 的三个系数以及无约束驻点 τ=B/A。t 是中间节点切线；左段宽 hL、弦斜率 dL、
// 另一端固定切线 a；右段宽 hR、弦斜率 dR、另一端固定切线 b。
// L 是相邻控制点张成的分段线性折线。A=(hL³+hR³)/105>0，故 J 严格凸。
func Quadratic(hL, hR, dL, dR, a, b float64) (A, B, C, tau float64) {
	l3, r3 := hL*hL*hL, hR*hR*hR

	A = l3*intH11Sq + r3*intH10Sq

	// 一次项系数 ℓ t，写成 -2B t：ℓ = 2(K0)，B=-K0。
	k0 := l3*(a*intH10H11-dL*intQH11) +
		r3*(b*intH10H11-dR*intQH10)
	B = -k0

	C = l3*(a*a*intH10Sq+dL*dL*intQSq-2*a*dL*intQH10) +
		r3*(b*b*intH11Sq+dR*dR*intQSq-2*b*dR*intQH11)

	tau = B / A
	return A, B, C, tau
}

// StationaryTangent 给出驻点切线的简洁闭式：
//
//	τ = [hL³(dL+3a) + hR³(dR+3b)] / [4(hL³+hR³)]
//
// 它是两侧“局部偏好切线” (dL+3a)/4、(dR+3b)/4 按 hL³:hR³ 的凸组合。
func StationaryTangent(hL, hR, dL, dR, a, b float64) float64 {
	l3, r3 := hL*hL*hL, hR*hR*hR
	return (l3*(dL+3*a) + r3*(dR+3*b)) / (4 * (l3 + r3))
}

// FCInterval 是单条切线 t=m_k 的允许闭区间。
//
//  1. 同号约束：t 必须与左右两条相邻弦斜率 dL、dR 同号；dL*dR≤0（数据本身在
//     此转折）时只能取 0。
//  2. 幅值约束：每个相邻段满足 Fritsch–Carlson 单调三次圆界
//     (m_i/d_i)²+(m_{i+1}/d_i)² ≤ 9，由此给出有限的外端点 R。
//
// 最终区间为 [0,R]（上升）、[-R,0]（下降）或退化的 [0,0]。
func FCInterval(dL, a, dR, b float64) (lo, hi float64) {
	if dL == 0 || dR == 0 || dL*dR <= 0 {
		return 0, 0
	}
	rL := math.Abs(dL) * math.Sqrt(math.Max(0, FCcircleK2-(a/dL)*(a/dL)))
	rR := math.Abs(dR) * math.Sqrt(math.Max(0, FCcircleK2-(b/dR)*(b/dR)))
	r := math.Min(rL, rR)
	if dL < 0 {
		return -r, 0
	}
	return 0, r
}

// OneSolution 是单切线约束优化的完整一组数。
type OneSolution struct {
	Lo, Hi  float64 // 允许区间
	A, B, C float64 // J(t)=A t²-2B t+C
	Tau     float64 // 无约束驻点 B/A
	That    float64 // 约束最优解 clamp(τ,Lo,Hi)
	JHat    float64 // J(That)
	Case    string  // "lower" | "upper" | "interior" | "degenerate"
}

// SolveOne 对给定几何与固定邻切线完成“区间内最优切线”求解。
func SolveOne(hL, hR, dL, dR, a, b float64) OneSolution {
	A, B, C, tau := Quadratic(hL, hR, dL, dR, a, b)
	lo, hi := FCInterval(dL, a, dR, b)

	sol := OneSolution{Lo: lo, Hi: hi, A: A, B: B, C: C, Tau: tau}
	eps := 1e-10 * math.Max(1, math.Max(math.Abs(lo), math.Abs(hi)))
	switch {
	case hi-lo <= eps:
		sol.Case = "degenerate"
		sol.That = lo
	case tau < lo-eps:
		sol.Case = "lower"
		sol.That = lo
	case tau > hi+eps:
		sol.Case = "upper"
		sol.That = hi
	default:
		sol.Case = "interior"
		sol.That = math.Min(math.Max(tau, lo), hi)
	}
	t := sol.That
	sol.JHat = A*t*t - 2*B*t + C
	return sol
}

// SolveKnot 解第 k 个内部节点（1≤k≤n-2）的单切线问题，邻切线取当前 c.M。
func (c *Curve) SolveKnot(k int) (OneSolution, error) {
	if k <= 0 || k+1 >= len(c.X) {
		return OneSolution{}, errors.New("只能优化内部节点切线")
	}
	return SolveOne(c.H[k-1], c.H[k], c.D[k-1], c.D[k], c.M[k-1], c.M[k+1]), nil
}

// Optimize 逐节点做精确的一维投影极小化并扫描到收敛。返回扫描轮数与最后一轮
// 最大切线变动。每一步都是该坐标在当前允许区间上的全局最优；目标二次凸。
func (c *Curve) Optimize(maxSweeps int, tol float64) (sweeps int, maxChange float64) {
	if maxSweeps <= 0 {
		maxSweeps = 1000
	}
	if tol <= 0 {
		tol = 1e-13
	}
	n := len(c.X)
	for s := 0; s < maxSweeps; s++ {
		maxCh := 0.0
		for k := 1; k+1 < n; k++ {
			sol, _ := c.SolveKnot(k)
			maxCh = math.Max(maxCh, math.Abs(sol.That-c.M[k]))
			c.M[k] = sol.That
		}
		sweeps = s + 1
		maxChange = maxCh
		if maxCh < tol {
			break
		}
	}
	return sweeps, maxChange
}

// FCFeasible 报告所有段的切线是否满足同号与 α²+β²≤9（带相对容差）。
func (c *Curve) FCFeasible(tol float64) bool {
	if tol <= 0 {
		tol = 1e-9
	}
	for i := 0; i < len(c.H); i++ {
		d := c.D[i]
		m0, m1 := c.M[i], c.M[i+1]
		if d == 0 {
			if math.Abs(m0) > tol || math.Abs(m1) > tol {
				return false
			}
			continue
		}
		if m0*d < -tol || m1*d < -tol {
			return false
		}
		a, b := m0/d, m1/d
		if a*a+b*b > FCcircleK2+tol*FCcircleK2 {
			return false
		}
	}
	return true
}

// FillImage 把当前切线对应的渐变填入一张已有的灰度图：第 r 行对应
// x=r/(H-1)（与 shading 包一致），全行同值，灰度按四舍五入量化到 8 bit。
func (c *Curve) FillImage(im *raster.Image) error {
	if im == nil {
		return errors.New("图像为空")
	}
	for r := 0; r < im.H; r++ {
		x := 0.0
		if im.H > 1 {
			x = float64(r) / float64(im.H-1)
		}
		v := c.Eval(x)
		b := Byte(v)
		for col := 0; col < im.W; col++ {
			im.Pix[r*im.W+col] = b
		}
	}
	return nil
}

// Byte 把 [0,1] 强度四舍五入并截断到 uint8。
func Byte(v float64) uint8 {
	b := int(math.Round(255 * v))
	if b < 0 {
		b = 0
	}
	if b > 255 {
		b = 255
	}
	return uint8(b)
}

// RowOvershoot 描述一行的过冲测量。
type RowOvershoot struct {
	Row          int     `json:"row"`
	X            float64 `json:"x"`
	Byte         int     `json:"byte"`
	AllowedLo    int     `json:"allowed_lo"`
	AllowedHi    int     `json:"allowed_hi"`
	ExcessLevels int     `json:"excess_levels"`
	Value        float64 `json:"value"`
	Segment      int     `json:"segment"`
}

// OvershootPixels 逐像素统计过冲：像素灰度超出其所在三次段两端控制点灰度
// （同一量化规则）所张区间即计为过冲。返回过冲像素总数（= 问题行数×宽）、
// 最大超出灰阶数与问题行明细。
func (c *Curve) OvershootPixels(im *raster.Image) (pixels, badRows, maxExcess int, rows []RowOvershoot) {
	rows = []RowOvershoot{}
	for r := 0; r < im.H; r++ {
		x := 0.0
		if im.H > 1 {
			x = float64(r) / float64(im.H-1)
		}
		k := c.segmentAt(x)
		bLo := int(Byte(math.Min(c.Y[k], c.Y[k+1])))
		bHi := int(Byte(math.Max(c.Y[k], c.Y[k+1])))
		b := int(im.Pix[r*im.W])
		if b < bLo || b > bHi {
			ex := bLo - b
			if b-bHi > ex {
				ex = b - bHi
			}
			if ex > maxExcess {
				maxExcess = ex
			}
			badRows++
			pixels += im.W
			rows = append(rows, RowOvershoot{
				Row: r, X: x, Byte: b, AllowedLo: bLo, AllowedHi: bHi,
				ExcessLevels: ex, Value: c.Eval(x), Segment: k,
			})
		}
	}
	return pixels, badRows, maxExcess, rows
}

func (c *Curve) segmentAt(x float64) int {
	k := 0
	for k+1 < len(c.H) && c.X[k+1] <= x {
		k++
	}
	if k >= len(c.H) {
		k = len(c.H) - 1
	}
	return k
}

// OvershootDense 在每段内部密集采样，报告连续意义下越出端点值域的最大偏差
// （强度单位）与采样点计数，供测试验证“零过冲”不只是像素网格巧合。
func (c *Curve) OvershootDense(perSpan int) (badSamples int, maxExcess float64) {
	if perSpan < 2 {
		perSpan = 400
	}
	for i := 0; i < len(c.H); i++ {
		lo := math.Min(c.Y[i], c.Y[i+1])
		hi := math.Max(c.Y[i], c.Y[i+1])
		for j := 0; j <= perSpan; j++ {
			u := float64(j) / float64(perSpan)
			v := Hermite(c.Y[i], c.Y[i+1], c.M[i], c.M[i+1], c.H[i], u)
			if v < lo {
				badSamples++
				maxExcess = math.Max(maxExcess, lo-v)
			} else if v > hi {
				badSamples++
				maxExcess = math.Max(maxExcess, v-hi)
			}
		}
	}
	return badSamples, maxExcess
}

// KnotDeviation 返回指定控制点下标集合上 |C(X_i)-Y_i| 的最大值。Hermite
// 严格过端点，因此解析值应为 0（只剩浮点误差）。
func (c *Curve) KnotDeviation(indices []int) float64 {
	maxDev := 0.0
	for _, i := range indices {
		if i < 0 || i >= len(c.X) {
			continue
		}
		maxDev = math.Max(maxDev, math.Abs(c.Eval(c.X[i])-c.Y[i]))
	}
	return maxDev
}

// ChordDeviation 在指定段集合上密集采样，返回三次曲线相对该段控制折线（弦）
// 的最大绝对偏差（强度单位）。它衡量“形状”在两点之间离直线有多远。
func (c *Curve) ChordDeviation(spans []int, perSpan int) float64 {
	if perSpan < 2 {
		perSpan = 400
	}
	maxDev := 0.0
	for _, i := range spans {
		if i < 0 || i >= len(c.H) {
			continue
		}
		for j := 0; j <= perSpan; j++ {
			u := float64(j) / float64(perSpan)
			v := Hermite(c.Y[i], c.Y[i+1], c.M[i], c.M[i+1], c.H[i], u)
			chord := c.Y[i] + u*(c.Y[i+1]-c.Y[i])
			maxDev = math.Max(maxDev, math.Abs(v-chord))
		}
	}
	return maxDev
}

// BrightPixelDeviation 核对亮部：在每个给定控制点对应的最近输出行上，比较
// 实际像素灰度与该控制点灰度 Byte(Y_i)，返回最大灰阶偏离与逐项明细。
func (c *Curve) BrightPixelDeviation(im *raster.Image, indices []int) (maxLevels int, details []int) {
	details = make([]int, 0, len(indices))
	for _, i := range indices {
		if i < 0 || i >= len(c.X) {
			continue
		}
		r := 0
		if im.H > 1 {
			r = int(math.Round(c.X[i] * float64(im.H-1)))
		}
		if r < 0 {
			r = 0
		}
		if r >= im.H {
			r = im.H - 1
		}
		got := int(im.Pix[r*im.W])
		want := int(Byte(c.Y[i]))
		d := got - want
		if d < 0 {
			d = -d
		}
		details = append(details, d)
		if d > maxLevels {
			maxLevels = d
		}
	}
	return maxLevels, details
}
