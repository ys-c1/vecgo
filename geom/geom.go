package geom

import (
	"errors"
	"math"
)

type Point struct{ X, Y float64 }

func Bezier(p0, p1, p2, p3 Point, t float64) Point {
	u := 1 - t
	a := u * u * u
	b := 3 * u * u * t
	c := 3 * u * t * t
	d := t * t * t
	return Point{a*p0.X + b*p1.X + c*p2.X + d*p3.X, a*p0.Y + b*p1.Y + c*p2.Y + d*p3.Y}
}

func Flatten(p0, p1, p2, p3 Point, segments int) ([]Point, error) {
	if segments < 1 {
		return nil, errors.New("segments 必须为正")
	}
	out := make([]Point, segments+1)
	for i := 0; i <= segments; i++ {
		out[i] = Bezier(p0, p1, p2, p3, float64(i)/float64(segments))
	}
	return out, nil
}

func Length(pts []Point) float64 {
	total := 0.0
	for i := 0; i+1 < len(pts); i++ {
		total += math.Hypot(pts[i+1].X-pts[i].X, pts[i+1].Y-pts[i].Y)
	}
	return total
}

func Inside(poly []Point, px, py float64) bool {
	inside := false
	n := len(poly)
	for i := 0; i < n; i++ {
		ax, ay := poly[i].X, poly[i].Y
		bx, by := poly[(i+1)%n].X, poly[(i+1)%n].Y
		if (ay > py) == (by > py) {
			continue
		}
		t := (py - ay) / (by - ay)
		if ax+(bx-ax)*t > px {
			inside = !inside
		}
	}
	return inside
}

// Clip 用凸窗口裁剪多边形（Sutherland–Hodgman），返回拓扑干净的环：
// 相邻重复点、共线中间点、零面积段都会被清掉，结果满足欧拉不变量
// V-E+F=2（单点/线段等“无面积”结果除外，见 ClipDiagnosis.HasArea）。
//
// 需要看 SH 原始顶点流（用于诊断“前后对不上”）请用 ClipRaw 或
// ClipReport。整体落在窗口外时返回 nil 环，不报错。
func Clip(poly, window []Point) ([]Point, error) {
	raw, err := ClipRaw(poly, window)
	if err != nil {
		return nil, err
	}
	return CleanPolygon(raw).Ring, nil
}

// ClipRaw 返回 Sutherland–Hodgman 的原始顶点流，不做退化清理。
// 它在贴边/退化窗口下会含重复相邻点，因此未必满足欧拉不变量；
// 生产路径请用 Clip。
func ClipRaw(poly, window []Point) ([]Point, error) {
	if len(poly) < 3 || len(window) < 3 {
		return nil, errors.New("裁剪需要至少三个顶点")
	}
	out := append([]Point{}, poly...)
	for i := range window {
		a := window[i]
		b := window[(i+1)%len(window)]
		ex, ey := b.X-a.X, b.Y-a.Y
		inside := func(p Point) bool {
			return ex*(p.Y-a.Y)-ey*(p.X-a.X) >= 0
		}
		nxt := []Point{}
		for j := range out {
			cur := out[j]
			prv := out[(j-1+len(out))%len(out)]
			ci, pi := inside(cur), inside(prv)
			if ci && !pi {
				nxt = append(nxt, intersect(prv, cur, a, b))
			}
			if ci {
				nxt = append(nxt, cur)
			} else if pi {
				nxt = append(nxt, intersect(prv, cur, a, b))
			}
		}
		out = nxt
		if len(out) == 0 {
			break
		}
	}
	return out, nil
}

func intersect(p, q, a, b Point) Point {
	den := (p.X-q.X)*(a.Y-b.Y) - (p.Y-q.Y)*(a.X-b.X)
	if math.Abs(den) < 1e-12 {
		return q
	}
	t := ((p.X-a.X)*(a.Y-b.Y) - (p.Y-a.Y)*(a.X-b.X)) / den
	return Point{p.X + (q.X-p.X)*t, p.Y + (q.Y-p.Y)*t}
}

func Stroke(pts []Point, width float64) ([]Point, error) {
	if width <= 0 {
		return nil, errors.New("width 必须为正")
	}
	half := width / 2
	left := make([]Point, 0, len(pts))
	right := make([]Point, 0, len(pts))
	for i, p := range pts {
		var dx, dy float64
		if i+1 < len(pts) {
			dx, dy = pts[i+1].X-p.X, pts[i+1].Y-p.Y
		} else {
			dx, dy = p.X-pts[i-1].X, p.Y-pts[i-1].Y
		}
		n := math.Hypot(dx, dy)
		if n < 1e-12 {
			return nil, errors.New("折线出现重复点")
		}
		nx, ny := -dy/n*half, dx/n*half
		left = append(left, Point{p.X + nx, p.Y + ny})
		right = append(right, Point{p.X - nx, p.Y - ny})
	}
	for i, j := 0, len(right)-1; i < j; i, j = i+1, j-1 {
		right[i], right[j] = right[j], right[i]
	}
	return append(left, right...), nil
}
