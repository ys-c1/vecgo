package geom

import (
	"math"
	"testing"
)

func TestBezierEnds(t *testing.T) {
	a := Bezier(Point{X: 0, Y: 0}, Point{X: 0, Y: 1}, Point{X: 1, Y: 1}, Point{X: 1, Y: 0}, 0)
	if a.X != 0 || a.Y != 0 {
		t.Fatalf("起点错误: %v", a)
	}
}

func TestFlattenCount(t *testing.T) {
	pts, err := Flatten(Point{X: 0, Y: 0}, Point{X: 0, Y: 10}, Point{X: 10, Y: 10}, Point{X: 10, Y: 0}, 8)
	if err != nil || len(pts) != 9 {
		t.Fatalf("细分点数错误: %v %d", err, len(pts))
	}
	if Length(pts) <= 10 {
		t.Fatal("长度应大于弦长")
	}
}

func TestInsideSquare(t *testing.T) {
	sq := []Point{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}
	if !Inside(sq, 5, 5) || Inside(sq, 15, 5) {
		t.Fatal("内外判定错误")
	}
}

func TestClipShrinks(t *testing.T) {
	sq := []Point{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}
	win := []Point{{X: 2, Y: 2}, {X: 8, Y: 2}, {X: 8, Y: 8}, {X: 2, Y: 8}}
	out, err := Clip(sq, win)
	if err != nil || len(out) < 3 {
		t.Fatalf("裁剪结果异常: %v %d", err, len(out))
	}
}

func TestStrokeRejectsZero(t *testing.T) {
	if _, err := Stroke([]Point{{X: 0, Y: 0}, {X: 1, Y: 0}}, 0); err == nil {
		t.Fatal("width=0 应报错")
	}
	if _, err := Stroke([]Point{{X: 0, Y: 0}, {X: 0, Y: 0}}, 1); err == nil {
		t.Fatal("重复点应报错")
	}
}

// square returns a CCW square, the canonical simple polygon.
func square() []Point {
	return []Point{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}
}

// TestSimplePolygonRulesAgree is the theorem on the simplest witness: on a
// simple polygon the two rules classify every sample identically.
func TestSimplePolygonRulesAgree(t *testing.T) {
	poly := square()
	for y := -2.0; y <= 12; y += 0.5 {
		for x := -2.0; x <= 12; x += 0.5 {
			if inside := InsideRule(poly, x, y, NonZero); inside != InsideRule(poly, x, y, EvenOdd) {
				t.Fatalf("简单多边形上两条规则不一致 at (%v,%v)", x, y)
			}
		}
	}
}

// TestSimplePolygonWindingIsUnitOrZero pins the proof's key fact: on a simple
// polygon the winding number is only ever 0 or ±1.
func TestSimplePolygonWindingIsUnitOrZero(t *testing.T) {
	poly := square()
	for y := -2.0; y <= 12; y += 0.25 {
		for x := -2.0; x <= 12; x += 0.25 {
			if w := WindingOf(poly, x, y); w < -1 || w > 1 {
				t.Fatalf("简单多边形绕数应为 0 或 ±1，得到 %d at (%v,%v)", w, x, y)
			}
		}
	}
}

// TestDisagreementCondition holds the sufficient direction across shapes: a
// sample disagrees exactly when |W| is a non-zero even number.
func TestDisagreementCondition(t *testing.T) {
	shapes := [][]Point{
		square(),
		regularStarPoints(5, 2, 100),
		regularStarPoints(7, 3, 100),
		regularStarPoints(9, 4, 100),
		{{X: 0, Y: -90}, {X: 78, Y: 45}, {X: -78, Y: 45}, {X: 0, Y: -90}, {X: 78, Y: 45}, {X: -78, Y: 45}},
	}
	for si, poly := range shapes {
		xs, ys := boundsOf(poly)
		for y := ys[0]; y <= ys[1]; y += 1.5 {
			for x := xs[0]; x <= xs[1]; x += 1.5 {
				p := Point{x, y}
				if OnBoundary(poly, p, 1e-9) {
					continue
				}
				s := Summarize(poly, x, y)
				if s.Agree() {
					if s.Winding%2 == 0 && s.Winding != 0 {
						t.Fatalf("形状%d (%v,%v): W=%d 偶非零却一致", si, x, y, s.Winding)
					}
				} else {
					if s.Winding == 0 || s.Winding%2 != 0 {
						t.Fatalf("形状%d (%v,%v): 不一致但 W=%d，条件 W 偶且非零被违反", si, x, y, s.Winding)
					}
				}
				if s.EvenOdd() && !s.NonZero() {
					t.Fatalf("形状%d (%v,%v): 奇偶为真而绕数为零，违反 even-odd => non-zero", si, x, y)
				}
			}
		}
	}
}

