// Package colorspace 给出灰度编码值与线性光值之间的标准传递函数，
// 以及 CIELAB 明度 L* 换算。全部按 IEC 61966-2-1 (sRGB) 实现，
// 用来支撑“在哪一个空间做渐变插值”的可选项与色带临界间距推导。
package colorspace

import "math"

// SRGBToLinear 把 sRGB 编码灰度 c (0..1) 解码为线性光亮度 Y (0..1)。
// 分段阈值 0.04045 / 12.92 / 1.055 / 2.4 均为 IEC 61966-2-1 常数。
func SRGBToLinear(c float64) float64 {
	if c <= 0.04045 {
		return c / 12.92
	}
	return math.Pow((c+0.055)/1.055, 2.4)
}

// LinearToSRGB 是 SRGBToLinear 的逆：线性光 Y (0..1) -> sRGB 编码值 (0..1)。
func LinearToSRGB(y float64) float64 {
	if y <= 0 {
		return 0
	}
	if y >= 1 {
		return 1
	}
	if y <= 0.0031308 {
		return 12.92 * y
	}
	return 1.055*math.Pow(y, 1/2.4) - 0.055
}

// LinearToLStar 把相对线性亮度 Y (0..1，参考白=1) 换成 CIELAB 明度 L* (0..100)。
func LinearToLStar(y float64) float64 {
	if y <= 0 {
		return 0
	}
	if y > 1 {
		y = 1
	}
	if y > 216.0/24389.0 { // (6/29)^3 ≈ 0.008856
		return 116*math.Cbrt(y) - 16
	}
	return y * 24389.0 / 27.0 // 903.3*y
}

// LStarToLinear 是 LinearToLStar 的逆。
func LStarToLinear(l float64) float64 {
	if l <= 0 {
		return 0
	}
	if l >= 100 {
		return 1
	}
	if l > 8 {
		t := (l + 16) / 116
		return t * t * t
	}
	return l * 27.0 / 24389.0
}

// ByteToLinear 直接把一个 8-bit 编码码 k (0..255) 解码成线性光亮度。
func ByteToLinear(k uint8) float64 {
	return SRGBToLinear(float64(k) / 255.0)
}
