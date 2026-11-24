package raster

import (
	"errors"
	"fmt"
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

// Fill 按超采样覆盖率填充全部多边形，同一像素被多个图形覆盖时只取
// 并集覆盖率（不会把覆盖率相加）。不收集统计信息；需要统计时用
// FillStats。
func (im *Image) Fill(polys [][]geom.Point, supersample int) error {
	return im.fill(polys, supersample, nil)
}

// FillStats 与 Fill 填充出的图像逐字节相同，同时记录每个像素被多少个
// 图形写入（即覆盖该像素的图形个数，同一图形只记一次）。
//
// 统计只读地观察填充过程：它额外的多边形内外判定不影响 hit 计数，
// 也不影响写入 Pix 的字节。
func (im *Image) FillStats(polys [][]geom.Point, supersample int) (*CoverageStats, error) {
	st := &CoverageStats{}
	if err := im.fill(polys, supersample, st); err != nil {
		return nil, err
	}
	return st, nil
}

func (im *Image) fill(polys [][]geom.Point, supersample int, st *CoverageStats) error {
	if supersample < 1 {
		return errors.New("supersample 必须为正")
	}
	for _, p := range polys {
		if len(p) < 3 {
			return errors.New("多边形至少三个顶点")
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
	if st != nil {
		st.begin(im.W, im.H, supersample, polys, minx, miny, spanx, spany)
	}
	var seen []bool
	if st != nil {
		seen = make([]bool, len(polys))
	}
	den := supersample * supersample
	for row := 0; row < im.H; row++ {
		for col := 0; col < im.W; col++ {
			hit := 0
			if st != nil {
				for i := range seen {
					seen[i] = false
				}
			}
			for sj := 0; sj < supersample; sj++ {
				for si := 0; si < supersample; si++ {
					px := minx + (float64(col)+(float64(si)+0.5)/float64(supersample))*spanx/float64(im.W)
					py := miny + (float64(row)+(float64(sj)+0.5)/float64(supersample))*spany/float64(im.H)
					found := false
					for k, p := range polys {
						if geom.Inside(p, px, py) {
							found = true
							if st == nil {
								break
							}
							// 统计开启时不提前跳出，才能记下所有覆盖
							// 该像素的图形；hit 仍按子采样点只记一次。
							seen[k] = true
						}
					}
					if found {
						hit++
					}
				}
			}
			im.Pix[row*im.W+col] = uint8(255 * hit / den)
			if st != nil {
				st.record(row*im.W+col, seen)
			}
		}
	}
	if st != nil {
		st.finish(polys)
	}
	return nil
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
