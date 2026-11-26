package strip_test

import (
	"bytes"
	"reflect"
	"testing"

	"vecgo/config"
	"vecgo/pipe"
	"vecgo/strip"
)

func TestCounterexampleGreedyOrder(t *testing.T) {
	m := strip.NewCounterexample()
	adj := strip.Adjacency(m)
	wantDeg := []int{3, 3, 1, 2, 1}
	for i, ns := range adj {
		if len(ns) != wantDeg[i] {
			t.Fatalf("T%d degree=%d, want %d; adj=%v", i, len(ns), wantDeg[i], ns)
		}
	}
	if c := strip.Components(adj); c != 1 {
		t.Fatalf("components=%d, want 1", c)
	}

	paths := strip.GreedyPaths(m)
	want := [][]int{{0, 1, 3}, {2}, {4}}
	if !reflect.DeepEqual(paths, want) {
		t.Fatalf("greedy paths=%v, want %v", paths, want)
	}

	streams := strip.EncodePaths(m, paths)
	gotVertices := 0
	for _, s := range streams {
		gotVertices += len(s)
	}
	if gotVertices != 11 {
		t.Fatalf("greedy stream vertices=%d, want 11; streams=%v", gotVertices, streams)
	}

	optimal := []int{2, 0, 3, 1, 4}
	optStream := strip.EncodePath(m, optimal)
	if len(optStream) != 7 {
		t.Fatalf("optimal stream length=%d, want 7: %v", len(optStream), optStream)
	}
}

func TestRibbonGreedyCounts(t *testing.T) {
	cases := []struct {
		name   string
		widths []int
	}{
		{"g1", []int{96}},
		{"g2", []int{64, 64}},
		{"g3", []int{48, 48, 48, 48}},
	}
	for _, tc := range cases {
		m := strip.NewRibbonMesh(tc.name, tc.widths)
		paths := strip.GreedyPaths(m)
		if len(paths) != 2*len(tc.widths) {
			t.Fatalf("%s strips=%d, want %d", tc.name, len(paths), 2*len(tc.widths))
		}
		streams := strip.EncodePaths(m, paths)
		n := 0
		for _, s := range streams {
			n += len(s)
		}
		tris := 0
		for _, w := range tc.widths {
			tris += 2 * w
		}
		want := tris + 4*len(tc.widths)
		if n != want {
			t.Fatalf("%s vertices=%d, want %d; lengths=%v", tc.name, n, want, streamLengths(streams))
		}
	}
}

func streamLengths(ss [][]int) []int {
	out := make([]int, len(ss))
	for i, s := range ss {
		out[i] = len(s)
	}
	return out
}

func TestPixelEquality(t *testing.T) {
	meshes := []*strip.Mesh{
		strip.NewRibbonMesh("g1", []int{96}),
		strip.NewRibbonMesh("g2", []int{64, 64}),
		strip.NewRibbonMesh("g3", []int{48, 48, 48, 48}),
		strip.NewCounterexample(),
	}
	for _, m := range meshes {
		rep, err := strip.Analyze(m, 72, 48, 2)
		if err != nil {
			t.Fatalf("%s analyze: %v", m.Name, err)
		}
		if rep.DifferentPixels != 0 {
			t.Fatalf("%s: %d/%d pixels differ", m.Name, rep.DifferentPixels, rep.PixelCount)
		}
	}
}

func TestEncodedStreamsContainEveryTriangle(t *testing.T) {
	meshes := []*strip.Mesh{
		strip.NewRibbonMesh("g1", []int{96}),
		strip.NewRibbonMesh("g2", []int{64, 64}),
		strip.NewRibbonMesh("g3", []int{48, 48, 48, 48}),
		strip.NewCounterexample(),
	}
	for _, m := range meshes {
		paths := strip.GreedyPaths(m)
		streams := strip.EncodePaths(m, paths)
		seen := map[[3]int]bool{}
		for _, s := range streams {
			for _, tri := range strip.StreamTriangles(s) {
				seen[[3]int{tri[0], tri[1], tri[2]}] = true
			}
		}
		for _, tri := range m.Triangles {
			key := [3]int{tri[0], tri[1], tri[2]}
			reversed := [3]int{tri[2], tri[1], tri[0]}
			if !seen[key] && !seen[reversed] {
				// Sliding windows preserve one of the two winding orders.
				found := false
				for k := range seen {
					if sameTriangleSet(k, key) {
						found = true
						break
					}
				}
				if !found {
					t.Fatalf("%s: triangle %v missing from stream", m.Name, tri)
				}
			}
		}
	}
}

func sameTriangleSet(a, b [3]int) bool {
	ca := map[int]int{}
	for _, v := range a {
		ca[v]++
	}
	for _, v := range b {
		ca[v]--
	}
	for _, n := range ca {
		if n != 0 {
			return false
		}
	}
	return true
}

func TestFullPipelinePixelEquality(t *testing.T) {
	meshes := []*strip.Mesh{
		strip.NewRibbonMesh("g1", []int{96}),
		strip.NewRibbonMesh("g2", []int{64, 64}),
		strip.NewRibbonMesh("g3", []int{48, 48, 48, 48}),
		strip.NewCounterexample(),
	}
	for _, m := range meshes {
		paths := strip.GreedyPaths(m)
		streams := strip.EncodePaths(m, paths)
		cfg := config.Default()
		cfg.Width, cfg.Height = 64, 48
		cfg.Supersample = 2
		cfg.Gamma = 1
		cfg.Blur = 0

		a, err := pipe.Run(strip.TrianglePolygons(m), cfg)
		if err != nil {
			t.Fatalf("%s independent pipe: %v", m.Name, err)
		}
		b, err := pipe.Run(strip.StreamPolygons(m, streams), cfg)
		if err != nil {
			t.Fatalf("%s strip pipe: %v", m.Name, err)
		}
		if !bytes.Equal(a.Pix, b.Pix) {
			diff := 0
			for i := range a.Pix {
				if a.Pix[i] != b.Pix[i] {
					diff++
				}
			}
			t.Fatalf("%s full pipeline: %d pixels differ", m.Name, diff)
		}
	}
}
