package strip

import "vecgo/geom"

// NewRibbonMesh builds c independent one-cell-high grid ribbons. widths[k] is
// the number of square cells in component k. Components are stacked with one
// empty grid row between them, so no vertex or edge is shared.
func NewRibbonMesh(name string, widths []int) *Mesh {
	m := &Mesh{Name: name}
	for comp, cells := range widths {
		y0 := float64(3 * comp)
		base := len(m.Vertices)
		for i := 0; i <= cells; i++ {
			m.Vertices = append(m.Vertices, geom.Point{X: float64(i), Y: y0})
		}
		topBase := len(m.Vertices)
		for i := 0; i <= cells; i++ {
			m.Vertices = append(m.Vertices, geom.Point{X: float64(i), Y: y0 + 1})
		}
		_ = topBase
		bottom := func(i int) int { return base + i }
		top := func(i int) int { return base + cells + 1 + i }
		for i := 0; i < cells; i++ {
			a, b := bottom(i), bottom(i+1)
			d, c := top(i), top(i+1)
			// Same diagonal convention as a grid split from lower-left to
			// upper-right. IDs are ordered lower triangle then upper triangle.
			m.Triangles = append(m.Triangles, [3]int{a, b, c})
			m.Triangles = append(m.Triangles, [3]int{a, c, d})
		}
	}
	return m
}

// NewCounterexample builds the five-triangle frame used in the greedy-failure
// note. Coordinates live in [0,16]x[0,16]; the occupied box is
// x in [2,12], y in [4,13].
//
// Dual graph and triangle indices:
//
//	0: top core hub OAB, degree 3
//	1: left core hub OCA, degree 3
//	2: leaf across AB, degree 1
//	3: lower-right core triangle OBC, degree 2
//	4: left leaf across CA, degree 1
//
// Edges: 0-1, 0-2, 0-3, 1-3, 1-4.
func NewCounterexample() *Mesh {
	p := []geom.Point{
		{X: 8, Y: 8},   // 0 O
		{X: 4, Y: 10},  // 1 A
		{X: 12, Y: 10}, // 2 B
		{X: 8, Y: 4},   // 3 C
		{X: 8, Y: 13},  // 4 D
		{X: 2, Y: 6},   // 5 E
	}
	return &Mesh{
		Name:     "degree-greedy-counterexample",
		Vertices: p,
		Triangles: [][3]int{
			{0, 1, 2}, // T0 O A B
			{0, 3, 1}, // T1 O C A
			{1, 2, 4}, // T2 A B D
			{0, 2, 3}, // T3 O B C
			{3, 1, 5}, // T4 C A E
		},
	}
}
