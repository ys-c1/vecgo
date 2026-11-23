// Package bvh 为二维三角形集合建立包围盒层次树（BVH）。
//
// 建树规则：每个节点取当前三角形区间，沿节点包围盒的最长轴按三角形质心的
// 中位数一分为二，递归到叶子只含一个三角形。查询时先看节点包围盒是否与
// 查询区域相交，不相交就整棵剪枝；叶子内再做精确判定。查询结果与逐三角形
// 暴力遍历逐项一致。
//
// 本包不修改任何既有渲染路径：raster.Fill 等原有全量遍历接口保持原样。
// BruteForcePoint / BruteForceBox 是冻结的暴力对照接口，供等值核验使用。
package bvh

import (
	"errors"
	"fmt"
	"math"
	"sort"

	"vecgo/geom"
)

// Triangle 是一个三角形。顶点顺序不影响内外判定。
type Triangle struct {
	A, B, C geom.Point
}

// Vertices 返回三个顶点。
func (t Triangle) Vertices() [3]geom.Point { return [3]geom.Point{t.A, t.B, t.C} }

// Bounds 返回三角形的轴对齐包围盒。
func (t Triangle) Bounds() Bounds {
	return Bounds{
		Min: geom.Point{
			X: math.Min(t.A.X, math.Min(t.B.X, t.C.X)),
			Y: math.Min(t.A.Y, math.Min(t.B.Y, t.C.Y)),
		},
		Max: geom.Point{
			X: math.Max(t.A.X, math.Max(t.B.X, t.C.X)),
			Y: math.Max(t.A.Y, math.Max(t.B.Y, t.C.Y)),
		},
	}
}

// Contains 判断点是否在三角形内。语义与 geom.Inside 相同，暴力对照与
// 剪枝查询调用同一个函数，因此结果不会因判定实现不同而出现差异。
func (t Triangle) Contains(p geom.Point) bool {
	poly := [3]geom.Point{t.A, t.B, t.C}
	return geom.Inside(poly[:], p.X, p.Y)
}

// FromPolygons 把顶点切片转成三角形列表，供既有管线（其场景是
// [][]geom.Point）接入本包。每个多边形必须恰好三个顶点，否则报错并
// 指出第一个不合规的下标。返回空切片时由 Build 报「三角形为空」。
func FromPolygons(polys [][]geom.Point) ([]Triangle, error) {
	out := make([]Triangle, len(polys))
	for i, p := range polys {
		if len(p) != 3 {
			return nil, fmt.Errorf("多边形 #%d 有 %d 个顶点，三角形必须恰好 3 个", i, len(p))
		}
		out[i] = Triangle{A: p[0], B: p[1], C: p[2]}
	}
	return out, nil
}

// Bounds 是轴对齐包围盒。
type Bounds struct {
	Min, Max geom.Point
}

// Width 返回 x 方向跨度。
func (b Bounds) Width() float64 { return b.Max.X - b.Min.X }

// Height 返回 y 方向跨度。
func (b Bounds) Height() float64 { return b.Max.Y - b.Min.Y }

// Degenerate 报告包围盒是否退化：宽或高不大于 0（含 NaN）。
// 退化包围盒无法继续按最长轴二分，建树时按输入错误处理。
func (b Bounds) Degenerate() bool { return !(b.Width() > 0 && b.Height() > 0) }

// ContainsPoint 报告点是否落在包围盒内（边界算内）。
func (b Bounds) ContainsPoint(p geom.Point) bool {
	return p.X >= b.Min.X && p.X <= b.Max.X && p.Y >= b.Min.Y && p.Y <= b.Max.Y
}

// Intersects 报告两个包围盒是否相交（贴边、贴角算相交）。
func (b Bounds) Intersects(o Bounds) bool {
	return b.Min.X <= o.Max.X && o.Min.X <= b.Max.X &&
		b.Min.Y <= o.Max.Y && o.Min.Y <= b.Max.Y
}

func unionBounds(a, b Bounds) Bounds {
	return Bounds{
		Min: geom.Point{X: math.Min(a.Min.X, b.Min.X), Y: math.Min(a.Min.Y, b.Min.Y)},
		Max: geom.Point{X: math.Max(a.Max.X, b.Max.X), Y: math.Max(a.Max.Y, b.Max.Y)},
	}
}

// 切分轴。
const (
	AxisX    = 0
	AxisY    = 1
	AxisNone = -1
)

