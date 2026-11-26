// Command scanlab 复现“凸多边形逐行增量扫描”的全部实验数据：
// 等价性核对、三组形状的更新量/按行边界检查计数/耗时、非凸反例。
//
//	go run ./cmd/scanlab
package main

import (
	"bytes"
	"fmt"
	"math"
	"time"

	"vecgo/geom"
	"vecgo/scan"
)

func main() {
	groups := scan.Groups()

	fmt.Println("================ 1. 逐像素等价性 ================")
	for _, g := range groups {
		a, _ := scan.Recompute(g.Rep, scan.CanvasW, scan.CanvasH)
		b, _ := scan.Incremental(g.Rep, scan.CanvasW, scan.CanvasH)
		d := scan.DiffPixels(a, b, scan.CanvasW, scan.CanvasH)
		fmt.Printf("%-16s 代表形状: 差异像素 %d\n", g.Name, len(d))
		bad := 0
		for k, p := range g.Corpus {
			x, _ := scan.Recompute(p, scan.CanvasW, scan.CanvasH)
			y, _ := scan.Incremental(p, scan.CanvasW, scan.CanvasH)
			if len(scan.DiffPixels(x, y, scan.CanvasW, scan.CanvasH)) != 0 {
				bad++
			}
			_ = k
		}
		fmt.Printf("%-16s 语料 %d 个形状: 不一致 %d\n", g.Name, len(g.Corpus), bad)
	}

	fmt.Println()
	fmt.Println("================ 2. 更新量 Δx 与边界检查 ================")
	for _, g := range groups {
		_, cr := scan.Recompute(g.Rep, scan.CanvasW, scan.CanvasH)
		_, ci := scan.Incremental(g.Rep, scan.CanvasW, scan.CanvasH)
		fmt.Printf("\n【%s】顶点数=%d  覆盖行=%d\n", g.Name, len(g.Rep), ci.RowsCovered)
		printDeltas(g.Rep)
		fmt.Printf("  逐行重算: 边跨线检查 %d 次, 端点除法 %d 次\n", cr.EdgeTests, cr.Divisions)
		fmt.Printf("  增量式  : x+=Δ %d 次, 越界判定 %d 次 (=题目要的每组一个数), 换边 %d 次, 除法 %d 次\n",
			ci.Adds, ci.OOBChecks, ci.Switches, ci.Divisions)
		fmt.Printf("  按行越界检查分布: %s\n", histogram(ci.PerRowOOB))
		fmt.Printf("  端点到像素中心判定线的最小余量: %.3e 像素\n", minMargin(g.Rep))
	}

	fmt.Println()
	fmt.Println("================ 3. 耗时（best-of，精简热路径）================")
	fmt.Println("-- 3a. 仅逐行区间端点计算（本题优化的部分）--")
	fmt.Printf("%-16s %14s %14s %8s\n", "组别", "重算 ns/次", "增量 ns/次", "加速比")
	for _, g := range groups {
		p := g.Rep
		tRec := bestNs(func() { sinkSpans = scan.RecomputeSpans(p, scan.CanvasW, scan.CanvasH) })
		tInc := bestNs(func() { sinkSpans = scan.IncrementalSpans(p, scan.CanvasW, scan.CanvasH) })
		fmt.Printf("%-16s %14.0f %14.0f %7.2fx\n", g.Name, float64(tRec), float64(tInc), float64(tRec)/float64(tInc))
	}

	fmt.Println("-- 3b. 端到端填充（区间计算 + 写像素，缓冲复用）--")
	fmt.Printf("%-16s %14s %14s %8s\n", "组别", "重算 ns/帧", "增量 ns/帧", "加速比")
	for _, g := range groups {
		p := g.Rep
		bufR := make([]uint8, scan.CanvasW*scan.CanvasH)
		bufI := make([]uint8, scan.CanvasW*scan.CanvasH)
		tRec := bestNs(func() {
			sp := scan.RecomputeSpans(p, scan.CanvasW, scan.CanvasH)
			scan.Fill(bufR, scan.CanvasW, scan.CanvasH, sp)
		})
		tInc := bestNs(func() {
			sp := scan.IncrementalSpans(p, scan.CanvasW, scan.CanvasH)
			scan.Fill(bufI, scan.CanvasW, scan.CanvasH, sp)
		})
		fmt.Printf("%-16s %14.0f %14.0f %7.2fx\n", g.Name, float64(tRec), float64(tInc), float64(tRec)/float64(tInc))
	}
	fmt.Println("（像素写对两条路径完全相同，边越多，端点阶段节省越显著。）")

	fmt.Println()
	fmt.Println("================ 4. 非凸反例 ================")
	notch := scan.NotchPolygon()
	seq := scan.LeftEndpointSeries(notch, 8)
	fmt.Println("侧凹七边形左端点序列（行 0..7）:")
	fmt.Printf("  %v\n", f3(seq))
	fmt.Println("  -> 0→2.25 向右、2.25→0 向左，x 方向不单调；固定 Δ=0 会一直填到 x=0。")
	const nw, nh = 9, 8
	eoNotch, _ := scan.EvenOdd(notch, nw, nh)
	bad := scan.FixedDeltaWrong(notch, nw, nh)
	fmt.Println("  奇偶正确结果:")
	indent(scan.ASCII(eoNotch, nw, nh))
	fmt.Println("  固定 Δ 增量（错误，# 为多填）:")
	indent(scan.ASCII(bad, nw, nh))
	fmt.Printf("  误填像素: %v\n", scan.DiffPixels(bad, eoNotch, nw, nh))

	oh := scan.OverhangPolygon()
	const ow, ohh = 12, 11
	incOH, _ := scan.Incremental(oh, ow, ohh)
	eoOH, _ := scan.EvenOdd(oh, ow, ohh)
	recOH, _ := scan.Recompute(oh, ow, ohh)
	fmt.Println("悬挑凹多边形 y=2.5 与四条边相交；凸性增量只留两个端点 -> 抹掉凹档。")
	fmt.Printf("  增量 vs 逐行min/max重算 差异: %d 像素（增量忠实复现了两区间假设）\n",
		len(scan.DiffPixels(incOH, recOH, ow, ohh)))
	fmt.Printf("  增量 vs 奇偶正确结果     差异: %d 像素\n", len(scan.DiffPixels(incOH, eoOH, ow, ohh)))
	fmt.Println("  凸性增量/min-max 重算（第 2 行第 6 列被误填）:")
	indent(scan.ASCII(incOH, ow, ohh))
	fmt.Println("  奇偶配对正确结果:")
	indent(scan.ASCII(eoOH, ow, ohh))
}

