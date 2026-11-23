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
		return nil, fmt.Errorf("画幅必须为正: width 字段=%d height 字段=%d", w, h)
	}
	return &Image{W: w, H: h, Pix: make([]uint8, w*h)}, nil
}

// validateFill 集中串行与并行两条路径共用的入参校验，错误约定与 Fill 一致：非法即返回 error。
func (im *Image) validateFill(polys [][]geom.Point, supersample int) error {
	if im == nil || im.W <= 0 || im.H <= 0 {
		w, h := 0, 0
		if im != nil {
			w, h = im.W, im.H
		}
		return fmt.Errorf("画幅为零: width 字段=%d height 字段=%d", w, h)
	}
	if supersample < 1 {
		return errors.New("supersample 必须为正")
	}
	for _, p := range polys {
		if len(p) < 3 {
			return errors.New("多边形至少三个顶点")
		}
	}
	return nil
}

// sceneBounds 计算场景包围盒，零跨度按 1 处理，串行/并行共用同一份计算。
func sceneBounds(polys [][]geom.Point) (minx, miny, spanx, spany float64) {
	minx, miny = polys[0][0].X, polys[0][0].Y
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
	spanx, spany = maxx-minx, maxy-miny
	if spanx == 0 {
		spanx = 1
	}
	if spany == 0 {
		spany = 1
	}
	return minx, miny, spanx, spany
}

// renderRows 渲染行区间 [startRow, endRow)，是串行与并行两条路径共用的唯一计算内核。
// 调用方必须保证不同调用的行区间互不相交，因此对 im.Pix 的写天然无共享。
func (im *Image) renderRows(polys [][]geom.Point, supersample int, minx, miny, spanx, spany float64, startRow, endRow int) {
	den := supersample * supersample
	for row := startRow; row < endRow; row++ {
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
}

func (im *Image) Fill(polys [][]geom.Point, supersample int) error {
	if err := im.validateFill(polys, supersample); err != nil {
		return err
	}
	// 空场景：没有任何多边形，结果为全黑图，不访问 polys[0]。
	if len(polys) == 0 {
		return nil
	}
	minx, miny, spanx, spany := sceneBounds(polys)
	im.renderRows(polys, supersample, minx, miny, spanx, spany, 0, im.H)
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