// Node 是层次树中的一个节点。节点号即其在 Tree.Nodes 中的下标，
// 根固定为 0，按先序分配。
type Node struct {
	Bounds Bounds
	Left   int // 左子节点号；叶子为 -1
	Right  int // 右子节点号；叶子为 -1
	Start  int // 叶子在 Tree.Order 中的起始位置
	Count  int // 本节点覆盖的三角形数
	Depth  int // 深度，根为 1
	Axis   int // 内部节点的切分轴；叶子为 AxisNone
}

// Leaf 报告节点是否为叶子。
func (n Node) Leaf() bool { return n.Left < 0 }

// Tree 是一棵建好的包围盒层次树。建好后只读，可并发查询。
type Tree struct {
	Nodes  []Node
	Order  []int      // 三角形下标的重排；叶子区间索引此切片
	Tris   []Triangle // 建树时复制的三角形
	Height int        // 实测树高（最大节点深度，根为 1）
	// MaxDepth 是建树时允许的树高上界。
	MaxDepth int
}

// NodeCount 返回节点总数。
func (t *Tree) NodeCount() int { return len(t.Nodes) }

// LeafCount 返回叶子数。
func (t *Tree) LeafCount() int {
	n := 0
	for i := range t.Nodes {
		if t.Nodes[i].Leaf() {
			n++
		}
	}
	return n
}

// Bounds 返回根节点包围盒。
func (t *Tree) Bounds() Bounds {
	if t == nil || len(t.Nodes) == 0 {
		return Bounds{}
	}
	return t.Nodes[0].Bounds
}

// Options 是建树参数。
type Options struct {
	// MaxDepth 是树高上界（根为第 1 层）。0 表示按三角形数自动反解，
	// 即 ExpectedHeight(n)。负数视为深度越界，建树直接报错。
	MaxDepth int
}

// ExpectedHeight 由三角形数 n 反解期望树高：中位数二分得到平衡二叉树，
// 叶子数 n，树高为 ceil(log2(n))+1（根为第 1 层）。
func ExpectedHeight(n int) int {
	if n <= 1 {
		return 1
	}
	h := 1
	for capacity := 1; capacity < n; capacity <<= 1 {
		h++
	}
	return h
}

// 建树错误类别。均可用 errors.Is 判断。
var (
	ErrEmptyTriangles   = errors.New("三角形为空")
	ErrDegenerateBounds = errors.New("包围盒退化")
	ErrDepthExceeded    = errors.New("深度越界")
)

// BuildError 指出建树失败的具体节点。
//
// Node 是出问题的节点号（即 Tree.Nodes 中的下标；根为 0）。深度越界时，
// Node 是越界那一层的节点号——该节点本应被创建，但深度已超过上界。
type BuildError struct {
	Kind     error // ErrEmptyTriangles / ErrDegenerateBounds / ErrDepthExceeded
	Node     int   // 出问题的节点号
	Triangle int   // 相关三角形下标；不适用时为 -1
	Depth    int   // 实际深度；仅深度越界有效
	MaxDepth int   // 允许上界；仅深度越界有效
	Bounds   Bounds
}

func (e *BuildError) Error() string {
	switch e.Kind {
	case ErrEmptyTriangles:
		return fmt.Sprintf("节点 %d: %v（没有可建树的三角形）", e.Node, e.Kind)
	case ErrDegenerateBounds:
		detail := ""
		if e.Triangle >= 0 {
			detail = fmt.Sprintf("，三角形 #%d", e.Triangle)
		}
		return fmt.Sprintf("节点 %d: %v（%smin=(%g,%g) max=(%g,%g)，宽高都必须大于 0）",
			e.Node, e.Kind, detail, e.Bounds.Min.X, e.Bounds.Min.Y, e.Bounds.Max.X, e.Bounds.Max.Y)
	case ErrDepthExceeded:
		return fmt.Sprintf("节点 %d: %v（深度 %d 超过上界 %d）",
			e.Node, e.Kind, e.Depth, e.MaxDepth)
	default:
		return fmt.Sprintf("节点 %d: %v", e.Node, e.Kind)
	}
}

// Unwrap 让 errors.Is(err, ErrXxx) 可用。
func (e *BuildError) Unwrap() error { return e.Kind }

// NodeIndex 返回出问题的节点号。
func (e *BuildError) NodeIndex() int { return e.Node }

