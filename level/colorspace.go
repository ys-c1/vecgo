package level

import "math"

// IEC 61966-2-1 sRGB 与 CIE L* 的标准常数。
const (
	srgbCutoff = 0.04045
	srgbToe    = 0.0031308
	cieEpsilon = 216.0 / 24389.0
	cieKappa   = 24389.0 / 27.0
)

// SRGBDecode 把归一化的 sRGB 编码值 c∈[0,1] 解码为线性光亮度 Y。
func SRGBDecode(c float64) float64 {
	if c <= 0 {
		return 0
	}
	if c <= srgbCutoff {
		return c / 12.92
	}
	return math.Pow((c+0.055)/1.055, 2.4)
}

// SRGBEncode 把线性光亮度 Y∈[0,1] 编码为归一化 sRGB 值。
func SRGBEncode(y float64) float64 {
	if y <= 0 {
		return 0
	}
	if y <= srgbToe {
		return 12.92 * y
	}
	return 1.055*math.Pow(y, 1/2.4) - 0.055
}

// LabDecode 是 Lab 分布使用的“编码参数 t → 线性光”解码，等价于 SRGBDecode。
func LabDecode(t float64) float64 { return SRGBDecode(t) }

// LabEncode 把线性光重新编码回参数轴，等价于 SRGBEncode。
func LabEncode(y float64) float64 { return SRGBEncode(y) }

// Lstar 返回 sRGB 编码灰 t∈[0,1] 对应的 CIE L* 明度（0..100）。
func Lstar(t float64) float64 {
	y := SRGBDecode(t)
	var f float64
	if y > cieEpsilon {
		f = math.Cbrt(y)
	} else {
		f = (cieKappa*y + 16) / 116
	}
	return 116*f - 16
}

// LfromStar 是 Lstar 的逆：由 L* 求线性光亮度 Y。
func LfromStar(L float64) float64 {
	if L <= 8 {
		return L / cieKappa
	}
	f := (L + 16) / 116
	y := f * f * f
	if y <= cieEpsilon {
		return L / cieKappa
	}
	return y
}
