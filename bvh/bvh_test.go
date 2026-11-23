package bvh

import (
	"errors"
	"fmt"
	"slices"
	"testing"

	"vecgo/geom"
)

// querySet 是一组确定性查询，覆盖随机点、顶点/质心等边界点与多种矩形。
type querySet struct {
	Points []geom.Point
	Boxes  []Bounds
}

func makeQueries(tris []Triangle, seed uint64, n int) querySet {
	r := newRNG(seed)
	b := tris[0].Bounds()
	for _, tri := range tris[1:] {
		b = unionBounds(b, tri.Bounds())
	}
	var qs querySet
	for i := 0; i < n; i++ {
		qs.Points = append(qs.Points, geom.Point{
			X: r.between(b.Min.X-10, b.Max.X+10),
			Y: r.between(b.Min.Y-10, b.Max.Y+10),
		})
	}
	// 边界样本：每个三角形的三个顶点与质心，外加包围盒四角。
	for k := 0; k < len(tris) && k < 40; k++ {
		t := tris[k]
		qs.Points = append(qs.Points, t.A, t.B, t.C)
		qs.Points = append(qs.Points, geom.Point{
			X: (t.A.X + t.B.X + t.C.X) / 3,
			Y: (t.A.Y + t.B.Y + t.C.Y) / 3,
		})
	}
	qs.Points = append(qs.Points,
		b.Min, b.Max,
		geom.Point{X: b.Min.X, Y: b.Max.Y},
		geom.Point{X: b.Max.X, Y: b.Min.Y},
	)
	// 随机矩形：一半小、一半大。
	for i := 0; i < n; i++ {
		w := r.between(0.5, 80)
		h := r.between(0.5, 80)
		if i%2 == 0 {
			w = r.between(80, 400)
			h = r.between(80, 400)
		}
		x := r.between(b.Min.X-50, b.Max.X)
		y := r.between(b.Min.Y-50, b.Max.Y)
		qs.Boxes = append(qs.Boxes, Bounds{
			Min: geom.Point{X: x, Y: y},
			Max: geom.Point{X: x + w, Y: y + h},
		})
	}
	// 恰好贴住三角形包围盒的矩形，专门压贴边判定。
	for k := 0; k < len(tris) && k < 40; k++ {
		qs.Boxes = append(qs.Boxes, tris[k].Bounds())
	}
	return qs
}

// checkEquivalence 单独核验「查询结果与暴力遍历完全一致」这一条，
// 点查询与矩形查询各自比对，逐项相等（顺序也一致，均为升序）。
func checkEquivalence(t *testing.T, tris []Triangle, tree *Tree, qs querySet) {
	t.Helper()
	for qi, p := range qs.Points {
		want := BruteForcePoint(tris, p)
		got := tree.QueryPoint(p, nil)
		if !slices.Equal(got, want) {
			t.Fatalf("点查询 #%d (%v): 剪枝结果 %v 与暴力结果 %v 不一致", qi, p, got, want)
		}
	}
	for qi, q := range qs.Boxes {
		want := BruteForceBox(tris, q)
		got := tree.QueryBox(q, nil)
		if !slices.Equal(got, want) {
			t.Fatalf("矩形查询 #%d (%v..%v): 剪枝结果 %v 与暴力结果 %v 不一致",
				qi, q.Min, q.Max, got, want)
		}
	}
}

