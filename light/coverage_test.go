package light

import "testing"

func TestUnitEdgeCoverageThreePointsAndIntegers(t *testing.T) {
	pts := SampleUnitEdge(0.25, 0.5, 0.75)
	want := []float64{0.25, 0.5, 0.75}
	if len(pts) != 3 {
		t.Fatalf("应取三个点, got %d", len(pts))
	}
	for i, p := range pts {
		if p.Coverage != want[i] {
			t.Fatalf("C(%.2f)=%v, 期望 %v", p.X, p.Coverage, want[i])
		}
	}
	// 整数位置只能取 0 或 1；区间外截断。
	if UnitEdgeCoverage(0) != 0 || UnitEdgeCoverage(1) != 1 {
		t.Fatal("整数位置必须为 0/1")
	}
	if UnitEdgeCoverage(-2) != 0 || UnitEdgeCoverage(3) != 1 {
		t.Fatal("区间外必须截断到 0/1")
	}
	// 本仓整数截断灰度口径：0.5 -> uint8(127.5)=127。
	if pts[1].GrayTrunc != 127 {
		t.Fatalf("C=0.5 的截断灰度应为 127, got %d", pts[1].GrayTrunc)
	}
}

func TestCoverageMatches16xSupersample(t *testing.T) {
	for _, x := range []float64{0.25, 0.5, 0.75} {
		hits, total, frac := SuperSampleUnitEdge(x, 16)
		if total != 256 {
			t.Fatalf("样本总数应为 256, got %d", total)
		}
		if frac != UnitEdgeCoverage(x) {
			t.Fatalf("x=%.2f SS=%d/%d=%v 与闭式 %v 不等", x, hits, total, frac, UnitEdgeCoverage(x))
		}
	}
}

func TestEffectiveCoverageFullIsConservation(t *testing.T) {
	r, err := ConservationWeight(0.1, 0.1, Options{})
	if err != nil {
		t.Fatal(err)
	}
	lin, p := EffectiveWithCoverage(r, 1.0)
	if lin != r.Out || p != r.Brightness {
		t.Fatalf("C=1 应回到守恒输出: lin=%v out=%v p=%v B=%v", lin, r.Out, p, r.Brightness)
	}
	lin0, p0 := EffectiveWithCoverage(r, 0)
	if lin0 != 0 || p0 != 0 {
		t.Fatal("C=0 应为全黑")
	}
	// C=0.5 时线性值恰好减半。
	linHalf, _ := EffectiveWithCoverage(r, 0.5)
	if d := linHalf - 0.5*r.Out; d > 1e-15 || d < -1e-15 {
		t.Fatalf("C=.5 应减半, got %v", linHalf)
	}
}
