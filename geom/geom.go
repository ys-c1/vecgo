package geom

import (
	"errors"
	"fmt"
	"math"
)

type Point struct{ X, Y float64 }

// FillRule selects how the interior of a (possibly self-intersecting) closed
// path is defined.
type FillRule int

const (
	// NonZero fills a point when the winding number of the path around it is
	// non-zero. Crossings of a +x ray are counted +1 when the boundary runs
	// upward (increasing y) and -1 when it runs downward; the winding number
	// is their sum.
	NonZero FillRule = iota
	// EvenOdd fills a point when a +x ray from it crosses the boundary an odd
	// number of times, regardless of crossing direction.
	EvenOdd
)

// Rules lists the fill rules in their canonical order (the default first).
var Rules = []FillRule{NonZero, EvenOdd}

func (r FillRule) String() string {
	switch r {
	case EvenOdd:
		return "even-odd"
	case NonZero:
		return "non-zero"
	default:
		return fmt.Sprintf("FillRule(%d)", int(r))
	}
}

// ParseFillRule maps the configuration spelling of a rule to its value.
func ParseFillRule(s string) (FillRule, error) {
	switch s {
	case "non-zero":
		return NonZero, nil
	case "even-odd":
		return EvenOdd, nil
	default:
		return NonZero, fmt.Errorf("未知填充规则: %q", s)
	}
}

func sign(v float64) int {
	if v > 0 {
		return 1
	}
	if v < 0 {
		return -1
	}
	return 0
}

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

// Crossing is one point where the +x ray from a sample meets the boundary.
// Index is the segment that produced it: poly[Index] -> poly[(Index+1)%n].
// Dir is +1 when the boundary runs upward (by > ay) at the crossing and -1
// when it runs downward.
type Crossing struct {
	Point Point
	Index int
	Dir   int
}

// CrossingSummary is the full result of the ray test that both fill rules are
// defined from. Every rule in this package is a predicate on this summary, so
// the two rules can never disagree because of different ray handling.
type CrossingSummary struct {
	Crossings []Crossing
	// Count is the number of crossings, C.
	Count int
	// Winding is the signed crossing sum, W = sum of crossing directions.
	Winding int
}

// EvenOdd is the even-odd predicate: C is odd.
func (s CrossingSummary) EvenOdd() bool { return s.Count%2 == 1 }

// NonZero is the non-zero predicate: W != 0.
func (s CrossingSummary) NonZero() bool { return s.Winding != 0 }

// Agree reports whether both rules classify the sample the same way.
func (s CrossingSummary) Agree() bool { return s.EvenOdd() == s.NonZero() }

// Directions returns the crossing directions in segment order (the order
// scanRay visits them, i.e. by polygon segment index).
func (s CrossingSummary) Directions() []int {
	out := make([]int, len(s.Crossings))
	for i, c := range s.Crossings {
		out[i] = c.Dir
	}
	return out
}

// scanRay visits every crossing of the +x ray from (px,py) with the boundary.
// The half-open endpoint test (ay > py) != (by > py) removes ray-through-vertex
// degeneracy and is shared by every rule.
func scanRay(poly []Point, px, py float64, visit func(Crossing)) {
	n := len(poly)
	for i := 0; i < n; i++ {
		ax, ay := poly[i].X, poly[i].Y
		bx, by := poly[(i+1)%n].X, poly[(i+1)%n].Y
		if (ay > py) == (by > py) {
			continue
		}
		t := (py - ay) / (by - ay)
		cx := ax + (bx-ax)*t
		if cx > px {
			visit(Crossing{Point: Point{cx, py}, Index: i, Dir: sign(by - ay)})
		}
	}
}

// Summarize runs the shared ray test and returns the crossing list.
func Summarize(poly []Point, px, py float64) CrossingSummary {
	s := CrossingSummary{}
	scanRay(poly, px, py, func(c Crossing) {
		s.Crossings = append(s.Crossings, c)
		s.Count++
		s.Winding += c.Dir
	})
	return s
}

// InsideRule is the authoritative point-in-path predicate. Boundary points are
// unspecified: give it a sample not exactly on an edge.
func InsideRule(poly []Point, px, py float64, rule FillRule) bool {
	count, winding := 0, 0
	scanRay(poly, px, py, func(c Crossing) {
		count++
		winding += c.Dir
	})
	if rule == EvenOdd {
		return count%2 == 1
	}
	return winding != 0
}

// Inside reports whether (px,py) is inside poly under the default rule,
// NonZero.
func Inside(poly []Point, px, py float64) bool {
	return InsideRule(poly, px, py, NonZero)
}

// WindingOf returns the winding number of poly around (px,py).
func WindingOf(poly []Point, px, py float64) int {
	w := 0
	scanRay(poly, px, py, func(c Crossing) { w += c.Dir })
	return w
}

// DisagreesByDirections is the disagreement inequality on crossing
// directions: the rules differ exactly when the signed sum is a non-zero even
// integer.
//
//	W = sum_k d_k  in  2Z \ {0}
//
// With u upward and d downward crossings this is u-d even and non-zero, i.e.
// u ≡ d (mod 2) and u != d, i.e. |u-d| >= 2 with u+d even.
func DisagreesByDirections(dirs []int) bool {
	w := 0
	for _, d := range dirs {
		w += d
	}
	return w != 0 && w%2 == 0
}

