// Package strip audits degenerate triangles produced while joining triangle strips.
package strip

import (
	"math"

	"vecgo/geom"
)

// Point is a grid-space vertex.
type Point = geom.Point

// Triangle is an expanded strip triple.
type Triangle struct{ A, B, C Point }

// Box is an axis-aligned raster window in grid coordinates.
type Box struct {
	MinX, MinY, MaxX, MaxY float64
}

// Area2 returns twice the signed triangle area. It is also the cross product
// (B-A) x (C-A). Exact zero means collinear (including repeated vertices).
func Area2(t Triangle) float64 {
	return (t.B.X-t.A.X)*(t.C.Y-t.A.Y) - (t.B.Y-t.A.Y)*(t.C.X-t.A.X)
}

// Collinear reports the requested geometric criterion.
func Collinear(t Triangle) bool { return Area2(t) == 0 }

// Degenerate is the same predicate as Collinear: zero signed area.
func Degenerate(t Triangle) bool { return Area2(t) == 0 }

// DegenerateTol is useful for externally generated floating-point data.
func DegenerateTol(t Triangle, eps float64) bool { return math.Abs(Area2(t)) <= eps }

// FromStrip expands a strip vertex stream into consecutive triangles.
func FromStrip(s []Point) []Triangle {
	if len(s) < 3 {
		return nil
	}
	out := make([]Triangle, 0, len(s)-2)
	for i := 0; i+2 < len(s); i++ {
		out = append(out, Triangle{A: s[i], B: s[i+1], C: s[i+2]})
	}
	return out
}

// FilterDegenerate returns only positive-area triangles.
func FilterDegenerate(ts []Triangle) []Triangle {
	out := make([]Triangle, 0, len(ts))
	for _, t := range ts {
		if !Degenerate(t) {
			out = append(out, t)
		}
	}
	return out
}

// CountDegenerate enumerates every triangle; there is no sampling.
func CountDegenerate(ts []Triangle) int {
	n := 0
	for _, t := range ts {
		if Degenerate(t) {
			n++
		}
	}
	return n
}

// ConnectDirect concatenates two canonical strips without a bridge vertex.
func ConnectDirect(a, b []Point) []Point {
	out := make([]Point, 0, len(a)+len(b))
	out = append(out, a...)
	out = append(out, b...)
	return out
}

// ConnectOptimal uses one copied anchor. For the audited topologies, A_last
// lies on the line of B's first cap, so the three crossing triples are all
// degenerate. This reaches E-1=3.
func ConnectOptimal(a, b []Point) []Point {
	out := make([]Point, 0, len(a)+len(b)+1)
	out = append(out, a...)
	out = append(out, a[len(a)-1])
	out = append(out, b...)
	return out
}

// ConnectNaive uses the usual two duplicated anchors and creates four
// degenerate triples. It is valid but one above the E-1 lower bound.
func ConnectNaive(a, b []Point) []Point {
	out := make([]Point, 0, len(a)+len(b)+2)
	out = append(out, a...)
	out = append(out, a[len(a)-1])
	out = append(out, b[0])
	out = append(out, b...)
	return out
}

// HorizontalStrip returns a column-zigzag strip for [x0,x0+cells] x [y0,y1].
func HorizontalStrip(x0 float64, cells int, y0, y1 float64) []Point {
	out := make([]Point, 0, 2*(cells+1))
	for i := 0; i <= cells; i++ {
		x := x0 + float64(i)
		out = append(out, Point{X: x, Y: y0}, Point{X: x, Y: y1})
	}
	return out
}

// VerticalStrip returns a row-zigzag strip for [x0,x1] x [y0,y0+direction*cells].
// direction must be +1 (up) or -1 (down).
func VerticalStrip(x0, x1, y0 float64, cells, direction int) []Point {
	out := make([]Point, 0, 2*(cells+1))
	dy := 1.0
	if direction < 0 {
		dy = -1
	}
	for i := 0; i <= cells; i++ {
		y := y0 + dy*float64(i)
		out = append(out, Point{X: x0, Y: y}, Point{X: x1, Y: y})
	}
	return out
}

