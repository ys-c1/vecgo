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

// SignedArea 按格林公式（鞋带公式）返回有向面积，顶点逆时针时为正。
// 只对简单多边形（不自相交）有意义；自相交时结果是各环绕区域的代数面积之和。
func SignedArea(poly []Point) float64 {
	n := len(poly)
	if n < 3 {
		return 0
	}
	sum := 0.0
	for i := 0; i < n; i++ {
		a := poly[i]
		b := poly[(i+1)%n]
		sum += a.X*b.Y - b.X*a.Y
	}
	return sum / 2
}

// Area 返回简单多边形的面积（格林公式取绝对值）。
func Area(poly []Point) float64 {
	return math.Abs(SignedArea(poly))
}

// Perimeter 返回闭合多边形的周长，包含末点到首点的一条边。
func Perimeter(poly []Point) float64 {
	n := len(poly)
	if n < 2 {
		return 0
	}
	total := 0.0
	for i := 0; i < n; i++ {
		a := poly[i]
		b := poly[(i+1)%n]
		total += math.Hypot(b.X-a.X, b.Y-a.Y)
	}
	return total
}

// IsSimple 判断闭合多边形是否简单：无零长边、且任意两条不相邻的边都不相交。
// 覆盖率加权填充依赖简单性，否则格林公式面积与像素覆盖面积不再一致。
func IsSimple(poly []Point) bool {
	n := len(poly)
	if n < 3 {
		return false
	}
	for i := 0; i < n; i++ {
		if poly[i] == poly[(i+1)%n] {
			return false
		}
	}
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			if j == i+1 || (i == 0 && j == n-1) {
				continue
			}
			if segmentsCross(poly[i], poly[(i+1)%n], poly[j], poly[(j+1)%n]) {
				return false
			}
		}
	}
	return true
}

func orient(a, b, c Point) float64 {
	return (b.X-a.X)*(c.Y-a.Y) - (b.Y-a.Y)*(c.X-a.X)
}

func onSegment(a, b, p Point) bool {
	return math.Min(a.X, b.X) <= p.X && p.X <= math.Max(a.X, b.X) &&
		math.Min(a.Y, b.Y) <= p.Y && p.Y <= math.Max(a.Y, b.Y)
}

// SegmentsIntersect 判断两条闭线段是否相交（含端点接触与共线重叠）。
func SegmentsIntersect(a, b, c, d Point) bool {
	return segmentsCross(a, b, c, d)
}

// segmentsCross 判断两条闭线段是否相交（含端点接触与共线重叠）。
func segmentsCross(a, b, c, d Point) bool {
	o1, o2 := orient(a, b, c), orient(a, b, d)
	o3, o4 := orient(c, d, a), orient(c, d, b)
	if ((o1 > 0 && o2 < 0) || (o1 < 0 && o2 > 0)) &&
		((o3 > 0 && o4 < 0) || (o3 < 0 && o4 > 0)) {
		return true
	}
	if o1 == 0 && onSegment(a, b, c) {
		return true
	}
	if o2 == 0 && onSegment(a, b, d) {
		return true
	}
	if o3 == 0 && onSegment(c, d, a) {
		return true
	}
	if o4 == 0 && onSegment(c, d, b) {
		return true
	}
	return false
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

func Clip(poly, window []Point) ([]Point, error) {
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
