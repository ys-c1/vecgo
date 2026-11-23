package sweep

import (
	"math"
	"testing"

	"vecgo/geom"
)

func seg(x0, y0, x1, y1 float64) Segment {
	return Segment{P0: geom.Point{X: x0, Y: y0}, P1: geom.Point{X: x1, Y: y1}}
}

// bruteForce 是 O(n²) 朴素求交，作为扫描线结果的独立对照。
// 段方向先做与 FindIntersections 相同的规范化，保证浮点算术路径一致、
// 坐标可逐位比较；遍历与输出顺序与扫描线完全无关。
func bruteForce(t *testing.T, in []Segment) []Intersection {
	t.Helper()
	var rows []Intersection
	for i := 0; i < len(in); i++ {
		for j := i + 1; j < len(in); j++ {
			a0, a1 := norm(in[i].P0, in[i].P1)
			b0, b1 := norm(in[j].P0, in[j].P1)
			for _, p := range intersectPoints(a0, a1, b0, b1) {
				rows = append(rows, Intersection{X: p.X, Y: p.Y, SegA: i, SegB: j})
			}
		}
	}
	sortRows(rows)
	return rows
}

func norm(p0, p1 geom.Point) (geom.Point, geom.Point) {
	if p0.X > p1.X || (p0.X == p1.X && p0.Y > p1.Y) {
		return p1, p0
	}
	return p0, p1
}

func rowsEqual(a, b []Intersection) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if math.Float64bits(a[i].X) != math.Float64bits(b[i].X) ||
			math.Float64bits(a[i].Y) != math.Float64bits(b[i].Y) ||
			a[i].SegA != b[i].SegA || a[i].SegB != b[i].SegB {
			return false
		}
	}
	return true
}

// 类别一：平行（含平行分离、竖直族、水平族；族与族之间也不交叉），应无交点。
func TestClassParallel(t *testing.T) {
	in := []Segment{
		seg(0, 0, 10, 2),
		seg(0, 1, 10, 3),
		seg(0, 4, 10, 6),
		seg(20, 0, 20, 9),   // 竖直线族，x 远离斜线
		seg(30, 0, 30, -5),  // 另一条竖直线，互相平行且不共线
		seg(-50, 20, 5, 20), // 水平线族，y 高于斜线可达范围
		seg(-50, 30, 5, 30),
	}
	rows, st, err := FindIntersections(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("平行线段不应有交点，得到 %d 行: %+v", len(rows), rows)
	}
	if st.InsertEvents != len(in) || st.DeleteEvents != len(in) {
		t.Fatalf("端点事件计数错误: %+v", st)
	}
}

// 类别二：一般相交（含十字交叉、多线共点、交叉于端点内部）。
func TestClassCrossing(t *testing.T) {
	in := []Segment{
		seg(0, 0, 10, 10),
		seg(0, 10, 10, 0),
		seg(-2, 5, 12, 5), // 三线共点 (5,5)
		seg(3, -3, 3, 12), // 竖直线穿过多条
		seg(7, 7, 20, 20), // 与段 0 共线，仅在 (7,7) 单点相接
	}
	rows, _, err := FindIntersections(in)
	if err != nil {
		t.Fatal(err)
	}
	want := bruteForce(t, in)
	if !rowsEqual(rows, want) {
		t.Fatalf("相交结果与朴素法不一致\n got %+v\nwant %+v", rows, want)
	}
	// 明确核对三线共点产生三对交点，坐标逐位相同。
	var common int
	for _, r := range rows {
		if math.Float64bits(r.X) == math.Float64bits(5.0) && math.Float64bits(r.Y) == math.Float64bits(5.0) {
			common++
		}
	}
	if common != 3 {
		t.Fatalf("(5,5) 共点应有 3 对，实际 %d", common)
	}
}

