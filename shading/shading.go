package shading

import (
	"errors"

	"vecgo/colorspace"
	"vecgo/config"
	"vecgo/raster"
)

// Apply 保持历史签名：等价于在默认编码空间 ("encoded") 着色，产物与最初版本逐字节相同。
func Apply(im *raster.Image, mode string, top, bottom float64) error {
	return ApplySpace(im, mode, top, bottom, "")
}

// ApplySpace 与 Apply 相同，但允许用 space 选择渐变插值空间。
//
// space 取值见 config.NormalizeSpace：
//   - encoded（默认）：逐行线性插值归一化编码强度 t，再 uint8(255*t) 截断量化，
//     运算顺序与历史实现完全一致，默认空间下逐字节不变；
//   - linear：把两端 sRGB 色标先解码成线性光，在线性光上插值，再编码回 sRGB 后一次量化；
//   - perceptual：在 CIELAB 明度 L* 上均匀插值，再转回 sRGB 后一次量化。
//
// flat 模式没有插值过程，任何 space 下都与历史行为一致。
func ApplySpace(im *raster.Image, mode string, top, bottom float64, space string) error {
	if mode != "flat" && mode != "gradient" {
		return errors.New("未知着色模式")
	}
	if top < 0 || top > 1 || bottom < 0 || bottom > 1 {
		return errors.New("强度超出范围")
	}
	sp := config.NormalizeSpace(space)
	if sp != "encoded" && sp != "linear" && sp != "perceptual" {
		return errors.New("未知插值空间")
	}

	// 线性/感知空间需要的端点换算。top/bottom 始终按 sRGB 编码归一化值理解。
	yTop, yBottom := 0.0, 0.0
	lTop, lBottom := 0.0, 0.0
	if sp == "linear" {
		yTop = colorspace.SRGBToLinear(top)
		yBottom = colorspace.SRGBToLinear(bottom)
	} else if sp == "perceptual" {
		lTop = colorspace.LinearToLStar(colorspace.SRGBToLinear(top))
		lBottom = colorspace.LinearToLStar(colorspace.SRGBToLinear(bottom))
	}

	for row := 0; row < im.H; row++ {
		var v uint8
		if mode == "flat" {
			v = uint8(255 * top)
		} else {
			t := top
			u := 0.0 // 0..1 的行参数，H==1 时退化为顶端
			if im.H > 1 {
				u = float64(row) / float64(im.H-1)
				// 历史运算顺序，默认空间必须逐字节保持：
				t = top + (bottom-top)*u
			}
			switch sp {
			case "encoded":
				v = uint8(255 * t)
			case "linear":
				y := yTop + (yBottom-yTop)*u
				v = uint8(255 * colorspace.LinearToSRGB(y))
			case "perceptual":
				l := lTop + (lBottom-lTop)*u
				y := colorspace.LStarToLinear(l)
				v = uint8(255 * colorspace.LinearToSRGB(y))
			}
		}
		for col := 0; col < im.W; col++ {
			if im.Pix[row*im.W+col] != 0 {
				im.Pix[row*im.W+col] = v
			}
		}
	}
	return nil
}
