package scan

import (
	"testing"

	"vecgo/geom"
)

func pt(x, y int) geom.Point { return geom.Point{X: float64(x), Y: float64(y)} }

func poly(ps ...geom.Point) []geom.Point { return ps }

func reverse(p []geom.Point) []geom.Point {
	out := make([]geom.Point, len(p))
	for i := range p {
		out[i] = p[len(p)-1-i]
	}
	return out
}

// 三类多边形，每类都有边与采样中心严格共线，覆盖不同的退化形态：
//   - 凸：一条斜边穿过 7 个采样中心；
//   - 凹：缺口的两条边同时贴着扫描行，一行上有两个贴边点；
//   - 自交：偶数奇偶规则把中心排除在外，贴边点分布在两条交叉边上。
var classes = []struct {
	name string
	poly []geom.Point
	// 不补翻转时，差异单元所贴的边（正向顶点序下标）。
	naiveEdges []int
	// 不补翻转时，正向与反向覆盖单元数。
	naiveFwd, naiveRev int
	// 不补翻转时，覆盖位不同的单元数。
	naiveCells int
}{
	{
		name:       "凸多边形·直角三角形",
		poly:       poly(pt(0, 0), pt(7, 0), pt(0, 7)),
		naiveEdges: []int{1},
		naiveFwd:   28,
		naiveRev:   21,
		naiveCells: 7,
	},
	{
		name:       "凹多边形·箭头形",
		poly:       poly(pt(0, 0), pt(8, 0), pt(8, 8), pt(4, 4), pt(0, 8)),
		naiveEdges: []int{2, 3},
		naiveFwd:   52,
		naiveRev:   44,
		naiveCells: 8,
	},
	{
		name:       "自交多边形·领结形",
		poly:       poly(pt(0, 0), pt(8, 8), pt(0, 8), pt(8, 0)),
		naiveEdges: []int{0, 2},
		naiveFwd:   32,
		naiveRev:   32, // 总数相同，但像素不同：领结是无翻转时唯一“数目对、位置错”的类
		naiveCells: 16,
	},
}

// 不满足守恒条件的样例：等腰直角三角形 (0,0),(5,0),(0,5)。
// 斜边只有一条，贴边两侧单元数不等，缺口无法互相抵消。
var brokenCase = struct {
	name string
	poly []geom.Point
}{
	name: "凸多边形·等腰直角三角形（反例）",
	poly: poly(pt(0, 0), pt(5, 0), pt(0, 5)),
}

const side = 8

func fillBoth(t *testing.T, p []geom.Point, correct bool) (fwd, rev *Mask) {
	t.Helper()
	var err error
	if correct {
		fwd, err = Fill(p, side, side, Forward)
	} else {
		fwd, err = FillUncorrected(p, side, side, Forward)
	}
	if err != nil {
		t.Fatal(err)
	}
	if correct {
		rev, err = Fill(p, side, side, Reverse)
	} else {
		rev, err = FillUncorrected(p, side, side, Reverse)
	}
	if err != nil {
		t.Fatal(err)
	}
	return fwd, rev
}

// TestThreeClassesEqualCounts 三类多边形各验一次：补翻转后，
// 正、反两个方向的覆盖单元数相等，掩码逐像素相同，面积差为 0。
func TestThreeClassesEqualCounts(t *testing.T) {
	for _, c := range classes {
		fwd, rev := fillBoth(t, c.poly, true)
		if fwd.Count() != rev.Count() {
			t.Fatalf("%s: 正向 %d ≠ 反向 %d", c.name, fwd.Count(), rev.Count())
		}
		if d := Differing(fwd, rev); len(d) != 0 {
			t.Fatalf("%s: 有 %d 个单元覆盖位不同，首个 %+v", c.name, len(d), d[0])
		}
		// 每格面积恰为 1，覆盖单元数就是面积；差是整数 0，不是近似 0。
		if diff := int64(fwd.Count()) - int64(rev.Count()); diff != 0 {
			t.Fatalf("%s: 面积差 %d", c.name, diff)
		}
		t.Logf("%s: 正向 %d，反向 %d，面积差 %d", c.name, fwd.Count(), rev.Count(), 0)
	}
}