// HorizontalStripReversedCap is HorizontalStrip with each cap emitted top,bottom.
// The reversed cap models the opposite strip winding required at a shared seam.
func HorizontalStripReversedCap(x0 float64, cells int, y0, y1 float64) []Point {
	out := make([]Point, 0, 2*(cells+1))
	for i := 0; i <= cells; i++ {
		x := x0 + float64(i)
		out = append(out, Point{X: x, Y: y1}, Point{X: x, Y: y0})
	}
	return out
}

// Topology is one full-grid verification case.
type Topology struct {
	ID        string
	Name      string
	Embedding string
	Kind      string
	E         int
	Cells     int
	A, B      []Point
	Optimal   []Point
	Naive     []Point
	Direct    []Point
	RenderBox Box
	PortProof string
}

// Topologies returns the three port-topology verifications: disjoint ports,
// one shared vertex, and one shared edge. After quotienting identified port
// coordinates, these have E=4, 3, and 2 distinct seam endpoints.
func Topologies(cells int) []Topology {
	const (
		y0 = 2.0
		y1 = 4.0
		x0 = 0.0
	)
	x1 := x0 + float64(cells)
	a := HorizontalStrip(x0, cells, y0, y1)

	// Topology 1: four distinct port occurrences. A_last lies on B's first cap
	// line, allowing one copied anchor to produce exactly three degenerates.
	disjointB := VerticalStrip(x1+2, x1+4, y1, cells, +1)
	disjoint := Topology{
		ID:        "disjoint-4port",
		Name:      "分离端口拓扑",
		Embedding: "两个网格条带不共享坐标；接缝端口集合为 u,A,B0,B1",
		Kind:      "disjoint",
		E:         4,
		Cells:     cells,
		A:         a,
		B:         disjointB,
		Optimal:   ConnectOptimal(a, disjointB),
		Naive:     ConnectNaive(a, disjointB),
		Direct:    ConnectDirect(a, disjointB),
		RenderBox: Box{MinX: -1, MinY: 1, MaxX: x1 + 5, MaxY: y1 + float64(cells) + 1},
		PortProof: "E=4：u=(x1,2)、A=(x1,4)、B0=(x1+2,4)、B1=(x1+4,4)。直接拼接的 (u,A,B0) 有非零面积；插入一个 A 引用后三个接缝三元组全部零面积。",
	}

	// Topology 2: A_last and B_first are the same grid vertex S. The seam has
	// three distinct ports u,S,B1; retaining the repeated S yields two degen.
	sharedVertexB := VerticalStrip(x1, x1+2, y1, cells, +1)
	sharedVertex := Topology{
		ID:        "shared-vertex-3port",
		Name:      "共点端口拓扑",
		Embedding: "L 形网格在一个角点 S 相接；A 的末顶点等于 B 的首顶点",
		Kind:      "shared-vertex",
		E:         3,
		Cells:     cells,
		A:         a,
		B:         sharedVertexB,
		Optimal:   ConnectDirect(a, sharedVertexB),
		Naive:     ConnectNaive(a, sharedVertexB),
		Direct:    ConnectDirect(a, sharedVertexB),
		RenderBox: Box{MinX: -1, MinY: 1, MaxX: x1 + 3, MaxY: y1 + float64(cells) + 1},
		PortProof: "E=3：u=(x1,2)、S=(x1,4)、B1=(x1+2,4)。流中保留 S,S 两个引用，接缝为 (u,S,S) 与 (S,S,B1)，恰有两个退化三角形。",
	}

	// Topology 3: the cap edge P-Q is shared, with B's cap emitted Q,P due to
	// opposite winding. Overlap the already-emitted Q=B0 and append B[1:].
	sharedEdgeB := HorizontalStripReversedCap(x1, cells, y0, y1)
	edgeOptimal := append(append([]Point{}, a...), sharedEdgeB[1:]...)
	sharedEdge := Topology{
		ID:        "shared-edge-2port",
		Name:      "共边端口拓扑",
		Embedding: "两个条带共享整条端帽边 P-Q；固定绕向使 B 以 Q,P 开始",
		Kind:      "shared-edge",
		E:         2,
		Cells:     cells,
		A:         a,
		B:         sharedEdgeB,
		Optimal:   edgeOptimal,
		Naive:     ConnectDirect(a, sharedEdgeB),
		Direct:    ConnectDirect(a, sharedEdgeB),
		RenderBox: Box{MinX: -1, MinY: y0 - 1, MaxX: 2*x1 + 1, MaxY: y1 + 1},
		PortProof: "E=2：P=(x1,2)、Q=(x1,4)。重叠 Q 后接缝三元组为 (P,Q,P)，三点共线且面积为零，恰有一个退化三角形。不重叠 Q 的朴素拼法产生两个退化三角形。",
	}

	return []Topology{disjoint, sharedVertex, sharedEdge}
}

