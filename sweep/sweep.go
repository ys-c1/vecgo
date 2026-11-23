// Package sweep 实现带稳定全序的 Bentley–Ottmann 扫描线线段求交。
//
// 全序设计（任意输入下事件顺序唯一，不依赖 map 迭代或随机数）：
//
//	事件排序键依次为 (x, y, kind, slope, a, b)
//	  1. x：事件点 x 坐标，浮点 < 直接比较（输入已拒绝 NaN）；
//	  2. y：事件点 y 坐标；
//	  3. kind：insert=0 < intersect=1 < delete=2，
//	     即同一点先插入、再处理交点、最后删除，保证共端点场景先建立邻居关系；
//	  4. slope：insert/delete 取该段斜率（竖直线统一定义为 +Inf），
//	     intersect 取低段号一方 a 的斜率；
//	  5. a：段号（insert/delete 即该段；intersect 为较小段号）；
//	  6. b：intersect 事件的另一段号（a<b）；insert/delete 固定为 -1。
//
// 二级比较规则：当坐标与斜率全部相同但段不同（例如同一点出发的两条等斜率
// 共线线段，或同一点上的两个交点事件）时，先比段号 a；段号仍相同（只可能
// 是同一对段的交点事件）再比 b。段号是输入下标，全局唯一，因此键不可能冲突。
//
// 活动段状态树（确定性 treap）采用“扫描线右侧”全序：
//
//	y_i(x) < y_j(x)；y 相等时比斜率；斜率仍相等时比段号。
//
// 交点事件按同点批处理：把经过该点的连续段块整体摘除、块内全配对、
// 再按右侧次序整体插回，树的 BST 不变量在任何时刻都成立。
// treap 优先级由段号经固定 splitmix64 派生，树结构本身也逐位确定。
package sweep

import (
	"container/heap"
	"encoding/binary"
	"fmt"
	"math"

	"vecgo/geom"
)

// Segment 是一条带输入段号的线段，段号即它在传入切片中的下标。
type Segment struct {
	P0, P1 geom.Point
}

// Intersection 是交点表中的一行，SegA < SegB。
type Intersection struct {
	X, Y       float64
	SegA, SegB int
}

// Stats 记录一次扫描的计数器，用于实测插入/删除复杂度。
type Stats struct {
	Segments         int
	InsertEvents     int
	DeleteEvents     int
	IntersectEvents  int
	ScheduledEvents  int
	IntersectionRows int
	MaxStatus        int
	// 端点插入/删除在 treap 上走过的路径长度（比较/旋转次数）。
	InsertDepthSum int64
	InsertDepthMax int
	DeleteDepthSum int64
	DeleteDepthMax int
	InsertNanos    int64
	DeleteNanos    int64
	// 交点批处理：批次数、块摘除/重建的累计路径长度、竖直线扫描次数。
	SwapBatches      int
	SwapRebuildDepth int64
	VerticalScans    int
}

const (
	kindInsert    = 0
	kindIntersect = 1
	kindDelete    = 2
)

type event struct {
	x, y  float64
	kind  uint8
	slope float64
	a, b  int // b == -1 表示端点事件
}

type eventHeap []event

func (h eventHeap) Len() int           { return len(h) }
func (h eventHeap) Less(i, j int) bool { return eventLess(h[i], h[j]) }
func (h eventHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *eventHeap) Push(v any)        { *h = append(*h, v.(event)) }
func (h *eventHeap) Pop() any {
	old := *h
	n := len(old)
	e := old[n-1]
	*h = old[:n-1]
	return e
}

// eventLess 是稳定全序的唯一入口，任何排序都走这一份比较逻辑。
func eventLess(p, q event) bool {
	if p.x != q.x {
		return p.x < q.x
	}
	if p.y != q.y {
		return p.y < q.y
	}
	if p.kind != q.kind {
		return p.kind < q.kind
	}
	if p.slope != q.slope {
		return p.slope < q.slope
	}
	if p.a != q.a {
		return p.a < q.a
	}
	return p.b < q.b
}

type segStore struct {
	p0, p1   geom.Point // 规范化后：p0 为左端（x 相同取低 y）
	slope    float64
	vertical bool
}

type scheduledKey struct {
	a, b         int
	xbits, ybits uint64
}

