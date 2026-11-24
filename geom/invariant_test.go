package geom

import (
	"fmt"
	"testing"
)

// 三类裁剪框（按与多边形的相对位置划分）：
//
//	A 包含型：窗口完整包住多边形，裁剪不产生新顶点，V/E 不变；
//	B 穿透型：窗口边逐条切过多边形，每条被切边产生 1 个交点，V、E 同增；
//	C 贴边/退化型：窗口边与多边形边共线、顶点落在窗口边上或只触到一角，
//	  SH 会产出重复相邻点与零面积段，是退化顶点的主要来源。
//
// 每个框都检查：清理后 V-E+F=2；ΔV、ΔE；退化顶点数与类别；
// 退化坐标最多列 5 个。
func TestEulerInvariantAcrossThreeWindowClasses(t *testing.T) {
	square := []Point{{0, 0}, {10, 0}, {10, 10}, {0, 10}}

	cases := []struct {
		class          string
		win            []Point
		wantDegenerate bool // 该类预期是否出现退化顶点
	}{
		{"A 包含型", []Point{{-2, -2}, {12, -2}, {12, 12}, {-2, 12}}, false},
		{"B 穿透型", []Point{{5, -1}, {11, 5}, {5, 11}, {-1, 5}}, false},
		{"C 贴边/退化型", []Point{{0, 0}, {10, 0}, {5, 5}, {-1, 5}}, true},
	}

	for _, c := range cases {
		d := ClipReport(square, c.win)
		if d.Err != nil {
			t.Fatalf("%s 裁剪报错: %v", c.class, d.Err)
		}
		t.Logf("%s 窗口=%v", c.class, c.win)
		t.Logf("  SH 原始流: %s", d.RawCounts)
		t.Logf("  清理后:    %s  ΔV=%+d ΔE=%+d",
			d.After, d.After.V-d.Before.V, d.After.E-d.Before.E)
		t.Logf("  退化顶点数=%d 类别=%s", len(d.Removed), kindSummary(d.Removed))
		t.Logf("  退化坐标(≤5): %s", Coords(d.Removed, 5))

		if err := d.CheckInvariant("square/" + c.class); err != nil {
			t.Fatalf("%s 违反不变量: %v", c.class, err)
		}
		// 不变量必须真正成立，而不只是被“空环/单点环”绕过。
		if !d.HasArea() {
			t.Fatalf("%s 裁剪结果应围出正面积", c.class)
		}
		if (len(d.Removed) > 0) != c.wantDegenerate {
			t.Fatalf("%s 退化顶点数=%d，与预期不符", c.class, len(d.Removed))
		}
	}
}

// 三类框各自逐框核对，防止某一类被顺带通过。三类都必须满足不变量，
// 且 C 类必须确实统计出退化顶点。
func TestThreeClassesEachSatisfyInvariant(t *testing.T) {
	square := []Point{{0, 0}, {10, 0}, {10, 10}, {0, 10}}
	windows := map[string][]Point{
		"A 包含":  {{-2, -2}, {12, -2}, {12, 12}, {-2, 12}},
		"B 穿透":  {{2, -5}, {12, -5}, {12, 5}, {2, 5}},
		"C 触整边": {{10, 0}, {20, 0}, {20, 10}, {10, 10}},
	}
	for cls, w := range windows {
		d := ClipReport(square, w)
		if err := d.CheckInvariant(cls); err != nil {
			t.Fatalf("%s: %v", cls, err)
		}
	}
}

