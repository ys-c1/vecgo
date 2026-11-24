package raster

import "vecgo/geom"

// Mapping 是多边形集体包围盒（世界坐标）到画幅像素坐标的逐轴拉伸映射，
// 与 Fill / FillRandom 以及 coverage 估计使用的坐标口径完全一致。
type Mapping struct {
	MinX, MinY   float64
	SpanX, SpanY float64
}

// NewMapping 按所有多边形全部顶点的包围盒建立映射；退化跨度按 Fill 的
// 老规矩置 1，避免除零。
func NewMapping(polys [][]geom.Point) Mapping {
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
	return Mapping{MinX: minx, MinY: miny, SpanX: spanx, SpanY: spany}
}

// PixelToWorld 把画幅内连续像素坐标 (u,v)（有效范围约 [0,W]×[0,H]）
// 映射回多边形所在的世界坐标。浮点运算顺序与原 Fill 内联表达式保持一致：
// Min + u*Span/float64(size)。
func (m Mapping) PixelToWorld(w, h int, u, v float64) (x, y float64) {
	x = m.MinX + u*m.SpanX/float64(w)
	y = m.MinY + v*m.SpanY/float64(h)
	return x, y
}