type sweeper struct {
	segs      []segStore
	active    []bool
	tree      *treap
	events    eventHeap
	scheduled map[scheduledKey]bool
	recorded  map[scheduledKey]bool
	curX      float64
	rows      []Intersection
	st        Stats
}

// FindIntersectionsChecked 与 FindIntersections 行为相同，但在每个事件处理后
// 校验 treap 结构不变量，供测试使用；发现破坏立即返回错误。
func FindIntersectionsChecked(in []Segment) ([]Intersection, *Stats, error) {
	rows, st, err := findIntersections(in, true)
	return rows, st, err
}

// FindIntersections 运行扫描线求交，返回确定性排序的交点表与计数器。
// 交点数值公式固定，排序改造不改变任何数值结果；同一输入多次调用得到
// 逐字节相同的交点表。
func FindIntersections(in []Segment) ([]Intersection, *Stats, error) {
	return findIntersections(in, false)
}

func findIntersections(in []Segment, check bool) ([]Intersection, *Stats, error) {
	if err := validate(in); err != nil {
		return nil, nil, err
	}
	s := &sweeper{
		segs:      make([]segStore, len(in)),
		active:    make([]bool, len(in)),
		tree:      &treap{},
		scheduled: make(map[scheduledKey]bool),
		recorded:  make(map[scheduledKey]bool),
	}
	for i, g := range in {
		p0, p1 := g.P0, g.P1
		// 规范化方向：先按 x，再按 y，保证 p0 是扫描时先遇到的端点。
		if p0.X > p1.X || (p0.X == p1.X && p0.Y > p1.Y) {
			p0, p1 = p1, p0
		}
		dx, dy := p1.X-p0.X, p1.Y-p0.Y
		st := segStore{p0: p0, p1: p1, slope: math.Inf(1), vertical: dx == 0}
		if dx != 0 {
			st.slope = dy / dx
			if st.slope == 0 {
				st.slope = 0 // 归一化 -0
			}
		}
		s.segs[i] = st
	}
	s.events = make(eventHeap, 0, len(in)*2)
	for i := range s.segs {
		g := s.segs[i]
		heap.Push(&s.events, event{x: g.p0.X, y: g.p0.Y, kind: kindInsert, slope: g.slope, a: i, b: -1})
		heap.Push(&s.events, event{x: g.p1.X, y: g.p1.Y, kind: kindDelete, slope: g.slope, a: i, b: -1})
	}
	s.st.Segments = len(in)
	heap.Init(&s.events)

	for s.events.Len() > 0 {
		ev := heap.Pop(&s.events).(event)
		s.curX = ev.x
		switch ev.kind {
		case kindInsert:
			s.handleInsert(ev.a)
		case kindDelete:
			s.handleDelete(ev.a)
		case kindIntersect:
			s.handleIntersect(ev)
		}
		if check {
			if err := s.tree.checkInvariants(); err != nil {
				return nil, nil, fmt.Errorf("事件 %+v 后 treap 不变量破坏: %w", ev, err)
			}
		}
	}
	sortRows(s.rows)
	s.st.IntersectionRows = len(s.rows)
	return s.rows, &s.st, nil
}

func validate(in []Segment) error {
	// 顺序固定：先 NaN（NaN 参与任何比较都不可靠），再零长，再重复端点对。
	for i, g := range in {
		if math.IsNaN(g.P0.X) || math.IsNaN(g.P0.Y) || math.IsNaN(g.P1.X) || math.IsNaN(g.P1.Y) {
			return fmt.Errorf("段 %d 坐标含 NaN", i)
		}
	}
	for i, g := range in {
		if g.P0.X == g.P1.X && g.P0.Y == g.P1.Y {
			return fmt.Errorf("段 %d 为零长段（两端点重合于 %v,%v）", i, g.P0.X, g.P0.Y)
		}
	}
	type pairKey struct{ x0, y0, x1, y1 uint64 }
	seen := make(map[pairKey]int, len(in))
	for i, g := range in {
		p0, p1 := g.P0, g.P1
		if p0.X > p1.X || (p0.X == p1.X && p0.Y > p1.Y) {
			p0, p1 = p1, p0
		}
		k := pairKey{
			math.Float64bits(p0.X), math.Float64bits(p0.Y),
			math.Float64bits(p1.X), math.Float64bits(p1.Y),
		}
		// 端点重复：两个不同段拥有完全相同的端点对（整段重复）。
		// 仅共享单个端点属于“共端点”，是合法输入（见 TestSharedEndpoints）。
		if first, ok := seen[k]; ok {
			return fmt.Errorf("段 %d 与段 %d 端点重复（端点对完全相同）", first, i)
		}
		seen[k] = i
	}
	return nil
}