// TestDisagreesByDirections checks the pure direction-inequality form.
func TestDisagreesByDirections(t *testing.T) {
	cases := []struct {
		dirs []int
		want bool
	}{
		{[]int{1}, false},          // odd count, W=1
		{[]int{1, 1}, true},        // W=2
		{[]int{-1, -1}, true},      // W=-2
		{[]int{1, -1}, false},      // W=0
		{[]int{1, 1, 1, 1}, true},  // W=4
		{[]int{1, 1, 1, -1}, true}, // W=2
		{[]int{1, 1, -1}, false},   // W=1, odd
		{[]int{1, 1, 1}, false},    // W=3
	}
	for _, c := range cases {
		if got := DisagreesByDirections(c.dirs); got != c.want {
			t.Fatalf("dirs=%v 期望 %v 得到 %v", c.dirs, c.want, got)
		}
	}
}

// TestUnpairedLeavesWindingMany verifies the leftover count equals |W| and that
// those leftovers are the same-direction crossings non-zero keeps.
func TestUnpairedLeavesWindingMany(t *testing.T) {
	poly := regularStarPoints(9, 4, 100)
	xs, ys := boundsOf(poly)
	checked := 0
	for y := ys[0]; y <= ys[1]; y += 2 {
		for x := xs[0]; x <= xs[1]; x += 2 {
			s := Summarize(poly, x, y)
			if s.Agree() {
				continue
			}
			left := Unpaired(s.Crossings)
			if len(left) != absInt(s.Winding) {
				t.Fatalf("未配对交点=%d 应等于 |W|=%d", len(left), absInt(s.Winding))
			}
			for _, c := range left {
				if c.Dir*s.Winding <= 0 {
					t.Fatalf("未配对交点方向 %d 与 W=%d 不同号", c.Dir, s.Winding)
				}
			}
			checked++
		}
	}
	if checked == 0 {
		t.Fatal("没有找到任何不一致样本")
	}
}

// TestSelfIntersectionsPentagram finds the five crossing points of a 5/2 star.
func TestSelfIntersectionsPentagram(t *testing.T) {
	poly := regularStarPoints(5, 2, 100)
	got := SelfIntersections(poly)
	if len(got) != 5 {
		t.Fatalf("五角星应有 5 个交点，得到 %d", len(got))
	}
}

func regularStarPoints(n, step int, r float64) []Point {
	verts := make([]Point, n)
	for k := 0; k < n; k++ {
		a := (-90 + 360*float64(k)/float64(n)) * math.Pi / 180
		verts[k] = Point{r * math.Cos(a), r * math.Sin(a)}
	}
	out := make([]Point, n)
	for k := 0; k < n; k++ {
		out[k] = verts[(k*step)%n]
	}
	return out
}

