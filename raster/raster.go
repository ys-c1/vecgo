package raster

import (
	"fmt"
	"os"

	"vecgo/geom"
	"vecgo/verr"
)

type Image struct {
	W, H int
	Pix  []uint8
}

// maxPixels 是单张图允许的最大像素数（4096x4096）。
// 有了上界，w*h 在 32 位平台也不会溢出，且超大画幅会返回编码错误而不是
// 让 make 抛 OOM panic。
const maxPixels = 4096 * 4096

func New(w, h int) (*Image, error) {
	if w <= 0 || h <= 0 {
		return nil, verr.Errorf(verr.KindRasterSize, "画幅必须为正: %dx%d", w, h)
	}
	if w*h > maxPixels || w*h/w != h {
		return nil, verr.Errorf(verr.KindRasterTooBig, "画幅超出上限或溢出: %dx%d", w, h)
	}
	return &Image{W: w, H: h, Pix: make([]uint8, w*h)}, nil
}

func (im *Image) Fill(polys [][]geom.Point, supersample int) error {
	if im == nil {
		return verr.Errorf(verr.KindRasterNilImage, "图像为空")
	}
	if len(im.Pix) != im.W*im.H {
		return verr.Errorf(verr.KindRasterBadImage, "像素缓冲与画幅不一致: len=%d %dx%d", len(im.Pix), im.W, im.H)
	}
	if supersample < 1 {
		return verr.Errorf(verr.KindRasterSuper, "supersample 必须为正")
	}
	if len(polys) == 0 {
		// 空场景不算失败：没有多边形就没有命中像素，返回未改动的图。
		return nil
	}
	for _, p := range polys {
		if len(p) < 3 {
			return verr.Errorf(verr.KindRasterPolyWide, "多边形至少三个顶点")
		}
	}
	minx, miny := polys[0][0].X, polys[0][0].Y
	maxx, maxy := minx, miny
	for _, p := range polys {
		for _, q := range p {
			if q.X < minx {
				minx = q.X
			}
			if q.X > maxx {
				maxx = q.X
			}
			if q.Y < miny {
				miny = q.Y
			}
			if q.Y > maxy {
				maxy = q.Y
			}
		}
	}
	spanx, spany := maxx-minx, maxy-miny
	if spanx == 0 {
		spanx = 1
	}
	if spany == 0 {
		spany = 1
	}
	den := supersample * supersample
	for row := 0; row < im.H; row++ {
		for col := 0; col < im.W; col++ {
			hit := 0
			for sj := 0; sj < supersample; sj++ {
				for si := 0; si < supersample; si++ {
					px := minx + (float64(col)+(float64(si)+0.5)/float64(supersample))*spanx/float64(im.W)
					py := miny + (float64(row)+(float64(sj)+0.5)/float64(supersample))*spany/float64(im.H)
					for _, p := range polys {
						if geom.Inside(p, px, py) {
							hit++
							break
						}
					}
				}
			}
			im.Pix[row*im.W+col] = uint8(255 * hit / den)
		}
	}
	return nil
}

func (im *Image) SavePGM(path string) (err error) {
	if im == nil || im.W == 0 || im.H == 0 {
		return verr.Errorf(verr.KindRasterEmptyImg, "空图不能导出")
	}
	f, err := os.Create(path)
	if err != nil {
		return verr.Wrap(verr.KindRasterCreate, err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = verr.Wrap(verr.KindRasterClose, cerr)
		}
	}()
	if _, err = fmt.Fprintf(f, "P5\n%d %d\n255\n", im.W, im.H); err != nil {
		return verr.Wrap(verr.KindRasterHeader, err)
	}
	_, err = f.Write(im.Pix)
	if err != nil {
		return verr.Wrap(verr.KindRasterWrite, err)
	}
	return nil
}

func (im *Image) CountOn(threshold uint8) int {
	n := 0
	for _, v := range im.Pix {
		if v >= threshold {
			n++
		}
	}
	return n
}
