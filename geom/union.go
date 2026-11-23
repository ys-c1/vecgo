package geom

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"time"
)

// 带序号的错误：E1 自交未拆分 / E2 顶点不足三个 / E3 两图形完全重合
var (
	ErrSelfIntersecting = errors.New("E1: 输入为自交多边形，必须先拆分再合并")
	ErrTooFewVertices   = errors.New("E2: 多边形顶点不足三个")
	ErrCoincident       = errors.New("E3: 两个图形完全重合，无需并集运算")
)

const (
	eps       = 1e-9
	keyDigits = 9
)

// UnionResult 是真并集运算的结果：外轮廓环序列、轮廓顶点总数与耗时。
type UnionResult struct {
	Rings       [][]Point
	VertexCount int
	Elapsed     time.Duration
	// SplitLoops 是自交输入拆分后、合并前参与运算的简单环个数。
	SplitLoops int
}

// SignedArea 按鞋带公式计算有向面积：正值表示逆时针（CCW），负值顺时针（CW）。
func SignedArea(ring []Point) float64 {
	n := len(ring)
	if n < 3 {
		return 0
	}
	s := 0.0
	for i := 0; i < n; i++ {
		a, b := ring[i], ring[(i+1)%n]
		s += a.X*b.Y - a.Y*b.X
	}
	return s / 2
}

// Orientation 返回 +1（CCW）、-1（CW）、0（退化）。
func Orientation(ring []Point) int {
	a := SignedArea(ring)
	switch {
	case a > eps:
		return 1
	case a < -eps:
		return -1
	default:
		return 0
	}
}

func orient2(ax, ay, bx, by, cx, cy float64) float64 {
	return (bx-ax)*(cy-ay) - (by-ay)*(cx-ax)
}

func pointsEqual(a, b Point) bool {
	return math.Abs(a.X-b.X) <= eps && math.Abs(a.Y-b.Y) <= eps
}

func nodeKey(p Point) string {
	return fmt.Sprintf("%.*g,%.*g", keyDigits, p.X, keyDigits, p.Y)
}

// cleanRing 去除重复点：既处理连续重复（含首尾闭合），也处理非连续重复
// （后者会形成“去-回”的退化刺边，必须在求交前消除，否则 T 型切分会漏段）。
func cleanRing(ring []Point) []Point {
	out := make([]Point, 0, len(ring))
	seen := map[string]int{}
	for _, p := range ring {
		k := nodeKey(p)
		if idx, dup := seen[k]; dup {
			// 非连续重复：丢弃从第一次出现到当前位置之间的折返段，保留首次出现点。
			_ = idx
			continue
		}
		seen[k] = len(out)
		out = append(out, p)
	}
	for len(out) >= 2 && pointsEqual(out[0], out[len(out)-1]) {
		out = out[:len(out)-1]
	}
	return out
}

