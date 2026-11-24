package raster

import (
	"errors"
	"fmt"
	"math"
	"os"

	"vecgo/geom"
)

// 单个图形校验失败时使用的哨兵错误。
var (
	ErrPolygonTooFewPoints = errors.New("多边形至少三个顶点")
	ErrInvalidCoordinate   = errors.New("多边形坐标非法（NaN/Inf）")
)

// Skipped 记录一个被跳过的图形：Index 为其在输入场景中的编号（从 0 起），
// Reason 为跳过原因。
type Skipped struct {
	Index  int
	Reason string
}

// SkippedList 按编号升序排列（构造时即按输入顺序追加）。
type SkippedList []Skipped

// IDs 返回升序的被跳过图形编号，供脚本直接读取。
func (l SkippedList) IDs() []int {
	ids := make([]int, len(l))
	for i, s := range l {
		ids[i] = s.Index
	}
	return ids
}

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

// Fill 以传统整帧事务方式光栅化：任一图形不合法即返回错误，整帧无产出。
// 行为与加入失败隔离之前完全一致。
func (im *Image) Fill(polys [][]geom.Point, supersample int) error {
	if supersample < 1 {
		return errors.New("supersample 必须为正")
	}
	for _, p := range polys {
		if len(p) < 3 {
			return ErrPolygonTooFewPoints
		}
	}
	im.rasterize(polys, supersample)
	return nil
}

// FillIsolated 开启失败隔离：逐个图形校验，非法图形只跳过自身并记录编号，
// 其余图形照常绘制。全部图形都被跳过时返回一张全黑图（已无图形可画），
// 不视为错误。返回的 SkippedList 按编号升序。
func (im *Image) FillIsolated(polys [][]geom.Point, supersample int) (SkippedList, error) {
	if supersample < 1 {
		return nil, errors.New("supersample 必须为正")
	}
	skipped := SkippedList{}
	valid := make([][]geom.Point, 0, len(polys))
	for i, p := range polys {
		if err := validatePolygon(p); err != nil {
			skipped = append(skipped, Skipped{Index: i, Reason: err.Error()})
			continue
		}
		valid = append(valid, p)
	}
	if len(valid) > 0 {
		im.rasterize(valid, supersample)
	}
	return skipped, nil
}

// validatePolygon 校验单个图形：顶点数与坐标有界性。
// 坐标含 NaN/Inf 时会污染整帧包围盒，必须在进入光栅化之前剔除。
func validatePolygon(p []geom.Point) error {
	if len(p) < 3 {
		return ErrPolygonTooFewPoints
	}
	for _, q := range p {
		if math.IsNaN(q.X) || math.IsNaN(q.Y) || math.IsInf(q.X, 0) || math.IsInf(q.Y, 0) {
			return ErrInvalidCoordinate
		}
	}
	return nil
}

// rasterize 是 Fill 与 FillIsolated 共用的光栅化核心，调用方须保证
// polys 非空且每个图形都通过 validatePolygon。两条路径调用同一函数，
// 因此对同一组合法图形保证逐像素相同。
func (im *Image) rasterize(polys [][]geom.Point, supersample int) {
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