// checkStructure 校验树的静态不变量：节点包围盒等于子树三角形包围盒并集；
// 叶子区间长短正确、互不重叠且恰好覆盖全部三角形一次。
func checkStructure(t *testing.T, tree *Tree) {
	t.Helper()
	if len(tree.Nodes) != 2*len(tree.Tris)-1 {
		t.Fatalf("满二叉树节点数应为 %d，实得 %d", 2*len(tree.Tris)-1, len(tree.Nodes))
	}
	seen := make([]int, len(tree.Tris))
	var walk func(id int) (Bounds, int)
	walk = func(id int) (Bounds, int) {
		n := &tree.Nodes[id]
		if n.Leaf() {
			if n.Count < 1 {
				t.Fatalf("节点 %d 是叶子但 Count=%d", id, n.Count)
			}
			b := tree.Tris[tree.Order[n.Start]].Bounds()
			for k := n.Start; k < n.Start+n.Count; k++ {
				ti := tree.Order[k]
				if ti < 0 || ti >= len(tree.Tris) {
					t.Fatalf("节点 %d 的 order[%d]=%d 越界", id, k, ti)
				}
				seen[ti]++
				b = unionBounds(b, tree.Tris[ti].Bounds())
			}
			if b != n.Bounds {
				t.Fatalf("叶子节点 %d 包围盒 %v 与三角形并集 %v 不符", id, n.Bounds, b)
			}
			if n.Depth < 1 || n.Depth > tree.Height {
				t.Fatalf("叶子节点 %d 深度 %d 超出 [1,%d]", id, n.Depth, tree.Height)
			}
			return b, 1
		}
		if n.Count < 2 {
			t.Fatalf("内部节点 %d Count=%d", id, n.Count)
		}
		lb, lc := walk(n.Left)
		rb, rc := walk(n.Right)
		b := unionBounds(lb, rb)
		if b != n.Bounds {
			t.Fatalf("内部节点 %d 包围盒 %v 与子树并集 %v 不符", id, n.Bounds, b)
		}
		if tree.Nodes[n.Left].Depth != n.Depth+1 || tree.Nodes[n.Right].Depth != n.Depth+1 {
			t.Fatalf("节点 %d 的子节点深度不是 %d", id, n.Depth+1)
		}
		if lc+rc != n.Count {
			t.Fatalf("节点 %d Count=%d 与子树叶子数 %d 不符", id, n.Count, lc+rc)
		}
		return b, lc + rc
	}
	walk(0)
	for i, c := range seen {
		if c != 1 {
			t.Fatalf("三角形 #%d 在叶子中出现 %d 次（应恰好 1 次）", i, c)
		}
	}
	if maxDepth := tree.Nodes[0].Depth; maxDepth != 1 {
		t.Fatalf("根深度应为 1，实得 %d", maxDepth)
	}
	// Height 必须恰好等于全树最大节点深度（根为 1）。
	actual := 0
	for i := range tree.Nodes {
		if tree.Nodes[i].Depth > actual {
			actual = tree.Nodes[i].Depth
		}
	}
	if tree.Height != actual {
		t.Fatalf("Height=%d 与最大节点深度 %d 不符", tree.Height, actual)
	}
	if tree.Height > tree.MaxDepth {
		t.Fatalf("树高 %d 超过上界 %d", tree.Height, tree.MaxDepth)
	}
}

func TestScenesQueryMatchesBruteForce(t *testing.T) {
	for _, sc := range Scenes() {
		for _, n := range []int{1, 2, 3, 7, 64, 1000} {
			if sc.Name == "single" && n != 1 {
				continue
			}
			tris := sc.Generate(n)
			tree, err := Build(tris, Options{})
			if err != nil {
				t.Fatalf("%s n=%d 建树失败: %v", sc.Name, n, err)
			}
			checkStructure(t, tree)
			qs := makeQueries(tris, uint64(n)+0x1000, 24)
			checkEquivalence(t, tris, tree, qs)
		}
	}
}

// TestLargeScalesQueryMatchesBruteForce 在两档规模上核验等值。
func TestLargeScalesQueryMatchesBruteForce(t *testing.T) {
	for _, sc := range Scenes() {
		if sc.Name == "single" {
			continue
		}
		for _, n := range []int{1000, 10000} {
			tris := sc.Generate(n)
			tree, err := Build(tris, Options{})
			if err != nil {
				t.Fatalf("%s n=%d 建树失败: %v", sc.Name, n, err)
			}
			qs := makeQueries(tris, uint64(n)*31, 120)
			checkEquivalence(t, tris, tree, qs)
		}
	}
}

