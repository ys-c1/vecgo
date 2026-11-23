package geom

import (
	"math"
	"sort"
)

// ReferenceResult 是逐点判定参照（网格采样 + marching squares）的结果。
type ReferenceResult struct {
	Rings       [][]Point
	VertexCount int
	GridArea    float64 // 按落入并集的采样格数统计的面积
	GridStep    float64
	Bounds      [2]Point // min, max
}

// ReferenceUnion 用逐点判定构造参照：对每个格点调用 Inside（多环取并集），
// 再以 marching squares 提取等值轮廓，缝合为环。网格面积即 hitCells*h*h。
func ReferenceUnion(polys [][]Point, cells int) (*ReferenceResult, error) {
	if cells < 4 {
		return nil, ErrTooFewVertices
	}
	// 与精确路径一致地先清理退化/重复点，否则折返刺边会污染逐点 Inside 判定。
	cleaned := make([][]Point, 0, len(polys))
	for _, r := range polys {
		c := cleanRing(r)
		if len(c) < 3 {
			return nil, ErrTooFewVertices
		}
		cleaned = append(cleaned, c)
	}
	polys = cleaned
	minx, miny := polys[0][0].X, polys[0][0].Y
	maxx, maxy := minx, miny
	for _, r := range polys {
		for _, p := range r {
			minx, maxx = math.Min(minx, p.X), math.Max(maxx, p.X)
			miny, maxy = math.Min(miny, p.Y), math.Max(maxy, p.Y)
		}
	}
	pad := math.Max(maxx-minx, maxy-miny) * 0.05
	if pad == 0 {
		pad = 1
	}
	minx -= pad
	miny -= pad
	maxx += pad
	maxy += pad
	hx := (maxx - minx) / float64(cells)
	hy := (maxy - miny) / float64(cells)

	// 约定：i 为 X 方向下标，j 为 Y 方向下标，格点 grid[i][j] 对应 (x_i, y_j)。
	insideAt := func(i, j int) bool {
		x := minx + float64(i)*hx
		y := miny + float64(j)*hy
		for _, r := range polys {
			if Inside(r, x, y) {
				return true
			}
		}
		return false
	}

	grid := make([][]bool, cells+1)
	hitCells := 0
	for i := range grid {
		grid[i] = make([]bool, cells+1)
		for j := range grid[i] {
			grid[i][j] = insideAt(i, j)
		}
	}
	// 边节点（等值线与格边交点，二值阈值取在中点）
	edgeNode := map[string]Point{}
	vNode := func(i, j int) Point {
		p := Point{minx + (float64(i)+0.5)*hx, miny + float64(j)*hy}
		edgeNode[nodeKey(p)] = p
		return p
	}
	hNode := func(i, j int) Point {
		p := Point{minx + float64(i)*hx, miny + (float64(j)+0.5)*hy}
		edgeNode[nodeKey(p)] = p
		return p
	}

	type seg struct{ a, b Point }
	var segs []seg
	emit := func(p, q Point) { segs = append(segs, seg{p, q}) }

	for j := 0; j < cells; j++ {
		for i := 0; i < cells; i++ {
			tl := grid[i][j+1]     // 左上 (i,j+1)
			tr := grid[i+1][j+1]   // 右上
			br := grid[i+1][j]     // 右下
			bl := grid[i][j]       // 左下
			bits := 0
			if tl {
				bits |= 1
			}
			if tr {
				bits |= 2
			}
			if br {
				bits |= 4
			}
			if bl {
				bits |= 8
			}
			if bits == 0 {
				continue
			}
			// 只要四角之一在并集内就计入该格（全内格 15 同样计入）。
			hitCells++
			top := func() Point { return hNode(i, j+1) }
			bot := func() Point { return hNode(i, j) }
			lft := func() Point { return vNode(i, j) }
			rgt := func() Point { return vNode(i+1, j) }
			switch bits {
			case 1, 14:
				emit(top(), lft())
			case 2, 13:
				emit(top(), rgt())
			case 3, 12:
				emit(lft(), rgt())
			case 4, 11:
				emit(bot(), rgt())
			case 5:
				emit(top(), rgt())
				emit(bot(), lft())
			case 6, 9:
				emit(top(), bot())
			case 7, 8:
				emit(bot(), lft())
			case 10:
				emit(top(), lft())
				emit(bot(), rgt())
			}
		}
	}

	// 缝合：无向邻接表（每个格边节点至多连接两个方向），从任意未访问端走线成环。
	adj := map[string][]string{}
	addAdj := func(a, b Point) {
		ka, kb := nodeKey(a), nodeKey(b)
		adj[ka] = append(adj[ka], kb)
		adj[kb] = append(adj[kb], ka)
	}
	for _, s := range segs {
		addAdj(s.a, s.b)
	}
	used := map[[2]string]bool{}
	var rings [][]Point
	for start := range adj {
		// 找一条未使用的边
		var first string
		for _, nb := range adj[start] {
			k := [2]string{start, nb}
			if !used[k] {
				first = nb
				break
			}
		}
		if first == "" {
			continue
		}
		var ring []Point
		prev, cur := start, first
		for {
			used[[2]string{prev, cur}] = true
			used[[2]string{cur, prev}] = true
			ring = append(ring, edgeNode[cur])
			nxt := ""
			for _, nb := range adj[cur] {
				if nb == prev {
					continue
				}
				if !used[[2]string{cur, nb}] {
					nxt = nb
					break
				}
			}
			if nxt == "" {
				break
			}
			prev, cur = cur, nxt
			if cur == start {
				break
			}
		}
		ring = cleanRing(ring)
		if len(ring) >= 3 {
			rings = append(rings, normalizeCCW(ring))
		}
	}
	sort.Slice(rings, func(i, j int) bool {
		return SignedArea(rings[i]) > SignedArea(rings[j])
	})
	vc := 0
	for _, r := range rings {
		vc += len(r)
	}
	return &ReferenceResult{
		Rings:       rings,
		VertexCount: vc,
		GridArea:    float64(hitCells) * hx * hy,
		GridStep:    math.Max(hx, hy),
		Bounds:      [2]Point{{minx, miny}, {maxx, maxy}},
	}, nil
}

// RingArea 返回（简单）环的绝对面积。
func RingArea(ring []Point) float64 {
	return math.Abs(SignedArea(ring))
}

// TotalArea 返回多个环的面积之和。
func TotalArea(rings [][]Point) float64 {
	s := 0.0
	for _, r := range rings {
		s += RingArea(r)
	}
	return s
}
