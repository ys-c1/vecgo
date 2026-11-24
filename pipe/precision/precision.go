// Package precision 把坐标变换拆成三个显式阶段（缩放 → 旋转 → 平移），
// 每阶段后按固定小数位 d 做十进制量化，并依据各阶段雅可比的条件数
// 反比分配误差预算、确定各阶段小数位。
package precision

import (
	"math"

	"vecgo/geom"
)

// 三个阶段的固定编号与名称。
const (
	StageScale     = iota // 第 1 步：缩放 S=kI
	StageRotate           // 第 2 步：旋转 R(θ)
	StageTranslate        // 第 3 步：平移 T(tx,ty)
	StageCount
)

// StageInfo 描述一个阶段的雅可比及其误差放大特性。
type StageInfo struct {
	Name      string
	Jacobian  [2][2]float64
	Cond2     float64 // 谱条件数 κ₂=σmax/σmin
	InfGain   float64 // 无穷范数增益 ‖J‖∞（十进制舍入误差按分量最坏情况传播时的放大倍数）
	AmplifyTo float64 // 该阶段量化误差传播到最终输出端的放大倍数 a_i
}

// Conditions 计算三阶段雅可比的条件数与传播到输出端的放大倍数。
//
// 放大倍数（链式法则，误差按分量取最坏情况，即无穷范数）：
//
//	a1 = ‖R‖∞·‖I‖∞ = ‖R‖∞,  a2 = ‖I‖∞ = 1,  a3 = 1
//
// 旋转的无穷范数增益 ‖R(θ)‖∞ = |cosθ|+|sinθ| 随 θ 变化（1~√2），
// 这正是旋转步误差放大倍数“随角度变、不能按常数摊”的来源。
func Conditions(k, theta, tx, ty float64) []StageInfo {
	c, s := math.Cos(theta), math.Sin(theta)
	rInf := math.Abs(c) + math.Abs(s) // ‖R(θ)‖∞，角度相关
	inf := func(m [2][2]float64) float64 {
		return math.Max(math.Abs(m[0][0])+math.Abs(m[0][1]),
			math.Abs(m[1][0])+math.Abs(m[1][1]))
	}
	scaleJ := [2][2]float64{{k, 0}, {0, k}}
	rotJ := [2][2]float64{{c, -s}, {s, c}}
	idJ := [2][2]float64{{1, 0}, {0, 1}}
	return []StageInfo{
		{Name: "scale", Jacobian: scaleJ, Cond2: 1, InfGain: inf(scaleJ), AmplifyTo: rInf},
		{Name: "rotate", Jacobian: rotJ, Cond2: 1, InfGain: rInf, AmplifyTo: 1},
		{Name: "translate", Jacobian: idJ, Cond2: 1, InfGain: 1, AmplifyTo: 1},
	}
}

// Plan 是一份每阶段小数位分配方案。
type Plan struct {
	Stages []StagePlan
	E      float64 // 总误差上界
}

// StagePlan 是单阶段的预算与位数。
type StagePlan struct {
	Index    int
	Name     string
	Amplify  float64 // a_i：传播到输出端的放大倍数
	Budget   float64 // 连续反比分配的分量误差预算 ε_i = E/(n·a_i)
	Target   float64 // 连续理想位数 x_i = log10(a_i/(2ε_i))（未取整）
	Digits   int     // 整数化后的小数位 d_i
	Realized float64 // 整数化后实际保证的分量误差上界 0.5·10^-d_i
}

func ceilInt(x float64) int { return int(math.Ceil(x)) }

// PlanAdaptive 先按放大倍数（有效条件数）反比分配连续预算
//
//	ε_i = E / (n·a_i)，  理想位数 x_i = log10( n·a_i²/(2E) )
//
// 再用贪心整数化（planGreedy）把 x_i 落成整数位。逐位独立 ceil 会在整数
// 边界上过度保守（实测可能比等精度更贵），贪心则在所有满足总误差 ≤ E 的
// 整数方案中总位数最少。
//
// 谱条件数 κ₂ 三步均为 1（旋转/平移正交或单位阵，缩放为 kI 各向同性），
// 对分量最坏情况传播不具区分度；真正决定十进制舍入误差放大的是无穷范数
// 增益 a_i，故预算反比于 a_i 分配。
func PlanAdaptive(k, theta, tx, ty, E float64) Plan {
	st := Conditions(k, theta, tx, ty)
	n := float64(len(st))
	out := make([]StagePlan, len(st))
	for i, s := range st {
		budget := E / (n * s.AmplifyTo)
		out[i] = StagePlan{
			Index: i, Name: s.Name, Amplify: s.AmplifyTo, Budget: budget,
			Target: math.Log10(s.AmplifyTo / (2 * budget)),
		}
	}
	planGreedy(out, E)
	return Plan{Stages: out, E: E}
}