// printDeltas 列出两条单调链上每条边的单位行增量 Δx。
func printDeltas(poly []geom.Point) {
	// 通过公开的区间统计拿不到边 Δ，这里直接按顶点做一次确定性计算展示。
	n := len(poly)
	iMin, iMax := 0, 0
	for i := 1; i < n; i++ {
		if poly[i].Y < poly[iMin].Y {
			iMin = i
		}
		if poly[i].Y > poly[iMax].Y {
			iMax = i
		}
	}
	chain := func(step int) []float64 {
		var ds []float64
		i := iMin
		for i != iMax {
			j := (i + step + n) % n
			a, b := poly[i], poly[j]
			if a.Y != b.Y {
				d := (b.X - a.X) / (b.Y - a.Y)
				ds = append(ds, d)
			}
			i = j
		}
		return ds
	}
	fmt.Printf("  链1 Δx 序列: %s\n", f3(chain(1)))
	fmt.Printf("  链2 Δx 序列: %s\n", f3(chain(-1)))
}

func f3(xs []float64) string {
	var b bytes.Buffer
	b.WriteString("[")
	for i, x := range xs {
		if i > 0 {
			b.WriteString(" ")
		}
		if x == math.Trunc(x) {
			fmt.Fprintf(&b, "%.1f", x)
		} else {
			fmt.Fprintf(&b, "%.4g", x)
		}
	}
	b.WriteString("]")
	return b.String()
}

// histogram 统计每种“单行越界检查次数”出现了多少行。
func histogram(perRow []int) string {
	m := map[int]int{}
	active := 0
	for _, v := range perRow {
		if v > 0 {
			m[v]++
			active++
		}
	}
	return fmt.Sprintf("覆盖行=%d, 各行检查次数分布=%v", active, m)
}

func minMargin(poly []geom.Point) float64 {
	spans, _ := scan.RecomputeIntervals(poly, scan.CanvasW, scan.CanvasH)
	m := math.Inf(1)
	for _, s := range spans {
		for _, x := range []float64{s.XL - 0.5, s.XR - 0.5} {
			d := math.Abs(x - math.Round(x))
			if d < m {
				m = d
			}
		}
	}
	return m
}

// sinkSpans 防止编译器把基准调用整体优化掉。
var sinkSpans []scan.Span

func bestNs(fn func()) time.Duration {
	const rounds = 9
	iters := 20000
	var best time.Duration = time.Duration(math.MaxInt64)
	for k := 0; k < rounds; k++ {
		start := time.Now()
		for i := 0; i < iters; i++ {
			fn()
		}
		d := time.Since(start)
		if d < best {
			best = d
		}
	}
	return best / time.Duration(iters)
}

func indent(s string) {
	for _, line := range splitLines(s) {
		fmt.Println("    " + line)
	}
}

func splitLines(s string) []string {
	var out []string
	cur := ""
	for _, r := range s {
		if r == '\n' {
			out = append(out, cur)
			cur = ""
		} else {
			cur += string(r)
		}
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}
