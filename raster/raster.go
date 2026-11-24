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

func (im *Image) Fill(polys [][]geom.Point, supersample int) error {
	return im.FillWithStats(polys, supersample, nil)
}

// FillWithStats 与 Fill 成像完全一致；若 st 非 nil 且已启用，则同时累积绘制计数。
// 统计只读取中间量，不改变任何被写入的像素值。
func (im *Image) FillWithStats(polys [][]geom.Point, supersample int, st *FillStats) error {
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
	den := supersample * supersample
	collect := st != nil && st.Enabled
	collectPerPixel := collect && st.KeepPerPixel
	if collect {
		st.width = im.W
	}
	if collectPerPixel {
		if cap(st.RepeatPerPixel) < im.W*im.H {
			st.RepeatPerPixel = make([]int, im.W*im.H)
		} else {
			st.RepeatPerPixel = st.RepeatPerPixel[:im.W*im.H]
		}
		if cap(st.CoveredPerPixel) < im.W*im.H {
			st.CoveredPerPixel = make([]int, im.W*im.H)
		} else {
			st.CoveredPerPixel = st.CoveredPerPixel[:im.W*im.H]
		}
	}
	for row := 0; row < im.H; row++ {
		for col := 0; col < im.W; col++ {
			covered := 0
			posts := 0
			// 每个子采样的几何判定；像素值与是否统计无关。
			for sj := 0; sj < supersample; sj++ {
				for si := 0; si < supersample; si++ {
					px := minx + (float64(col)+(float64(si)+0.5)/float64(supersample))*spanx/float64(im.W)
					py := miny + (float64(row)+(float64(sj)+0.5)/float64(supersample))*spany/float64(im.H)
					inside := 0
					firstHit := 0 // 现行路径在该样本上执行的几何判定次数
					for i, p := range polys {
						if geom.Inside(p, px, py) {
							inside++
							if firstHit == 0 {
								firstHit = i + 1
							}
							if !collect {
								break
							}
						}
					}
					if firstHit == 0 {
						firstHit = len(polys)
					}
					if inside > 0 {
						covered++
					}
					posts += inside
					if collect {
						st.ActualPosts++
						st.Tests += firstHit
						st.Samples++
						st.Posts += inside
						if inside > 0 {
							st.CoveredSamples++
						}
						if inside > 1 {
							st.OverlapSamples++
						}
					}
				}
			}
			im.Pix[row*im.W+col] = uint8(255 * covered / den)
			if collect {
				st.recordPixel(row, col, covered, posts, collectPerPixel)
			}
		}
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
