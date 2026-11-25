package strip

import (
	"testing"
)

func TestAreaCriterion(t *testing.T) {
	deg := Triangle{Point{X: 0, Y: 0}, Point{X: 2, Y: 4}, Point{X: 3, Y: 6}}
	if !Collinear(deg) || !Degenerate(deg) {
		t.Fatalf("expected collinear/zero area: area2=%v", Area2(deg))
	}
	positive := Triangle{Point{X: 0, Y: 0}, Point{X: 1, Y: 0}, Point{X: 0, Y: 1}}
	if Degenerate(positive) || Area2(positive) != 1 {
		t.Fatalf("unexpected degenerate positive triangle: area2=%v", Area2(positive))
	}
}

func TestThreeTopologiesMeetEndpointLowerBound(t *testing.T) {
	topos := Topologies(5)
	if len(topos) != 3 {
		t.Fatalf("topology count = %d, want 3", len(topos))
	}
	wantE := []int{4, 3, 2}
	for i, topo := range topos {
		a := Audit(topo)
		c := a.Counts
		if c.E != wantE[i] {
			t.Fatalf("%s E=%d, want %d", topo.ID, c.E, wantE[i])
		}
		if c.LowerBound != c.E-1 {
			t.Fatalf("%s lower bound %d != E-1 %d", topo.ID, c.LowerBound, c.E-1)
		}
		if c.OptimalDegenerate != c.LowerBound {
			t.Fatalf("%s D=%d, want E-1=%d", topo.ID, c.OptimalDegenerate, c.LowerBound)
		}
		if !c.OptimalMeetsLowerBound || c.OptimalExtraPositive != 0 {
			t.Fatalf("%s optimal connector invalid: %+v", topo.ID, c)
		}
		if !c.NaiveAboveBound {
			t.Fatalf("%s naive D=%d should exceed LB=%d", topo.ID, c.NaiveDegenerate, c.LowerBound)
		}
		for _, tri := range degenerateTriangles(a.Before) {
			if Area2(tri) != 0 {
				t.Fatalf("%s counted non-zero-area triangle as degenerate: %+v area2=%v", topo.ID, tri, Area2(tri))
			}
		}
	}
}

func TestThreeEmbeddingTopologiesMeetEndpointLowerBound(t *testing.T) {
	topos := EmbeddingTopologies(5)
	if len(topos) != 3 {
		t.Fatalf("embedding topology count = %d, want 3", len(topos))
	}
	for _, topo := range topos {
		a := Audit(topo)
		c := a.Counts
		if c.E != 4 || c.LowerBound != 3 {
			t.Fatalf("%s E=%d LB=%d, want 4/3", topo.ID, c.E, c.LowerBound)
		}
		if c.OptimalDegenerate != 3 || !c.OptimalMeetsLowerBound {
			t.Fatalf("%s D=%d counts=%+v", topo.ID, c.OptimalDegenerate, c)
		}
		if c.NaiveDegenerate != 4 || !c.NaiveAboveBound {
			t.Fatalf("%s naive D=%d should be 4>3", topo.ID, c.NaiveDegenerate)
		}
	}
}

func TestGridSweepCountsEveryTriangle(t *testing.T) {
	for _, cells := range []int{1, 2, 4, 8, 16} {
		groups := [][]Topology{Topologies(cells), EmbeddingTopologies(cells)}
		for _, group := range groups {
			for _, topo := range group {
				a := Audit(topo)
				c := a.Counts
				if c.RealTriangles != 4*cells {
					t.Fatalf("%s cells=%d real=%d want %d", topo.ID, cells, c.RealTriangles, 4*cells)
				}
				if len(FromStrip(topo.Optimal)) != c.RealTriangles+c.LowerBound {
					t.Fatalf("%s cells=%d expanded count mismatch", topo.ID, cells)
				}
				if c.OptimalDegenerate != c.E-1 {
					t.Fatalf("%s cells=%d D=%d LB=%d", topo.ID, cells, c.OptimalDegenerate, c.LowerBound)
				}
			}
		}
	}
}

func TestDegenerateTrianglesCoverNoSamples(t *testing.T) {
	for _, topo := range Topologies(3) {
		a := Audit(topo)
		for _, tri := range degenerateTriangles(a.Before) {
			box := Box{
				MinX: minFloat(tri.A.X, tri.B.X, tri.C.X) - 1,
				MinY: minFloat(tri.A.Y, tri.B.Y, tri.C.Y) - 1,
				MaxX: maxFloat(tri.A.X, tri.B.X, tri.C.X) + 1,
				MaxY: maxFloat(tri.A.Y, tri.B.Y, tri.C.Y) + 1,
			}
			got := Render([]Triangle{tri}, box, RasterConfig{Width: 24, Height: 24, SuperSample: 2})
			if got.Covered != 0 {
				t.Fatalf("%s degenerate triangle covered %d pixels: %+v", topo.ID, got.Covered, tri)
			}
		}
	}
}

func TestBeforeAndAfterRasterBuffersIdentical(t *testing.T) {
	cfg := RasterConfig{Width: 72, Height: 56, SuperSample: 2}
	groups := [][]Topology{Topologies(4), EmbeddingTopologies(4)}
	for _, group := range groups {
		for _, topo := range group {
			a := Audit(topo)
			before := Render(a.Before, topo.RenderBox, cfg)
			after := Render(a.After, topo.RenderBox, cfg)
			diff, _ := PixelDiff(before.Image, after.Image)
			if diff != 0 {
				t.Fatalf("%s before/after differ in %d pixels", topo.ID, diff)
			}
		}
	}
}

func degenerateTriangles(ts []Triangle) []Triangle {
	var out []Triangle
	for _, tri := range ts {
		if Degenerate(tri) {
			out = append(out, tri)
		}
	}
	return out
}

func minFloat(xs ...float64) float64 {
	m := xs[0]
	for _, x := range xs[1:] {
		if x < m {
			m = x
		}
	}
	return m
}

func maxFloat(xs ...float64) float64 {
	m := xs[0]
	for _, x := range xs[1:] {
		if x > m {
			m = x
		}
	}
	return m
}
