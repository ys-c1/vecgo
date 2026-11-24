package geom

import (
	"errors"
	"math"

	"vecgo/bounds"
)

type Point struct{ X, Y float64 }

func toBounds(pts []Point) []bounds.Point {
	out := make([]bounds.Point, len(pts))
	for i, p := range pts {
		out[i] = bounds.Point{X: p.X, Y: p.Y}
	}
	return out
}

// Within 是几何阶段“越界即拒绝”的入口：所有顶点必须是有限数且落在 frame 内
// （四条边算界内）。成功时返回一份逐位不变的拷贝；越界时返回 *bounds.Error，
// 里面写明包名、入口名、点下标与坐标。不做截断、不做夹取。
//
// Clip 的职责是裁剪，部分跨出窗口属于它的正常输入；需要“一个点都不许出界”
// 的调用方用 Within。
func Within(poly []Point, frame bounds.Frame) ([]Point, error) {
	if err := bounds.Points("geom", "Within", "poly", frame, toBounds(poly)); err != nil {
		return nil, err
	}
	return append([]Point{}, poly...), nil
}

func Bezier(p0, p1, p2, p3 Point, t float64) Point {
	u := 1 - t
	a := u * u * u
	b := 3 * u * u * t
	c := 3 * u * t * t
	d := t * t * t
	return Point{a*p0.X + b*p1.X + c*p2.X + d*p3.X, a*p0.Y + b*p1.Y + c*p2.Y + d*p3.Y}
}

// Flatten 把三次贝塞尔细分为 segments+1 个点。控制点必须是有限数，
// 否则返回 *bounds.Error（旧实现让 NaN 静默传播到后续阶段）。
func Flatten(p0, p1, p2, p3 Point, segments int) ([]Point, error) {
	if segments < 1 {
		return nil, errors.New("segments 必须为正")
	}
	if err := bounds.FinitePoints("geom", "Flatten", "p",
		[]bounds.Point{{X: p0.X, Y: p0.Y}, {X: p1.X, Y: p1.Y}, {X: p2.X, Y: p2.Y}, {X: p3.X, Y: p3.Y}}); err != nil {
		return nil, err
	}
	if segments == math.MaxInt {
		// segments+1 会溢出成负数，make 会直接 panic。
		return nil, errors.New("segments 过大，segments+1 溢出")
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

// Clip 用凸窗口裁剪多边形（Sutherland–Hodgman）。
//
// 越界由 bounds 统一判定，规则：
//   - 顶点含 NaN/±Inf → 拒绝，报出 poly[i] 或 window[i] 的坐标；
//   - 窗口宽或高为 0（退化）→ 拒绝，报出 window 包围盒；
//   - 裁剪结果为 0 个顶点（多边形整体在窗口外）→ 拒绝，报出 poly[0]；
//   - 部分跨出窗口仍然裁剪——这是 Clip 的定义，不算输入越界。
//
// 旧实现对“整体在窗口外”静默返回空切片，调用方无法区分“裁没了”和
// “本来就没有”。
func Clip(poly, window []Point) ([]Point, error) {
	if len(poly) < 3 || len(window) < 3 {
		return nil, errors.New("裁剪需要至少三个顶点")
	}
	if err := bounds.FinitePoints("geom", "Clip", "poly", toBounds(poly)); err != nil {
		return nil, err
	}
	windowPts := toBounds(window)
	if err := bounds.FinitePoints("geom", "Clip", "window", windowPts); err != nil {
		return nil, err
	}
	frame := bounds.FrameOf(windowPts)
	if err := bounds.CheckFrame("geom", "Clip", frame); err != nil {
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
	if len(out) == 0 {
		// 整体落在窗口外：旧实现静默返回空切片，调用方无从区分“裁没了”。
		// 只拒绝空结果；仅贴边（退化成 1~2 个顶点）的窗口内图形仍然接受。
		v := bounds.Classify(poly[0].X, poly[0].Y, frame)
		if v.OK() {
			v = bounds.Degenerate
		}
		return nil, bounds.Fail("geom", "Clip", "poly[0]", poly[0].X, poly[0].Y, frame, v,
			"裁剪结果为 0 个顶点：多边形整体落在 window 之外（旧实现静默返回空切片）")
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

// Stroke 把折线扩展成带宽度的多边形。width 与各点坐标必须是有限数，
// 否则返回 *bounds.Error。
func Stroke(pts []Point, width float64) ([]Point, error) {
	if err := bounds.Finite("geom", "Stroke", "width", width); err != nil {
		return nil, err
	}
	if width <= 0 {
		return nil, errors.New("width 必须为正")
	}
	if err := bounds.FinitePoints("geom", "Stroke", "pts", toBounds(pts)); err != nil {
		return nil, err
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