// 类别三：共线重叠（部分重叠、包含、仅端点相接三种）。
func TestClassOverlap(t *testing.T) {
	good := []Segment{
		seg(0, 0, 10, 0),  // 0
		seg(4, 0, 8, 0),   // 1 被包含 -> 两个边界点 (4,0),(8,0)
		seg(8, 0, 14, 0),  // 2 部分重叠 -> (8,0),(10,0)
		seg(20, 0, 30, 0), // 3 共线分离 -> 无
		seg(-5, -5, 0, 0), // 4 仅在 (0,0) 与段 0 相接 -> 单点
	}
	rows, _, err := FindIntersections(good)
	if err != nil {
		t.Fatal(err)
	}
	want := bruteForce(t, good)
	if !rowsEqual(rows, want) {
		t.Fatalf("重叠结果与朴素法不一致\n got %+v\nwant %+v", rows, want)
	}
	// 重叠区间边界点显式断言（(8,0) 处 0/1、0/2、1/2 三对共点）。
	wantPts := map[[2]float64]int{
		{4, 0}: 1, {8, 0}: 3, {10, 0}: 1, {0, 0}: 1,
	}
	got := map[[2]float64]int{}
	for _, r := range rows {
		got[[2]float64{r.X, r.Y}]++
	}
	for k, c := range wantPts {
		if got[k] != c {
			t.Fatalf("重叠边界点 %v 期望 %d 次，实际 %d 次（全表 %+v）", k, c, got[k], rows)
		}
	}
}

// 类别四：退化输入——零长段、重复端点对、NaN，各自带段号报错。
func TestClassDegenerateErrors(t *testing.T) {
	cases := []struct {
		name string
		in   []Segment
		want string
	}{
		{"零长段", []Segment{seg(0, 0, 1, 1), seg(3, 3, 3, 3)}, "段 1 为零长段"},
		{"端点重复", []Segment{seg(0, 0, 5, 5), seg(2, 2, 2, 9), seg(5, 5, 0, 0)}, "段 0 与段 2 端点重复"},
		{"NaN-x", []Segment{seg(math.NaN(), 0, 1, 1)}, "段 0 坐标含 NaN"},
		{"NaN-y", []Segment{seg(0, 0, 1, math.NaN())}, "段 0 坐标含 NaN"},
		{"NaN 在第二段", []Segment{seg(0, 0, 1, 1), seg(2, 2, math.NaN(), 3)}, "段 1 坐标含 NaN"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, _, err := FindIntersections(c.in)
			if err == nil {
				t.Fatalf("%s 应返回错误", c.name)
			}
			if !contains(err.Error(), c.want) {
				t.Fatalf("%s 错误信息 %q 不包含 %q", c.name, err.Error(), c.want)
			}
		})
	}
}

// 类别五：共端点——多条段从同一点出发/在同一点结束，属合法输入，
// 不允许与“端点重复（整段重复）”混淆。
func TestClassSharedEndpoints(t *testing.T) {
	in := []Segment{
		seg(0, 0, 10, 0),    // 0
		seg(0, 0, 0, 10),    // 1 与段 0 共左端点
		seg(0, 0, -7, -7),   // 2 同点出发的斜线
		seg(10, 0, 10, -10), // 3 与段 0 共右端点
		seg(0, 10, 10, 10),  // 4 与段 1 共上端点
	}
	rows, _, err := FindIntersections(in)
	if err != nil {
		t.Fatal(err)
	}
	want := bruteForce(t, in)
	if !rowsEqual(rows, want) {
		t.Fatalf("共端点结果与朴素法不一致\n got %+v\nwant %+v", rows, want)
	}
	// (0,0) 处应有 0/1、0/2、1/2 三对。
	var origin int
	for _, r := range rows {
		if r.X == 0 && r.Y == 0 {
			origin++
		}
	}
	if origin != 3 {
		t.Fatalf("原点共端点应有 3 对，实际 %d", origin)
	}
}