// yAt 返回段在扫描位置 x 处的 y。竖直线在其自身 x 上返回下端点 y：
// 它在该 x 的次序由批处理/显式扫描单独处理，绝不产生 Inf*0=NaN。
func (s *sweeper) yAt(i int, x float64) float64 {
	g := s.segs[i]
	if g.vertical {
		return g.p0.Y
	}
	return g.p0.Y + g.slope*(x-g.p0.X)
}

// lessAt 是活动段在扫描位置 x 处的“扫描线右侧”全序比较。
//
// 关键约定：交点事件发生时两条段在 x 处的 y 按位比较可能仍相差 1 ulp
// （斜率式与参数式两条算术路径的舍入差），因此 y 差落在 onPoint 容差内
// 一律视为“同 y”，此时右侧次序由斜率决定——斜率小的段在 x+ε 处 y 更低、
// 排在前面；斜率也相同（共线）时再用段号兜底。若不用容差而直接比 yAt，
// 交叉后的块重建会保持旧次序、永不翻转，导致交点丢失。
func (s *sweeper) lessAt(i, j int, x float64) bool {
	yi, yj := s.yAt(i, x), s.yAt(j, x)
	if !equalWithinTol(yi, yj) {
		return yi < yj
	}
	si, sj := s.segs[i].slope, s.segs[j].slope
	if si != sj {
		return si < sj
	}
	return i < j
}

// equalWithinTol 与 onPoint 共用同一套量级缩放容差。
func equalWithinTol(a, b float64) bool {
	scale := 1.0
	for _, v := range []float64{a, b} {
		if m := math.Abs(v); m > scale {
			scale = m
		}
	}
	return math.Abs(a-b) <= 1e-12*scale
}

func (s *sweeper) cmp(a, b int) bool { return s.lessAt(a, b, s.curX) }

// onPoint 判定段 id 在扫描位置 x 的实际 y 是否与给定 y 重合。
// 不能做位比较：事件点 y 来自参数式 p0+t*r，而 yAt 走斜率式
// p0+slope*(x-p0)，两条算术路径在舍入后可能相差 1 ulp，位比较会把
// 同一交点上的段拆成不同的块、从而丢事件。容差沿用仓库 geom 包 1e-12
// 的既有风格，并按坐标量级缩放；块内每个段对仍会用 intersectPoints
// 重算并按位匹配，容差只决定块的成员，不会产生假交点行。
func (s *sweeper) onPoint(id int, x, y float64) bool {
	got := s.yAt(id, x)
	scale := 1.0
	for _, v := range []float64{got, y} {
		if a := math.Abs(v); a > scale {
			scale = a
		}
	}
	return math.Abs(got-y) <= 1e-12*scale
}

func (s *sweeper) handleInsert(id int) {
	s.st.InsertEvents++
	start := now()
	depth := s.tree.insert(id, s.cmp)
	s.st.InsertNanos += since(start)
	s.st.InsertDepthSum += int64(depth)
	if depth > s.st.InsertDepthMax {
		s.st.InsertDepthMax = depth
	}
	s.active[id] = true
	if s.tree.size > s.st.MaxStatus {
		s.st.MaxStatus = s.tree.size
	}
	pred, succ := s.tree.neighbors(id)
	s.trySchedule(pred, id)
	s.trySchedule(id, succ)
	if s.segs[id].vertical {
		// 竖直线与所有当前活动段显式配对（含竖直线之间的共线重叠与共端点）。
		s.scanVertical(id)
	} else {
		// 共端点（一）：与所有经过该插入点的非竖直活动段配对，
		// 多段共点时彼此未必相邻；不交叉的对在 intersectPoints 返回空。
		for _, u := range s.throughPoint(id) {
			if u == id || s.segs[u].vertical {
				continue
			}
			s.schedulePair(id, u)
		}
		// 共端点（二）：竖直段的 yAt 是下端点，throughPoint 按 y 扩展
		// 抓不到它们，这里与树中全部竖直活动段显式配对，去重交给 scheduled。
		for _, u := range s.tree.inOrder() {
			if u != id && s.segs[u].vertical {
				s.schedulePair(id, u)
			}
		}
	}
}

