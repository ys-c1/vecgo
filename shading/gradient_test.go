package shading

import (
	"math"
	"testing"

	"vecgo/geom"
)

func TestSignedDistanceMatchesRangeCondition(t *testing.T) {
	lin := LinearGradient{
		A: geom.Point{X: 10, Y: 20},
		B: geom.Point{X: 50, Y: 40},
	}
	L := lin.length()

	for y := -20.0; y <= 80; y += 2.5 {
		for x := -30.0; x <= 90; x += 2.5 {
			p := geom.Point{X: x, Y: y}
			tv := lin.Parameter(p)
			sd := lin.SignedDistance(p)
			wantOutside := tv < 0 || tv > 1
			if got := sd < -1e-12; got != wantOutside {
				t.Fatalf("linear p=%v t=%v sd=%v outside=%v distance=%v", p, tv, sd, wantOutside, got)
			}
			wantE := math.Max(math.Max(-tv, tv-1), 0)
			if got := math.Max(0, -sd/L); math.Abs(got-wantE) > 1e-12 {
				t.Fatalf("linear overshoot p=%v got %v want %v", p, got, wantE)
			}
		}
	}

	rad := RadialGradient{Center: geom.Point{X: 7, Y: 9}, Radius: 4.5}
	for y := -5.0; y <= 25; y += 1.25 {
		for x := -5.0; x <= 25; x += 1.25 {
			p := geom.Point{X: x, Y: y}
			tv := rad.Parameter(p)
			sd := rad.SignedDistance(p)
			if got := sd < -1e-12; got != (tv > 1) {
				t.Fatalf("radial p=%v t=%v sd=%v", p, tv, sd)
			}
			wantE := math.Max(0, tv-1)
			if got := math.Max(0, -sd/rad.Radius); math.Abs(got-wantE) > 1e-12 {
				t.Fatalf("radial overshoot p=%v got %v want %v", p, got, wantE)
			}
		}
	}
}

func TestTruncationIdentityInsideParameterDomain(t *testing.T) {
	g := RadialGradient{
		Center: geom.Point{X: 5, Y: 5}, Radius: 4,
		Inner: RGB{R: 0.1, G: 0.2, B: 0.3},
		Outer: RGB{R: 0.7, G: 0.6, B: 0.5},
	}
	for r := 0.0; r < g.Radius-1e-9; r += 0.25 {
		for a := 0.0; a < 2*math.Pi; a += math.Pi / 12 {
			p := geom.Point{X: 5 + r*math.Cos(a), Y: 5 + r*math.Sin(a)}
			tv := g.Parameter(p)
			if tv < 0 || tv > 1 {
				t.Fatalf("test setup crossed domain: t=%v", tv)
			}
			if got := clampUnit(tv); got != tv {
				t.Fatalf("内部参数被截断: %v -> %v", tv, got)
			}
		}
	}

	lin := LinearGradient{A: geom.Point{X: 0, Y: 0}, B: geom.Point{X: 8, Y: 0}}
	for x := 0.0; x <= 8; x += 0.5 {
		p := geom.Point{X: x, Y: 3}
		if sd := lin.SignedDistance(p); sd < 0 {
			t.Fatalf("slab interior should have nonnegative signed distance, got %v", sd)
		}
	}
}

func TestMeasureRadialFullScanFindsNonCornerOverflow(t *testing.T) {
	// 六边形位于 31x31 中心，半径 10；四个画幅角点在多边形外。
	poly := regularTestPolygon(6, 15.5, 15.5, 10)
	g := RadialGradient{
		Center: geom.Point{X: 15.5, Y: 15.5}, Radius: 4,
		Inner: RGB{R: 0.05, G: 0.1, B: 0.2},
		Outer: RGB{R: 0.9, G: 0.85, B: 0.8},
	}
	const n = 31
	before := RenderRadial(poly, g, n, n, 2, false)
	after := RenderRadial(poly, g, n, n, 2, true)
	m := MeasureRadial("t", "test", poly, g, before, after, 2)

	for _, c := range [][2]int{{0, 0}, {n - 1, 0}, {0, n - 1}, {n - 1, n - 1}} {
		p := geom.Point{X: float64(c[0]) + 0.5, Y: float64(c[1]) + 0.5}
		if geom.Inside(poly, p.X, p.Y) {
			t.Fatalf("测试前置失效：角点 %v 不应在多边形内", c)
		}
	}
	if m.ParameterOverflowPixels == 0 {
		t.Fatal("整幅扫描应发现边中区域的参数溢出；只查四角会得到 0")
	}
	if m.MaxPixelDelta == 0 {
		t.Fatal("截断前后应存在可测像素差")
	}
	if m.MaxPixelDelta > m.Bound8Bit {
		t.Fatalf("最大色差 %d 超过距离/Lipschitz 界 %d", m.MaxPixelDelta, m.Bound8Bit)
	}
	if m.WhitePixelsAfter != 0 {
		t.Fatalf("截断后不应有纯白死白像素，得到 %d", m.WhitePixelsAfter)
	}
	if m.MaxPixelDeltaAt[0] == 0 || m.MaxPixelDeltaAt[0] == n-1 ||
		m.MaxPixelDeltaAt[1] == 0 || m.MaxPixelDeltaAt[1] == n-1 {
		t.Fatalf("最大差异不应只来自四角: %v", m.MaxPixelDeltaAt)
	}
}

func regularTestPolygon(n int, cx, cy, r float64) []geom.Point {
	out := make([]geom.Point, n)
	for i := 0; i < n; i++ {
		a := -math.Pi/2 + 2*math.Pi*float64(i)/float64(n)
		out[i] = geom.Point{X: cx + r*math.Cos(a), Y: cy + r*math.Sin(a)}
	}
	return out
}