// TestAreaDifferenceIsInteger 面积守恒用整数格点验证：
// 覆盖单元数是 int，鞋带公式 Area2 是 int64，二者都不经过浮点。
// 正向与反向的 2A 逐位互为相反数，相加严格为 0。
func TestAreaDifferenceIsInteger(t *testing.T) {
	for _, c := range classes {
		a2f, err := Area2(c.poly)
		if err != nil {
			t.Fatal(err)
		}
		a2r, err := Area2(reverse(c.poly))
		if err != nil {
			t.Fatal(err)
		}
		if a2f+a2r != 0 {
			t.Fatalf("%s: 2A 正向 %d + 反向 %d = %d，不是整数 0", c.name, a2f, a2r, a2f+a2r)
		}
		fwd, rev := fillBoth(t, c.poly, true)
		if int64(fwd.Count()-rev.Count()) != 0 {
			t.Fatalf("%s: 面积差不是整数 0", c.name)
		}
		t.Logf("%s: 2A=±%d（和=0），覆盖面积差=0", c.name, a2f)
	}
}

// TestNaiveShowsDirectionDependence 省掉补翻转，三类多边形都会暴露方向
// 依赖，差异只出现在贴边单元上，并能定位到具体的边。
func TestNaiveShowsDirectionDependence(t *testing.T) {
	for _, c := range classes {
		fwd, rev := fillBoth(t, c.poly, false)
		if fwd.Count() != c.naiveFwd || rev.Count() != c.naiveRev {
			t.Fatalf("%s: 不补翻转 正向 %d 反向 %d，期望 %d/%d",
				c.name, fwd.Count(), rev.Count(), c.naiveFwd, c.naiveRev)
		}
		diffs, err := Diagnose(c.poly, side, side, false)
		if err != nil {
			t.Fatal(err)
		}
		if len(diffs) != c.naiveCells {
			t.Fatalf("%s: 不补翻转差异 %d 个单元，期望 %d", c.name, len(diffs), c.naiveCells)
		}
		for _, d := range diffs {
			if len(d.Edges) == 0 {
				t.Fatalf("%s: 单元 (%d,%d) 有差异却没有贴边", c.name, d.Col, d.Row)
			}
			if !onlyAmong(d.Edges, c.naiveEdges) {
				t.Fatalf("%s: 单元 (%d,%d) 贴边 %v，期望只落在 %v",
					c.name, d.Col, d.Row, d.Edges, c.naiveEdges)
			}
		}
		t.Logf("%s: 不补翻转 正向 %d，反向 %d，差异 %d 个单元，全部贴在边 %v",
			c.name, fwd.Count(), rev.Count(), len(diffs), c.naiveEdges)
	}
}

