package shading

import (
	"math"

	"vecgo/raster"
	"vecgo/verr"
)

func Apply(im *raster.Image, mode string, top, bottom float64) error {
	if im == nil {
		return verr.Errorf(verr.KindShadingNilImage, "图像为空")
	}
	if im.W <= 0 || im.H <= 0 || len(im.Pix) != im.W*im.H {
		return verr.Errorf(verr.KindShadingBadImage, "像素缓冲与画幅不一致: len=%d %dx%d", len(im.Pix), im.W, im.H)
	}
	if mode != "flat" && mode != "gradient" {
		return verr.Errorf(verr.KindShadingMode, "未知着色模式: %q", mode)
	}
	if math.IsNaN(top) || math.IsNaN(bottom) ||
		top < 0 || top > 1 || bottom < 0 || bottom > 1 {
		return verr.Errorf(verr.KindShadingStrength, "强度超出范围: top=%v bottom=%v", top, bottom)
	}
	for row := 0; row < im.H; row++ {
		var v uint8
		if mode == "flat" {
			v = uint8(255 * top)
		} else {
			t := top
			if im.H > 1 {
				t = top + (bottom-top)*float64(row)/float64(im.H-1)
			}
			v = uint8(255 * t)
		}
		for col := 0; col < im.W; col++ {
			if im.Pix[row*im.W+col] != 0 {
				im.Pix[row*im.W+col] = v
			}
		}
	}
	return nil
}
