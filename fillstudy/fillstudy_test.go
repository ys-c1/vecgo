package fillstudy

import (
	"sort"
	"testing"

	"vecgo/geom"
)

// measureAll is the fixture every assertion below reads from.
func measureAll(t *testing.T, width, height, ss int) []RuleStats {
	t.Helper()
	var out []RuleStats
	for _, shape := range ShapeLibrary() {
		s, err := Measure(shape, width, height, ss)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, s)
	}
	return out
}

// TestEveryDisagreementSatisfiesCondition re-derives the necessary and
// sufficient condition from the measured direction histograms: each pattern
// has an even number of crossings, an even non-zero sum, and every witness's
// unpaired crossings have the winding's sign.
func TestEveryDisagreementSatisfiesCondition(t *testing.T) {
	for _, s := range measureAll(t, 64, 64, 2) {
		if s.DisagreeSamples == 0 {
			t.Fatalf("%s: 应有不一致样本", s.Shape.Name)
		}
		for _, w := range s.Witnesses {
			if len(w.Directions)%2 != 0 {
				t.Fatalf("%s [%s]: 交叉数为奇数 %d", s.Shape.Name, w.Pattern, len(w.Directions))
			}
			if !geom.DisagreesByDirections(w.Directions) {
				t.Fatalf("%s [%s]: 不满足 W 偶且非零", s.Shape.Name, w.Pattern)
			}
			if len(w.Unpaired) != abs(w.W) {
				t.Fatalf("%s [%s]: 未配对 %d != |W| %d", s.Shape.Name, w.Pattern, len(w.Unpaired), abs(w.W))
			}
			for _, c := range w.Unpaired {
				if c.Dir*w.W <= 0 {
					t.Fatalf("%s [%s]: 未配对方向 %d 与 W=%d 不同号", s.Shape.Name, w.Pattern, c.Dir, w.W)
				}
			}
		}
	}
}

// TestCoveredCounts locks the published numbers for the three shapes, so the
// report and any later change to the renderer cannot drift silently.
func TestCoveredCounts(t *testing.T) {
	want := map[string]struct{ nz, eo int }{
		"pentagram":              {1292, 864},
		"double-traced-triangle": {2048, 0},
		"star-9/4":               {1076, 824},
	}
	for _, s := range measureAll(t, 64, 64, 2) {
		w, ok := want[s.Shape.Name]
		if !ok {
			t.Fatalf("形状 %q 没有期望值", s.Shape.Name)
		}
		if s.NonZeroOn != w.nz || s.EvenOddOn != w.eo {
			t.Fatalf("%s: non-zero/even-odd = %d/%d，期望 %d/%d",
				s.Shape.Name, s.NonZeroOn, s.EvenOddOn, w.nz, w.eo)
		}
		if s.DeltaOn() != w.nz-w.eo {
			t.Fatalf("%s: 差值 %d，期望 %d", s.Shape.Name, s.DeltaOn(), w.nz-w.eo)
		}
	}
}

// TestCoverageDeltaMatchesSampleCount checks the fractional coverage delta
// equals the number of disagreeing supersamples divided by the sample count, up
// to the 8-bit quantization of each pixel. A disagreeing pixel's coverage can
// lose at most one 8-bit step (1/255) to truncation, so the gap is bounded by
// DisagreePixels/255.
func TestCoverageDeltaMatchesSampleCount(t *testing.T) {
	for _, s := range measureAll(t, 64, 64, 2) {
		want := float64(s.DisagreeSamples) / float64(s.Supersample*s.Supersample)
		got := s.CoverageDelta()
		bound := float64(s.DisagreePixels) / 255
		if diff := got - want; diff < -bound || diff > bound {
			t.Fatalf("%s: 覆盖当量差 %.6f 与 不一致样本/每像素样本 %.6f 偏差 %.6f 超过量化界 %.6f",
				s.Shape.Name, got, want, diff, bound)
		}
		if got < 0 {
			t.Fatalf("%s: 覆盖当量差不应为负: %v", s.Shape.Name, got)
		}
	}
}

// TestDeltaIsNegativeForHoleShape checks the direction histogram flips sign for
// a clockwise pentagram while the covered-pixel counts are unchanged, since
// only |W| matters.
func TestDeltaIsNegativeForHoleShape(t *testing.T) {
	sh := Pentagram()
	rev := make([]geom.Point, len(sh.Poly))
	for i, p := range sh.Poly {
		rev[len(sh.Poly)-1-i] = p
	}
	s, err := Measure(Shape{Name: "pentagram-cw", Poly: rev}, 64, 64, 2)
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range s.Witnesses {
		if w.W >= 0 {
			t.Fatalf("反向五角星绕数应为负，得到 %d [%s]", w.W, w.Pattern)
		}
	}
	if s.NonZeroAny != s.EvenOddAny+384 {
		t.Fatalf("任意覆盖差应与正向一致: %d vs %d", s.NonZeroAny, s.EvenOddAny)
	}
}

// TestDoubleTracedTriangleEmptiesUnderEvenOdd checks the extreme case: the
// whole shape is dropped by even-odd, and non-zero sees it all.
func TestDoubleTracedTriangleEmptiesUnderEvenOdd(t *testing.T) {
	s, err := Measure(DoubleTracedTriangle(), 64, 64, 2)
	if err != nil {
		t.Fatal(err)
	}
	if s.EvenOddOn != 0 || s.EvenOddAny != 0 {
		t.Fatalf("even-odd 应完全不覆盖，得到 on=%d any=%d", s.EvenOddOn, s.EvenOddAny)
	}
	if s.NonZeroOn == 0 {
		t.Fatal("non-zero 应有覆盖")
	}
}

// TestPatternHistogramSumMatchesDisagreements checks the histogram is a
// partition of the disagreement samples.
func TestPatternHistogramSumMatchesDisagreements(t *testing.T) {
	for _, s := range measureAll(t, 32, 32, 2) {
		sum := 0
		for _, n := range s.PatternHistogram {
			sum += n
		}
		if sum != s.DisagreeSamples {
			t.Fatalf("%s: 直方图和 %d != 不一致样本 %d", s.Shape.Name, sum, s.DisagreeSamples)
		}
	}
}

// TestWitnessDirectionsRecompute locates each witness on the sampling frame and
// re-runs the ray test, confirming the recorded crossings.
func TestWitnessDirectionsRecompute(t *testing.T) {
	for _, s := range measureAll(t, 32, 32, 2) {
		for _, w := range s.Witnesses {
			sum := geom.Summarize(s.Shape.Poly, w.P.X, w.P.Y)
			if sum.Winding != w.W || len(sum.Crossings) != len(w.Crossings) {
				t.Fatalf("%s: witness 重算不符", s.Shape.Name)
			}
			dirs := sum.Directions()
			if len(dirs) != len(w.Directions) {
				t.Fatalf("%s: 方向数不符", s.Shape.Name)
			}
			for i := range dirs {
				if dirs[i] != w.Directions[i] {
					t.Fatalf("%s: 方向[%d] 不符", s.Shape.Name, i)
				}
			}
		}
	}
}

// TestWitnessPatternsSorted checks determinism of witness order.
func TestWitnessPatternsSorted(t *testing.T) {
	for _, s := range measureAll(t, 32, 32, 2) {
		pats := make([]string, len(s.Witnesses))
		for i, w := range s.Witnesses {
			pats[i] = w.Pattern
		}
		if !sort.StringsAreSorted(pats) {
			t.Fatalf("%s: witness 顺序不稳定: %v", s.Shape.Name, pats)
		}
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
