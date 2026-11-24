// Package junction 处理三条边交汇接点处的抗锯齿覆盖率分配。
//
// 问题：三角形剖分后，一个三边接点 O 会被三个图元（三角形/描边段）各覆盖一次。
// 若各图元的覆盖率直接相加，接点周围一圈像素的总覆盖被重复计数，灰度比周围高出一圈。
//
// 模型：三条边（射线）把 O 的邻域切成三个扇形，夹角为 θ0,θ1,θ2（θ0+θ1+θ2=2π）。
// 半径 r 的扇形面积为
//
//	A_i = 1/2 · r² · θ_i，且 Σ A_i = πr²。
//
// 把接点环的总覆盖面积 πr² 作为守恒预算，按面积守恒方程
//
//	w_i · πr² = A_i
//
// 反解出每个图元的权重 w_i = A_i/ΣA = θ_i/(2π)。于是
//
//	Σ w_i = 1
//
// 是每个接点都必须单独成立的配分不变量：加权后的接点亮度不会被放大，
// 与周围普通边缘像素连续。均匀除以 3（w_i=1/3）只在三等分接点成立，
// 对非对称夹角会在大扇形一侧留下覆盖缺口（见 EqualThirdsGap）。
package junction

import (
	"errors"
	"fmt"
	"math"
	"sort"
)

// Tau 是整圆角（弧度）。
const Tau = 2 * math.Pi

// Angles 是三个扇形夹角（弧度），恒有 Sum ≈ Tau。
type Angles [3]float64

// Weights 是三个图元的配分权重，恒有 Sum ≈ 1。
type Weights [3]float64

// Areas 是三个扇形面积（像素²）。
type Areas [3]float64

// 校验容差：角度和允许 1e-9 弧度误差；权重和允许 1e-12。
const (
	AngleSumTol  = 1e-9
	WeightSumTol = 1e-12
)

// IndexedError 指出三元组中第几个分量非法（下标 0..2）。
type IndexedError struct {
	Index int
	Value float64
	Msg   string
}

func (e *IndexedError) Error() string {
	return fmt.Sprintf("第 %d 项非法 (%g): %s", e.Index, e.Value, e.Msg)
}

// AngleError 是夹角校验错误的向后兼容名称。
type AngleError = IndexedError

// SumAngles 返回三个夹角之和。
func SumAngles(a Angles) float64 { return a[0] + a[1] + a[2] }

// SumWeights 返回三个权重之和，必须恒为 1。
func SumWeights(w Weights) float64 { return w[0] + w[1] + w[2] }

// SumAreas 返回三个扇形面积之和。
func SumAreas(a Areas) float64 { return a[0] + a[1] + a[2] }

// ValidateAngles 要求三个夹角均为有限正数且和为 2π。
func ValidateAngles(a Angles) error {
	for i, v := range a {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return &IndexedError{Index: i, Value: v, Msg: "夹角必须是有限数"}
		}
		if v <= 0 {
			return &IndexedError{Index: i, Value: v, Msg: "夹角必须为正（射线不能重合）"}
		}
		if v >= Tau {
			return &IndexedError{Index: i, Value: v, Msg: "单个夹角必须小于 2π"}
		}
	}
	if d := math.Abs(SumAngles(a) - Tau); d > AngleSumTol {
		return fmt.Errorf("三个夹角之和必须为 2π，实际 %.12f，偏差 %.3e", SumAngles(a), d)
	}
	return nil
}

// SectorAngles 从三条射线的方向角（弧度，任意顺序、任意基准方向）
// 反解出按周向排列的三个扇形夹角。
func SectorAngles(rays [3]float64) (Angles, error) {
	rs := make([]float64, 3)
	for i, r := range rays {
		if math.IsNaN(r) || math.IsInf(r, 0) {
			return Angles{}, &AngleError{Index: i, Value: r, Msg: "射线方向必须是有限数"}
		}
		r = math.Mod(r, Tau)
		if r < 0 {
			r += Tau
		}
		rs[i] = r
	}
	sort.Float64s(rs)
	var out Angles
	for i := 0; i < 3; i++ {
		out[i] = math.Mod(rs[(i+1)%3]-rs[i]+Tau, Tau)
	}
	// 三条射线重合会产生 0 角，交给 ValidateAngles 拒绝并指出下标。
	if err := ValidateAngles(out); err != nil {
		return Angles{}, err
	}
	return out, nil
}

// MustAnglesDeg 用角度（度）构造合法的三角组，仅供固定用例/报表使用；
// 非法输入直接 panic。
func MustAnglesDeg(deg [3]float64) Angles {
	a, err := AnglesDeg(deg)
	if err != nil {
		panic(err)
	}
	return a
}

// AnglesDeg 把角度制三元组转成弧度并校验。
func AnglesDeg(deg [3]float64) (Angles, error) {
	var a Angles
	for i, d := range deg {
		a[i] = d * math.Pi / 180
	}
	if err := ValidateAngles(a); err != nil {
		return Angles{}, err
	}
	return a, nil
}

// SectorAreas 返回半径 r 下三个扇形的面积 A_i = 1/2 r² θ_i。
func SectorAreas(theta Angles, r float64) (Areas, error) {
	if err := ValidateAngles(theta); err != nil {
		return Areas{}, err
	}
	if math.IsNaN(r) || math.IsInf(r, 0) || r <= 0 {
		return Areas{}, fmt.Errorf("扇形半径必须为正有限数，实际 %g", r)
	}
	var a Areas
	k := 0.5 * r * r
	for i, t := range theta {
		a[i] = k * t
	}
	return a, nil
}

// TotalArea 是半径 r 的整圆面积 πr²，即三个扇形的守恒预算。
func TotalArea(r float64) float64 { return math.Pi * r * r }

// ConservationWeights 是核心反解：
//
//	w_i = A_i / ΣA = θ_i / 2π
//
// 由面积守恒方程 w_i·πr² = A_i 逐个解出。返回的权重之和在浮点误差内恒为 1。
func ConservationWeights(theta Angles) (Weights, error) {
	if err := ValidateAngles(theta); err != nil {
		return Weights{}, err
	}
	var w Weights
	for i, t := range theta {
		w[i] = t / Tau
	}
	return w, nil
}

// WeightsFromAreas 直接从三个扇形面积反解权重 w_i = A_i/ΣA。
// 这是“按面积守恒反解”的面积入口，与 ConservationWeights 必须一致。
func WeightsFromAreas(a Areas) (Weights, error) {
	s := SumAreas(a)
	if !(s > 0) || math.IsNaN(s) || math.IsInf(s, 0) {
		return Weights{}, fmt.Errorf("扇形面积总和必须为正有限数，实际 %g", s)
	}
	var w Weights
	for i, v := range a {
		if v < 0 || math.IsNaN(v) || math.IsInf(v, 0) {
			return Weights{}, &AngleError{Index: i, Value: v, Msg: "扇形面积不能为负或非有限"}
		}
		w[i] = v / s
	}
	return w, nil
}

// CheckWeightSum 单独核对某一个接点的权重和是否为 1。
// 返回绝对偏差；超过 WeightSumTol 即报错。
func CheckWeightSum(w Weights) (float64, error) {
	s := SumWeights(w)
	if math.IsNaN(s) || math.IsInf(s, 0) {
		return math.NaN(), errors.New("权重和不是有限数")
	}
	d := math.Abs(s - 1)
	if d > WeightSumTol {
		return d, fmt.Errorf("权重和必须恒为 1，实际 %.15f，偏差 %.3e 超过容差 %.0e", s, d, WeightSumTol)
	}
	return d, nil
}
