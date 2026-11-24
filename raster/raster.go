package raster

import (
	"errors"
	"fmt"
	"math"
	"os"

	"vecgo/geom"
	"vecgo/sum"
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
	if supersample < 1 {
		return errors.New("supersample 必须为正")
	}
	if len(polys) == 0 {
		return errors.New(`字段 "polys" 为空数组`)
	}
	for pi, p := range polys {
		if len(p) < 3 {
			return fmt.Errorf(`字段 "polys[%d]" 至少三个顶点`, pi)
		}
		for vi, q := range p {
			if math.IsNaN(q.X) || math.IsNaN(q.Y) {
				return fmt.Errorf(`字段 "polys[%d][%d]" 坐标为 NaN`, pi, vi)
			}
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
	den := float64(supersample * supersample)
	// 复用缓冲，避免逐像素分配；长度随命中数增长，容量上限为 supersample²。
	samples := make([]float64, 0, supersample*supersample)
	for row := 0; row < im.H; row++ {
		for col := 0; col < im.W; col++ {
			samples = samples[:0]
			for sj := 0; sj < supersample; sj++ {
				for si := 0; si < supersample; si++ {
					px := minx + (float64(col)+(float64(si)+0.5)/float64(supersample))*spanx/float64(im.W)
					py := miny + (float64(row)+(float64(sj)+0.5)/float64(supersample))*spany/float64(im.H)
					for _, p := range polys {
						if geom.Inside(p, px, py) {
							// 命中样本按 sj、si 的固定下标顺序收集，
							// 再交由 sum.Float64 确定性左折叠。
							samples = append(samples, 1)
							break
						}
					}
				}
			}
			if len(samples) == 0 {
				im.Pix[row*im.W+col] = 0
				continue
			}
			// 覆盖率 = 确定性累加的命中数 / 总样本数；
			// sum 已保证跨机器、跨优化选项逐位一致。
			hits, err := sum.Float64("coverage", samples)
			if err != nil {
				return err
			}
			im.Pix[row*im.W+col] = uint8(255 * hits / den)
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