// 稳定全序：同一输入连跑十次，交点表逐字节相同，哈希全串一致。
func TestStableAcrossRuns(t *testing.T) {
	in := MakeFixture(300, 20261124)
	var first [32]byte
	var firstRows []Intersection
	for k := 0; k < 10; k++ {
		rows, _, err := FindIntersections(in)
		if err != nil {
			t.Fatal(err)
		}
		h := HashTable(rows)
		if k == 0 {
			first, firstRows = h, rows
			continue
		}
		if h != first {
			t.Fatalf("第 %d 次运行哈希不一致：%x vs %x", k, h, first)
		}
		if !rowsEqual(rows, firstRows) {
			t.Fatalf("第 %d 次运行交点表不一致", k)
		}
	}
}

// 打乱输入顺序后，段对归属与交点几何保持一致。
//
// 注意比较口径：浮点参数式求交的结果依赖段端点的书写顺序，同一段对在两种
// 输入排列下可能相差 1 ulp（这是 IEEE-754 浮点的固有性质，非不确定性）。
// 因此这里对坐标做 1 ulp 内匹配；真正的“逐字节确定性”由 TestStableAcrossRuns
// 在同一输入上以哈希全串保证。
func TestOrderIndependenceGeometry(t *testing.T) {
	base := MakeFixture(120, 777)
	rows1, _, err := FindIntersections(base)
	if err != nil {
		t.Fatal(err)
	}
	idx := make([]int, len(base)) // idx[新段号] = 原段号
	perm := make([]Segment, len(base))
	for i := range base {
		idx[i] = len(base) - 1 - i
	}
	for newI, oldI := range idx {
		perm[newI] = base[oldI]
	}
	rows2, _, err := FindIntersections(perm)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows1) != len(rows2) {
		t.Fatalf("行数随输入顺序变化: %d vs %d", len(rows1), len(rows2))
	}
	type pair struct{ a, b int }
	got := map[pair][][2]float64{} // 段对（原编号）-> 坐标候选
	for _, r := range rows2 {
		oa, ob := idx[r.SegA], idx[r.SegB]
		if oa > ob {
			oa, ob = ob, oa
		}
		got[pair{oa, ob}] = append(got[pair{oa, ob}], [2]float64{r.X, r.Y})
	}
	for _, r := range rows1 {
		cands, ok := got[pair{r.SegA, r.SegB}]
		if !ok {
			t.Fatalf("段对 (%d,%d) 在重排输入中缺失", r.SegA, r.SegB)
		}
		matched := false
		for _, c := range cands {
			if ulpEqual(r.X, c[0]) && ulpEqual(r.Y, c[1]) {
				matched = true
				break
			}
		}
		if !matched {
			t.Fatalf("段对 (%d,%d) 坐标在 1 ulp 内不匹配: %v vs %v", r.SegA, r.SegB, [2]float64{r.X, r.Y}, cands)
		}
	}
}

// ulpEqual 判定两浮点数相等或仅相差 1 个 ulp（含 NaN 外的全部有限值）。
func ulpEqual(a, b float64) bool {
	if a == b {
		return true
	}
	ai, bi := math.Float64bits(a), math.Float64bits(b)
	if ai > bi {
		ai, bi = bi, ai
	}
	return bi-ai <= 1
}

// 数值固定：显式核对一次交叉的浮点位模式，锁定求交公式。
func TestNumericResultFrozen(t *testing.T) {
	in := []Segment{
		seg(1, 2, 9, 8),
		seg(2, 9, 8, 1),
	}
	rows, _, err := FindIntersections(in)
	if err != nil || len(rows) != 1 {
		t.Fatalf("期望单交点，err=%v rows=%d", err, len(rows))
	}
	// t = ((q-p)×s)/(r×s)：手工同一公式的参考值。
	p0 := geom.Point{X: 1, Y: 2}
	p1 := geom.Point{X: 9, Y: 8}
	q0 := geom.Point{X: 2, Y: 9}
	q1 := geom.Point{X: 8, Y: 1}
	rx, ry := p1.X-p0.X, p1.Y-p0.Y
	sx, sy := q1.X-q0.X, q1.Y-q0.Y
	den := rx*sy - ry*sx
	tt := ((q0.X-p0.X)*sy - (q0.Y-p0.Y)*sx) / den
	rx2, ry2 := p0.X+tt*rx, p0.Y+tt*ry
	if math.Float64bits(rows[0].X) != math.Float64bits(rx2) ||
		math.Float64bits(rows[0].Y) != math.Float64bits(ry2) {
		t.Fatalf("数值结果偏离固定公式: got (%v,%v) want (%v,%v)", rows[0].X, rows[0].Y, rx2, ry2)
	}
	if rows[0].SegA != 0 || rows[0].SegB != 1 {
		t.Fatalf("段号错误: %+v", rows[0])
	}
}