// throughPoint 返回当前树中与段 id 经过同一事件点 (curX, 端点 y) 的
// 全部活动段，按树序（确定性顺序，不使用 map 迭代）返回，含 id 自身。
func (s *sweeper) throughPoint(id int) []int {
	n := s.tree.nodes[id]
	if n == nil {
		return nil
	}
	y := s.yAt(id, s.curX)
	left := n
	for {
		p, ok := s.tree.predOf(left)
		if !ok || !s.onPoint(p.id, s.curX, y) {
			break
		}
		left = p
	}
	right := n
	for {
		sc, ok := s.tree.succOf(right)
		if !ok || !s.onPoint(sc.id, s.curX, y) {
			break
		}
		right = sc
	}
	var out []int
	for cur := left; ; {
		out = append(out, cur.id)
		if cur == right {
			break
		}
		cur = s.tree.succOfMust(cur)
	}
	return out
}

func (s *sweeper) handleDelete(id int) {
	s.st.DeleteEvents++
	pred, succ := s.tree.neighbors(id)
	if s.segs[id].vertical {
		// 第二次扫描：兜住在本 x 才插入、没赶上插入扫描的段。
		// 此刻 id 仍在树中（摘除在扫描之后），共端点对可以正常配对。
		s.scanVertical(id)
	}
	start := now()
	depth := s.tree.remove(id)
	s.st.DeleteNanos += since(start)
	s.st.DeleteDepthSum += int64(depth)
	if depth > s.st.DeleteDepthMax {
		s.st.DeleteDepthMax = depth
	}
	s.active[id] = false
	s.trySchedule(pred, succ)
}

// scanVertical 让竖直线 v 与当前全部活动段求交并入队，重复由 scheduled 去重。
// 这也是竖直段与其他段共享端点的唯一配对路径：非竖直段的 throughPoint
// 按 y 值扩展，无法把竖直线纳入块。
func (s *sweeper) scanVertical(v int) {
	s.st.VerticalScans++
	for _, u := range s.tree.inOrder() {
		if u == v {
			continue
		}
		s.schedulePair(v, u)
	}
}

