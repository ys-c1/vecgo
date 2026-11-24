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

// EdgeRule 是贴边像素的归属规则。
//
// 半像素规则（本渲染器的明文约定，见 docs/half-pixel-rule.md）：
//   - 采样点取像素中心，超采样时取每个子像素的中心；
//   - 点恰好落在多边形边上时，按“左上角规则”决定归属——
//     顶边（水平边且多边形内部在其下方）与左边（非水平边且内部在边的左侧）算内，
//     底边与右边算外。共享边因此只归属一侧，既不漏也不重。
//
// RuleLegacy 保留旧行为：Inside 的射线法没有明文边界约定，
// 点是否算内取决于浮点除法是否恰好落在边上，等价于“顶边/右边算内”的偶然结果，
// 跨平台可能因浮点实现差一行。仅用于复现旧产物，新场景不要使用。
type EdgeRule int

const (
	// RuleHalfPixel 半像素规则：中心采样 + 左上角边界归属。默认规则。
	RuleHalfPixel EdgeRule = iota
	// RuleLegacy 旧规则：保持 Inside 射线法的历史行为，用于逐字节复现旧产物。
	RuleLegacy
)

// InsideWith 按指定边界规则判定采样点 (px,py) 是否属于多边形。
func InsideWith(poly []Point, px, py float64, rule EdgeRule) bool {
	if rule == RuleLegacy {
		return Inside(poly, px, py)
	}
	return insideTopLeft(poly, px, py)
}

// insideTopLeft 实现左上角规则。
func insideTopLeft(poly []Point, px, py float64) bool {
	n := len(poly)
	ccw := signedArea(poly) > 0

	// 第一步：点恰好落在某条边（含顶点）上时，按顶边/左边算内、底边/右边算外。
	// 顶点可能同时贴两条边，采用 OR 语义：只要任一条是顶边或左边就算内，
	// 因此结果不依赖边的遍历顺序，也不依赖多边形的绕向（CCW/CW 等价）。
	onBoundary := false
	for i := 0; i < n; i++ {
		a, b := poly[i], poly[(i+1)%n]
		cross := (b.X-a.X)*(py-a.Y) - (b.Y-a.Y)*(px-a.X)
		if cross != 0 {
			continue
		}
		if !onSegment(a, b, px, py) {
			continue
		}
		onBoundary = true
		ex, ey := b.X-a.X, b.Y-a.Y
		if ey == 0 {
			// 水平边：内部在 py 更大一侧（屏幕下方）的是顶边。
			// 本库 py 随扫描行增大：CCW（面积正）多边形的顶边沿 +X，CW 沿 -X。
			if ccw && ex > 0 || !ccw && ex < 0 {
				return true
			}
			continue
		}
		// 非水平边：内部在 px 更大一侧（屏幕右方）的是左边。
		// CCW 多边形的左边沿 -Y，CW 沿 +Y。
		if ccw && ey < 0 || !ccw && ey > 0 {
			return true
		}
	}
	if onBoundary {
		return false
	}

	// 第二步：不在任何边上，射线法判严格内部（与历史实现同构，但贴边点已在上面处理）。
	inside := false
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

func signedArea(poly []Point) float64 {
	a := 0.0
	for i := range poly {
		p, q := poly[i], poly[(i+1)%len(poly)]
		a += p.X*q.Y - q.X*p.Y
	}
	return a
}

func onSegment(a, b Point, px, py float64) bool {
	return px >= math.Min(a.X, b.X) && px <= math.Max(a.X, b.X) &&
		py >= math.Min(a.Y, b.Y) && py <= math.Max(a.Y, b.Y)
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