// planGreedy 整数化策略（一行版）：
//
//	全部 d_i 从 0 起，只要 Σ a_i·0.5·10^-d_i > E，就把当前 a_i·0.5·10^-d_i
//	最大的那一步 d_i 加 1，直到上界 ≤ E。
//
// 最优性：第 i 步从 t 升到 t+1 的边际收益恒为 0.45·a_i·10^-t（与其他步
// 无关），且对固定 i 随 t 严格递减；贪心每步取走全局最大的可用边际收益，
// 故它选中的 m 个增益就是全体边际收益里最大的 m 个——任何总位数为 m 的
// 方案收益都不超过它，贪心以最少总位数使总误差降到 ≤ E。
// 不超界：循环终止条件本身就是 Σ a_i·0.5·10^-d_i ≤ E（浮点留 1e-12 容差）。
func planGreedy(out []StagePlan, E float64) {
	d := make([]int, len(out))
	bound := func() float64 {
		b := 0.0
		for i, s := range out {
			b += s.Amplify * 0.5 * math.Pow10(-d[i])
		}
		return b
	}
	// 64 位上限只用于防病态 E 死循环；正常配置远在该上限内收敛。
	for iter := 0; iter < 64 && bound() > E*(1+1e-12); iter++ {
		worst, wv := 0, -1.0
		for i, s := range out {
			if v := s.Amplify * 0.5 * math.Pow10(-d[i]); v > wv {
				wv, worst = v, i
			}
		}
		d[worst]++
	}
	for i := range out {
		out[i].Digits = d[i]
		out[i].Realized = 0.5 * math.Pow10(-d[i])
	}
}

// PlanEqual 是对照方案：三步等精度，统一位数 d，该位数让最坏放大后的
// 总误差仍 ≤ E，即
//
//	0.5·10^-d·Σa_i ≤ E  ⇒  d = ⌈log10( A/(2E) )⌉,  A = Σa_i
//
// 若忽略放大链、按 E/n 摊位数，等精度方案在 ‖R‖∞>1 时会实际超界，
// 因此这里用“同样保证 ≤ E”的公平口径定 d。
func PlanEqual(k, theta, tx, ty, E float64) Plan {
	st := Conditions(k, theta, tx, ty)
	A := 0.0
	for _, s := range st {
		A += s.AmplifyTo
	}
	d := ceilInt(math.Log10(A / (2 * E)))
	if d < 0 {
		d = 0
	}
	out := make([]StagePlan, len(st))
	for i, s := range st {
		out[i] = StagePlan{
			Index: i, Name: s.Name, Amplify: s.AmplifyTo,
			Budget: E / A, Target: math.Log10(A / (2 * E)),
			Digits: d, Realized: 0.5 * math.Pow10(-d),
		}
	}
	return Plan{Stages: out, E: E}
}

// Bound 返回方案在最坏情况下保证的输出端分量误差上界 Σ a_i·0.5·10^-d_i。
func (p Plan) Bound() float64 {
	b := 0.0
	for _, s := range p.Stages {
		b += s.Amplify * s.Realized
	}
	return b
}

// TotalDigits 返回三阶段小数位之和（成本指标）。
func (p Plan) TotalDigits() int {
	t := 0
	for _, s := range p.Stages {
		t += s.Digits
	}
	return t
}

// quantize 把单个坐标按 d 位小数四舍五入（半数远离零），|误差| ≤ 0.5·10^-d。
func quantize(v float64, d int) float64 {
	p := math.Pow10(d)
	return math.Round(v*p) / p
}

func roundPoint(q geom.Point, d int) geom.Point {
	return geom.Point{X: quantize(q.X, d), Y: quantize(q.Y, d)}
}

// Apply 按给定方案执行三步变换，每步后按该步位数量化。
//
//	第 1 步 q1 = Q_{d1}(k·p)
//	第 2 步 q2 = Q_{d2}(R(θ)·q1)
//	第 3 步 q3 = Q_{d3}(q2 + (tx,ty))
func Apply(p geom.Point, k, theta, tx, ty float64, digits [StageCount]int) geom.Point {
	c, s := math.Cos(theta), math.Sin(theta)
	q1 := roundPoint(geom.Point{X: k * p.X, Y: k * p.Y}, digits[StageScale])
	q2 := roundPoint(geom.Point{X: q1.X*c - q1.Y*s, Y: q1.X*s + q1.Y*c}, digits[StageRotate])
	q3 := roundPoint(geom.Point{X: q2.X + tx, Y: q2.Y + ty}, digits[StageTranslate])
	return q3
}

// Exact 是不做任何量化的参考结果（全程 float64），用于实测误差。
func Exact(p geom.Point, k, theta, tx, ty float64) geom.Point {
	c, s := math.Cos(theta), math.Sin(theta)
	q1 := geom.Point{X: k * p.X, Y: k * p.Y}
	q2 := geom.Point{X: q1.X*c - q1.Y*s, Y: q1.X*s + q1.Y*c}
	return geom.Point{X: q2.X + tx, Y: q2.Y + ty}
}

// DigitsOf 从方案中取出三阶段位数数组。
func (p Plan) DigitsOf() [StageCount]int {
	var d [StageCount]int
	for _, s := range p.Stages {
		d[s.Index] = s.Digits
	}
	return d
}
