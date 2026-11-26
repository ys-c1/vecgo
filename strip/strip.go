// Package strip builds edge-adjacency graphs and degree-greedy triangle
// strips for indexed triangle meshes.
//
// The accounting convention is "submitted vertex references in a strip
// block": a block containing t triangles uses t+2 references before optional
// orientation swaps. The encoder in this package records those swap/refetch
// references explicitly, so the reported vertex count is the real stream
// length rather than only the ideal slot count.
package strip

import (
	"sort"

	"vecgo/geom"
)

// Mesh is an indexed triangle mesh. Triangles reference Vertices by index.
type Mesh struct {
	Name      string
	Vertices  []geom.Point
	Triangles [][3]int
}

// Edge is a canonical undirected edge key.
type Edge [2]int

type edgeIncidence struct {
	edge Edge
	tris []int
}

// CanonicalEdge returns the sorted undirected edge key.
func CanonicalEdge(a, b int) Edge {
	if a <= b {
		return Edge{a, b}
	}
	return Edge{b, a}
}

// SharedEdge returns the two vertex indices shared by two triangles.
// It returns false when they do not share exactly two indexed vertices.
func SharedEdge(m *Mesh, a, b int) (Edge, bool) {
	counts := make(map[int]int, 3)
	for _, v := range m.Triangles[a] {
		counts[v]++
	}
	shared := make([]int, 0, 2)
	for _, v := range m.Triangles[b] {
		if counts[v] > 0 {
			shared = append(shared, v)
		}
	}
	if len(shared) != 2 {
		return Edge{}, false
	}
	return CanonicalEdge(shared[0], shared[1]), true
}

// Adjacency builds the dual graph: one vertex per triangle, an edge when two
// triangles share a mesh edge. Non-manifold edges with more than two incident
// triangles connect every incident pair; the benchmark meshes are manifold.
func Adjacency(m *Mesh) [][]int {
	byEdge := make(map[Edge][]int)
	for ti, tri := range m.Triangles {
		for k := 0; k < 3; k++ {
			a, b := tri[k], tri[(k+1)%3]
			e := CanonicalEdge(a, b)
			byEdge[e] = append(byEdge[e], ti)
		}
	}

	adj := make([][]int, len(m.Triangles))
	seen := make([]map[int]struct{}, len(m.Triangles))
	for i := range seen {
		seen[i] = make(map[int]struct{})
	}
	add := func(a, b int) {
		if a == b {
			return
		}
		if _, ok := seen[a][b]; !ok {
			seen[a][b] = struct{}{}
			adj[a] = append(adj[a], b)
		}
		if _, ok := seen[b][a]; !ok {
			seen[b][a] = struct{}{}
			adj[b] = append(adj[b], a)
		}
	}
	for _, tris := range byEdge {
		for i := 0; i < len(tris); i++ {
			for j := i + 1; j < len(tris); j++ {
				add(tris[i], tris[j])
			}
		}
	}
	for i := range adj {
		sort.Ints(adj[i])
	}
	return adj
}

// Components returns the number of connected components in the dual graph.
func Components(adj [][]int) int {
	seen := make([]bool, len(adj))
	c := 0
	for start := range adj {
		if seen[start] {
			continue
		}
		c++
		seen[start] = true
		stack := []int{start}
		for len(stack) > 0 {
			u := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			for _, v := range adj[u] {
				if !seen[v] {
					seen[v] = true
					stack = append(stack, v)
				}
			}
		}
	}
	return c
}

// LowerBound is T+2c for T triangles in c edge-connected components.
func LowerBound(triangles, components int) int {
	return triangles + 2*components
}

// GreedyPaths runs the deterministic degree-greedy strip walker.
//
// Rule: start at the unused triangle with largest original dual degree,
// breaking ties by triangle index. From the current end, move to the unused
// neighbor with largest original dual degree, again breaking ties by index.
// When no such neighbor exists, close the strip and restart.
func GreedyPaths(m *Mesh) [][]int {
	adj := Adjacency(m)
	deg := make([]int, len(adj))
	for i, ns := range adj {
		deg[i] = len(ns)
	}
	unused := make([]bool, len(adj))
	for i := range unused {
		unused[i] = true
	}

	var paths [][]int
	for {
		seed := -1
		for i := range unused {
			if !unused[i] {
				continue
			}
			if seed == -1 || deg[i] > deg[seed] || (deg[i] == deg[seed] && i < seed) {
				seed = i
			}
		}
		if seed == -1 {
			return paths
		}

		unused[seed] = false
		path := []int{seed}
		cur := seed
		for {
			next := -1
			for _, v := range adj[cur] {
				if !unused[v] {
					continue
				}
				if next == -1 || deg[v] > deg[next] || (deg[v] == deg[next] && v < next) {
					next = v
				}
			}
			if next == -1 {
				break
			}
			unused[next] = false
			path = append(path, next)
			cur = next
		}
		paths = append(paths, path)
	}
}

// SlotVertices returns the ideal t+2-per-strip count: T+2s.
func SlotVertices(totalTriangles, strips int) int {
	return totalTriangles + 2*strips
}
