// Package kernel 给出复合变换的雅可比、椭圆采样核的长短轴反解，
// 以及面积守恒约束、误差上界和直边灰雾审计。
//
// 题目场景：先旋转 45° 再缩到一半，边缘出现一层灰雾；原因是采样核在
// 复合变换下面积被雅可比行列式缩放，轴被拉长/压扁后没有按面积守恒重新归一。
package kernel

import "math"

// Mat2 是行主序的二阶矩阵：J = [[A, B], [C, D]]。
type Mat2 struct{ A, B, C, D float64 }

// CompositeJacobian 是复合变换
//
//	x' = Scale(sx, sy) · Rot(theta) · x
//
// 的雅可比。题目要求“先旋转再缩放”，所以缩放矩阵在左：
//
//	J = diag(sx, sy) R(theta)
//	  = [[ sx cosθ, -sx sinθ],
//	     [ sy sinθ,  sy cosθ]]
//
// 四个分量显式保留，便于复算轴比。
func CompositeJacobian(sx, sy, theta float64) Mat2 {
	c, s := math.Cos(theta), math.Sin(theta)
	return Mat2{
		A: sx * c, B: -sx * s,
		C: sy * s, D: sy * c,
	}
}

// Slice 按 [A, B, C, D] 返回雅可比四分量。
func (j Mat2) Slice() [4]float64 { return [4]float64{j.A, j.B, j.C, j.D} }

// Det 返回行列式 det J，即可微映射的面积缩放率。
func (j Mat2) Det() float64 { return j.A*j.D - j.B*j.C }

// singularValues 返回 J 的两个奇异值（小、大），即 JᵀJ 特征值的平方根。
// 二维下用 tr/2 ± sqrt((tr/2)^2 - det(J)^2) 直接求解。
func (j Mat2) singularValues() (lo, hi float64) {
	g00 := j.A*j.A + j.C*j.C
	g11 := j.B*j.B + j.D*j.D
	tr := g00 + g11
	detG := j.A*j.D - j.B*j.C
	disc := (tr/2)*(tr/2) - detG*detG
	if disc < 0 && disc > -1e-12 {
		disc = 0
	}
	root := math.Sqrt(math.Max(0, disc))
	lMax := tr/2 + root
	lMin := tr/2 - root
	if lMin < 0 && lMin > -1e-12 {
		lMin = 0
	}
	sHi := math.Sqrt(math.Max(0, lMax))
	sLo := math.Sqrt(math.Max(0, lMin))
	if sLo > sHi {
		sLo, sHi = sHi, sLo
	}
	return sLo, sHi
}

// SingularValues 导出奇异值（小、大）。
func (j Mat2) SingularValues() (lo, hi float64) { return j.singularValues() }

// AxisRatio 返回雅可比的条件数 κ = σmax/σmin ≥ 1，
// 也就是变换把单位圆拉成椭圆后的长短轴之比。
func (j Mat2) AxisRatio() float64 {
	lo, hi := j.singularValues()
	if lo < 1e-15 {
		return math.Inf(1)
	}
	return hi / lo
}

// MajorAngle 返回雅可比主方向（长轴方向）与 x 轴的夹角（弧度）：
//
//	φ = 1/2 atan2(2 g01, g00-g11)
//
// 各向同性（κ≈1）时方向无定义，返回 0。
func (j Mat2) MajorAngle() float64 {
	lo, hi := j.singularValues()
	if hi-lo <= 1e-9*math.Max(1, hi) {
		return 0
	}
	g00 := j.A*j.A + j.C*j.C
	g11 := j.B*j.B + j.D*j.D
	g01 := j.A*j.B + j.C*j.D
	return 0.5 * math.Atan2(2*g01, g00-g11)
}
