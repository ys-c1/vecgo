// Package frame 提供自动取景：按场景包围盒与目标留白比例反解窗口四边，
// 保证图形完整落在画面内；并分别核验占屏比例与不裁顶点两项约束。
// 本包为新增路径，pipe/raster 的固定窗口路径保持不变。
package frame

import (
	"fmt"
	"math"

	"vecgo/geom"
)

// Window 取景窗口，世界坐标四边。
type Window struct{ MinX, MinY, MaxX, MaxY float64 }

func (w Window) SpanX() float64 { return w.MaxX - w.MinX }
func (w Window) SpanY() float64 { return w.MaxY - w.MinY }

// Spec 取景约束。
type Spec struct {
	Aspect  float64 // 画布宽高比 W/H，必须为正有限数
	Margin  float64 // 每边目标留白比例，[0, 0.5)
	FillMin float64 // 占屏比例下限，(0,1]
	FillMax float64 // 占屏比例上限，[FillMin,1]
}

func (s Spec) Validate() error {
	if math.IsNaN(s.Aspect) || math.IsInf(s.Aspect, 0) || s.Aspect <= 0 {
		return fmt.Errorf("aspect 必须为正有限数: %v", s.Aspect)
	}
	if math.IsNaN(s.Margin) || s.Margin < 0 || s.Margin >= 0.5 {
		return fmt.Errorf("margin 超出范围 [0,0.5): %v", s.Margin)
	}
	if math.IsNaN(s.FillMin) || s.FillMin <= 0 || s.FillMin > 1 {
		return fmt.Errorf("fillMin 超出范围 (0,1]: %v", s.FillMin)
	}
	if math.IsNaN(s.FillMax) || s.FillMax < s.FillMin || s.FillMax > 1 {
		return fmt.Errorf("fillMax 超出范围 [%v,1]: %v", s.FillMin, s.FillMax)
	}
	return nil
}

// BBox 计算场景包围盒；场景为空或无顶点时报错并指出字段 scene。
func BBox(scene [][]geom.Point) (lo, hi geom.Point, err error) {
	if len(scene) == 0 {
		return lo, hi, fmt.Errorf("scene 为空")
	}
	first := true
	for _, poly := range scene {
		for _, p := range poly {
			if first {
				lo, hi = p, p
				first = false
				continue
			}
			lo.X = math.Min(lo.X, p.X)
			lo.Y = math.Min(lo.Y, p.Y)
			hi.X = math.Max(hi.X, p.X)
			hi.Y = math.Max(hi.Y, p.Y)
		}
	}
	if first {
		return lo, hi, fmt.Errorf("scene 为空（无顶点）")
	}
	return lo, hi, nil
}

// Solve 按包围盒与留白比例反解窗口四边。
// 窗口宽高比等于画布宽高比，包围盒居中，图形完整落在画面内。
func Solve(scene [][]geom.Point, spec Spec) (Window, error) {
	if err := spec.Validate(); err != nil {
		return Window{}, err
	}
	lo, hi, err := BBox(scene)
	if err != nil {
		return Window{}, err
	}
	bw, bh := hi.X-lo.X, hi.Y-lo.Y
	if !(bw > 0) || !(bh > 0) {
		return Window{}, fmt.Errorf("bbox 退化: spanX=%v spanY=%v", bw, bh)
	}
	inner := 1 - 2*spec.Margin // 图形在受限方向上的占屏边长比
	wy := math.Max(bh, bw/spec.Aspect) / inner
	wx := wy * spec.Aspect
	cx, cy := (lo.X+hi.X)/2, (lo.Y+hi.Y)/2
	return Window{
		MinX: cx - wx/2,
		MinY: cy - wy/2,
		MaxX: cx + wx/2,
		MaxY: cy + wy/2,
	}, nil
}

// Checks 取景核验结果；占屏比例与不裁顶点分别核验。
type Checks struct {
	FillRatio float64 // 占屏比例：包围盒面积 / 窗口面积
	FillOK    bool    // 占屏比例落在 [FillMin, FillMax]
	Clipped   int     // 落在窗口外的顶点数
	NoClipOK  bool    // 无顶点被裁
}

// Verify 核验窗口：占屏比例须落在给定区间，且不得裁掉任何顶点。
func Verify(scene [][]geom.Point, w Window, spec Spec) (Checks, error) {
	if err := spec.Validate(); err != nil {
		return Checks{}, err
	}
	lo, hi, err := BBox(scene)
	if err != nil {
		return Checks{}, err
	}
	bw, bh := hi.X-lo.X, hi.Y-lo.Y
	if !(bw > 0) || !(bh > 0) {
		return Checks{}, fmt.Errorf("bbox 退化: spanX=%v spanY=%v", bw, bh)
	}
	area := w.SpanX() * w.SpanY()
	if !(area > 0) {
		return Checks{}, fmt.Errorf("window 退化: spanX=%v spanY=%v", w.SpanX(), w.SpanY())
	}
	c := Checks{FillRatio: bw * bh / area}
	c.FillOK = c.FillRatio >= spec.FillMin && c.FillRatio <= spec.FillMax
	const eps = 1e-9
	for _, poly := range scene {
		for _, p := range poly {
			if p.X < w.MinX-eps || p.X > w.MaxX+eps || p.Y < w.MinY-eps || p.Y > w.MaxY+eps {
				c.Clipped++
			}
		}
	}
	c.NoClipOK = c.Clipped == 0
	return c, nil
}