// Build 按最长轴中位数分裂递归建立 BVH。
func Build(tris []Triangle, opts Options) (*Tree, error) {
	if len(tris) == 0 {
		return nil, &BuildError{Kind: ErrEmptyTriangles, Node: 0, Triangle: -1}
	}
	maxDepth := opts.MaxDepth
	if maxDepth == 0 {
		maxDepth = ExpectedHeight(len(tris))
	}
	if maxDepth < 0 {
		return nil, &BuildError{Kind: ErrDepthExceeded, Node: 0, Triangle: -1, Depth: 1, MaxDepth: maxDepth}
	}

	b := &builder{
		tris:     append([]Triangle(nil), tris...),
		order:    make([]int, len(tris)),
		maxDepth: maxDepth,
		centX:    make([]float64, len(tris)),
		centY:    make([]float64, len(tris)),
	}
	for i := range b.order {
		b.order[i] = i
		t := b.tris[i]
		b.centX[i] = (t.A.X + t.B.X + t.C.X) / 3
		b.centY[i] = (t.A.Y + t.B.Y + t.C.Y) / 3
	}

	root, err := b.build(0, len(b.order), 1)
	if err != nil {
		return nil, err
	}
	if root != 0 {
		return nil, fmt.Errorf("内部错误：根节点号为 %d", root)
	}
	tree := &Tree{
		Nodes:    b.nodes,
		Order:    b.order,
		Tris:     b.tris,
		Height:   b.height,
		MaxDepth: maxDepth,
	}
	return tree, nil
}

type builder struct {
	tris     []Triangle
	order    []int
	nodes    []Node
	centX    []float64
	centY    []float64
	maxDepth int
	height   int
}

func (b *builder) build(lo, hi, depth int) (int, error) {
	nodeID := len(b.nodes)
	if depth > b.maxDepth {
		return -1, &BuildError{
			Kind: ErrDepthExceeded, Node: nodeID, Triangle: -1,
			Depth: depth, MaxDepth: b.maxDepth,
		}
	}
	if lo >= hi {
		return -1, &BuildError{Kind: ErrEmptyTriangles, Node: nodeID, Triangle: -1}
	}

	bounds := b.tris[b.order[lo]].Bounds()
	for k := lo + 1; k < hi; k++ {
		bounds = unionBounds(bounds, b.tris[b.order[k]].Bounds())
	}
	if bounds.Degenerate() {
		culprit := -1
		for k := lo; k < hi; k++ {
			if b.tris[b.order[k]].Bounds().Degenerate() {
				culprit = b.order[k]
				break
			}
		}
		return -1, &BuildError{
			Kind: ErrDegenerateBounds, Node: nodeID, Triangle: culprit, Bounds: bounds,
		}
	}

	axis := AxisX
	if bounds.Height() > bounds.Width() {
		axis = AxisY
	}
	leaf := hi-lo == 1
	node := Node{
		Bounds: bounds,
		Left:   -1,
		Right:  -1,
		Start:  lo,
		Count:  hi - lo,
		Depth:  depth,
		Axis:   AxisNone,
	}
	if !leaf {
		node.Axis = axis
	}
	b.nodes = append(b.nodes, node)
	if depth > b.height {
		b.height = depth
	}
	if leaf {
		return nodeID, nil
	}

	// 沿最长轴按质心中位数重排当前区间；质心相同用三角形下标决胜，
	// 保证同一输入每次建出完全相同的树。
	if axis == AxisX {
		sort.Slice(b.order[lo:hi], func(i, j int) bool {
			a, c := b.order[lo+i], b.order[lo+j]
			if b.centX[a] != b.centX[c] {
				return b.centX[a] < b.centX[c]
			}
			return a < c
		})
	} else {
		sort.Slice(b.order[lo:hi], func(i, j int) bool {
			a, c := b.order[lo+i], b.order[lo+j]
			if b.centY[a] != b.centY[c] {
				return b.centY[a] < b.centY[c]
			}
			return a < c
		})
	}
	mid := lo + (hi-lo)/2
	left, err := b.build(lo, mid, depth+1)
	if err != nil {
		return -1, err
	}
	right, err := b.build(mid, hi, depth+1)
	if err != nil {
		return -1, err
	}
	// 递归可能扩容 nodes，必须按下标重新取址。
	b.nodes[nodeID].Left = left
	b.nodes[nodeID].Right = right
	return nodeID, nil
}

// Stats 记录一次查询的工作量。
type Stats struct {
	VisitedNodes int // 访问过的节点数（根计入，被剪枝的节点也计入）
	PrunedNodes  int // 因包围盒不相交而整棵剪枝的节点数
	Tested       int // 叶子内实际做精确判定的三角形数
	Matched      int // 命中数
}

