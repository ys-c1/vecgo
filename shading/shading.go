package shading

import (
	"errors"

	"vecgo/color"
	"vecgo/raster"
)

// Apply 按线性光口径给已填充像素着色。
// top/bottom 是 0..1 的线性强度；渐变在线性空间插值后，统一通过
// color.LinearToByte 量化进线性缓冲，编码转换只在 color 包内存在一份。
func Apply(im *raster.Image, mode string, top, bottom float64) error {
	if mode != "flat" && mode != "gradient" {
		return errors.New("shading: mode 非法")
	}
	if top < 0 || top > 1 || bottom < 0 || bottom > 1 {
		return errors.New("shading: top/bottom 强度超出范围")
	}
	for row := 0; row < im.H; row++ {
		t := top
		if mode == "gradient" && im.H > 1 {
			t = top + (bottom-top)*float64(row)/float64(im.H-1)
		}
		v := color.LinearToByte(t)
		for col := 0; col < im.W; col++ {
			if im.Pix[row*im.W+col] != 0 {
				im.Pix[row*im.W+col] = v
			}
		}
	}
	return nil
}
