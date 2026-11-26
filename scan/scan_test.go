package scan

import (
	"math"
	"math/rand"
	"testing"

	"vecgo/geom"
)

// TestGroupsPixelIdentical 要求三组形状的代表与整个计时语料上，
// 增量实现与逐行重算逐像素相同。
func TestGroupsPixelIdentical(t *testing.T) {
	for _, g := range Groups() {
		pixA, cA := Recompute(g.Rep, CanvasW, CanvasH)
		pixB, cB := Incremental(g.Rep, CanvasW, CanvasH)
		if d := DiffPixels(pixA, pixB, CanvasW, CanvasH); len(d) != 0 {
			t.Fatalf("%s 代表形状不一致，首个差异行/列: %v（共 %d 点）", g.Name, d[0], len(d))
		}
		if cA.RowsCovered != cB.RowsCovered {
			t.Fatalf("%s 覆盖行数不一致 %d vs %d", g.Name, cA.RowsCovered, cB.RowsCovered)
		}
		for k, p := range g.Corpus {
			a, _ := Recompute(p, CanvasW, CanvasH)
			b, _ := Incremental(p, CanvasW, CanvasH)
			if d := DiffPixels(a, b, CanvasW, CanvasH); len(d) != 0 {
				t.Fatalf("%s 语料 #%d 不一致: 首异 %v 共 %d", g.Name, k, d[0], len(d))
			}
		}
	}
}

// TestRandomConvexDifferential 在大量随机凸包上做差分测试。
func TestRandomConvexDifferential(t *testing.T) {
	rng := rand.New(rand.NewSource(20261126))
	const nPoly = 4000
	skipped := 0
	for k := 0; k < nPoly; k++ {
		pts := randomConvexHull(rng, 3+rng.Intn(14))
		if len(pts) < 3 {
			continue
		}
		// 排除端点恰好压在像素中心判定线上的退化样本：
		// 浮点累加与除法在精确平局两侧可能相差 1 ulp。
		if tooCloseToDecision(pts, 64, 64, 1e-9) {
			skipped++
			continue
		}
		a, _ := Recompute(pts, 64, 64)
		b, _ := Incremental(pts, 64, 64)
		if d := DiffPixels(a, b, 64, 64); len(d) != 0 {
			t.Fatalf("随机凸包 #%d (顶点 %d) 不一致: 首异 %v 共 %d\n%v",
				k, len(pts), d[0], len(d), pts)
		}
	}
	t.Logf("随机差分完成，跳过近平局样本 %d", skipped)
}

// tooCloseToDecision 检查交点是否离列中心判定边界过近。
func tooCloseToDecision(poly []geom.Point, w, h int, eps float64) bool {
	spans, _ := RecomputeIntervals(poly, w, h)
	for _, s := range spans {
		for _, x := range []float64{s.XL, s.XR} {
			v := x - 0.5
			if math.Abs(v-math.Round(v)) < eps {
				return true
			}
		}
	}
	return false
}

// randomConvexHull 生成随机点并返回其凸包（单调链），保证凸性。
func randomConvexHull(rng *rand.Rand, n int) []geom.Point {
	pts := make([]geom.Point, n)
	for i := range pts {
		pts[i] = geom.Point{
			X: 8 + rng.Float64()*48,
			Y: 8 + rng.Float64()*48,
		}
	}
	return convexHull(pts)
}

func cross(o, a, b geom.Point) float64 {
	return (a.X-o.X)*(b.Y-o.Y) - (a.Y-o.Y)*(b.X-o.X)
}

func convexHull(p []geom.Point) []geom.Point {
	// 按 x 再 y 排序（去重）。
	pts := append([]geom.Point{}, p...)
	for i := 0; i < len(pts); i++ {
		for j := i + 1; j < len(pts); j++ {
			if pts[j].X < pts[i].X || (pts[j].X == pts[i].X && pts[j].Y < pts[i].Y) {
				pts[i], pts[j] = pts[j], pts[i]
			}
		}
	}
	uniq := pts[:0]
	for _, q := range pts {
		if len(uniq) == 0 || q != uniq[len(uniq)-1] {
			uniq = append(uniq, q)
		}
	}
	pts = uniq
	if len(pts) < 3 {
		return pts
	}
	lower := []geom.Point{}
	for _, q := range pts {
		for len(lower) >= 2 && cross(lower[len(lower)-2], lower[len(lower)-1], q) <= 0 {
			lower = lower[:len(lower)-1]
		}
		lower = append(lower, q)
	}
	upper := []geom.Point{}
	for i := len(pts) - 1; i >= 0; i-- {
		q := pts[i]
		for len(upper) >= 2 && cross(upper[len(upper)-2], upper[len(upper)-1], q) <= 0 {
			upper = upper[:len(upper)-1]
		}
		upper = append(upper, q)
	}
	return append(lower[:len(lower)-1], upper[:len(upper)-1]...)
}

