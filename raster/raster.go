package raster

import (
	"errors"
	"fmt"
	"os"

	"vecgo/bounds"
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

// toBounds 把多边形列表转成 bounds 包的点，保留下标信息。
func toBounds(polys [][]geom.Point) [][]bounds.Point {
	out := make([][]bounds.Point, len(polys))
	for i, p := range polys {
		out[i] = make([]bounds.Point, len(p))
		for j, q := range p {
			out[i][j] = bounds.Point{X: q.X, Y: q.Y}
		}
	}
	return out
}

// Fill 用超采样把一组多边形填进画布，坐标域由场景自身决定（与原实现一致：
// 全部多边形的总包围盒铺满画幅）。因此这里没有“画幅越界”的概念，
// 只判定两件事，全部经 bounds：
//
//   - 顶点含 NaN/±Inf → 拒绝，报出 polys[i][j]；
//   - 多边形包围盒宽或高为 0（退化）→ 拒绝，报出 polys[i]。
//     旧实现把 0 跨度静默换成 1，把一条线段拉伸成整幅图。
//
// 需要“顶点必须落在某个世界坐标域内”的调用方用 FillAt。
func (im *Image) Fill(polys [][]geom.Point, supersample int) error {
	return im.fill(polys, supersample, nil)
}

// FillAt 与 Fill 相同，但额外要求每个顶点都落在 frame 内（四条边算界内）。
// 负坐标、超画幅、非有限数一律拒绝，错误里带 polys[i][j] 与坐标。
// 采样映射仍由图形总包围盒决定，frame 只约束输入坐标的合法性。
func (im *Image) FillAt(polys [][]geom.Point, supersample int, frame bounds.Frame) error {
	if err := bounds.CheckFrame("raster", "FillAt", frame); err != nil {
		return err
	}
	return im.fill(polys, supersample, &frame)
}

func (im *Image) fill(polys [][]geom.Point, supersample int, frame *bounds.Frame) error {
	if supersample < 1 {
		return errors.New("supersample 必须为正")
	}
	if len(polys) == 0 {
		return errors.New("polys 不能为空")
	}
	for i, p := range polys {
		if len(p) < 3 {
			return fmt.Errorf("polys[%d] 至少三个顶点", i)
		}
	}
	groups := toBounds(polys)
	if frame != nil {
		// Groups 同时判定有限数与域归属，错误字段为 raster.FillAt.polys[i][j]。
		if err := bounds.Groups("raster", "FillAt", "polys", *frame, groups); err != nil {
			return err
		}
	} else if err := bounds.FiniteGroups("raster", "Fill", "polys", groups); err != nil {
		return err
	}
	entry := "Fill"
	if frame != nil {
		entry = "FillAt"
	}
	for i, p := range groups {
		bb := bounds.FrameOf(p)
		sx, sy := bb.Size()
		if err := bounds.Extent("raster", entry, fmt.Sprintf("polys[%d]", i), sx, sy); err != nil {
			return err
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

// InFrame 判定像素 (x,y) 是否落在画幅内。这是像素索引判定（半开区间），
// 供 post 的卷积邻域与需要按像素取值的调用方使用，判定只有 bounds 一处。
func (im *Image) InFrame(x, y int) bool {
	return bounds.ContainsPixel(x, y, im.W, im.H)
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