func (s *sweeper) handleIntersect(first event) {
	x, y := first.x, first.y
	xb, yb := math.Float64bits(x), math.Float64bits(y)
	if !s.active[first.a] || !s.active[first.b] {
		// 陈旧事件：某段已删除。同点的后续事件由各自的批处理单独判断。
		return
	}
	// 1) 抽干同一事件点上的全部交点事件（堆按稳定全序，它们必然相邻）。
	batch := []event{first}
	for s.events.Len() > 0 {
		next := s.events[0]
		if next.kind != kindIntersect || math.Float64bits(next.x) != xb || math.Float64bits(next.y) != yb {
			break
		}
		batch = append(batch, heap.Pop(&s.events).(event))
	}
	s.st.IntersectEvents += len(batch)

	// 2) 竖直配对直接登记行（几何在入队时算过，这里重新核对位模式）。
	//    batch 按堆的稳定全序弹出，顺序确定，不使用 map 迭代。
	for _, e := range batch {
		if s.segs[e.a].vertical || s.segs[e.b].vertical {
			if s.rowPointValid(e.a, e.b, x, y) {
				s.appendRow(e.a, e.b, geom.Point{X: x, Y: y})
			}
		}
	}

	// 3) 非竖直段：从批中第一个仍在树中的段出发，向两侧扩展经过该点的
	//    连续段块，覆盖多线共点时彼此不是邻居的段对。
	seed := -1
	for _, e := range batch {
		for _, id := range []int{e.a, e.b} {
			if !s.segs[id].vertical && s.tree.nodes[id] != nil {
				seed = id
				break
			}
		}
		if seed >= 0 {
			break
		}
	}
	if seed < 0 {
		return
	}
	var block []int
	left := s.tree.nodes[seed]
	for {
		p, ok := s.tree.predOf(left)
		if !ok || !s.onPoint(p.id, s.curX, y) {
			break
		}
		left = p
	}
	right := s.tree.nodes[seed]
	for {
		sc, ok := s.tree.succOf(right)
		if !ok || !s.onPoint(sc.id, s.curX, y) {
			break
		}
		right = sc
	}
	// 块内节点此刻全部在树中，按中序（succ 链）收集，顺序即左侧次序。
	for n := left; ; {
		block = append(block, n.id)
		if n == right {
			break
		}
		n = s.tree.succOfMust(n)
	}

	// 5) 块内全配对：所有经过该点的段对都登记一行（recorded 去重），
	//    共线重叠的另一边界点确保也已入队。
	for i := 0; i < len(block); i++ {
		for j := i + 1; j < len(block); j++ {
			a, b := block[i], block[j]
			pts := intersectPoints(s.segs[a].p0, s.segs[a].p1, s.segs[b].p0, s.segs[b].p1)
			for _, p := range pts {
				// 这里的重算段顺序可能与事件入队时相反，参数式端点顺序不同会
				// 产生 1 ulp 差，因此成员判定用容差；一旦确认是本事件点，
				// 行坐标固定写 (x,y)，数值结果只由入队时的唯一计算决定。
				if equalWithinTol(p.X, x) && equalWithinTol(p.Y, y) {
					s.appendRowAt(a, b, x, y)
				} else {
					s.schedulePoint(a, b, p)
				}
			}
		}
	}

	// 6) 整块摘除，再按扫描线右侧次序插回；指针摘除不依赖比较器。
	//    重建会生成全新节点，且块内次序可能整体翻转，旧 tnode 指针
	//    与“块内端点 id”的左右位置都不可靠，边界邻居一律重建后重新查。
	s.st.SwapBatches++
	for _, id := range block {
		d := s.tree.remove(id)
		s.st.SwapRebuildDepth += int64(d)
	}
	for _, id := range block {
		d := s.tree.insert(id, s.cmp)
		s.st.SwapRebuildDepth += int64(d)
	}

	// 7) 重建后块在中序中仍是连续段。找到块的两个端点：
	//    前驱不在块内者为左端，后继不在块内者为右端；再与块外邻居补调度。
	inBlock := make(map[int]bool, len(block))
	for _, id := range block {
		inBlock[id] = true
	}
	lEnd, rEnd := s.tree.nodes[block[0]], s.tree.nodes[block[0]]
	for _, id := range block {
		n := s.tree.nodes[id]
		if p, ok := s.tree.predOf(n); ok && !inBlock[p.id] {
			lEnd = n
		}
		if sc, ok := s.tree.succOf(n); ok && !inBlock[sc.id] {
			rEnd = n
		}
	}
	if p, ok := s.tree.predOf(lEnd); ok {
		s.trySchedule(p.id, lEnd.id)
	}
	if sc, ok := s.tree.succOf(rEnd); ok {
		s.trySchedule(rEnd.id, sc.id)
	}
}

func (s *sweeper) rowPointValid(a, b int, x, y float64) bool {
	pts := intersectPoints(s.segs[a].p0, s.segs[a].p1, s.segs[b].p0, s.segs[b].p1)
	for _, p := range pts {
		if math.Float64bits(p.X) == math.Float64bits(x) && math.Float64bits(p.Y) == math.Float64bits(y) {
			return true
		}
	}
	return false
}

func (s *sweeper) appendRow(a, b int, p geom.Point) {
	s.appendRowAt(a, b, p.X, p.Y)
}

// appendRowAt 以指定坐标登记交点行。同点多事件、块内全配对会重复命中，
// 由 recorded 按坐标位模式去重。
func (s *sweeper) appendRowAt(a, b int, x, y float64) {
	if a > b {
		a, b = b, a
	}
	key := scheduledKey{a: a, b: b, xbits: math.Float64bits(x), ybits: math.Float64bits(y)}
	// recorded 与 scheduled 分离：scheduled 管事件入队去重，
	// recorded 管交点表行去重。
	if s.recorded[key] {
		return
	}
	s.recorded[key] = true
	s.rows = append(s.rows, Intersection{X: x, Y: y, SegA: a, SegB: b})
}

func (s *sweeper) schedulePoint(a, b int, p geom.Point) {
	if a > b {
		a, b = b, a
	}
	key := scheduledKey{a: a, b: b, xbits: math.Float64bits(p.X), ybits: math.Float64bits(p.Y)}
	if s.scheduled[key] {
		return
	}
	if p.X < s.curX {
		return
	}
	s.scheduled[key] = true
	s.st.ScheduledEvents++
	heap.Push(&s.events, event{
		x: p.X, y: p.Y, kind: kindIntersect,
		slope: s.segs[a].slope, a: a, b: b,
	})
}

