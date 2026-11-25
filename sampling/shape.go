package sampling

import (
	"math"
	"math/rand"

	"vecgo/geom"
)

// Shape 是单位像素 [0,1]^2 内的被测区域。覆盖率的真值由 Area 给出，
// 对一般多边形这是解析值，不依赖采样，因此可作为无偏性与方差的标尺。
type Shape interface {
	Contains(p geom.Point) bool
	Area() float64
}

// HalfPlane 表示单位正方形与半平面 {u·p <= c} 的交。
// u 为单位法向量，c 为有符号截距。
type HalfPlane struct {
	Ux, Uy, C float64
}

func (h HalfPlane) Contains(p geom.Point) bool {
	return h.Ux*p.X+h.Uy*p.Y <= h.C
}

// Area 用半平面解析裁剪单位正方形求交面积。
func (h HalfPlane) Area() float64 {
	return polygonArea(clipHalfPlane(unitSquare(), h))
}

// Strip 表示单位正方形与条带 {lo <= u·p <= hi} 的交，
// 用来制造细长、易与规则网格混叠的形状。
type Strip struct {
	Ux, Uy, Lo, Hi float64
}

func (s Strip) Contains(p geom.Point) bool {
	v := s.Ux*p.X + s.Uy*p.Y
	return v >= s.Lo && v <= s.Hi
}

func (s Strip) Area() float64 {
	lo := HalfPlane{Ux: s.Ux, Uy: s.Uy, C: s.Hi}
	hi := HalfPlane{Ux: -s.Ux, Uy: -s.Uy, C: -s.Lo}
	poly := clipHalfPlane(unitSquare(), lo)
	poly = clipHalfPlane(poly, hi)
	return polygonArea(poly)
}

// RandomShape 抽取一个随机形状：约一半为半平面，一半为随机取向的细条带。
// 所有形状保证与单位正方形有正面积的交，避免退化（面积 0 或 1）。
func RandomShape(rng *rand.Rand) Shape {
	theta := rng.Float64() * math.Pi
	ux, uy := math.Cos(theta), math.Sin(theta)
	if rng.Float64() < 0.5 {
		// 截距限制在 (0.1, 0.9)：既不退化，也覆盖各种边缘位置。
		c := 0.1 + 0.8*rng.Float64()
		return HalfPlane{Ux: ux, Uy: uy, C: c}
	}
	center := 0.25 + 0.5*rng.Float64()
	width := 0.05 + 0.15*rng.Float64()
	lo, hi := center-width/2, center+width/2
	return Strip{Ux: ux, Uy: uy, Lo: lo, Hi: hi}
}

func unitSquare() []geom.Point {
	return []geom.Point{
		{X: 0, Y: 0},
		{X: 1, Y: 0},
		{X: 1, Y: 1},
		{X: 0, Y: 1},
	}
}

// clipHalfPlane 用 Sutherland–Hodgman 算法把多边形裁剪到 h 的内侧。
func clipHalfPlane(poly []geom.Point, h HalfPlane) []geom.Point {
	if len(poly) == 0 {
		return nil
	}
	inside := func(p geom.Point) bool { return h.Contains(p) }
	out := make([]geom.Point, 0, len(poly)+1)
	for i := range poly {
		cur := poly[i]
		prv := poly[(i-1+len(poly))%len(poly)]
		ci, pi := inside(cur), inside(prv)
		switch {
		case ci && !pi:
			out = append(out, intersectLine(prv, cur, h))
			out = append(out, cur)
		case ci:
			out = append(out, cur)
		case pi:
			out = append(out, intersectLine(prv, cur, h))
		}
	}
	return out
}

// intersectLine 求线段 p->q 与边界直线 u·x = c 的交点（调用方保证不平行）。
func intersectLine(p, q geom.Point, h HalfPlane) geom.Point {
	dx, dy := q.X-p.X, q.Y-p.Y
	den := h.Ux*dx + h.Uy*dy
	t := (h.C - (h.Ux*p.X + h.Uy*p.Y)) / den
	return geom.Point{X: p.X + t*dx, Y: p.Y + t*dy}
}

// polygonArea 用鞋带公式计算多边形面积。
func polygonArea(poly []geom.Point) float64 {
	if len(poly) < 3 {
		return 0
	}
	sum := 0.0
	for i := range poly {
		q := poly[(i+1)%len(poly)]
		sum += poly[i].X*q.Y - q.X*poly[i].Y
	}
	return math.Abs(sum) / 2
}