// EmbeddingTopologies returns three full-grid E=4 cases. They are distinct
// placements/embeddings of two non-merged strips; every case reaches D=3.
func EmbeddingTopologies(cells int) []Topology {
	const (
		ay0 = 2.0
		ay1 = 4.0
		ax0 = 0.0
	)
	ax1 := ax0 + float64(cells)
	bx0 := ax1 + 2
	bx1 := bx0 + 2

	makeTopo := func(id, name, embedding string, b []Point, box Box, proof string) Topology {
		a := HorizontalStrip(ax0, cells, ay0, ay1)
		return Topology{
			ID:        id,
			Name:      name,
			Embedding: embedding,
			Kind:      "e4-embedding",
			E:         4,
			Cells:     cells,
			A:         a,
			B:         b,
			Optimal:   ConnectOptimal(a, b),
			Naive:     ConnectNaive(a, b),
			Direct:    ConnectDirect(a, b),
			RenderBox: box,
			PortProof: proof,
		}
	}

	rise := VerticalStrip(bx0, bx1, ay1, cells, +1)
	fall := VerticalStrip(bx0, bx1, ay1, cells, -1)
	backX0, backX1 := -bx1-2, -bx0-2
	back := VerticalStrip(backX0, backX1, ay1, cells, +1)

	return []Topology{
		makeTopo(
			"grid-rise", "右端上升网格拓扑", "A 条带水平向右，B 条带在右端向上延伸",
			rise,
			Box{MinX: -1, MinY: 1, MaxX: bx1 + 1, MaxY: ay1 + float64(cells) + 1},
			"E=4：u=(x1,2)、A=(x1,4)、B0=(x1+2,4)、B1=(x1+4,4)。直接拼接的 (u,A,B0) 有非零面积；插入一个 A 引用后三个接缝三元组全部零面积。",
		),
		makeTopo(
			"grid-fall", "右端下沉网格拓扑", "A 条带水平向右，B 条带在右端向下折返",
			fall,
			Box{MinX: -1, MinY: ay1 - float64(cells) - 1, MaxX: bx1 + 1, MaxY: 5},
			"端口判据与 grid-rise 相同；B 的延伸方向反向，用来验证下沉/折返嵌入不改变计数。",
		),
		makeTopo(
			"grid-backfold", "左侧回折网格拓扑", "B 端帽位于 A 的 A 端延长线上，但条带主体回到 A 的左侧",
			back,
			Box{MinX: backX0 - 1, MinY: 1, MaxX: ax1 + 1, MaxY: ay1 + float64(cells) + 1},
			"B0,B1 位于 y=4，A=(ax1,4) 位于该端帽直线的延长线上；这是第三种 E=4 网格嵌入。",
		),
	}
}
