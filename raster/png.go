package raster

import (
	"errors"
	"image"
	"image/png"
	"os"
)

// SavePNG 把灰度覆盖图编码为 8-bit PNG。
func (im *Image) SavePNG(path string) error {
	if im.W == 0 || im.H == 0 {
		return errors.New("空图不能导出")
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	g := image.NewGray(image.Rect(0, 0, im.W, im.H))
	copy(g.Pix, im.Pix)
	return png.Encode(f, g)
}
