// Package light 处理多光源叠加时的权重求解。
//
// 核心问题：两盏灯的线性强度直接相加会得到单灯的两倍能量，高亮处容易裁切；
// 朴素的“线性归一”（把权重归一到和为 1，两灯即各 1/2、取算术平均）又不符合
// 感知亮度的可加性，暗部层次会被抬平/发闷发灰。
//
// 本包按“感知亮度守恒”求一个公共增益权重 w，使
//
//	P(w*(I1+I2+...)) = P(I1) + P(I2) + ...
//
// 其中 P 是显式定义的感知亮度函数（默认 CIE L* 归一化明度）。这是关于 w 的
// 非线性方程，用带二分保护的 Newton 迭代求解，并报告迭代次数与残差。
//
// 单光源是该方程的平凡特例：w=1、输出=输入，Combine 对 n==1 直接原样返回，
// 不走迭代、不做任何缩放，保证单灯路径逐值/逐字节不变。
package light

import "math"

// Rec.709 / sRGB 在线性光域的相对亮度系数。彩色时
//
//	Y = 0.2126 R_lin + 0.7152 G_lin + 0.0722 B_lin。
//
// 本仓是灰度 PGM，R=G=B=I，于是 Y=I，系数自然退化为标量强度。
func Rec709Luminance(r, g, b float64) float64 {
	return 0.2126*r + 0.7152*g + 0.0722*b
}

// CIE L* 分段常数：ε=(6/29)^3=216/24389，κ=24389/27。
const (
	CIEEpsilon = 216.0 / 24389.0
	CIEKappa   = 24389.0 / 27.0
)

// Lightness 返回 CIE 1976 明度 L*（0..100；Y>1 时按公式可超过 100，
// 用于 HDR 中间量）。入参 y 是相对亮度 Y，灰度图即线性强度 I。
//
//	L*(Y) = 116 f(Y) - 16
//	f(t)  = t^(1/3),                 t > ε
//	      = t/(3δ²) + 4/29 = κt/116, t ≤ ε, δ=6/29
//
// 线性支保证 L* 在 Y=0 处为 0，且与立方根支在 Y=ε 处一阶导数连续。
func Lightness(y float64) float64 {
	if y <= 0 {
		return 0
	}
	if y <= CIEEpsilon {
		return CIEKappa * y
	}
	return 116*math.Cbrt(y) - 16
}

// Perceived 返回归一化感知亮度 P(Y)=L*(Y)/100，量程参考白为 1。
// 本包所有“感知亮度守恒”方程都在这个 [0,1] 尺度上写。
func Perceived(y float64) float64 {
	return Lightness(y) / 100.0
}

// InvLightness 是 Lightness 的逆：给定 L*（0..100）反解相对亮度 Y。
func InvLightness(l float64) float64 {
	switch {
	case l <= 0:
		return 0
	case l <= 8.0: // L*=κ ε=8，恰好对应线性支
		return l / CIEKappa
	default:
		f := (l + 16) / 116.0
		return f * f * f
	}
}

// InvPerceived 是 Perceived 的逆：P^{-1}(p)。
func InvPerceived(p float64) float64 {
	return InvLightness(100.0 * p)
}

// dLightness 是 dL*/dY 的解析导数，供 Newton 迭代使用。
// 立方支：d/dY [116 Y^(1/3)-16] = 116/(3 Y^(2/3)) = 116/(3 cbrt(Y)^2)。
func dLightness(y float64) float64 {
	if y <= CIEEpsilon {
		return CIEKappa // 含 y≤0：线性支斜率恒定
	}
	c := math.Cbrt(y)
	return 116.0 / (3.0 * c * c)
}

// GammaBrightness 是另一常用感知亮度定义（Stevens 幂律 / 显示 gamma 近似）：
// P_γ(I)=I^(1/γ)，γ 典型 2.2（闭式工作台可取 2）。用于和 CIE L* 交叉核对，
// 不是求解器默认口径。
func GammaBrightness(i, gamma float64) float64 {
	if i <= 0 {
		return 0
	}
	if gamma <= 0 {
		return math.NaN()
	}
	return math.Pow(i, 1.0/gamma)
}

// InvGammaBrightness 是 GammaBrightness 的逆。
func InvGammaBrightness(p, gamma float64) float64 {
	if p <= 0 {
		return 0
	}
	return math.Pow(p, gamma)
}