// trySchedule 调度一对当前邻居的非竖直段；竖直线配对走 scanVertical。
func (s *sweeper) trySchedule(i, j int) {
	if i < 0 || j < 0 || i == j {
		return
	}
	if s.segs[i].vertical || s.segs[j].vertical {
		return
	}
	s.schedulePair(i, j)
}

func (s *sweeper) schedulePair(i, j int) {
	a, b := i, j
	if a > b {
		a, b = b, a
	}
	gi, gj := s.segs[a], s.segs[b]
	for _, p := range intersectPoints(gi.p0, gi.p1, gj.p0, gj.p1) {
		s.schedulePoint(a, b, p)
	}
}

// intersectPoints 返回两段在各自参数范围内的交点：
// 一般相交返回 1 个点；平行分离返回空；共线重叠返回 1~2 个边界点。
// 数值公式固定：p + t*r，t = ((q-p)×s)/(r×s)，任何调用方结果一致。
func intersectPoints(p0, p1, q0, q1 geom.Point) []geom.Point {
	rx, ry := p1.X-p0.X, p1.Y-p0.Y
	sx, sy := q1.X-q0.X, q1.Y-q0.Y
	qx, qy := q0.X-p0.X, q0.Y-p0.Y
	den := rx*sy - ry*sx
	if den != 0 {
		t := (qx*sy - qy*sx) / den
		u := (qx*ry - qy*rx) / den
		if t < 0 || t > 1 || u < 0 || u > 1 {
			return nil
		}
		return []geom.Point{{X: p0.X + t*rx, Y: p0.Y + t*ry}}
	}
	// 平行：叉积非零即分离，零则共线。
	if qx*ry-qy*rx != 0 {
		return nil
	}
	len2 := rx*rx + ry*ry
	if len2 == 0 {
		return nil // 零长段已在校验阶段拒绝，这里仅作防御
	}
	t0 := (qx*rx + qy*ry) / len2
	t1 := ((q1.X-p0.X)*rx + (q1.Y-p0.Y)*ry) / len2
	lo, hi := t0, t1
	if lo > hi {
		lo, hi = hi, lo
	}
	if lo < 0 {
		lo = 0
	}
	if hi > 1 {
		hi = 1
	}
	if hi < lo {
		return nil
	}
	a := geom.Point{X: p0.X + lo*rx, Y: p0.Y + lo*ry}
	if lo == hi {
		return []geom.Point{a} // 共线且仅单点相接（共端点）
	}
	b := geom.Point{X: p0.X + hi*rx, Y: p0.Y + hi*ry}
	return []geom.Point{a, b}
}

func sortRows(rows []Intersection) {
	// 确定性排序：坐标位模式 + 段号，与事件处理过程解耦。
	for i := 1; i < len(rows); i++ {
		for j := i; j > 0 && rowLess(rows[j], rows[j-1]); j-- {
			rows[j], rows[j-1] = rows[j-1], rows[j]
		}
	}
}

func rowLess(p, q Intersection) bool {
	if bx := math.Float64bits(p.X); bx != math.Float64bits(q.X) {
		return bx < math.Float64bits(q.X)
	}
	if by := math.Float64bits(p.Y); by != math.Float64bits(q.Y) {
		return by < math.Float64bits(q.Y)
	}
	if p.SegA != q.SegA {
		return p.SegA < q.SegA
	}
	return p.SegB < q.SegB
}

// HashTable 对交点表做规范编码后取 SHA-256，输出可跨进程比对的全串十六进制。
func HashTable(rows []Intersection) [32]byte {
	buf := make([]byte, 0, 8+len(rows)*24)
	var hdr [8]byte
	binary.BigEndian.PutUint64(hdr[:], uint64(len(rows)))
	buf = append(buf, hdr[:]...)
	var tmp [8]byte
	for _, r := range rows {
		binary.BigEndian.PutUint64(tmp[:], math.Float64bits(r.X))
		buf = append(buf, tmp[:]...)
		binary.BigEndian.PutUint64(tmp[:], math.Float64bits(r.Y))
		buf = append(buf, tmp[:]...)
		binary.BigEndian.PutUint32(tmp[:4], uint32(r.SegA))
		binary.BigEndian.PutUint32(tmp[4:], uint32(r.SegB))
		buf = append(buf, tmp[:8]...)
	}
	return sha256Sum(buf)
}