// 全序键唯一性：构造同点多事件场景，堆弹出顺序必须严格非递减，
// 且不存在两个无法区分的事件。
func TestEventTotalOrder(t *testing.T) {
	in := []Segment{
		seg(0, 0, 10, 0),   // 水平
		seg(0, 0, 0, 10),   // 竖直，同点插入
		seg(0, 0, 10, 10),  // 同点斜线
		seg(0, 0, -10, 10), // 同点反向斜线
	}
	_, _, err := FindIntersections(in)
	if err != nil {
		t.Fatal(err)
	}
	// 直接构造事件键验证二级比较：同 x,y,kind,slope 时由段号区分。
	e1 := event{x: 0, y: 0, kind: kindInsert, slope: 0, a: 0, b: -1}
	e2 := event{x: 0, y: 0, kind: kindInsert, slope: 0, a: 1, b: -1}
	if !eventLess(e1, e2) || eventLess(e2, e1) {
		t.Fatal("段号二级比较失效")
	}
	// 交点事件同点同斜率：a 相同则由 b 区分。
	g1 := event{x: 5, y: 5, kind: kindIntersect, slope: 1, a: 2, b: 3}
	g2 := event{x: 5, y: 5, kind: kindIntersect, slope: 1, a: 2, b: 4}
	if !eventLess(g1, g2) || eventLess(g2, g1) {
		t.Fatal("交点事件 b 级比较失效")
	}
	// kind 顺序：insert < intersect < delete。
	if !(eventLess(event{kind: kindInsert}, event{kind: kindIntersect}) &&
		eventLess(event{kind: kindIntersect}, event{kind: kindDelete})) {
		t.Fatal("事件类型顺序错误")
	}
}

// TestTreapInvariantsUnderSweep 在完整扫描过程中逐事件校验 treap 结构，
// 覆盖常规随机档与极端退化档（共线首尾相接 + 近平行线）。
func TestTreapInvariantsUnderSweep(t *testing.T) {
	for _, n := range []int{60, 120, 300} {
		if _, _, err := FindIntersectionsChecked(MakeFixture(n, 777)); err != nil {
			t.Fatalf("随机档 n=%d 不变量破坏: %v", n, err)
		}
	}
	if _, _, err := FindIntersectionsChecked(MakeExtremeFixture(500, 777)); err != nil {
		t.Fatalf("极端档不变量破坏: %v", err)
	}
}

// treap 不变量：插入后中序遍历符合扫描线全序，删除后依然成立。
func TestTreapOrdering(t *testing.T) {
	tr := &treap{}
	less := func(a, b int) bool {
		// 用 id 本身当键的简化全序。
		return a < b
	}
	for _, id := range []int{5, 2, 8, 1, 4, 7, 9, 3, 6} {
		tr.insert(id, less)
	}
	got := tr.inOrder()
	want := []int{1, 2, 3, 4, 5, 6, 7, 8, 9}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("treap 中序错误: got %v want %v", got, want)
		}
	}
	p, s := tr.neighbors(5)
	if p != 4 || s != 6 {
		t.Fatalf("邻居错误: pred=%d succ=%d", p, s)
	}
	tr.remove(5)
	got = tr.inOrder()
	want = []int{1, 2, 3, 4, 6, 7, 8, 9}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("删除后中序错误: got %v want %v", got, want)
		}
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
