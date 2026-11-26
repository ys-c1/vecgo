package geom

import (
	"errors"
	"fmt"
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

// SignedArea 返回环的有向（带符号）面积：S = 1/2 * Σ(x_i*y_{i+1} - x_{i+1}*y_i)。
// 屏幕坐标（y 向下）下 S>0 表示逆时针(CCW)，S<0 表示顺时针(CW)。
// 方向规则的判定只看符号：外环与洞环的有向面积符号必须相反。
func SignedArea(poly []Point) float64 {
	var s float64
	n := len(poly)
	for i := 0; i < n; i++ {
		a := poly[i]
		b := poly[(i+1)%n]
		s += a.X*b.Y - b.X*a.Y
	}
	return s * 0.5
}

// Winding 返回点 (px,py) 关于单个环的绕数（整数）。
// 向上穿越射线且交点在点右侧记 +1，向下穿越记 -1。
// 绕数的符号由环的绕向决定，大小由嵌套层数决定。
func Winding(poly []Point, px, py float64) int {
	w := 0
	n := len(poly)
	for i := 0; i < n; i++ {
		ax, ay := poly[i].X, poly[i].Y
		bx, by := poly[(i+1)%n].X, poly[(i+1)%n].Y
		if ay <= py {
			if by > py && (bx-ax)*(py-ay)-(by-ay)*(px-ax) > 0 {
				w++ // 向上穿越，点在边左侧
			}
		} else {
			if by <= py && (bx-ax)*(py-ay)-(by-ay)*(px-ax) < 0 {
				w-- // 向下穿越，点在边右侧
			}
		}
	}
	return w
}

// WindingSum 返回点关于一组环（外环 + 洞环）的绕数之和 W = Σ W_k。
// 非零绕数填充规则：W ≠ 0 视为在实体内部，W = 0 视为在外部或洞中。
func WindingSum(rings [][]Point, px, py float64) int {
	w := 0
	for _, r := range rings {
		w += Winding(r, px, py)
	}
	return w
}

// CheckOrientation 按有向面积符号执行方向规则：
// 外环符号与每个洞环符号必须相反（sign(S_outer) * sign(S_hole) < 0），
// 否则洞的绕数无法与外环抵消，返回错误。
func CheckOrientation(rings [][]Point) error {
	if len(rings) == 0 {
		return errors.New("至少需要一个环")
	}
	outer := math.Signbit(SignedArea(rings[0])) // true 表示负（CW）
	for i := 1; i < len(rings); i++ {
		if math.Signbit(SignedArea(rings[i])) == outer {
			return fmt.Errorf("第 %d 个环与外环绕向相同，有向面积符号未相反，洞无法抵消", i)
		}
	}
	return nil
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
