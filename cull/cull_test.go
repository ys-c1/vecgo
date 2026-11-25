package cull

import (
	"math"
	"math/rand"
	"testing"

	"vecgo/geom"
)

func rngTri(rng *rand.Rand) []geom.Point {
	base := []geom.Point{
		{X: rng.NormFloat64() * 6, Y: rng.NormFloat64() * 6},
		{X: rng.NormFloat64() * 6, Y: rng.NormFloat64() * 6},
		{X: rng.NormFloat64() * 6, Y: rng.NormFloat64() * 6},
	}
	return base
}

// TestConservativeSphereContainsBox：包围球必须完整包住包围盒的四角与随机点，
// 这是保守性的几何前提。
func TestConservativeSphereContainsBox(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for n := 0; n < 2000; n++ {
		tri := rngTri(rng)
		b := TriBounds(tri)
		s := ConservativeSphere(b)
		corners := []geom.Point{
			{X: b.MinX, Y: b.MinY}, {X: b.MaxX, Y: b.MinY},
			{X: b.MinX, Y: b.MaxY}, {X: b.MaxX, Y: b.MaxY},
		}
		tol := 1e-12*s.R + 1e-12
		for _, c := range corners {
			if d := math.Hypot(c.X-s.CX, c.Y-s.CY); d > s.R+tol {
				t.Fatalf("盒角落在球外: d=%v r=%v", d, s.R)
			}
		}
		for _, p := range tri {
			if d := math.Hypot(p.X-s.CX, p.Y-s.CY); d > s.R+tol {
				t.Fatalf("三角形顶点落在球外: d=%v r=%v", d, s.R)
			}
		}
	}
}

// TestConservativeNoFalseKill：所有“精确可见”的三角形，保守球一律不许剔除。
// 这是用户要求的硬指标：误杀数为零。
func TestConservativeNoFalseKill(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	v := View{MinX: -1, MinY: -1, MaxX: 1, MaxY: 1}
	falseKills := 0
	for n := 0; n < 500; n++ {
		tri := rngTri(rng)
		b := TriBounds(tri)
		s := ConservativeSphere(b)
		if s.Culled(v, false) && ExactVisible(tri, v, 32, 32, 2) {
			falseKills++
			t.Fatalf("保守球误杀完全可见三角形 #%d: %+v", n, tri)
		}
	}
	if falseKills != 0 {
		t.Fatalf("误杀数 %d，必须为 0", falseKills)
	}
}

// TestRadiusDiagonalRatio：保守球 r/D 恒为 0.5；缩水球 r/D <= 0.5，
// 非正方形盒严格小于 0.5，违背保守性条件。
func TestRadiusDiagonalRatio(t *testing.T) {
	b := Bounds{MinX: 0, MinY: 0, MaxX: 4, MaxY: 3}
	D := b.Diagonal()
	if got := ConservativeSphere(b).R / D; math.Abs(got-0.5) > 1e-15 {
		t.Fatalf("保守球 r/D = %v, 期望 0.5", got)
	}
	loose := LooseSphere(b)
	if loose.R/D >= 0.5 {
		t.Fatalf("缩水球 r/D = %v 应小于 0.5", loose.R/D)
	}
}

// TestLooseSphereFalseKill：缩水球对跨视窗角的细长三角形产生误杀，
// 而保守球与精确判定都保留它。复现“画面缺一角”。
// 该三角形三个顶点全在视窗外，但一条边从角部切入，精确判定有 21 个采样点命中。
func TestLooseSphereFalseKill(t *testing.T) {
	v := View{MinX: 0, MinY: 0, MaxX: 10, MaxY: 10}
	tri := []geom.Point{{X: -20, Y: -20}, {X: 3, Y: -2}, {X: -2, Y: 3}}
	if !ExactVisible(tri, v, 32, 32, 2) {
		t.Fatal("测试三角形应当精确可见")
	}
	b := TriBounds(tri)
	if ConservativeSphere(b).Culled(v, false) {
		t.Fatal("保守球不应剔除可见三角形")
	}
	if !LooseSphere(b).Culled(v, true) {
		t.Fatal("缩水球在此场景应触发误杀，用于复现缺陷")
	}
}
