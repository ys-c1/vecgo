// Package mipstudy 研究像素足迹非正方形时，纹理缩小层号（mip level / LOD）公式的误差。
//
// 设像素在纹理空间的足迹为轴对齐矩形，边长（单位：纹素）为 a、b，且 a >= b。
// 足迹长短轴之比 r = a/b。
//
// 标准层号按最长边决定（保证最长方向每个输出像素至少覆盖一条纹素）：
//
//	L* = log2(a)
//
// 常见的“面积近似”公式按足迹面积开方决定，等价于取两边几何平均：
//
//	L_0 = log2(sqrt(a*b)) = log2(a) - 1/2*log2(a/b) = L* - 1/2*log2(r)
//
// 因此面积公式的层号误差只与轴比 r 有关：
//
//	e(r) = L_0 - L* = -1/2*log2(r)
//
// 按轴比修正即可还原标准公式，修正项：
//
//	C(r) = +1/2*log2(r)，  L* = L_0 + C(r)
//
// r 趋一时 C(1) = 0，修正退化。
package mipstudy

import "math"

// AxisRatio 返回足迹长短轴之比 r = a/b（r >= 1），入参为足迹两边的纹素长度。
func AxisRatio(a, b float64) float64 {
	if a < b {
		a, b = b, a
	}
	if b <= 0 {
		// 退化足迹：短轴为零时按无穷轴比处理，交给上层决定。
		return math.Inf(1)
	}
	return a / b
}

// LevelArea 面积近似层号：log2(sqrt(a*b))，即未修正公式。
func LevelArea(a, b float64) float64 {
	if a <= 0 || b <= 0 {
		return math.Inf(-1)
	}
	return 0.5 * math.Log2(a*b)
}

// LevelMaxAxis 标准层号：log2(max(a,b))，以最长边为准。
func LevelMaxAxis(a, b float64) float64 {
	if a <= 0 || b <= 0 {
		return math.Inf(-1)
	}
	return math.Log2(math.Max(a, b))
}

// Correction 按轴比 r 定义的修正项 C(r) = +1/2*log2(r)。r=1 时为零。
func Correction(r float64) float64 {
	if r < 1 {
		r = 1 / r
	}
	return 0.5 * math.Log2(r)
}

// LevelCorrected 按轴比修正后的层号：L_0 + C(r)，与 LevelMaxAxis 恒等。
func LevelCorrected(a, b float64) float64 {
	return LevelArea(a, b) + Correction(AxisRatio(a, b))
}

// ErrorArea 未修正公式相对标准层号的误差 e(r) = -1/2*log2(r)。
func ErrorArea(r float64) float64 {
	if r < 1 {
		r = 1 / r
	}
	return -0.5 * math.Log2(r)
}

// FloorLevel 离散层号：连续层号向下取整并夹到 [0, maxLevel]，模拟整数 mip 级选择。
func FloorLevel(level float64, maxLevel int) int {
	k := int(math.Floor(level))
	if k < 0 {
		return 0
	}
	if k > maxLevel {
		return maxLevel
	}
	return k
}
