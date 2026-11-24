package raster

import (
	"errors"
	"fmt"
	"math"
	"os"

	"vecgo/geom"
)

type Image struct {
	W, H int
	Pix  []uint8
}

func New(w, h int) (*Image, error) {
	if w <= 0 || h <= 0 {
		return nil, errors.New("画幅必须为正")
	}
	return &Image{W: w, H: h, Pix: make([]uint8, w*h)}, nil
}

func (im *Image) Fill(polys [][]geom.Point, supersample int) error {
	return im.fill(polys, supersample, false)
}

// FillContain 保持世界坐标的横纵比例，把场景包围盒等比缩放到画幅内并居中。
// 换画幅时只改变平移留白，不会把正方形拉成宽矩形。
func (im *Image) FillContain(polys [][]geom.Point, supersample int) error {
	return im.fill(polys, supersample, true)
}

func (im *Image) fill(polys [][]geom.Point, supersample int, contain bool) error {
	if supersample < 1 {
		return errors.New("supersample 必须为正")
	}
	if len(polys) == 0 {
		return errors.New("多边形至少一个")
	}
	for _, p := range polys {
		if len(p) < 3 {
			return errors.New("多边形至少三个顶点")
		}
	}
	minx, miny, maxx, maxy := sceneBounds(polys)
	spanx, spany := maxx-minx, maxy-miny
	if spanx == 0 {
		spanx = 1
	}
	if spany == 0 {
		spany = 1
	}

	// sx/sy 的单位是“输出像素 / 世界单位”。旧 Fill 允许 sx != sy，
	// 因而宽画幅会把同一正方形横向拉伸；contain 强制二者相同。
	sx := float64(im.W) / spanx
	sy := float64(im.H) / spany
	offsetX, offsetY := 0.0, 0.0
	if contain {
		s := math.Min(sx, sy)
		sx, sy = s, s
		offsetX = (float64(im.W) - spanx*s) / 2
		offsetY = (float64(im.H) - spany*s) / 2
	}

	den := supersample * supersample
	for row := 0; row < im.H; row++ {
		for col := 0; col < im.W; col++ {
			hit := 0
			for sj := 0; sj < supersample; sj++ {
				for si := 0; si < supersample; si++ {
					ux := float64(col) + (float64(si)+0.5)/float64(supersample)
					uy := float64(row) + (float64(sj)+0.5)/float64(supersample)
					px := minx + (ux-offsetX)/sx
					py := miny + (uy-offsetY)/sy
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

func sceneBounds(polys [][]geom.Point) (minx, miny, maxx, maxy float64) {
	minx, miny = polys[0][0].X, polys[0][0].Y
	maxx, maxy = minx, miny
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
	return minx, miny, maxx, maxy
}

func (im *Image) SavePGM(path string) error {
	if im.W == 0 || im.H == 0 {
		return errors.New("空图不能导出")
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := fmt.Fprintf(f, "P5\n%d %d\n255\n", im.W, im.H); err != nil {
		return err
	}
	_, err = f.Write(im.Pix)
	return err
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
