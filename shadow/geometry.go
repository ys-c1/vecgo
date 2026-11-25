// Package shadow 由面光源的相似三角形几何出发，统一实现硬/软阴影。
//
// 截面约定（垂直于遮挡边）：
//
//	接收面 R：y = 0
//	遮挡边 B：(0, h)          h = Gap，遮挡边到接收面的距离（遮挡距离）
//	光源平面：y = H           H = Dist，光源到接收面的距离
//	面光源范围：x ∈ [-L/2, +L/2]，L = Size；L = 0 即点光源
//
// 两条边界光线（光源端点 -> 遮挡边 -> 接收面）各对应一对相似直角三角形，
// 给出左右两个半影半宽，相加即总半影宽：w = L·h/(H-h)。
package shadow

import (
	"errors"
	"math"
)

// Geometry 是半影公式所需的全部几何量（长度单位自洽即可）。
type Geometry struct {
	// Size 是面光源在截面方向上的尺寸 L；0 表示点光源（极限情形）。
	Size float64
	// Gap 是遮挡边到接收面的距离 h（“遮挡距离”），必须为正。
	Gap float64
	// Dist 是光源平面到接收面的总距离 H，必须严格大于 Gap。
	Dist float64
}

// Validate 检查几何是否能构造出有限的相似三角形。
func (g Geometry) Validate() error {
	if math.IsNaN(g.Size) || math.IsInf(g.Size, 0) || g.Size < 0 {
		return errors.New("shadow: 光源尺寸 Size 必须为非负有限值")
	}
	if !(g.Gap > 0) {
		return errors.New("shadow: 遮挡距离 Gap 必须为正")
	}
	if !(g.Dist > g.Gap) {
		return errors.New("shadow: 光源距离 Dist 必须严格大于遮挡距离 Gap（H>h>0）")
	}
	return nil
}

// ratio 是两次相似三角形共用的投影比 r = h/(H-h)。
func (g Geometry) ratio() float64 {
	return g.Gap / (g.Dist - g.Gap)
}

// TriangleRatio 记录一条边界光线对应的一对相似直角三角形。
//
// 比例（以右侧为例）：
//
//	ReceiverRun / Gap = SourceHalf / LightToBlocker
//	    x+      / h   =    L/2     /     H-h
type TriangleRatio struct {
	// Side 为 "right" 或 "left"（以光源中心/遮挡边为 x=0）。
	Side string
	// SourceHalf = L/2，光源端点到光源中心的直角边。
	SourceHalf float64
	// LightToBlocker = H-h，光源平面到遮挡平面的直角边。
	LightToBlocker float64
	// Gap = h，遮挡平面到接收面的竖直直角边。
	Gap float64
	// ReceiverRun = |x±|，该侧半影半宽。
	ReceiverRun float64
	// Proportion 是该对相似三角形的无量纲比：h/(H-h) = |x±|/(L/2)。
	Proportion float64
}

// TriangleRatios 返回两条边界光线各自的相似三角形比例。
// 两个比例都显式给出，便于直接观察 L→0 时两个半宽同时趋零。
func (g Geometry) TriangleRatios() (right, left TriangleRatio, err error) {
	if err := g.Validate(); err != nil {
		return TriangleRatio{}, TriangleRatio{}, err
	}
	r := g.ratio()
	half := g.Size / 2
	run := half * r
	right = TriangleRatio{
		Side:           "right",
		SourceHalf:     half,
		LightToBlocker: g.Dist - g.Gap,
		Gap:            g.Gap,
		ReceiverRun:    run,
		Proportion:     r,
	}
	left = right
	left.Side = "left"
	return right, left, nil
}

// EdgeShift 是光源坐标 ell 处的点光源，把遮挡边投影到接收面时
// 相对中心投影的平移量（世界单位）：
//
//	δ(ell) = -ell · h/(H-h)
//
// 它正是单次相似三角形比例在任意光源点上的应用；
// 两个光源端点的平移跨度 δ(-L/2)-δ(+L/2) = L·h/(H-h) = Penumbra。
func (g Geometry) EdgeShift(ell float64) (float64, error) {
	if err := g.Validate(); err != nil {
		return 0, err
	}
	return -ell * g.ratio(), nil
}

// HalfPenumbra 分别返回右、左半影半宽（世界单位）。
func (g Geometry) HalfPenumbra() (right, left float64, err error) {
	r, l, err := g.TriangleRatios()
	if err != nil {
		return 0, 0, err
	}
	return r.ReceiverRun, l.ReceiverRun, nil
}

// Penumbra 是解析总半影宽（世界单位）：w = L·h/(H-h)。
// Size=0（点光源）时严格返回 0，即极限公式本身。
func (g Geometry) Penumbra() (float64, error) {
	if err := g.Validate(); err != nil {
		return 0, err
	}
	return g.Size * g.ratio(), nil
}

// PenumbraPixels 把解析半影宽换算成输出像素：w_px = pxPerUnit·L·h/(H-h)。
func (g Geometry) PenumbraPixels(pxPerUnit float64) (float64, error) {
	w, err := g.Penumbra()
	if err != nil {
		return 0, err
	}
	if !(pxPerUnit > 0) {
		return 0, errors.New("shadow: pxPerUnit 必须为正")
	}
	return w * pxPerUnit, nil
}

// PointLimit 演示极限：光源尺寸趋零时，半影宽趋零。
// 返回 Size=0 的 Geometry 在同一 H、h 下的解析半影宽（恒为 0）。
func PointLimit(g Geometry) (float64, error) {
	point := g
	point.Size = 0
	return point.Penumbra()
}