func boundsOf(poly []Point) ([2]float64, [2]float64) {
	xs := [2]float64{poly[0].X, poly[0].X}
	ys := [2]float64{poly[0].Y, poly[0].Y}
	for _, p := range poly {
		xs[0] = math.Min(xs[0], p.X)
		xs[1] = math.Max(xs[1], p.X)
		ys[0] = math.Min(ys[0], p.Y)
		ys[1] = math.Max(ys[1], p.Y)
	}
	return xs, ys
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// combPolygon is a simple but strongly concave polygon: a baseline with four
// teeth. A +x ray can cross it up to seven times, always with winding -1.
func combPolygon() []Point {
	w, h, gap := 20.0, 60.0, 10.0
	pts := []Point{{X: 0, Y: 0}, {X: 0, Y: 10}}
	for t := 0; t < 4; t++ {
		x := float64(t) * (w + gap)
		pts = append(pts, Point{x, 10}, Point{x, h}, Point{x + w, h}, Point{x + w, 10})
	}
	pts = append(pts, Point{X: 4*(w+gap) - gap, Y: 10}, Point{X: 4*(w+gap) - gap, Y: 0})
	return pts
}

// TestConcaveSimplePolygonHasHighOddCounts is the non-trivial half of the
// theorem: on a simple polygon the ray may cross many times, so "one crossing
// means one enclosure" is false, yet the two rules still agree because the
// winding stays ±1.
func TestConcaveSimplePolygonHasHighOddCounts(t *testing.T) {
	poly := combPolygon()
	maxCross, sawUnitWinding := 0, false
	for y := 1.0; y < 59; y += 1.5 {
		for x := 1.0; x < 109; x += 1.5 {
			p := Point{x, y}
			if OnBoundary(poly, p, 1e-9) {
				continue
			}
			s := Summarize(poly, x, y)
			if s.Count > maxCross {
				maxCross = s.Count
			}
			if s.Count >= 3 && s.Winding == -1 {
				sawUnitWinding = true
			}
			if !s.Agree() {
				t.Fatalf("凹简单多边形上规则不一致 at (%v,%v): C=%d W=%d", x, y, s.Count, s.Winding)
			}
		}
	}
	if maxCross < 5 {
		t.Fatalf("梳形多边形应产生至少 5 次交叉，得到 %d", maxCross)
	}
	if !sawUnitWinding {
		t.Fatal("应观察到交叉数 >=3 且绕数为 -1 的样本")
	}
}

// TestEvenOddAlwaysImpliesNonZero is the strict inclusion: whenever even-odd
// accepts a sample, non-zero accepts it too. Hence non-zero's coverage is a
// superset of even-odd's and the pixel difference can never be negative.
func TestEvenOddAlwaysImpliesNonZero(t *testing.T) {
	// Deterministic pseudo-random self-intersecting polygons.
	state := uint64(0x2545F4914F6CDD1D)
	next := func() float64 {
		state ^= state << 13
		state ^= state >> 7
		state ^= state << 17
		return float64(state%1_000_000)/1_000_000*200 - 100
	}
	for trial := 0; trial < 200; trial++ {
		n := 3 + trial%7
		poly := make([]Point, n)
		for i := range poly {
			poly[i] = Point{next(), next()}
		}
		for k := 0; k < 500; k++ {
			x, y := next(), next()
			s := Summarize(poly, x, y)
			if s.EvenOdd() && !s.NonZero() {
				t.Fatalf("trial %d: even-odd 接受但 non-zero 拒绝 at (%v,%v)", trial, x, y)
			}
			// And the equivalence's exceptional set is impossible:
			if s.EvenOdd() && s.Winding == 0 {
				t.Fatalf("trial %d: 奇偶为真却绕数为 0", trial)
			}
		}
	}
}

// TestEveryUnpairedCrossingIsEssential is the precise "which crossing caused
// it" statement: on a disagreement, dropping any single unpaired crossing turns
// the winding odd, at which point both rules accept the point. So each
// unpaired crossing is individually essential to the disagreement.
func TestEveryUnpairedCrossingIsEssential(t *testing.T) {
	for _, poly := range [][]Point{
		regularStarPoints(5, 2, 100),
		regularStarPoints(9, 4, 100),
	} {
		xs, ys := boundsOf(poly)
		for y := ys[0]; y <= ys[1]; y += 2 {
			for x := xs[0]; x <= xs[1]; x += 2 {
				s := Summarize(poly, x, y)
				if s.Agree() {
					continue
				}
				left := Unpaired(s.Crossings)
				for _, drop := range left {
					w := s.Winding - drop.Dir
					count := s.Count - 1
					nz := w != 0
					eo := count%2 == 1
					if nz != eo {
						t.Fatalf("(%v,%v): 去掉一个未配对交点后仍分歧 W=%d C=%d", x, y, w, count)
					}
					if !nz || !eo {
						t.Fatalf("(%v,%v): 去掉一个未配对交点后应两者都判内", x, y)
					}
				}
			}
		}
	}
}