// TestHeightWithinUpperBound 单独核验「树高不超过给定上界」这一条：
// 同一批三角形换多个上界，能建则 Height ≤ 上界，不能建则报深度越界。
func TestHeightWithinUpperBound(t *testing.T) {
	tris := uniformScene(257, newRNG(7))
	expect := ExpectedHeight(len(tris))
	for _, bound := range []int{expect, expect + 1, expect + 8} {
		tree, err := Build(tris, Options{MaxDepth: bound})
		if err != nil {
			t.Fatalf("上界 %d 应可建树: %v", bound, err)
		}
		if tree.Height > bound {
			t.Fatalf("树高 %d 超过上界 %d", tree.Height, bound)
		}
		if tree.Height != expect {
			t.Fatalf("树高 %d 与期望 %d 不符", tree.Height, expect)
		}
	}
	// 上界比期望少 1：必须报深度越界，且指出具体节点。
	_, err := Build(tris, Options{MaxDepth: expect - 1})
	var be *BuildError
	if !errors.As(err, &be) || !errors.Is(err, ErrDepthExceeded) {
		t.Fatalf("上界不足应报深度越界，实得 %v", err)
	}
	if be.Depth != expect || be.MaxDepth != expect-1 {
		t.Fatalf("深度错误信息不符: depth=%d max=%d", be.Depth, be.MaxDepth)
	}
	if be.NodeIndex() < 0 {
		t.Fatalf("深度错误应指出节点号，实得 %d", be.NodeIndex())
	}
}

// TestExpectedHeightInvertsTriangleCount 单独核验「由三角形数反解期望树高」：
// ExpectedHeight(n) 必须是最小的、能容纳 n 个叶子的树高，且实测不高过它。
func TestExpectedHeightInvertsTriangleCount(t *testing.T) {
	for n := 1; n <= 20000; n = nextCount(n) {
		h := ExpectedHeight(n)
		// 反解的定义：高度 h 的满二叉树最多 2^(h-1) 个叶子。
		if leaves := 1 << (h - 1); leaves < n {
			t.Fatalf("n=%d: 反解高度 %d 只能容纳 %d 个叶子", n, h, leaves)
		}
		if h > 1 {
			if leavesPrev := 1 << (h - 2); leavesPrev >= n {
				t.Fatalf("n=%d: 反解高度 %d 不是最小（高度 %d 已够）", n, h, h-1)
			}
		}
		tris := uniformScene(n, newRNG(uint64(n)))
		tree, err := Build(tris, Options{})
		if err != nil {
			t.Fatalf("n=%d 建树失败: %v", n, err)
		}
		if tree.Height > h {
			t.Fatalf("n=%d 实测树高 %d 超过反解结果 %d", n, tree.Height, h)
		}
	}
}

func nextCount(n int) int {
	switch {
	case n < 16:
		return n + 1
	case n < 100:
		return n + 7
	case n < 1000:
		return n + 63
	default:
		return n + 997
	}
}

func TestBuildEmptyTriangles(t *testing.T) {
	for _, tris := range [][]Triangle{nil, {}} {
		_, err := Build(tris, Options{})
		var be *BuildError
		if !errors.As(err, &be) || !errors.Is(err, ErrEmptyTriangles) {
			t.Fatalf("空三角形应报错，实得 %v", err)
		}
		if be.NodeIndex() != 0 {
			t.Fatalf("空输入的节点号应为 0，实得 %d", be.NodeIndex())
		}
	}
}

