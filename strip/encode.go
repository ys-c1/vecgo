package strip

import "fmt"

// EncodePaths encodes every triangle path as an explicit vertex-ID stream.
func EncodePaths(m *Mesh, paths [][]int) [][]int {
	streams := make([][]int, 0, len(paths))
	for _, p := range paths {
		streams = append(streams, EncodePath(m, p))
	}
	return streams
}

// EncodePath returns a shortest stream under a small dynamic program.
//
// At a path boundary the next triangle must share the current frontier edge,
// which is the final ordered pair of the stream. If the orientation or edge
// is wrong, one or two references from the current triangle rotate the pair;
// BFS finds the minimum such alignment. Keeping only the cheapest prefix for
// each possible final ordered pair is sufficient because all future cost
// depends only on that pair.
func EncodePath(m *Mesh, path []int) []int {
	if len(path) == 0 {
		return nil
	}
	if len(path) == 1 {
		t := m.Triangles[path[0]]
		return []int{t[0], t[1], t[2]}
	}

	type pair = [2]int
	type node struct {
		pair   pair
		cost   int
		parent *node
		chunk  []int
	}

	states := make(map[pair]*node, 6)
	rootTri := m.Triangles[path[0]]
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			if i == j {
				continue
			}
			k := 3 - i - j // indices are 0+1+2=3
			a, b, c := rootTri[i], rootTri[j], rootTri[k]
			n := &node{pair: pair{a, b}, cost: 3, chunk: []int{c, a, b}}
			if old, ok := states[n.pair]; !ok || n.cost < old.cost {
				states[n.pair] = n
			}
		}
	}

	for pi := 1; pi < len(path); pi++ {
		prevTri := m.Triangles[path[pi-1]]
		curTri := m.Triangles[path[pi]]
		e, ok := SharedEdge(m, path[pi-1], path[pi])
		if !ok {
			// Greedy paths are adjacent by construction. Treat a violation as
			// an unrecoverable programming error rather than emitting geometry.
			panic(fmt.Sprintf("strip path is not adjacent at %d->%d", path[pi-1], path[pi]))
		}
		z, ok := thirdVertex(curTri, e)
		if !ok {
			panic("shared edge does not have a unique third vertex")
		}

		targets := []pair{{e[0], e[1]}, {e[1], e[0]}}
		next := make(map[pair]*node, 6)
		for _, parent := range states {
			for _, target := range targets {
				align := shortestAlignment(parent.pair, target, prevTri)
				if align == nil {
					panic("no strip alignment sequence")
				}
				chunk := make([]int, 0, len(align)+1)
				chunk = append(chunk, align...)
				chunk = append(chunk, z)
				np := pair{target[1], z}
				nn := &node{
					pair:   np,
					cost:   parent.cost + len(chunk),
					parent: parent,
					chunk:  chunk,
				}
				if old, ok := next[np]; !ok || nn.cost < old.cost ||
					(nn.cost == old.cost && lessInts(nn.chunk, old.chunk)) {
					next[np] = nn
				}
			}
		}
		states = next
	}

	var best *node
	for _, n := range states {
		if best == nil || n.cost < best.cost || (n.cost == best.cost && lessInts(n.chunk, best.chunk)) {
			best = n
		}
	}

	var reversedChunks [][]int
	for n := best; n != nil; n = n.parent {
		reversedChunks = append(reversedChunks, n.chunk)
	}
	stream := make([]int, 0, best.cost)
	for i := len(reversedChunks) - 1; i >= 0; i-- {
		stream = append(stream, reversedChunks[i]...)
	}
	return stream
}

func thirdVertex(tri [3]int, e Edge) (int, bool) {
	for _, v := range tri {
		if v != e[0] && v != e[1] {
			return v, true
		}
	}
	return -1, false
}

// shortestAlignment returns appended IDs transforming final pair from into to.
// A BFS over the at most nine ordered pairs keeps the result deterministic.
func shortestAlignment(from, to [2]int, tri [3]int) []int {
	if from == to {
		return []int{}
	}
	type pair = [2]int
	parent := make(map[pair]pair)
	appended := make(map[pair]int)
	visited := map[pair]bool{from: true}
	queue := []pair{from}

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, x := range tri {
			np := pair{cur[1], x}
			if visited[np] {
				continue
			}
			visited[np] = true
			parent[np] = cur
			appended[np] = x
			if np == to {
				var rev []int
				for p := to; p != from; p = parent[p] {
					rev = append(rev, appended[p])
				}
				out := make([]int, len(rev))
				for i, v := range rev {
					out[len(rev)-1-i] = v
				}
				return out
			}
			queue = append(queue, np)
		}
	}
	return nil
}

func lessInts(a, b []int) bool {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return len(a) < len(b)
}

// StreamTriangles returns every sliding length-three window as indexed IDs.
func StreamTriangles(stream []int) [][3]int {
	if len(stream) < 3 {
		return nil
	}
	out := make([][3]int, 0, len(stream)-2)
	for i := 0; i+2 < len(stream); i++ {
		out = append(out, [3]int{stream[i], stream[i+1], stream[i+2]})
	}
	return out
}
