package kernel

import (
	"math"
)

// Ellipse 是采样核椭圆：Long/Short 为长/短半轴（像素单位），
// Angle 为长轴相对 x 轴的朝向（弧度）。
type Ellipse struct {
	Long  float64 `json:"long"`
	Short float64 `json:"short"`
	Angle float64 `json:"angle_rad"`
}

// AxisProduct 是长短半轴之积 a·b，即雅可比意义下的“面积因子”：
// 线性映射 M 对面积的缩放就是 |det M|，而对角轴矩阵 diag(a,b) 的行列式正是 a·b。
func (e Ellipse) AxisProduct() float64 { return e.Long * e.Short }

// GeometricArea 是椭圆的真实几何面积 π·a·b。
func (e Ellipse) GeometricArea() float64 { return math.Pi * e.Long * e.Short }

// ConservedEllipse 按面积守恒反解长短半轴。
//
// 给定轴比 κ = a/b ≥ 1 与面积约束 πab = 1（核面积等于一），解得
//
//	a = sqrt(κ/π),  b = 1/sqrt(πκ)
//
// 恒有 a/b = κ 且 πab = 1。
func ConservedEllipse(ratio, angle float64) Ellipse {
	if ratio < 1 {
		ratio = 1 / ratio
	}
	if ratio < 1 || math.IsNaN(ratio) || math.IsInf(ratio, 0) {
		ratio = 1
	}
	return Ellipse{
		Long:  math.Sqrt(ratio / math.Pi),
		Short: 1 / math.Sqrt(math.Pi*ratio),
		Angle: angle,
	}
}

// ConservedAxesProduct 给出另一种常用归一约定：轴积 ab = 1。
// 此时 a = sqrt(κ), b = 1/sqrt(κ)。它与 πab=1 的解只差公共缩放 1/sqrt(π)，
// 轴比、朝向以及任何灰度结论都不受影响，仅核宽整体差一个形状常数。
func ConservedAxesProduct(ratio float64) (long, short float64) {
	if ratio < 1 {
		ratio = 1
	}
	return math.Sqrt(ratio), 1 / math.Sqrt(ratio)
}

// ConservedForJacobian 直接由复合变换雅可比反解面积守恒椭圆核：
// 轴比取奇异值之比，朝向取主方向。
func ConservedForJacobian(j Mat2) Ellipse {
	return ConservedEllipse(j.AxisRatio(), j.MajorAngle())
}

// AreaResidual 是核几何面积相对“面积等于一”的守恒残差 |πab - 1|。
func AreaResidual(e Ellipse) float64 {
	return math.Abs(e.GeometricArea() - 1)
}

// AreaErrorBound 是守恒残差的严格上界。
//
// 设正确轴为 a0,b0（πa0b0=1），实际轴 a=a0+δa, b=b0+δb，
// 且已知认证偏差预算 |δa|≤εa、|δb|≤εb，则
//
//	|πab - 1|
//	= π|b0 δa + a0 δb + δa δb|
//	≤ π(b0 εa + a0 εb + εa εb)
//
// 返回值 U 即该上界。任何满足预算的实现，其实测面积残差都必须落在 U 内。
func AreaErrorBound(epsLong, epsShort, long0, short0 float64) float64 {
	return math.Pi * (short0*epsLong + long0*epsShort + epsLong*epsShort)
}

// AxisDeviations 返回实际轴相对守恒解的相对偏差（带符号）。
func AxisDeviations(e Ellipse, long0, short0 float64) (relLong, relShort float64) {
	return e.Long/long0 - 1, e.Short/short0 - 1
}

// RatioErrorBound 给出轴比误差 |a/b - κ| 的严格上界。
//
// 正确轴 a0,b0 满足 κ=a0/b0；实际轴在认证预算 |δa|≤εa、|δb|≤εb 内，
// 且 εb<b0。则实际轴比 ρ=a/b 落在
//
//	(a0-εa)/(b0+εb) ≤ ρ ≤ (a0+εa)/(b0-εb)
//
// 所以 |ρ-κ| ≤ max(κ-lo, hi-κ)。
func RatioErrorBound(kappa, epsLong, epsShort, long0, short0 float64) float64 {
	lo := math.Max(0, long0-epsLong) / (short0 + epsShort)
	hi := (long0 + epsLong) / math.Max(1e-15, short0-epsShort)
	return math.Max(math.Abs(kappa-lo), math.Abs(hi-kappa))
}

// OffendingAxis 指出守恒偏差落在哪条轴上：
// "long" 仅长轴超差、"short" 仅短轴超差、"both" 两轴都超差、"none" 均在 tol 内。
func OffendingAxis(e Ellipse, long0, short0, tol float64) string {
	dl, ds := AxisDeviations(e, long0, short0)
	badL := math.Abs(dl) > tol
	badS := math.Abs(ds) > tol
	switch {
	case badL && badS:
		return "both"
	case badL:
		return "long"
	case badS:
		return "short"
	default:
		return "none"
	}
}