func TestBuildDegenerateBoundsReportsNode(t *testing.T) {
	// 左半：水平退化三角形（三点同 y），右半：正常三角形。
	// 根按 x 中位数恰好 4/4 分开，左子树自身包围盒高度为 0，
	// 且左子树根按先序固定为节点 1。
	var tris []Triangle
	for k := 0; k < 4; k++ {
		x := float64(k) * 20
		tris = append(tris, Triangle{
			A: geom.Point{X: x, Y: 100},
			B: geom.Point{X: x + 15, Y: 100},
			C: geom.Point{X: x + 5, Y: 100},
		})
	}
	for k := 0; k < 4; k++ {
		x := 800 + float64(k)*20
		tris = append(tris, Triangle{
			A: geom.Point{X: x, Y: 400},
			B: geom.Point{X: x + 15, Y: 405},
			C: geom.Point{X: x + 5, Y: 415},
		})
	}
	_, err := Build(tris, Options{})
	var be *BuildError
	if !errors.As(err, &be) || !errors.Is(err, ErrDegenerateBounds) {
		t.Fatalf("退化包围盒应报错，实得 %v", err)
	}
	if be.NodeIndex() != 1 {
		t.Fatalf("退化节点号应为左子树根 1，实得 %d", be.NodeIndex())
	}
	if be.Triangle < 0 || be.Triangle >= 4 {
		t.Fatalf("应指出左半的退化三角形下标，实得 %d", be.Triangle)
	}
	if be.Bounds.Width() <= 0 || be.Bounds.Height() != 0 {
		t.Fatalf("错误里应带退化包围盒: %+v", be.Bounds)
	}
	if !contains(be.Error(), "节点 1") || !contains(be.Error(), "三角形 #") {
		t.Fatalf("退化错误文本信息不全: %s", be.Error())
	}

	// 单点三角形：根即为退化节点。
	_, err = Build([]Triangle{{
		A: geom.Point{X: 5, Y: 5},
		B: geom.Point{X: 5, Y: 5},
		C: geom.Point{X: 5, Y: 5},
	}}, Options{})
	if !errors.As(err, &be) || !errors.Is(err, ErrDegenerateBounds) {
		t.Fatalf("单点退化三角形应报退化，实得 %v", err)
	}
	if be.NodeIndex() != 0 || be.Triangle != 0 {
		t.Fatalf("单三角形退化应指向节点 0 / 三角形 0，实得 %d / %d", be.NodeIndex(), be.Triangle)
	}
}

func TestBuildDepthExceededReportsNode(t *testing.T) {
	tris := uniformScene(4, newRNG(11))
	_, err := Build(tris, Options{MaxDepth: 1})
	var be *BuildError
	if !errors.As(err, &be) || !errors.Is(err, ErrDepthExceeded) {
		t.Fatalf("深度越界应报错，实得 %v", err)
	}
	if be.NodeIndex() != 1 {
		t.Fatalf("越界节点号应为第一个子节点 1，实得 %d", be.NodeIndex())
	}
	if be.Depth != 2 || be.MaxDepth != 1 {
		t.Fatalf("深度信息不符: depth=%d max=%d", be.Depth, be.MaxDepth)
	}
	if _, err := Build(tris, Options{MaxDepth: -3}); !errors.Is(err, ErrDepthExceeded) {
		t.Fatalf("负上界应报深度越界，实得 %v", err)
	}
}

// TestTriangleIntersectsBoxAgainstIndependentTest 用与分离轴无关的办法
// （顶点在盒内 / 盒角在三角形内 / 边相交）复核三角形-矩形判定本身。
func TestTriangleIntersectsBoxAgainstIndependentTest(t *testing.T) {
	tris := uniformScene(60, newRNG(41))
	r := newRNG(42)
	for i := 0; i < 400; i++ {
		x := r.between(-20, 1020)
		y := r.between(-20, 1020)
		w := r.between(0, 120)
		h := r.between(0, 120)
		q := Bounds{Min: geom.Point{X: x, Y: y}, Max: geom.Point{X: x + w, Y: y + h}}
		want := independentIntersects(tris[i%len(tris)], q)
		if got := triangleIntersectsBox(tris[i%len(tris)], q); got != want {
			t.Fatalf("三角形 #%d 与矩形 %v..%v: SAT 判定 %v，独立判定 %v",
				i%len(tris), q.Min, q.Max, got, want)
		}
	}
}

