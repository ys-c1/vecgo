package geom

import (
	"math"

	"vecgo/param"
)

type Point struct{ X, Y float64 }

// Polygon 是已完成参数校验的多边形，可在热循环中反复做内外判定而无需重复校验。
// 只能通过 NewPolygon 构造。
type Polygon struct {
	pts []Point
}

// NewPolygon 校验并复制一份顶点。顶点至少三个、坐标均为有限数，否则返回
// *param.Error，字段形如 geom.Polygon[i]。
func NewPolygon(pts []Point) (Polygon, error) {
	return newPolygon("geom.Polygon", pts)
}

// NewPolygonFor 与 NewPolygon 相同，但用 entry 作为错误字段前缀，便于上层包
// 报告自己的入口名（如 "raster.Fill.polys[0]"）。
func NewPolygonFor(entry string, pts []Point) (Polygon, error) {
	return newPolygon(entry, pts)
}

func newPolygon(field string, pts []Point) (Polygon, error) {
	if err := param.Polygon(field, len(pts), func(i int) (float64, float64) {
		return pts[i].X, pts[i].Y
	}); err != nil {
		return Polygon{}, err
	}
	return Polygon{pts: append([]Point{}, pts...)}, nil
}

// Contains 判断点是否落在多边形内（射线法）。调用方须已用 NewPolygon 校验过
// 顶点；px、py 须由调用方保证有限。
func (p Polygon) Contains(px, py float64) bool {
	poly := p.pts
	if len(poly) == 0 {
		return false
	}
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

// Bezier 求三次贝塞尔在参数 t 处的点。控制点坐标与 t 都必须是有限数，
// 否则返回 *param.Error（字段如 geom.Bezier.t）。
func Bezier(p0, p1, p2, p3 Point, t float64) (Point, error) {
	if err := param.All(
		param.Coord("geom.Bezier.p0", p0.X, p0.Y),
		param.Coord("geom.Bezier.p1", p1.X, p1.Y),
		param.Coord("geom.Bezier.p2", p2.X, p2.Y),
		param.Coord("geom.Bezier.p3", p3.X, p3.Y),
		param.Finite("geom.Bezier.t", t),
	); err != nil {
		return Point{}, err
	}
	return bezier(p0, p1, p2, p3, t), nil
}

func bezier(p0, p1, p2, p3 Point, t float64) Point {
	u := 1 - t
	a := u * u * u
	b := 3 * u * u * t
	c := 3 * u * t * t
	d := t * t * t
	return Point{a*p0.X + b*p1.X + c*p2.X + d*p3.X, a*p0.Y + b*p1.Y + c*p2.Y + d*p3.Y}
}

// Flatten 把三次贝塞尔细分为 segments+1 个点。
// segments 必须不小于 1（不设业务上限），四个控制点坐标必须是有限数；否则返回
// *param.Error（字段如 geom.Flatten.segments、geom.Flatten.p0）。
func Flatten(p0, p1, p2, p3 Point, segments int) ([]Point, error) {
	if err := param.All(
		param.AtLeast("geom.Flatten.segments", segments, param.SegmentsMin),
		param.Coord("geom.Flatten.p0", p0.X, p0.Y),
		param.Coord("geom.Flatten.p1", p1.X, p1.Y),
		param.Coord("geom.Flatten.p2", p2.X, p2.Y),
		param.Coord("geom.Flatten.p3", p3.X, p3.Y),
	); err != nil {
		return nil, err
	}
	if segments == math.MaxInt {
		// segments+1 会溢出成负数，make 会直接 panic。
		return nil, param.Invalid("geom.Flatten.segments", segments, "分段数过大，segments+1 会溢出")
	}
	out := make([]Point, segments+1)
	for i := 0; i <= segments; i++ {
		out[i] = bezier(p0, p1, p2, p3, float64(i)/float64(segments))
	}
	return out, nil
}

// Length 求折线总长。pts 可任意长（含空），但每个坐标必须是有限数，否则返回
// *param.Error；空折线长度为 0。
func Length(pts []Point) (float64, error) {
	if err := param.Points("geom.Length.pts", len(pts), 0, func(i int) (float64, float64) {
		return pts[i].X, pts[i].Y
	}); err != nil {
		return 0, err
	}
	total := 0.0
	for i := 0; i+1 < len(pts); i++ {
		total += math.Hypot(pts[i+1].X-pts[i].X, pts[i+1].Y-pts[i].Y)
	}
	return total, nil
}

// Inside 判断点是否在多边形内。poly 至少三个顶点、坐标有限，px/py 有限，
// 否则返回 *param.Error。热循环里请改用 NewPolygon + Contains。
func Inside(poly []Point, px, py float64) (bool, error) {
	p, err := newPolygon("geom.Inside.poly", poly)
	if err != nil {
		return false, err
	}
	if err := param.Coord("geom.Inside.point", px, py); err != nil {
		return false, err
	}
	return p.Contains(px, py), nil
}

// Clip 用凸窗口裁剪多边形（Sutherland–Hodgman）。poly 与 window 都至少三个
// 顶点且坐标有限，否则返回 *param.Error。
func Clip(poly, window []Point) ([]Point, error) {
	if err := param.All(
		param.Polygon("geom.Clip.poly", len(poly), func(i int) (float64, float64) {
			return poly[i].X, poly[i].Y
		}),
		param.Polygon("geom.Clip.window", len(window), func(i int) (float64, float64) {
			return window[i].X, window[i].Y
		}),
	); err != nil {
		return nil, err
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

// Stroke 把折线扩展成带宽度的多边形。width 必须是有限正数；pts 至少两个点、
// 坐标有限且相邻点不重合，否则返回 *param.Error。
func Stroke(pts []Point, width float64) ([]Point, error) {
	if err := param.All(
		param.FloatPositive("geom.Stroke.width", width),
		param.Points("geom.Stroke.pts", len(pts), param.StrokeMinPoints, func(i int) (float64, float64) {
			return pts[i].X, pts[i].Y
		}),
	); err != nil {
		return nil, err
	}
	for i := 1; i < len(pts); i++ {
		if math.Hypot(pts[i].X-pts[i-1].X, pts[i].Y-pts[i-1].Y) < 1e-12 {
			return nil, param.DuplicatePoint("geom.Stroke.pts", i)
		}
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
		nx, ny := -dy/n*half, dx/n*half
		left = append(left, Point{p.X + nx, p.Y + ny})
		right = append(right, Point{p.X - nx, p.Y - ny})
	}
	for i, j := 0, len(right)-1; i < j; i, j = i+1, j-1 {
		right[i], right[j] = right[j], right[i]
	}
	return append(left, right...), nil
}