// TestCounterexampleOverhang 表明：增量凸性法与逐行 min/max 重算
// 彼此逐像素相同，但在悬挑非凸形状上一起错；奇偶参照能抓到差异。
func TestCounterexampleOverhang(t *testing.T) {
	p := OverhangPolygon()
	const w, h = 12, 11
	rec, _ := Recompute(p, w, h)
	inc, _ := Incremental(p, w, h)
	eo, _ := EvenOdd(p, w, h)

	if d := DiffPixels(rec, inc, w, h); len(d) != 0 {
		t.Fatalf("增量法应忠实复现 min/max 重算，却在 %v 不同", d[0])
	}
	d := DiffPixels(inc, eo, w, h)
	if len(d) == 0 {
		t.Fatal("悬挑反例上增量凸性法应当与奇偶正确结果不同")
	}
	found := false
	for _, q := range d {
		if q[0] == 2 && q[1] == 6 {
			found = true
		}
	}
	if !found {
		t.Fatalf("预期第 2 行第 6 列被误填，实际差异: %v", d)
	}
}

// TestNotchMonotonicityAndFixedDelta 验证侧凹形状的端点序列反转，
// 且“永远 x+=Δ”的最简增量写法确实出错；带越界换边的完整增量法
// 在这个 y 仍单调的凹形上反而仍与奇偶结果一致。
func TestNotchMonotonicityAndFixedDelta(t *testing.T) {
	p := NotchPolygon()
	seq := LeftEndpointSeries(p, 8)
	want := []float64{0, 0, 0.75, 2.25, 2.25, 0.75, 0, 0}
	if len(seq) != len(want) {
		t.Fatalf("端点序列长度 %d，应为 %d", len(seq), len(want))
	}
	for i := range want {
		if math.Abs(seq[i]-want[i]) > 1e-12 {
			t.Fatalf("第 %d 行左端点 %v，应为 %v（全序列 %v）", i, seq[i], want[i], seq)
		}
	}
	reversed := false
	for i := 2; i+1 < len(seq); i++ {
		if seq[i+1] < seq[i] {
			reversed = true
		}
	}
	if !reversed {
		t.Fatal("端点序列应出现下降，即 x 方向反转")
	}

	const w, h = 9, 8
	bad := FixedDeltaWrong(p, w, h)
	eo, _ := EvenOdd(p, w, h)
	wantDiff := [][2]int{{2, 0}, {3, 0}, {3, 1}, {4, 0}, {4, 1}, {5, 0}}
	if d := DiffPixels(bad, eo, w, h); len(d) != len(wantDiff) {
		t.Fatalf("固定 Δ 写法应恰好误填 %d 点，实际 %d: %v", len(wantDiff), len(d), d)
	} else {
		for i, q := range wantDiff {
			if d[i] != q {
				t.Fatalf("第 %d 个误填点应为 %v，实际 %v（全部 %v）", i, q, d[i], d)
			}
		}
	}
	inc, _ := Incremental(p, w, h)
	if d := DiffPixels(inc, eo, w, h); len(d) != 0 {
		t.Fatalf("带越界换边的增量法在该 y-单调凹形上应仍正确，差异 %v", d)
	}
}

// TestBoundaryCheckCounts 锁定“按行统计”的越界判定总数：
// 三角形 320、平行四边形 240、凸十六边形 334（160 行×2 链 + 14 次换边复检）。
func TestBoundaryCheckCounts(t *testing.T) {
	want := []struct {
		total   int
		rows2   int // 恰好 2 次检查的行数
		rows4   int // 恰好 4 次检查的行数（换边行）
		covered int
	}{
		{total: 320, rows2: 160, covered: 160},
		{total: 240, rows2: 120, covered: 120},
		{total: 334, rows2: 153, rows4: 7, covered: 160},
	}
	for i, g := range Groups() {
		_, c := IncrementalIntervals(g.Rep, CanvasW, CanvasH)
		w := want[i]
		if c.OOBChecks != w.total {
			t.Fatalf("%s 越界检查总数 %d，应为 %d", g.Name, c.OOBChecks, w.total)
		}
		if c.RowsCovered != w.covered {
			t.Fatalf("%s 覆盖行 %d，应为 %d", g.Name, c.RowsCovered, w.covered)
		}
		n2, n4 := 0, 0
		for _, v := range c.PerRowOOB {
			switch v {
			case 2:
				n2++
			case 4:
				n4++
			case 0:
			default:
				t.Fatalf("%s 出现非预期的单行检查数 %d", g.Name, v)
			}
		}
		if n2 != w.rows2 || n4 != w.rows4 {
			t.Fatalf("%s 按行分布 2次行=%d(应%d) 4次行=%d(应%d)", g.Name, n2, w.rows2, n4, w.rows4)
		}
	}
}

// Benchmark 三组代表形状：区间端点阶段（精简路径）。
func BenchmarkSpans(b *testing.B) {
	for _, g := range Groups() {
		p := g.Rep
		b.Run(g.Name+"_recompute", func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				sink = RecomputeSpans(p, CanvasW, CanvasH)
			}
		})
		b.Run(g.Name+"_incremental", func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				sink = IncrementalSpans(p, CanvasW, CanvasH)
			}
		})
	}
}

// BenchmarkFill 量端到端填充（缓冲复用）。
func BenchmarkFill(b *testing.B) {
	for _, g := range Groups() {
		p := g.Rep
		buf := make([]uint8, CanvasW*CanvasH)
		b.Run(g.Name+"_recompute", func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				Fill(buf, CanvasW, CanvasH, RecomputeSpans(p, CanvasW, CanvasH))
			}
		})
		b.Run(g.Name+"_incremental", func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				Fill(buf, CanvasW, CanvasH, IncrementalSpans(p, CanvasW, CanvasH))
			}
		})
	}
}

var sink []Span
