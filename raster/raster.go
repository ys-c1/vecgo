package raster

import (
	"errors"
	"fmt"
	"os"
	"sort"

	"vecgo/geom"
)

type Image struct {
	W, H int
	Pix  []uint8
}

// Sample 是像素内部的亚像素采样点，用整数格点坐标表示：
// 归一化位置为 ((I+0.5)/Grid, (J+0.5)/Grid)，坐标分量均落在 [0, Grid)。
type Sample struct {
	I, J int
}

// SampleSet 是一组以整数格点描述的亚像素采样点。
type SampleSet struct {
	Grid int
	Pts  []Sample
}

// Validate 检查布局合法：网格为正、点位不越界、且不存在重复点。
func (s SampleSet) Validate() error {
	if s.Grid <= 0 {
		return errors.New("采样网格必须为正")
	}
	if len(s.Pts) == 0 {
		return errors.New("采样点不能为空")
	}
	seen := make(map[Sample]bool, len(s.Pts))
	for _, p := range s.Pts {
		if p.I < 0 || p.I >= s.Grid || p.J < 0 || p.J >= s.Grid {
			return fmt.Errorf("采样点越界: %+v", p)
		}
		if seen[p] {
			return fmt.Errorf("采样点重复: %+v", p)
		}
		seen[p] = true
	}
	return nil
}

// mirrorX 给出点关于像素竖直中线镜像后的整数格点：i -> Grid-1-i，j 不变。
func (s SampleSet) mirrorX(p Sample) Sample {
	return Sample{I: s.Grid - 1 - p.I, J: p.J}
}

// rotate180 给出点关于像素中心旋转 180° 后的整数格点：(i,j) -> (Grid-1-i, Grid-1-j)。
func (s SampleSet) rotate180(p Sample) Sample {
	return Sample{I: s.Grid - 1 - p.I, J: s.Grid - 1 - p.J}
}

func asSet(pts []Sample) map[Sample]bool {
	m := make(map[Sample]bool, len(pts))
	for _, p := range pts {
		m[p] = true
	}
	return m
}

// MirrorSymmetric 判定布局是否关于竖直中线对称，即集合等式 S = M(S)。
// 同时返回两边的集合差，便于核对不满足时的差异点位。
func (s SampleSet) MirrorSymmetric() (bool, []Sample, []Sample) {
	img := make([]Sample, len(s.Pts))
	for k, p := range s.Pts {
		img[k] = s.mirrorX(p)
	}
	left, right := asSet(s.Pts), asSet(img)
	return setEquals(left, right), setDiff(left, right), setDiff(right, left)
}

// CentralSymmetric 判定布局是否关于像素中心中心对称，即集合等式 S = R(S)。
// 同时返回两边的集合差。
func (s SampleSet) CentralSymmetric() (bool, []Sample, []Sample) {
	img := make([]Sample, len(s.Pts))
	for k, p := range s.Pts {
		img[k] = s.rotate180(p)
	}
	left, right := asSet(s.Pts), asSet(img)
	return setEquals(left, right), setDiff(left, right), setDiff(right, left)
}

func setEquals(a, b map[Sample]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for p := range a {
		if !b[p] {
			return false
		}
	}
	return true
}

// setDiff 返回 a \\ b，按字典序输出以便比对。
func setDiff(a, b map[Sample]bool) []Sample {
	out := make([]Sample, 0)
	for p := range a {
		if !b[p] {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(k, l int) bool {
		if out[k].I != out[l].I {
			return out[k].I < out[l].I
		}
		return out[k].J < out[l].J
	})
	return out
}

func New(w, h int) (*Image, error) {
	if w <= 0 || h <= 0 {
		return nil, errors.New("画幅必须为正")
	}
	return &Image{W: w, H: h, Pix: make([]uint8, w*h)}, nil
}

// bounds 求所有多边形顶点的包围盒，零宽/高时退化为 1。
func bounds(polys [][]geom.Point) (minx, miny, maxx, maxy float64) {
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
	if maxx == minx {
		maxx = minx + 1
	}
	if maxy == miny {
		maxy = miny + 1
	}
	return minx, miny, maxx, maxy
}

// FillSamples 用显式的整数格点采样布局做多边形覆盖率光栅化。
// 像素灰度为 255*命中点数/总采样点数（整数除法，与 Fill 口径一致）。
func (im *Image) FillSamples(polys [][]geom.Point, ss SampleSet) error {
	if err := ss.Validate(); err != nil {
		return err
	}
	for _, p := range polys {
		if len(p) < 3 {
			return errors.New("多边形至少三个顶点")
		}
	}
	minx, miny, maxx, maxy := bounds(polys)
	spanx, spany := maxx-minx, maxy-miny
	den := len(ss.Pts)
	for row := 0; row < im.H; row++ {
		for col := 0; col < im.W; col++ {
			hit := 0
			for _, sp := range ss.Pts {
				px := minx + (float64(col)+(float64(sp.I)+0.5)/float64(ss.Grid))*spanx/float64(im.W)
				py := miny + (float64(row)+(float64(sp.J)+0.5)/float64(ss.Grid))*spany/float64(im.H)
				for _, p := range polys {
					if geom.Inside(p, px, py) {
						hit++
						break
					}
				}
			}
			im.Pix[row*im.W+col] = uint8(255 * hit / den)
		}
	}
	return nil
}

func (im *Image) Fill(polys [][]geom.Point, supersample int) error {
	if supersample < 1 {
		return errors.New("supersample 必须为正")
	}
	grid := make([]Sample, 0, supersample*supersample)
	for sj := 0; sj < supersample; sj++ {
		for si := 0; si < supersample; si++ {
			grid = append(grid, Sample{I: si, J: sj})
		}
	}
	return im.FillSamples(polys, SampleSet{Grid: supersample, Pts: grid})
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
