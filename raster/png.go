package raster

import (
	"os"

	"vecgo/pngx"
)

// SavePNG 以 PNG(8 位灰度) 导出, 编码由 vecgo/pngx 自实现。
// 不影响 SavePGM 的任何行为。
func (im *Image) SavePNG(path string) error {
	data, err := pngx.Encode(im.W, im.H, im.Pix)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
