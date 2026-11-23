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

// Mapping 是多边形坐标到像素坐标的仿射映射，由整帧场景的包围盒决定。
// 全量路径与增量路径必须共用同一份映射，否则同一位置的采样值会不同。
type Mapping struct {
	MinX, MinY   float64
	SpanX, SpanY float64
}

// SceneMapping 计算整帧场景的映射，规则与原 Fill 完全一致。
func SceneMapping(polys [][]geom.Point) (Mapping, error) {
	if len(polys) == 0 || len(polys[0]) == 0 {
		return Mapping{}, errors.New("场景为空")
	}
	minx, miny := polys[0][0].X, polys[0][0].Y
	maxx, maxy := minx, miny
	for _, p := range polys {
		if len(p) == 0 {
			return Mapping{}, errors.New("多边形至少三个顶点")
		}
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
	return Mapping{MinX: minx, MinY: miny, SpanX: spanx, SpanY: spany}, nil
}

// Same 判断两帧的映射是否逐位相同。映射变化意味着所有像素都会变。
func (m Mapping) Same(o Mapping) bool {
	return m == o
}

// PixelBBox 返回一组点（已按映射变换）可能覆盖的像素矩形。
// 结果未裁剪；完全落在画幅外时可能得到负坐标或超出画幅的矩形。
func (m Mapping) PixelBBox(pts []geom.Point, w, h int) Rect {
	if len(pts) == 0 {
		return Rect{}
	}
	minx, miny := pts[0].X, pts[0].Y
	maxx, maxy := minx, miny
	for _, q := range pts {
		minx = math.Min(minx, q.X)
		miny = math.Min(miny, q.Y)
		maxx = math.Max(maxx, q.X)
		maxy = math.Max(maxy, q.Y)
	}
	fx := float64(w) / m.SpanX
	fy := float64(h) / m.SpanY
	// 采样点位于像素内部 (offset ∈ (0,1))，因此用 floor/ceil 足够覆盖，
	// 再多留 1 像素余量以吸收浮点误差。宁大勿小：多画的像素结果仍与全量一致。
	x0 := int(math.Floor((minx-m.MinX)*fx)) - 1
	y0 := int(math.Floor((miny-m.MinY)*fy)) - 1
	x1 := int(math.Ceil((maxx-m.MinX)*fx)) + 1
	y1 := int(math.Ceil((maxy-m.MinY)*fy)) + 1
	return Rect{X0: x0, Y0: y0, X1: x1, Y1: y1}.Clamp(w, h)
}

// fillRect 用与全量 Fill 完全相同的采样公式填充矩形 r。
// dst 是整帧像素缓冲，索引一律用绝对行号，全量与局部路径共用这一函数。
// 映射依赖整张图的维度 w/h，因此即使只填局部也必须传入整帧尺寸。
func (m Mapping) fillRect(polys [][]geom.Point, supersample, w, h int, r Rect, dst []uint8) {
	den := supersample * supersample
	ssf := float64(supersample)
	for row := r.Y0; row < r.Y1; row++ {
		for col := r.X0; col < r.X1; col++ {
			hit := 0
			for sj := 0; sj < supersample; sj++ {
				for si := 0; si < supersample; si++ {
					px := m.MinX + (float64(col)+(float64(si)+0.5)/ssf)*m.SpanX/float64(w)
					py := m.MinY + (float64(row)+(float64(sj)+0.5)/ssf)*m.SpanY/float64(h)
					for _, p := range polys {
						if geom.Inside(p, px, py) {
							hit++
							break
						}
					}
				}
			}
			dst[row*w+col] = uint8(255 * hit / den)
		}
	}
}

func (im *Image) Fill(polys [][]geom.Point, supersample int) error {
	if supersample < 1 {
		return errors.New("supersample 必须为正")
	}
	for _, p := range polys {
		if len(p) < 3 {
			return errors.New("多边形至少三个顶点")
		}
	}
	m, err := SceneMapping(polys)
	if err != nil {
		return err
	}
	return im.FillWithMapping(polys, supersample, m)
}

// FillWithMapping 用调用方给定的映射填满整张图。
// 增量会话必须固定首帧映射，否则任何一个图形移动都会改变全部采样坐标。
func (im *Image) FillWithMapping(polys [][]geom.Point, supersample int, m Mapping) error {
	if supersample < 1 {
		return errors.New("supersample 必须为正")
	}
	for _, p := range polys {
		if len(p) < 3 {
			return errors.New("多边形至少三个顶点")
		}
	}
	m.fillRect(polys, supersample, im.W, im.H, Rect{X0: 0, Y0: 0, X1: im.W, Y1: im.H}, im.Pix)
	return nil
}

// FillRectInto 只填充矩形区域，像素值与全量填充在相同位置完全一致。
func (im *Image) FillRectInto(polys [][]geom.Point, supersample int, m Mapping, r Rect) error {
	if supersample < 1 {
		return errors.New("supersample 必须为正")
	}
	for _, p := range polys {
		if len(p) < 3 {
			return errors.New("多边形至少三个顶点")
		}
	}
	if !r.InBounds(im.W, im.H) {
		return fmt.Errorf("填充区域越界: %+v", r)
	}
	m.fillRect(polys, supersample, im.W, im.H, r, im.Pix)
	return nil
}

func (im *Image) Clone() *Image {
	out := &Image{W: im.W, H: im.H, Pix: make([]uint8, len(im.Pix))}
	copy(out.Pix, im.Pix)
	return out
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

// Bytes 返回与 SavePGM 完全一致的完整文件字节，便于逐字节比对。
func (im *Image) Bytes() []byte {
	head := fmt.Sprintf("P5\n%d %d\n255\n", im.W, im.H)
	out := make([]byte, 0, len(head)+len(im.Pix))
	out = append(out, head...)
	out = append(out, im.Pix...)
	return out
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