func independentIntersects(t Triangle, q Bounds) bool {
	pts := t.Vertices()
	for _, p := range pts {
		if q.ContainsPoint(p) {
			return true
		}
	}
	corners := [4]geom.Point{
		{X: q.Min.X, Y: q.Min.Y},
		{X: q.Max.X, Y: q.Min.Y},
		{X: q.Max.X, Y: q.Max.Y},
		{X: q.Min.X, Y: q.Max.Y},
	}
	for _, c := range corners {
		if geom.Inside(pts[:], c.X, c.Y) {
			return true
		}
	}
	// 边与边相交（含共线情况由上面的包含判定兜底）。
	boxEdges := [4][2]geom.Point{
		{corners[0], corners[1]},
		{corners[1], corners[2]},
		{corners[2], corners[3]},
		{corners[3], corners[0]},
	}
	for i := 0; i < 3; i++ {
		a, b := pts[i], pts[(i+1)%3]
		for _, e := range boxEdges {
			if segmentsCross(a, b, e[0], e[1]) {
				return true
			}
		}
	}
	return false
}

func segmentsCross(p, q, a, b geom.Point) bool {
	d1 := cross(q.X-p.X, q.Y-p.Y, a.X-p.X, a.Y-p.Y)
	d2 := cross(q.X-p.X, q.Y-p.Y, b.X-p.X, b.Y-p.Y)
	d3 := cross(b.X-a.X, b.Y-a.Y, p.X-a.X, p.Y-a.Y)
	d4 := cross(b.X-a.X, b.Y-a.Y, q.X-a.X, q.Y-a.Y)
	if ((d1 > 0 && d2 < 0) || (d1 < 0 && d2 > 0)) &&
		((d3 > 0 && d4 < 0) || (d3 < 0 && d4 > 0)) {
		return true
	}
	return false
}

func cross(ax, ay, bx, by float64) float64 { return ax*by - ay*bx }

func TestTreeBoundsAndAccessors(t *testing.T) {
	tris := uniformScene(37, newRNG(43))
	tree, err := Build(tris, Options{})
	if err != nil {
		t.Fatal(err)
	}
	var want Bounds
	for i, tri := range tris {
		if i == 0 {
			want = tri.Bounds()
			continue
		}
		want = unionBounds(want, tri.Bounds())
	}
	if got := tree.Bounds(); got != want {
		t.Fatalf("根包围盒 %v 与全部三角形并集 %v 不符", got, want)
	}
	var nilTree *Tree
	if b := nilTree.Bounds(); b != (Bounds{}) {
		t.Fatalf("nil 树的包围盒应为零值，实得 %v", b)
	}
}

// TestBuildDeterministicAndInputFrozen 确认同一输入建出同一棵树，
// 且 Build 不改写调用方的三角形切片。
func TestBuildDeterministicAndInputFrozen(t *testing.T) {
	tris := clusteredScene(500, newRNG(31))
	before := slices.Clone(tris)
	t1, err := Build(tris, Options{})
	if err != nil {
		t.Fatal(err)
	}
	t2, err := Build(tris, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(before, tris) {
		t.Fatal("Build 改写了输入三角形切片")
	}
	if t1.Height != t2.Height || t1.NodeCount() != t2.NodeCount() {
		t.Fatalf("两次建树结果不同: height %d/%d nodes %d/%d",
			t1.Height, t2.Height, t1.NodeCount(), t2.NodeCount())
	}
	for i := range t1.Nodes {
		if t1.Nodes[i] != t2.Nodes[i] {
			t.Fatalf("节点 %d 两次建树不同:\n%+v\n%+v", i, t1.Nodes[i], t2.Nodes[i])
		}
	}
	if !slices.Equal(t1.Order, t2.Order) {
		t.Fatal("两次建树的 Order 不同")
	}
	qs := makeQueries(tris, 77, 30)
	for _, p := range qs.Points {
		if !slices.Equal(t1.QueryPoint(p, nil), t2.QueryPoint(p, nil)) {
			t.Fatalf("点 %v 两次查询结果不同", p)
		}
	}
}

func TestFromPolygons(t *testing.T) {
	tris, err := FromPolygons([][]geom.Point{
		{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 0, Y: 10}},
		{{X: 20, Y: 0}, {X: 30, Y: 0}, {X: 20, Y: 10}},
	})
	if err != nil || len(tris) != 2 {
		t.Fatalf("三顶点多边形应转为三角形: %v %d", err, len(tris))
	}
	if tris[0].A != (geom.Point{X: 0, Y: 0}) || tris[1].C != (geom.Point{X: 20, Y: 10}) {
		t.Fatalf("顶点映射错误: %+v", tris)
	}
	_, err = FromPolygons([][]geom.Point{
		{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 1, Y: 1}},
		{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 1, Y: 1}, {X: 0, Y: 1}},
	})
	if err == nil || !contains(err.Error(), "#1") {
		t.Fatalf("四边形应报错并指出下标 1，实得 %v", err)
	}
}

