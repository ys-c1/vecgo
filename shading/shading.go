package shading

import (
	"errors"
	"math"

	"vecgo/raster"
)

func Apply(im *raster.Image, mode string, top, bottom float64) error {
	if mode != "flat" && mode != "gradient" {
		return errors.New("未知着色模式")
	}
	for _, v := range []float64{top, bottom} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return errors.New("强度非有限值")
		}
	}
	if top < 0 || top > 1 || bottom < 0 || bottom > 1 {
		return errors.New("强度超出范围")
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
