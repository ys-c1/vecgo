package shading

import (
	"vecgo/param"
	"vecgo/raster"
)

// Apply 按 mode 给画布上色。mode 须为 "flat" 或 "gradient"，top/bottom 须是
// [0, 1] 内的有限数；否则返回 *param.Error，字段如 shading.Apply.top。
// 画布本身（nil、宽高、像素数）也一并校验。
func Apply(im *raster.Image, mode string, top, bottom float64) error {
	if err := validate(im); err != nil {
		return err
	}
	if err := param.All(
		param.Choice("shading.Apply.mode", mode, param.ShadingModes),
		param.FloatIn("shading.Apply.top", top, param.IntensityMin, param.IntensityMax),
		param.FloatIn("shading.Apply.bottom", bottom, param.IntensityMin, param.IntensityMax),
	); err != nil {
		return err
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

func validate(im *raster.Image) error {
	return raster.ValidateImage("shading.Apply", im)
}