func normalizeCCW(ring []Point) []Point {
	if SignedArea(ring) >= 0 {
		return ring
	}
	out := append([]Point{}, ring...)
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

func onSegment(p, a, b Point) bool {
	if math.Abs(orient2(a.X, a.Y, b.X, b.Y, p.X, p.Y)) > eps {
		return false
	}
	return p.X >= math.Min(a.X, b.X)-eps && p.X <= math.Max(a.X, b.X)+eps &&
		p.Y >= math.Min(a.Y, b.Y)-eps && p.Y <= math.Max(a.Y, b.Y)+eps
}

// segmentCross 返回两条线段的全部交点（去重），proper 标记是否为真穿越。
// 共线重叠时返回重叠区间的端点；端点相接返回接触点。
func segmentCross(p1, p2, q1, q2 Point) (pts []Point, proper bool) {
	d1 := orient2(q1.X, q1.Y, q2.X, q2.Y, p1.X, p1.Y)
	d2 := orient2(q1.X, q1.Y, q2.X, q2.Y, p2.X, p2.Y)
	d3 := orient2(p1.X, p1.Y, p2.X, p2.Y, q1.X, q1.Y)
	d4 := orient2(p1.X, p1.Y, p2.X, p2.Y, q2.X, q2.Y)
	if (d1 > eps && d2 < -eps || d1 < -eps && d2 > eps) &&
		(d3 > eps && d4 < -eps || d3 < -eps && d4 > eps) {
		den := (p1.X-p2.X)*(q1.Y-q2.Y) - (p1.Y-p2.Y)*(q1.X-q2.X)
		if math.Abs(den) < eps {
			return nil, false
		}
		t := ((p1.X-q1.X)*(q1.Y-q2.Y) - (p1.Y-q1.Y)*(q1.X-q2.X)) / den
		return []Point{{p1.X + (p2.X-p1.X)*t, p1.Y + (p2.Y-p1.Y)*t}}, true
	}
	// 共线重叠：收集落在另一线段上的端点
	if math.Abs(d1) <= eps && math.Abs(d2) <= eps && math.Abs(d3) <= eps {
		cand := []Point{p1, p2, q1, q2}
		seen := map[string]bool{}
		for _, c := range cand {
			if (onSegment(c, p1, p2) && onSegment(c, q1, q2)) && !seen[nodeKey(c)] {
				seen[nodeKey(c)] = true
				pts = append(pts, c)
			}
		}
		return pts, false
	}
	// 端点落在另一线段上
	touch := func(p, a, b Point) {
		if onSegment(p, a, b) {
			for _, e := range pts {
				if pointsEqual(e, p) {
					return
				}
			}
			pts = append(pts, p)
		}
	}
	touch(p1, q1, q2)
	touch(p2, q1, q2)
	touch(q1, p1, p2)
	touch(q2, p1, p2)
	return pts, false
}

type crossing struct {
	i, j int
	pt   Point
}

// findSelfCrossing 在非相邻边之间寻找第一个自交点。
func findSelfCrossing(ring []Point) (crossing, bool) {
	n := len(ring)
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			if j == i || j == (i+1)%n || i == (j+1)%n {
				continue
			}
			pts, _ := segmentCross(ring[i], ring[(i+1)%n], ring[j], ring[(j+1)%n])
			if len(pts) > 0 {
				return crossing{i: i, j: j, pt: pts[0]}, true
			}
		}
	}
	return crossing{}, false
}

// IsSimple 判定环是否为无自交的简单环。
func IsSimple(ring []Point) bool {
	_, ok := findSelfCrossing(cleanRing(ring))
	return !ok
}

// SplitSelfIntersecting 把自交环在交点处切开，递归拆分为若干简单 CCW 环。
func SplitSelfIntersecting(ring []Point) ([][]Point, error) {
	r := cleanRing(ring)
	if len(r) < 3 {
		return nil, fmt.Errorf("%w: 清理后仍不足三个顶点", ErrTooFewVertices)
	}
	queue := [][]Point{r}
	var out [][]Point
	for steps := 0; len(queue) > 0; steps++ {
		if steps > 64 {
			return nil, errors.New("自交拆分次数超出上限，输入可能异常复杂")
		}
		cur := cleanRing(queue[0])
		queue = queue[1:]
		if len(cur) < 3 {
			continue // 拆分产生的退化碎片直接丢弃
		}
		c, hit := findSelfCrossing(cur)
		if !hit {
			out = append(out, normalizeCCW(cur))
			continue
		}
		n := len(cur)
		// 环1：交点 -> V[i+1] -> ... -> V[j] -> 交点
		loop1 := []Point{c.pt}
		for k := c.i + 1; k <= c.j; k++ {
			loop1 = append(loop1, cur[k])
		}
		loop1 = append(loop1, c.pt)
		// 环2：交点 -> V[j+1] -> ... -> 末尾 -> 开头 -> ... -> V[i] -> 交点
		loop2 := []Point{c.pt}
		for k := c.j + 1; k < n; k++ {
			loop2 = append(loop2, cur[k])
		}
		for k := 0; k <= c.i; k++ {
			loop2 = append(loop2, cur[k])
		}
		loop2 = append(loop2, c.pt)
		queue = append(queue, cleanRing(loop1), cleanRing(loop2))
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%w: 拆分后无有效简单环", ErrTooFewVertices)
	}
	return out, nil
}

