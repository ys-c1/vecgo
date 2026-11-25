// Package cull 实现三角形相对视窗的包围球剔除与逐三角形精确可见性判定。
package cull

import (
	"math"

	"vecgo/geom"
)

// Bounds 是一组点的轴对齐包围盒。
type Bounds struct {
	MinX, MinY, MaxX, MaxY float64
}

// View 是视窗矩形（与包围盒同结构，语义独立）。
type View struct {
	MinX, MinY, MaxX, MaxY float64
}

// Sphere 是二维包围球（圆盘），CX/CY 为球心，R 为半径。
type Sphere struct {
	CX, CY, R float64
}

// TriBounds 求三角形的轴对齐包围盒。
func TriBounds(tri []geom.Point) Bounds {
	b := Bounds{
		MinX: tri[0].X, MaxX: tri[0].X,
		MinY: tri[0].Y, MaxY: tri[0].Y,
	}
	for _, p := range tri[1:] {
		if p.X < b.MinX {
			b.MinX = p.X
		}
		if p.X > b.MaxX {
			b.MaxX = p.X
		}
		if p.Y < b.MinY {
			b.MinY = p.Y
		}
		if p.Y > b.MaxY {
			b.MaxY = p.Y
		}
	}
	return b
}

// Diagonal 返回包围盒对角线长度 D。
func (b Bounds) Diagonal() float64 {
	return math.Hypot(b.MaxX-b.MinX, b.MaxY-b.MinY)
}

// ConservativeSphere 取盒心为球心、r = D/2 为半径的包围球。
// 该球完整包含包围盒（盒内任意点到盒心距离不超过 D/2），保守。
func ConservativeSphere(b Bounds) Sphere {
	return Sphere{
		CX: (b.MinX + b.MaxX) / 2,
		CY: (b.MinY + b.MaxY) / 2,
		R:  b.Diagonal() / 2,
	}
}

// LooseSphere 取 r = max(w,h)/2 的缩水球，是常见的错误实现：
// 对非正方形包围盒 r/D < 0.5，球包不住盒角，会误杀完全可见的三角形。
// 仅用于复现“画面缺一角”，不应用于生产路径。
func LooseSphere(b Bounds) Sphere {
	w, h := b.MaxX-b.MinX, b.MaxY-b.MinY
	return Sphere{
		CX: (b.MinX + b.MaxX) / 2,
		CY: (b.MinY + b.MaxY) / 2,
		R:  math.Max(w, h) / 2,
	}
}

// Culled 判定球是否完全落在视窗外。
// strict=false（保守模式）：带相对容差，吸收浮点舍入；
// strict=true（严格模式）：纯浮点比较，配合 LooseSphere 复现边界误杀。
func (s Sphere) Culled(v View, strict bool) bool {
	nx := math.Max(v.MinX, math.Min(s.CX, v.MaxX))
	ny := math.Max(v.MinY, math.Min(s.CY, v.MaxY))
	d := math.Hypot(s.CX-nx, s.CY-ny)
	if strict {
		return d > s.R
	}
	tol := 1e-12*s.R + 1e-12
	return d > s.R+tol
}

// ExactVisible 是逐三角形精确判定：复用光栅器的超采样点网格，
// 只要有一个采样点落在三角形内即判可见。与 raster.FillView 逐像素一致。
func ExactVisible(tri []geom.Point, v View, w, h, supersample int) bool {
	spanx, spany := v.MaxX-v.MinX, v.MaxY-v.MinY
	for row := 0; row < h; row++ {
		for col := 0; col < w; col++ {
			for sj := 0; sj < supersample; sj++ {
				for si := 0; si < supersample; si++ {
					px := v.MinX + (float64(col)+(float64(si)+0.5)/float64(supersample))*spanx/float64(w)
					py := v.MinY + (float64(row)+(float64(sj)+0.5)/float64(supersample))*spany/float64(h)
					if geom.Inside(tri, px, py) {
						return true
					}
				}
			}
		}
	}
	return false
}
