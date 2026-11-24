package raster

import (
	"errors"
	"math"

	"vecgo/geom"
)

// SampleKind 选择子采样点的分布规则。
type SampleKind uint8

const (
	// SampleGrid 是默认的 m×m 居中均匀网格（与历史产物逐字节兼容）。
	SampleGrid SampleKind = iota
	// SampleLowDiscrepancy 是 Hammersley(2) 低差异点集，可再叠加
	// Cranley–Patterson 平移（FillOptions.ShiftX/ShiftY）做随机重复。
	SampleLowDiscrepancy
)

// ViewBox 固定世界坐标到像素的映射窗口 [X, X+W]×[Y, Y+H]。
type ViewBox struct {
	X, Y, W, H float64
}

// FillOptions 控制 FillSampled 的采样分布。
type FillOptions struct {
	// Kind 选择网格或低差异序列。
	Kind SampleKind
	// Samples 是每像素样本总数。网格模式下必须是完全平方数（Samples=m²）。
	Samples int
	// ShiftX/ShiftY 是在单位像素正方形上叠加的 Cranley–Patterson 平移，
	// 取值 [0,1)；零值即无平移的确定性点集。
	ShiftX float64
	ShiftY float64
	// View 为 nil 时沿用 Image.Fill 的多边形包围盒映射；非 nil 时使用固定
	// 世界窗口（让边落在画面内部，便于做重复出图的方差实验）。
	View *ViewBox
}

// FillSampled 与 Image.Fill 使用完全相同的包围盒映射和覆盖率量化，
// 区别只在于子采样点可选用低差异序列并叠加种子确定的环面平移。
// 默认渲染链路（pipe.Run -> Image.Fill）不经过这里，因此默认 PGM 不变。
func (im *Image) FillSampled(polys [][]geom.Point, o FillOptions) error {
	if o.Samples < 1 {
		return errors.New("samples 必须为正")
	}
	m := 0
	switch o.Kind {
	case SampleGrid:
		m = int(math.Sqrt(float64(o.Samples)))
		if m*m != o.Samples {
			return errors.New("网格采样数必须是完全平方数")
		}
	case SampleLowDiscrepancy:
	default:
		return errors.New("未知采样类型")
	}
	if math.IsNaN(o.ShiftX) || math.IsNaN(o.ShiftY) ||
		o.ShiftX < 0 || o.ShiftX >= 1 || o.ShiftY < 0 || o.ShiftY >= 1 {
		return errors.New("平移量必须落在 [0,1)")
	}
	for _, p := range polys {
		if len(p) < 3 {
			return errors.New("多边形至少三个顶点")
		}
	}
	var minx, miny, spanx, spany float64
	if o.View != nil {
		if o.View.W <= 0 || o.View.H <= 0 {
			return errors.New("视口宽高必须为正")
		}
		minx, miny = o.View.X, o.View.Y
		spanx, spany = o.View.W, o.View.H
	} else {
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
	}
	for row := 0; row < im.H; row++ {
		for col := 0; col < im.W; col++ {
			hit := 0
			for i := 0; i < o.Samples; i++ {
				var sx, sy float64
				if o.Kind == SampleGrid {
					sx, sy = GridSample(m, i/m, i%m)
				} else {
					sx, sy = HammersleySample(o.Samples, i)
				}
				sx, sy = ShiftCP(sx, sy, o.ShiftX, o.ShiftY)
				px := minx + (float64(col)+sx)*spanx/float64(im.W)
				py := miny + (float64(row)+sy)*spany/float64(im.H)
				for _, p := range polys {
					if geom.Inside(p, px, py) {
						hit++
						break
					}
				}
			}
			im.Pix[row*im.W+col] = uint8(255 * hit / o.Samples)
		}
	}
	return nil
}