// C 类里 SH 的原始流确实违反不变量（V 多算、χ≠2）——说明欧拉公式
// 只有在清理后才成立，且“原始流顶点数”本身就是膨胀的来源。
func TestRawClipViolatesBeforeCleanup(t *testing.T) {
	square := []Point{{0, 0}, {10, 0}, {10, 10}, {0, 10}}
	cases := []struct {
		name string
		win  []Point
	}{
		{"边共线", []Point{{0, 0}, {10, 0}, {5, 5}, {-1, 5}}},
		{"只触一角", []Point{{10, 10}, {20, 10}, {20, 20}, {10, 20}}},
		{"触整条边", []Point{{10, 0}, {20, 0}, {20, 10}, {10, 10}}},
	}
	for _, c := range cases {
		raw, err := ClipRaw(square, c.win)
		if err != nil {
			t.Fatal(err)
		}
		rawCounts := EulerOf(raw)
		if rawCounts.Delta == 0 {
			t.Fatalf("%s: 原始流应违反不变量（Δ≠0），实际 %s", c.name, rawCounts)
		}
		clean := CleanPolygon(raw)
		if !EulerOf(clean.Ring).EulerSatisfied() {
			t.Fatalf("%s: 清理后应满足不变量，实际 %s", c.name, EulerOf(clean.Ring))
		}
		if len(clean.Removed) == 0 {
			t.Fatalf("%s: 应至少清理出一个退化顶点", c.name)
		}
	}
}

// 清理必须先合并、再删冗余、再合并：删除一个共线/尖刺点后两侧可能
// 变成新的相邻重复点，若阶段顺序固定就会漏掉。这里用一个会触发二次
// 重复的输入兜住该回退。
func TestCleanupReachesFixpoint(t *testing.T) {
	ring := []Point{{0, 0}, {5, 0}, {5, 0}, {10, 0}, {10, 10}, {0, 10}}
	res := CleanPolygon(ring)
	if !EulerOf(res.Ring).EulerSatisfied() {
		t.Fatalf("清理后应满足不变量: %s 环=%v", EulerOf(res.Ring), res.Ring)
	}
	if len(res.Ring) != 4 {
		t.Fatalf("应清理为 4 个顶点的矩形，实际 %d: %v", len(res.Ring), res.Ring)
	}
}

// 折返尖刺（a→b→a）不围面积，必须被删除；它既是“共线”又不是“中间点”，
// 只删中间点的实现会漏掉。
func TestSpikeRemoved(t *testing.T) {
	ring := []Point{{0, 0}, {10, 0}, {12, 0}, {10, 0}, {10, 10}, {0, 10}}
	res := CleanPolygon(ring)
	for _, d := range res.Removed {
		if d.Kind == KindSpike {
			return
		}
	}
	t.Fatalf("应识别出折返尖刺，实际清理记录=%v 环=%v", res.Removed, res.Ring)
}

// 每个被清理的退化顶点必须归入唯一类别，类别计数之和等于总数。
func TestDegenerateCountsSumByKind(t *testing.T) {
	square := []Point{{0, 0}, {10, 0}, {10, 10}, {0, 10}}
	windows := [][]Point{
		{{0, 0}, {10, 0}, {5, 5}, {-1, 5}},
		{{10, 10}, {20, 10}, {20, 20}, {10, 20}},
		{{0, 0}, {10, 0}, {10, 5}, {0, 5}},
	}
	for i, w := range windows {
		d := ClipReport(square, w)
		sum := 0
		for _, n := range KindCounts(d.Removed) {
			sum += n
		}
		if sum != len(d.Removed) {
			t.Fatalf("框 %d 类别计数之和不等于总数: %d vs %d", i, sum, len(d.Removed))
		}
	}
}

// 共线中间点会同时抬 V 和 E，欧拉差仍为 0——只报 χ 会漏掉它。
// 本测试确认 CleanPolygon 的 Removed 抓到了这类点；这是仅靠欧拉公式
// 发现不了的退化，必须靠逐点类别统计补上。
func TestCollinearVerticesInvisibleToEulerAlone(t *testing.T) {
	ring := []Point{{0, 0}, {5, 0}, {10, 0}, {10, 10}, {0, 10}}
	before := EulerOf(ring)
	if before.Delta != 0 {
		t.Fatalf("共线顶点的欧拉差应为 0（V 与 E 同时 +1），实际 %s", before)
	}
	res := CleanPolygon(ring)
	if len(res.Removed) != 1 || res.Removed[0].Kind != KindCollinear {
		t.Fatalf("应清理 1 个共线顶点，实际 %v", res.Removed)
	}
	if !EulerOf(res.Ring).EulerSatisfied() {
		t.Fatalf("清理后应满足不变量: %s", EulerOf(res.Ring))
	}
}