// QueryPoint 返回包含点 p 的三角形下标，按升序排列。
func (t *Tree) QueryPoint(p geom.Point, st *Stats) []int {
	if st != nil {
		*st = Stats{}
	}
	out := make([]int, 0, 8)
	if t == nil || len(t.Nodes) == 0 {
		return out
	}
	t.queryPoint(0, p, st, &out)
	sort.Ints(out)
	if st != nil {
		st.Matched = len(out)
	}
	return out
}

func (t *Tree) queryPoint(nodeID int, p geom.Point, st *Stats, out *[]int) {
	n := &t.Nodes[nodeID]
	if st != nil {
		st.VisitedNodes++
	}
	if !n.Bounds.ContainsPoint(p) {
		if st != nil {
			st.PrunedNodes++
		}
		return
	}
	if n.Leaf() {
		for k := n.Start; k < n.Start+n.Count; k++ {
			ti := t.Order[k]
			if st != nil {
				st.Tested++
			}
			if t.Tris[ti].Contains(p) {
				*out = append(*out, ti)
			}
		}
		return
	}
	t.queryPoint(n.Left, p, st, out)
	t.queryPoint(n.Right, p, st, out)
}

// QueryBox 返回与查询矩形实际重叠的三角形下标，按升序排列。
// 节点层用包围盒剪枝，叶子内用三角形-矩形分离轴判定。
func (t *Tree) QueryBox(q Bounds, st *Stats) []int {
	if st != nil {
		*st = Stats{}
	}
	out := make([]int, 0, 8)
	if t == nil || len(t.Nodes) == 0 {
		return out
	}
	t.queryBox(0, q, st, &out)
	sort.Ints(out)
	if st != nil {
		st.Matched = len(out)
	}
	return out
}

func (t *Tree) queryBox(nodeID int, q Bounds, st *Stats, out *[]int) {
	n := &t.Nodes[nodeID]
	if st != nil {
		st.VisitedNodes++
	}
	if !n.Bounds.Intersects(q) {
		if st != nil {
			st.PrunedNodes++
		}
		return
	}
	if n.Leaf() {
		for k := n.Start; k < n.Start+n.Count; k++ {
			ti := t.Order[k]
			if st != nil {
				st.Tested++
			}
			if triangleIntersectsBox(t.Tris[ti], q) {
				*out = append(*out, ti)
			}
		}
		return
	}
	t.queryBox(n.Left, q, st, out)
	t.queryBox(n.Right, q, st, out)
}

// triangleIntersectsBox 用分离轴定理判断三角形与轴对齐矩形是否相交，
// 贴边、贴角算相交。
func triangleIntersectsBox(t Triangle, q Bounds) bool {
	if !t.Bounds().Intersects(q) {
		return false
	}
	pts := t.Vertices()
	corners := [4]geom.Point{
		{X: q.Min.X, Y: q.Min.Y},
		{X: q.Max.X, Y: q.Min.Y},
		{X: q.Max.X, Y: q.Max.Y},
		{X: q.Min.X, Y: q.Max.Y},
	}
	// 包围盒已排除了 x、y 两轴，只剩三条边法线需要检查。
	for i := 0; i < 3; i++ {
		p, r := pts[i], pts[(i+1)%3]
		nx, ny := -(r.Y - p.Y), r.X-p.X
		if nx == 0 && ny == 0 {
			continue
		}
		tmin, tmax := math.Inf(1), math.Inf(-1)
		for _, v := range pts {
			d := nx*v.X + ny*v.Y
			tmin = math.Min(tmin, d)
			tmax = math.Max(tmax, d)
		}
		bmin, bmax := math.Inf(1), math.Inf(-1)
		for _, v := range corners {
			d := nx*v.X + ny*v.Y
			bmin = math.Min(bmin, d)
			bmax = math.Max(bmax, d)
		}
		if tmax < bmin || bmax < tmin {
			return false
		}
	}
	return true
}

// BruteForcePoint 是点查询的暴力对照接口：逐个三角形做内外判定，
// 返回升序下标。该接口的结果作为等值基准，保持不变。
func BruteForcePoint(tris []Triangle, p geom.Point) []int {
	out := make([]int, 0)
	for i := range tris {
		if tris[i].Contains(p) {
			out = append(out, i)
		}
	}
	return out
}

// BruteForceBox 是矩形查询的暴力对照接口：逐个三角形做三角形-矩形
// 精确相交判定，返回升序下标。该接口的结果作为等值基准，保持不变。
func BruteForceBox(tris []Triangle, q Bounds) []int {
	out := make([]int, 0)
	for i := range tris {
		if triangleIntersectsBox(tris[i], q) {
			out = append(out, i)
		}
	}
	return out
}