// TestBruteForceInterfaceFrozen 锁定暴力对照接口的行为，表明它未被改动。
func TestBruteForceInterfaceFrozen(t *testing.T) {
	tris := []Triangle{
		{A: geom.Point{X: 0, Y: 0}, B: geom.Point{X: 10, Y: 0}, C: geom.Point{X: 0, Y: 10}},
		{A: geom.Point{X: 20, Y: 20}, B: geom.Point{X: 30, Y: 20}, C: geom.Point{X: 20, Y: 30}},
		{A: geom.Point{X: 5, Y: 5}, B: geom.Point{X: 15, Y: 5}, C: geom.Point{X: 5, Y: 15}},
	}
	cases := []struct {
		p    geom.Point
		want []int
	}{
		{geom.Point{X: 1, Y: 1}, []int{0}},
		{geom.Point{X: 25, Y: 21}, []int{1}},
		{geom.Point{X: 100, Y: 100}, []int{}},
		{geom.Point{X: 0, Y: 0}, []int{0}},
	}
	for _, c := range cases {
		if got := BruteForcePoint(tris, c.p); !slices.Equal(got, c.want) {
			t.Fatalf("BruteForcePoint(%v)=%v，期望 %v", c.p, got, c.want)
		}
	}
	boxCases := []struct {
		q    Bounds
		want []int
	}{
		{Bounds{Min: geom.Point{X: -1, Y: -1}, Max: geom.Point{X: 2, Y: 2}}, []int{0}},
		{Bounds{Min: geom.Point{X: 40, Y: 40}, Max: geom.Point{X: 50, Y: 50}}, []int{}},
	}
	for _, c := range boxCases {
		if got := BruteForceBox(tris, c.q); !slices.Equal(got, c.want) {
			t.Fatalf("BruteForceBox(%v..%v)=%v，期望 %v", c.q.Min, c.q.Max, got, c.want)
		}
	}
}