// 空结果不是“不变量成立”，而是“无覆盖”；Applicable 必须区分二者。
func TestEmptyRingNotApplicable(t *testing.T) {
	c := EulerOf(nil)
	if c.Applicable() {
		t.Fatalf("空环不应声称满足不变量: %s", c)
	}
	d := ClipReport([]Point{{0, 0}, {1, 0}, {0, 1}}, []Point{{5, 5}, {6, 5}, {6, 6}, {5, 6}})
	if d.Err != nil {
		t.Fatal(d.Err)
	}
	if d.HasArea() || len(d.Clean) != 0 {
		t.Fatalf("完全在外的多边形应得到空环，实际 %v", d.Clean)
	}
	if d.CheckInvariant("empty") != nil {
		t.Fatal("空环不应被当作违反不变量（应作为无覆盖单独报告）")
	}
}

// 凸主体在三类窗口下的全部扫描应无一违反不变量；若回退，立即报出是
// 哪个窗口、差多少、哪一类。
func TestConvexSweepAllSatisfy(t *testing.T) {
	subjects := map[string][]Point{
		"square":  {{0, 0}, {10, 0}, {10, 10}, {0, 10}},
		"tri":     {{0, 0}, {10, 2}, {3, 9}},
		"hexagon": {{2, 0}, {8, 0}, {10, 5}, {8, 10}, {2, 10}, {0, 5}},
		"collin":  {{0, 0}, {5, 0}, {10, 0}, {10, 10}, {0, 10}},
	}
	for name, poly := range subjects {
		if !IsConvex(poly) {
			t.Fatalf("%s 应是凸主体", name)
		}
		for cx := -3; cx <= 13; cx++ {
			for cy := -3; cy <= 13; cy++ {
				for r := 1; r <= 2; r++ {
					x, y, s := float64(cx), float64(cy), float64(2*r)
					windows := [][]Point{
						{{x, y}, {x + s, y}, {x + s, y + s}, {x, y + s}},
						{{x, y}, {x + s, y}, {x, y + s}},
					}
					for _, w := range windows {
						d := ClipReport(poly, w)
						if d.Err != nil {
							t.Fatalf("%s 窗口 %v 报错: %v", name, w, d.Err)
						}
						if len(d.Clean) == 0 {
							continue // 无覆盖
						}
						if err := d.CheckInvariant(name); err != nil {
							t.Fatalf("%s 窗口 %v: %v", name, w, err)
						}
					}
				}
			}
		}
	}
}

// 凹主体不是 SH 的适用范围：裁剪会在凹角处产生桥接，形成捏点/T 型接触。
// 要求工具必须*报出来*，而不是静默当作合法结果。
func TestConcaveSubjectIsReportedNotSilent(t *testing.T) {
	concave := []Point{{0, 0}, {10, 0}, {10, 10}, {6, 6}, {4, 6}, {0, 10}}
	if IsConvex(concave) {
		t.Fatal("该主体应为凹")
	}
	d := ClipReport(concave, []Point{{3, 6}, {7, 6}, {7, 10}, {3, 10}})
	if d.Err != nil {
		t.Fatal(d.Err)
	}
	if err := d.CheckInvariant("concave"); err == nil {
		t.Fatalf("凹主体裁剪应被标记为不满足简单环前提，实际静默通过: %s", d.After)
	}
	if !HasTopologicalDefect(d.Residual) {
		t.Fatalf("应报出残留拓扑缺陷（T 型接触/重叠边），实际 %v", d.Residual)
	}
}

// 退化顶点坐标必须报出原始坐标，且最多 5 个。
func TestCoordsCapAtFive(t *testing.T) {
	degs := make([]DegenerateVertex, 9)
	for i := range degs {
		degs[i] = DegenerateVertex{Index: i, P: Point{float64(i), float64(i)}, Kind: KindDuplicate}
	}
	out := Coords(degs, 5)
	for i := 0; i < 5; i++ {
		want := fmt.Sprintf("(%.9g,%.9g)", float64(i), float64(i))
		if !contains(out, want) {
			t.Fatalf("坐标文本缺少 %s: %s", want, out)
		}
	}
	if contains(out, "(7,7)") {
		t.Fatalf("超出 5 个仍在输出: %s", out)
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
