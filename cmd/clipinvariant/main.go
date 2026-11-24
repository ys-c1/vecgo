// Command clipinvariant 逐框核对裁剪前后的欧拉不变量，
// 并对每个裁剪框单独统计退化顶点。
//
// 用法：
//
//	go run ./cmd/clipinvariant           # 打印报告
//	go run ./cmd/clipinvariant -fail     # 不变量被破坏时以非零码退出
package main

import (
	"flag"
	"fmt"
	"os"

	"vecgo/geom"
)

func p(x, y float64) geom.Point { return geom.Point{X: x, Y: y} }

// 被测多边形：轴对齐正方形（凸、逆时针）。
var subject = []geom.Point{p(0, 0), p(10, 0), p(10, 10), p(0, 10)}

// 三类裁剪框，按与多边形的相对位置划分：
//
//	A 包含型：窗口完整包住主体，不切边；
//	B 穿透型：窗口边逐条切过主体，每条被切边产生一个交点；
//	C 贴边退化型：窗口边与主体边共线、顶点落在窗口边上或只触到一角。
var windows = []struct {
	Class string
	Name  string
	Win   []geom.Point
}{
	{"A 包含型", "A1 全包", []geom.Point{p(-2, -2), p(12, -2), p(12, 12), p(-2, 12)}},
	{"A 包含型", "A2 等大", []geom.Point{p(0, 0), p(10, 0), p(10, 10), p(0, 10)}},

	{"B 穿透型", "B1 菱形切角", []geom.Point{p(5, -1), p(11, 5), p(5, 11), p(-1, 5)}},
	{"B 穿透型", "B2 切三条边", []geom.Point{p(2, -5), p(12, -5), p(12, 5), p(2, 5)}},
	{"B 穿透型", "B3 斜三角窗", []geom.Point{p(-1, 2), p(5, 2), p(5, 8), p(-1, 8)}},

	{"C 贴边退化型", "C1 边共线", []geom.Point{p(0, 0), p(10, 0), p(5, 5), p(-1, 5)}},
	{"C 贴边退化型", "C2 只触一角", []geom.Point{p(10, 10), p(20, 10), p(20, 20), p(10, 20)}},
	{"C 贴边退化型", "C3 触整条边", []geom.Point{p(10, 0), p(20, 0), p(20, 10), p(10, 10)}},
	{"C 贴边退化型", "C4 顶点落边", []geom.Point{p(5, 0), p(15, 0), p(15, 10), p(5, 10)}},
}

func main() {
	failOnViolation := flag.Bool("fail", false, "有任何框违反不变量时以退出码 1 结束")
	flag.Parse()

	fmt.Printf("主体: %v  (凸=%v, 有向面积=%.1f)\n\n", subject, geom.IsConvex(subject), geom.SignedArea(subject))
	fmt.Println("约定：V=环上顶点表长度(重复按次计)，E=不同无向边数(零长边不计)，F=1+有界面数；")
	fmt.Println("      不变量 V-E+F=2。先报 SH 原始流，再报清理后结果；ΔV/ΔE 相对“裁剪前的输入”。")
	fmt.Println()

	violations := 0
	type classAgg struct {
		count, removed, bad int
		kinds               map[geom.DegenerateKind]int
	}
	byClass := map[string]*classAgg{}
	order := []string{}

	for _, w := range windows {
		d := geom.ClipReport(subject, w.Win)
		if _, ok := byClass[w.Class]; !ok {
			order = append(order, w.Class)
			byClass[w.Class] = &classAgg{kinds: map[geom.DegenerateKind]int{}}
		}
		agg := byClass[w.Class]
		agg.count++
		agg.removed += len(d.Removed)
		for k, n := range geom.KindCounts(d.Removed) {
			agg.kinds[k] += n
		}

		fmt.Printf("── %s / %s ──\n", w.Class, w.Name)
		fmt.Printf("  窗口: %v\n", w.Win)
		if d.Err != nil {
			fmt.Printf("  错误: %v\n\n", d.Err)
			continue
		}
		fmt.Printf("  SH 原始流: %s  顶点表=%v\n", d.RawCounts, d.Raw)
		fmt.Printf("  清理后:    %s  顶点表=%v\n", d.After, d.Clean)
		fmt.Printf("  前: %s\n", d.Before)
		fmt.Printf("  后: %s   差 ΔV=%+d ΔE=%+d Δχ=%+d\n",
			d.After, d.After.V-d.Before.V, d.After.E-d.Before.E, d.After.Chi-d.Before.Chi)
		fmt.Printf("  退化顶点数=%d  类别=%s\n", len(d.Removed), geom.KindSummary(d.Removed))
		fmt.Printf("  退化顶点坐标(≤5): %s\n", geom.Coords(d.Removed, 5))
		if len(d.Residual) > 0 {
			fmt.Printf("  残留退化=%d 类别=%s 坐标(≤5): %s\n",
				len(d.Residual), geom.KindSummary(d.Residual), geom.Coords(d.Residual, 5))
		}
		if err := d.CheckInvariant(w.Name); err != nil {
			violations++
			agg.bad++
			fmt.Printf("  ✗ 不变量: %v\n", err)
		} else if !d.HasArea() {
			fmt.Printf("  ✓ 不变量: χ=2，但无面积（无覆盖）——按空结果处理，不算有效面\n")
		} else {
			fmt.Printf("  ✓ 不变量: %s\n", geom.Explain(d.After, d.Removed))
		}
		fmt.Println()
	}

	fmt.Println("═══ 按裁剪框类别汇总 ═══")
	for _, cls := range order {
		agg := byClass[cls]
		fmt.Printf("%s: %d 框, 退化顶点共 %d 个, 违反不变量 %d 框\n",
			cls, agg.count, agg.removed, agg.bad)
		var degs []geom.DegenerateVertex
		for k, n := range agg.kinds {
			for i := 0; i < n; i++ {
				degs = append(degs, geom.DegenerateVertex{Kind: k})
			}
		}
		for _, k := range geom.SortedKinds(degs) {
			fmt.Printf("    - %s ×%d\n", k, agg.kinds[k])
		}
	}

	fmt.Println()
	if violations == 0 {
		fmt.Println("结论: 全部框清理后满足 V-E+F=2，无残缺陷。")
	} else {
		fmt.Printf("结论: %d 个框违反不变量，见上方 ✗ 行。\n", violations)
		if *failOnViolation {
			os.Exit(1)
		}
	}
}
