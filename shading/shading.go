package shading

import (
	"errors"

	"vecgo/raster"
)

func Apply(im *raster.Image, mode string, top, bottom float64) error {
	return ApplyRect(im, mode, top, bottom, raster.Rect{X0: 0, Y0: 0, X1: im.W, Y1: im.H})
}

// ApplyRect 只处理矩形区域。逐像素结果与 Apply 完全一致：
// 行号 row 仍取整幅图的绝对行号，因此渐变按整帧高度计算。
func ApplyRect(im *raster.Image, mode string, top, bottom float64, r raster.Rect) error {
	if mode != "flat" && mode != "gradient" {
		return errors.New("未知着色模式")
	}
	if top < 0 || top > 1 || bottom < 0 || bottom > 1 {
		return errors.New("强度超出范围")
	}
	if !r.InBounds(im.W, im.H) {
		return errors.New("着色区域越界")
	}
	for row := r.Y0; row < r.Y1; row++ {
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
		for col := r.X0; col < r.X1; col++ {
			if im.Pix[row*im.W+col] != 0 {
				im.Pix[row*im.W+col] = v
			}
		}
	}
	return nil
}