// Unpaired cancels crossings of opposite direction in list order and returns the
// crossings left over. The leftover count is |Winding| and they all share the
// majority direction. The leftovers are exactly the crossings that make the
// winding number non-zero: even-odd is blind to direction and cancels every
// two crossings, so a disagreement needs at least two leftover same-direction
// crossings. Any pairing of opposite crossings leaves the same multiset of
// leftovers up to relabelling; earliest-first pairing keeps the report's
// coordinates deterministic.
func Unpaired(cs []Crossing) []Crossing {
	var plus, minus []int
	matched := make([]bool, len(cs))
	for i, c := range cs {
		if c.Dir > 0 {
			if len(minus) > 0 {
				matched[minus[0]] = true
				matched[i] = true
				minus = minus[1:]
			} else {
				plus = append(plus, i)
			}
		} else if c.Dir < 0 {
			if len(plus) > 0 {
				matched[plus[0]] = true
				matched[i] = true
				plus = plus[1:]
			} else {
				minus = append(minus, i)
			}
		}
	}
	var out []Crossing
	for i := range cs {
		if !matched[i] && cs[i].Dir != 0 {
			out = append(out, cs[i])
		}
	}
	return out
}

// OnBoundary reports whether p lies on an edge of poly, within eps.
func OnBoundary(poly []Point, p Point, eps float64) bool {
	n := len(poly)
	for i := 0; i < n; i++ {
		a, b := poly[i], poly[(i+1)%n]
		d := math.Hypot(b.X-a.X, b.Y-a.Y)
		if d == 0 {
			continue
		}
		dist := math.Abs(cross(a, b, p)) / d
		if dist > eps {
			continue
		}
		if math.Min(a.X, b.X)-eps <= p.X && p.X <= math.Max(a.X, b.X)+eps &&
			math.Min(a.Y, b.Y)-eps <= p.Y && p.Y <= math.Max(a.Y, b.Y)+eps {
			return true
		}
	}
	return false
}

// SegmentsIntersect reports whether closed segments p0p1 and q0q1 meet.
func SegmentsIntersect(p0, p1, q0, q1 Point) bool {
	d1 := cross(q0, q1, p0)
	d2 := cross(q0, q1, p1)
	d3 := cross(p0, p1, q0)
	d4 := cross(p0, p1, q1)
	if ((d1 > 0 && d2 < 0) || (d1 < 0 && d2 > 0)) &&
		((d3 > 0 && d4 < 0) || (d3 < 0 && d4 > 0)) {
		return true
	}
	if d1 == 0 && onSegment(q0, q1, p0) {
		return true
	}
	if d2 == 0 && onSegment(q0, q1, p1) {
		return true
	}
	if d3 == 0 && onSegment(p0, p1, q0) {
		return true
	}
	if d4 == 0 && onSegment(p0, p1, q1) {
		return true
	}
	return false
}

// SelfIntersections returns the proper intersection points of non-adjacent
// segments of poly. Shared endpoints of adjacent segments are not returned.
func SelfIntersections(poly []Point) []Point {
	var out []Point
	n := len(poly)
	for i := 0; i < n; i++ {
		a0, a1 := poly[i], poly[(i+1)%n]
		for j := i + 2; j < n; j++ {
			if i == 0 && j == n-1 {
				continue // adjacent through the closing vertex
			}
			b0, b1 := poly[j], poly[(j+1)%n]
			if p, ok := properIntersect(a0, a1, b0, b1); ok {
				out = append(out, p)
			}
		}
	}
	return out
}

func cross(a, b, c Point) float64 {
	return (b.X-a.X)*(c.Y-a.Y) - (b.Y-a.Y)*(c.X-a.X)
}

func onSegment(a, b, p Point) bool {
	return math.Min(a.X, b.X) <= p.X && p.X <= math.Max(a.X, b.X) &&
		math.Min(a.Y, b.Y) <= p.Y && p.Y <= math.Max(a.Y, b.Y)
}

func properIntersect(p0, p1, q0, q1 Point) (Point, bool) {
	d1 := cross(q0, q1, p0)
	d2 := cross(q0, q1, p1)
	d3 := cross(p0, p1, q0)
	d4 := cross(p0, p1, q1)
	if (d1 > 0 && d2 < 0 || d1 < 0 && d2 > 0) && (d3 > 0 && d4 < 0 || d3 < 0 && d4 > 0) {
		rx, ry := p1.X-p0.X, p1.Y-p0.Y
		sx, sy := q1.X-q0.X, q1.Y-q0.Y
		den := rx*sy - ry*sx
		if math.Abs(den) < 1e-15 {
			return Point{}, false
		}
		t := ((q0.X-p0.X)*sy - (q0.Y-p0.Y)*sx) / den
		return Point{p0.X + rx*t, p0.Y + ry*t}, true
	}
	return Point{}, false
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