func TestSingleTriangle(t *testing.T) {
	tris := singleScene(1, newRNG(3))
	tree, err := Build(tris, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if tree.Height != 1 || tree.LeafCount() != 1 || tree.NodeCount() != 1 {
		t.Fatalf("单三角形应只有一个叶根: height=%d leaves=%d nodes=%d",
			tree.Height, tree.LeafCount(), tree.NodeCount())
	}
	checkStructure(t, tree)
	checkEquivalence(t, tris, tree, makeQueries(tris, 99, 40))
	st := Stats{}
	got := tree.QueryPoint(geom.Point{X: 40, Y: 30}, &st)
	if !slices.Equal(got, []int{0}) {
		t.Fatalf("内部点应命中 0，实得 %v", got)
	}
	if st.VisitedNodes != 1 || st.PrunedNodes != 0 {
		t.Fatalf("单叶根查询统计异常: %+v", st)
	}
	miss := tree.QueryPoint(geom.Point{X: 0, Y: 0}, &st)
	if len(miss) != 0 || st.PrunedNodes != 1 {
		t.Fatalf("外部点应被根剪枝: %v %+v", miss, st)
	}
	// 零面积矩形正好落在顶点上：贴边算相交，与暴力对照一致。
	q := Bounds{Min: geom.Point{X: 90, Y: 20}, Max: geom.Point{X: 90, Y: 20}}
	if got, want := tree.QueryBox(q, nil), BruteForceBox(tris, q); !slices.Equal(got, want) {
		t.Fatalf("贴顶点矩形查询 %v 与暴力 %v 不一致", got, want)
	}
	if got := tree.QueryBox(q, nil); !slices.Equal(got, []int{0}) {
		t.Fatalf("贴顶点矩形应命中该三角形，实得 %v", got)
	}
}

func TestStatsCountVisitedNodes(t *testing.T) {
	tris := uniformScene(1000, newRNG(21))
	tree, err := Build(tris, Options{})
	if err != nil {
		t.Fatal(err)
	}
	all := Bounds{Min: geom.Point{X: -1, Y: -1}, Max: geom.Point{X: 1001, Y: 1001}}
	st := Stats{}
	got := tree.QueryBox(all, &st)
	if len(got) != len(tris) {
		t.Fatalf("全覆盖查询应返回全部三角形: %d/%d", len(got), len(tris))
	}
	if st.VisitedNodes != tree.NodeCount() {
		t.Fatalf("全覆盖查询应访问全部节点: %d/%d", st.VisitedNodes, tree.NodeCount())
	}
	if st.PrunedNodes != 0 {
		t.Fatalf("全覆盖查询不应剪枝: %d", st.PrunedNodes)
	}
	empty := Bounds{Min: geom.Point{X: -100, Y: -100}, Max: geom.Point{X: -50, Y: -50}}
	st = Stats{}
	if got := tree.QueryBox(empty, &st); len(got) != 0 {
		t.Fatalf("空区域不应命中: %v", got)
	}
	if st.VisitedNodes != 1 || st.PrunedNodes != 1 || st.Tested != 0 {
		t.Fatalf("空区域应只访问根并剪枝: %+v", st)
	}
}

func TestEmptyTreeQuery(t *testing.T) {
	var tree *Tree
	if got := tree.QueryPoint(geom.Point{X: 0, Y: 0}, nil); len(got) != 0 {
		t.Fatalf("nil 树的点查询应返回空: %v", got)
	}
	if got := tree.QueryBox(Bounds{Min: geom.Point{X: 0, Y: 0}, Max: geom.Point{X: 1, Y: 1}}, nil); len(got) != 0 {
		t.Fatalf("nil 树的矩形查询应返回空: %v", got)
	}
}

// TestErrorMessagesMentionNode 确保三类错误文本里都带节点号。
func TestErrorMessagesMentionNode(t *testing.T) {
	_, err := Build(nil, Options{})
	if err == nil {
		t.Fatal("空输入应报错")
	}
	if want := "节点 0"; !contains(err.Error(), want) {
		t.Fatalf("错误文本应包含 %q: %s", want, err)
	}
	tris := uniformScene(3, newRNG(5))
	_, err = Build(tris, Options{MaxDepth: 1})
	if err == nil || !contains(err.Error(), "节点 1") {
		t.Fatalf("深度越界错误文本应包含节点 1: %v", err)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestTreeHeightTwoScales(t *testing.T) {
	for _, n := range []int{1000, 10000} {
		for _, sc := range Scenes() {
			if sc.Name == "single" {
				continue
			}
			tris := sc.Generate(n)
			tree, err := Build(tris, Options{})
			if err != nil {
				t.Fatalf("%s n=%d: %v", sc.Name, n, err)
			}
			if tree.Height > ExpectedHeight(n) {
				t.Fatalf("%s n=%d: 身高 %d 超过反解 %d", sc.Name, n, tree.Height, ExpectedHeight(n))
			}
			t.Logf("%-11s n=%5d height=%2d expected=%2d nodes=%6d",
				sc.Name, n, tree.Height, ExpectedHeight(n), tree.NodeCount())
		}
	}
}

func ExampleExpectedHeight() {
	for _, n := range []int{1, 2, 3, 4, 5, 8, 9, 1000, 10000} {
		fmt.Printf("%d->%d ", n, ExpectedHeight(n))
	}
	// Output: 1->1 2->2 3->3 4->3 5->4 8->4 9->5 1000->11 10000->15
}