// ringsCoincident 判定两个环是否顶点集完全一致（允许起点/绕序不同）。
func ringsCoincident(a, b []Point) bool {
	if len(a) != len(b) {
		return false
	}
	if math.Abs(SignedArea(a)-SignedArea(b)) > eps {
		return false
	}
	for _, p := range a {
		found := false
		for _, q := range b {
			if pointsEqual(p, q) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

type hnode struct {
	p   Point
	out []*halfedge
	in  []*halfedge
}

type halfedge struct {
	from, to *hnode
	ring     int
}

type edgeKey struct{ from, to string }

// MergeSimple 合并两个保证简单（无自交）的环，输出并集外轮廓（CCW）。
// 自交输入直接返回 E1，证明“先拆分再合并”的闸门存在。
func MergeSimple(a, b []Point) ([][]Point, error) {
	a, b = cleanRing(a), cleanRing(b)
	if len(a) < 3 || len(b) < 3 {
		return nil, ErrTooFewVertices
	}
	if !IsSimple(a) || !IsSimple(b) {
		return nil, ErrSelfIntersecting
	}
	a, b = normalizeCCW(a), normalizeCCW(b)
	if ringsCoincident(a, b) {
		// 内部合并阶段遇到重合子环：并集不变，直接保留其一。
		return [][]Point{a}, nil
	}
	return mergeRings([][]Point{a, b})
}

// mergeRings 是核心：按交点切分所有边，再按“左侧是否仍被并集覆盖”
// 筛选轮廓半边，最后在交点处按角度配对重建外轮廓环。
func mergeRings(rings [][]Point) ([][]Point, error) {
	nodes := map[string]*hnode{}
	bucket := map[[2]int][]*hnode{}
	// 同一几何交点由不同边独立计算会有浮点末位差异，按容差桶归并节点，
	// 否则轮廓会在交点处被错误地切成多环。
	tol := 1e-7
	bucketOf := func(p Point) (int, int) {
		return int(math.Floor(p.X / tol)), int(math.Floor(p.Y / tol))
	}
	getNode := func(p Point) *hnode {
		k := nodeKey(p)
		if nd, ok := nodes[k]; ok {
			return nd
		}
		bx, by := bucketOf(p)
		for dx := -1; dx <= 1; dx++ {
			for dy := -1; dy <= 1; dy++ {
				for _, nd := range bucket[[2]int{bx + dx, by + dy}] {
					if pointsEqual(nd.p, p) {
						return nd
					}
				}
			}
		}
		nd := &hnode{p: p}
		nodes[k] = nd
		bucket[[2]int{bx, by}] = append(bucket[[2]int{bx, by}], nd)
		return nd
	}

	// 1. 收集每个原始环每条边的切分参数 t（交点投影到边参数上）。
	type edge struct {
		u, v Point
		ring int
	}
	var edges []edge
	splits := map[int][]float64{}
	for ri, r := range rings {
		for i := 0; i < len(r); i++ {
			idx := len(edges)
			edges = append(edges, edge{u: r[i], v: r[(i+1)%len(r)], ring: ri})
			splits[idx] = []float64{0, 1}
		}
	}
	for x := 0; x < len(edges); x++ {
		for y := x + 1; y < len(edges); y++ {
			if edges[x].ring == edges[y].ring {
				continue // 简单环自身不相交，只需跨环求交
			}
			pts, _ := segmentCross(edges[x].u, edges[x].v, edges[y].u, edges[y].v)
			for _, p := range pts {
				splits[x] = append(splits[x], projectT(edges[x].u, edges[x].v, p))
				splits[y] = append(splits[y], projectT(edges[y].u, edges[y].v, p))
			}
			// T 型接触：一条边的端点落在另一条边内部，必须切开被接触边，
			// 否则轮廓会在接触点处漏段（典型于退化/顶点碰边的输入）。
			addT := func(tip, u, v Point, targetIdx int) {
				if onSegment(tip, u, v) {
					t := projectT(u, v, tip)
					if t > eps && t < 1-eps {
						splits[targetIdx] = append(splits[targetIdx], t)
					}
				}
			}
			addT(edges[x].u, edges[y].u, edges[y].v, y)
			addT(edges[x].v, edges[y].u, edges[y].v, y)
			addT(edges[y].u, edges[x].u, edges[x].v, x)
			addT(edges[y].v, edges[x].u, edges[x].v, x)
		}
	}

	// 2. 按切分点建半边与节点。
	var all []*halfedge
	for idx, e := range edges {
		ts := uniqueSorted(splits[idx])
		for k := 0; k+1 < len(ts); k++ {
			u := pointAt(e.u, e.v, ts[k])
			v := pointAt(e.u, e.v, ts[k+1])
			if pointsEqual(u, v) {
				continue
			}
			nu, nv := getNode(u), getNode(v)
			h := &halfedge{from: nu, to: nv, ring: e.ring}
			nu.out = append(nu.out, h)
			nv.in = append(nv.in, h)
			all = append(all, h)
		}
	}

	// 3. 轮廓判定（CCW 环，内部恒在有向边左侧）：
	//    左侧微偏点必须落在并集内（自身环或其它环），右侧微偏点若也落在
	//    其它环内，则该边两侧都是并集、属于内部边，剔除。
	//    反向共线的重叠边两侧都在并集内，同样剔除（否则重叠区被重复计入）。
	var boundary []*halfedge
	for _, h := range all {
		if hasOppositeCollinear(h, all) {
			continue
		}
		mx, my := (h.from.p.X+h.to.p.X)/2, (h.from.p.Y+h.to.p.Y)/2
		dx, dy := h.to.p.X-h.from.p.X, h.to.p.Y-h.from.p.Y
		l := math.Hypot(dx, dy)
		delta := math.Max(1e-7, l*1e-6)
		lx, ly := mx-dy/l*delta, my+dx/l*delta
		rx, ry := mx+dy/l*delta, my-dx/l*delta
		leftCovered := Inside(rings[h.ring], lx, ly) // 自身环（CCW 左侧即内部）
		rightCoveredByOther := false
		for ri, r := range rings {
			if ri == h.ring {
				continue
			}
			if Inside(r, lx, ly) {
				leftCovered = true
			}
			if Inside(r, rx, ry) {
				rightCoveredByOther = true
			}
		}
		if leftCovered && !rightCoveredByOther {
			boundary = append(boundary, h)
		}
	}
	if len(boundary) == 0 {
		return nil, errors.New("并集轮廓为空")
	}

	// 4. 节点处按极角排序，进入边取其反向方向的顺时针相邻出边（左手法则），
	//    保证重建出的环内部在左侧、绕序为 CCW。
	kept := map[edgeKey]*halfedge{}
	for _, h := range boundary {
		kept[edgeKey{nodeKey(h.from.p), nodeKey(h.to.p)}] = h
	}
	visited := map[edgeKey]bool{}
	var loops [][]Point
	for _, start := range boundary {
		sk := edgeKey{nodeKey(start.from.p), nodeKey(start.to.p)}
		if visited[sk] {
			continue
		}
		var ring []Point
		cur := start
		for guard := 0; guard < 2*len(boundary)+4; guard++ {
			ck := edgeKey{nodeKey(cur.from.p), nodeKey(cur.to.p)}
			if visited[ck] {
				break
			}
			visited[ck] = true
			ring = append(ring, cur.from.p)
			nd := cur.to
			if len(nd.out) == 0 {
				break
			}
			// 入边 a->v 到达节点 v：沿入边反向方向 v->a 的极角，
			// 找逆时针最相邻的轮廓出边（严格大于该角的最小角，回绕取最小）。
			revBeta := math.Atan2(cur.from.p.Y-cur.to.p.Y, cur.from.p.X-cur.to.p.X)
			next := pickCounterClockwise(nd.out, revBeta, kept)
			if next == nil {
				break
			}
			cur = next
			nk := edgeKey{nodeKey(cur.from.p), nodeKey(cur.to.p)}
			if nk == sk {
				visited[nk] = true
				break
			}
		}
		ring = cleanRing(ring)
		if len(ring) >= 3 && SignedArea(ring) > eps {
			loops = append(loops, ring)
		}
	}
	if len(loops) == 0 {
		return nil, errors.New("重建外轮廓失败")
	}
	// 稳定输出顺序：按首顶点坐标排序。
	sort.Slice(loops, func(i, j int) bool {
		return nodeKey(loops[i][0]) < nodeKey(loops[j][0])
	})
	return loops, nil
}

// pickCounterClockwise 实现左手法则：入边以方向角 beta 到达节点后，
// 选择方向角严格大于 beta 的最小角出边（相对入边逆时针最相邻），
// 不存在则回绕到最小角。共线时按目标节点键做确定性决胜。
func pickCounterClockwise(out []*halfedge, beta float64, kept map[edgeKey]*halfedge) *halfedge {
	type cand struct {
		h    *halfedge
		ang  float64
		rank string
	}
	var cs []cand
	for _, h := range out {
		if _, ok := kept[edgeKey{nodeKey(h.from.p), nodeKey(h.to.p)}]; !ok {
			continue
		}
		a := math.Atan2(h.to.p.Y-h.from.p.Y, h.to.p.X-h.from.p.X)
		cs = append(cs, cand{h, a, nodeKey(h.to.p)})
	}
	if len(cs) == 0 {
		return nil
	}
	sort.Slice(cs, func(i, j int) bool {
		if math.Abs(cs[i].ang-cs[j].ang) > eps {
			return cs[i].ang < cs[j].ang
		}
		return cs[i].rank < cs[j].rank
	})
	for i := range cs {
		if cs[i].ang > beta+eps {
			return cs[i].h
		}
	}
	return cs[0].h
}

func projectT(u, v, p Point) float64 {
	dx, dy := v.X-u.X, v.Y-u.Y
	l2 := dx*dx + dy*dy
	if l2 < eps {
		return 0
	}
	t := ((p.X-u.X)*dx + (p.Y-u.Y)*dy) / l2
	if t < eps {
		return 0
	}
	if t > 1-eps {
		return 1
	}
	return t
}

func pointAt(u, v Point, t float64) Point {
	return Point{u.X + (v.X-u.X)*t, u.Y + (v.Y-u.Y)*t}
}

// hasOppositeCollinear 判断 h 是否与另一环的某条半边反向共线且中点重合
// （并集内部的共享边，两侧都在并集内，不能出现在外轮廓上）。
func hasOppositeCollinear(h *halfedge, all []*halfedge) bool {
	mx, my := (h.from.p.X+h.to.p.X)/2, (h.from.p.Y+h.to.p.Y)/2
	dx, dy := h.to.p.X-h.from.p.X, h.to.p.Y-h.from.p.Y
	lh := math.Hypot(dx, dy)
	if lh < eps {
		return false
	}
	for _, g := range all {
		if g.ring == h.ring {
			continue
		}
		ex, ey := g.to.p.X-g.from.p.X, g.to.p.Y-g.from.p.Y
		lg := math.Hypot(ex, ey)
		if lg < eps {
			continue
		}
		cross := dx*ey - dy*ex
		dot := dx*ex + dy*ey
		if math.Abs(cross) > eps*lh*lg { // 必须共线
			continue
		}
		if dot >= 0 { // 必须反向
			continue
		}
		gmx, gmy := (g.from.p.X+g.to.p.X)/2, (g.from.p.Y+g.to.p.Y)/2
		if math.Abs(gmx-mx) <= eps && math.Abs(gmy-my) <= eps {
			return true
		}
	}
	return false
}

func uniqueSorted(ts []float64) []float64 {
	sort.Float64s(ts)
	out := ts[:0]
	for _, t := range ts {
		if len(out) == 0 || math.Abs(t-out[len(out)-1]) > 1e-10 {
			out = append(out, t)
		}
	}
	return out
}

// Union 计算两个多边形的真并集外轮廓。自交输入先拆分为简单环再逐对合并；
// 顶点不足三个返回 E2，两图形完全重合返回 E3。
func Union(a, b []Point) (*UnionResult, error) {
	start := time.Now()
	if len(cleanRing(a)) < 3 || len(a) < 3 {
		return nil, fmt.Errorf("%w: 图形1", ErrTooFewVertices)
	}
	if len(cleanRing(b)) < 3 || len(b) < 3 {
		return nil, fmt.Errorf("%w: 图形2", ErrTooFewVertices)
	}
	ca, cb := cleanRing(a), cleanRing(b)
	if ringsCoincident(ca, cb) {
		return nil, ErrCoincident
	}
	var rings [][]Point
	if IsSimple(ca) {
		rings = append(rings, normalizeCCW(ca))
	} else {
		parts, err := SplitSelfIntersecting(ca)
		if err != nil {
			return nil, err
		}
		rings = append(rings, parts...)
	}
	if IsSimple(cb) {
		rings = append(rings, normalizeCCW(cb))
	} else {
		parts, err := SplitSelfIntersecting(cb)
		if err != nil {
			return nil, err
		}
		rings = append(rings, parts...)
	}

	// 所有简单环一次性做 arrangement：相离环各自成外环，相交/包含环共享切分点，
	// 天然收敛，无需逐对配对。
	queue, err := mergeRings(rings)
	if err != nil {
		return nil, err
	}
	if len(queue) == 0 {
		return nil, errors.New("并集为空")
	}
	vc := 0
	for _, r := range queue {
		vc += len(r)
	}
	return &UnionResult{
		Rings:       queue,
		VertexCount: vc,
		Elapsed:     time.Since(start),
		SplitLoops:  len(rings),
	}, nil
}

// OrientationReport 是绕序方向检查结论；方向反转时定位到具体顶点。
type OrientationReport struct {
	SignedArea    float64
	CCW           bool
	ReversedAt    int // -1 表示方向正确；否则为最顺时针转角顶点的下标
	ReversedPoint Point
	MinCross      float64
}

// InspectOrientation 以有向面积符号为判定依据检查绕序，
// 方向反了时用局部叉积定位最“顺时针”的那个顶点。
func InspectOrientation(ring []Point) OrientationReport {
	rep := OrientationReport{SignedArea: SignedArea(ring), CCW: SignedArea(ring) > eps, ReversedAt: -1}
	if rep.CCW {
		return rep
	}
	minCross := math.Inf(1)
	idx := -1
	n := len(ring)
	for i := 0; i < n; i++ {
		prev, cur, nxt := ring[(i-1+n)%n], ring[i], ring[(i+1)%n]
		c := (cur.X-prev.X)*(nxt.Y-cur.Y) - (cur.Y-prev.Y)*(nxt.X-cur.X)
		if c < minCross {
			minCross, idx = c, i
		}
	}
	rep.ReversedAt, rep.ReversedPoint, rep.MinCross = idx, ring[idx], minCross
	return rep
}