// TestBrokenCasePointsToEdge 反例的差异必须全部归到同一条边（下标 1，
// 即斜边 (5,0)→(0,5)），补翻转后差异清零。
func TestBrokenCasePointsToEdge(t *testing.T) {
	diffs, err := Diagnose(brokenCase.poly, side, side, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(diffs) != 5 {
		t.Fatalf("反例应出现 5 个差异单元，实际 %d", len(diffs))
	}
	for _, d := range diffs {
		if !onlyAmong(d.Edges, []int{1}) {
			t.Fatalf("单元 (%d,%d) 贴边 %v，应只落在斜边 1", d.Col, d.Row, d.Edges)
		}
	}
	fwd, rev := fillBoth(t, brokenCase.poly, false)
	if fwd.Count() != 15 || rev.Count() != 10 {
		t.Fatalf("反例不补翻转 正 %d 反 %d，期望 15/10", fwd.Count(), rev.Count())
	}
	fixed, err := Diagnose(brokenCase.poly, side, side, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(fixed) != 0 {
		t.Fatalf("补翻转后仍有 %d 个差异单元", len(fixed))
	}
	t.Logf("反例: 不补翻转 正 %d 反 %d，差 %d 个单元全在边 1（斜边）",
		fwd.Count(), rev.Count(), len(diffs))
}

// TestCyclicRotationInvariant 遍历方向还包括从不同顶点起算。
// 顶点序循环移位不改变边的无向集合，结果也应不变。
func TestCyclicRotationInvariant(t *testing.T) {
	for _, c := range classes {
		base, err := Fill(c.poly, side, side, Forward)
		if err != nil {
			t.Fatal(err)
		}
		for k := 1; k < len(c.poly); k++ {
			rot := append(append([]geom.Point{}, c.poly[k:]...), c.poly[:k]...)
			got, err := Fill(rot, side, side, Forward)
			if err != nil {
				t.Fatal(err)
			}
			if d := Differing(base, got); len(d) != 0 {
				t.Fatalf("%s: 起点移到 %d 后有 %d 个单元改变", c.name, k, len(d))
			}
		}
	}
}

// TestFlipOnlyUsedOnTies 补翻转只出现在贴边分支。取一张没有任何采样
// 中心落在边上的几何，补翻转前后必须逐像素相同。
func TestFlipOnlyUsedOnTies(t *testing.T) {
	// 边都在整数线 x=1、x=7、y=1、y=7 上；采样中心是半整数，不落线。
	sq := poly(pt(1, 1), pt(7, 1), pt(7, 7), pt(1, 7))
	for _, dir := range []Direction{Forward, Reverse} {
		a, err := Fill(sq, side, side, dir)
		if err != nil {
			t.Fatal(err)
		}
		b, err := FillUncorrected(sq, side, side, dir)
		if err != nil {
			t.Fatal(err)
		}
		if d := Differing(a, b); len(d) != 0 {
			t.Fatalf("%s: 无贴边采样点却出现补翻转差异 %d", dir, len(d))
		}
	}
}

// TestEdgeFlipFlag 直接核对补翻转的触发条件：Flip 恰在存储方向的起点
// 按字典序大于终点时为真，与绕向无关。
func TestEdgeFlipFlag(t *testing.T) {
	// 顶点序取顺时针（屏幕坐标），边 0 是 (0,0)->(4,0)，规范序为真。
	p := poly(pt(0, 0), pt(4, 0), pt(4, 4), pt(0, 4))
	fwd, err := Edges(p, Forward)
	if err != nil {
		t.Fatal(err)
	}
	rev, err := Edges(p, Reverse)
	if err != nil {
		t.Fatal(err)
	}
	for i := range fwd {
		if fwd[i].Flip == rev[i].Flip {
			t.Fatalf("边 %d: 正反两个方向的 Flip 不应相同", i)
		}
	}
	wantFwd := []bool{false, false, true, true} // (4,4)->(0,4) 与 (0,4)->(0,0) 起点字典序更大
	wantRev := []bool{true, true, false, false} // 同一条边掉头后 Flip 取反
	for i := range wantFwd {
		if fwd[i].Flip != wantFwd[i] || rev[i].Flip != wantRev[i] {
			t.Fatalf("边 %d: 正向 Flip=%v 反向 Flip=%v，期望 %v/%v",
				i, fwd[i].Flip, rev[i].Flip, wantFwd[i], wantRev[i])
		}
	}
}

// TestExactCounts 把三类多边形的具体覆盖数、2A 与反例缺口锁进测试，
// 数字同时写进 docs/boundary-flip-rule.md，实现被改动会立刻暴露。
func TestExactCounts(t *testing.T) {
	want := []struct {
		count int
		area2 int64
	}{
		{count: 21, area2: 49},
		{count: 44, area2: 96},
		{count: 32, area2: 0},
	}
	for i, c := range classes {
		m, err := Fill(c.poly, side, side, Forward)
		if err != nil {
			t.Fatal(err)
		}
		if got := m.Count(); got != want[i].count {
			t.Fatalf("%s: 覆盖 %d，期望 %d", c.name, got, want[i].count)
		}
		a2, err := Area2(c.poly)
		if err != nil {
			t.Fatal(err)
		}
		if a2 != want[i].area2 {
			t.Fatalf("%s: 2A=%d，期望 %d", c.name, a2, want[i].area2)
		}
	}
}

// TestInputValidation 坐标与画幅的边界校验。
func TestInputValidation(t *testing.T) {
	if _, err := Fill(poly(pt(0, 0), pt(1, 0)), side, side, Forward); err == nil {
		t.Fatal("少于三个顶点应报错")
	}
	if _, err := Fill(poly(pt(0, 0), pt(1, 0), pt(0, 1)), 0, side, Forward); err == nil {
		t.Fatal("画幅为 0 应报错")
	}
	if _, err := Fill(poly(geom.Point{X: 0.5, Y: 0}, pt(1, 0), pt(0, 1)), side, side, Forward); err == nil {
		t.Fatal("非整数顶点应报错")
	}
	if _, err := Fill(poly(pt(0, 0), pt(1, 0), pt(0, 1)), side, side, Direction(9)); err == nil {
		t.Fatal("未知方向应报错")
	}
	if _, err := Area2(poly(pt(0, 0), pt(1, 0))); err == nil {
		t.Fatal("Area2 少于三个顶点应报错")
	}
}

func onlyAmong(got, want []int) bool {
	if len(got) == 0 {
		return false
	}
	for _, g := range got {
		ok := false
		for _, w := range want {
			if g == w {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	return true
}
