// Command fillreport prints the even-odd vs non-zero comparison for the study's
// shape library: covered-pixel counts, crossing-direction statistics, and the
// witness crossings that explain each disagreement pattern.
package main

import (
	"fmt"
	"os"

	"vecgo/fillstudy"
	"vecgo/geom"
)

func main() {
	const width, height, ss = 64, 64, 2
	var all []fillstudy.RuleStats
	for _, shape := range fillstudy.ShapeLibrary() {
		stats, err := fillstudy.Measure(shape, width, height, ss)
		if err != nil {
			fmt.Fprintln(os.Stderr, "失败:", err)
			os.Exit(1)
		}
		all = append(all, stats)
		report(stats)
	}

	fmt.Println("\n== 覆盖像素数 (threshold 128, 64x64, supersample 2) ==")
	fmt.Printf("%-24s %8s %8s %8s %10s %10s %12s\n",
		"shape", "non-zero", "even-odd", "差值", "任意覆盖差", "不一致像素", "覆盖当量差")
	for _, s := range all {
		fmt.Printf("%-24s %8d %8d %8d %10d %10d %12.3f\n",
			s.Shape.Name, s.NonZeroOn, s.EvenOddOn, s.DeltaOn(), s.DeltaAny(),
			s.DisagreePixels, s.CoverageDelta())
	}
}

func report(s fillstudy.RuleStats) {
	fmt.Printf("\n===== %s =====\n", s.Shape.Name)
	fmt.Printf("为什么自交: %s\n", s.Shape.Why)
	fmt.Printf("采样框: min=(%.3f,%.3f) span=(%.3f,%.3f) 图=%dx%d supersample=%d\n",
		s.Frame.MinX, s.Frame.MinY, s.Frame.SpanX, s.Frame.SpanY, s.Width, s.Height, s.Supersample)
	fmt.Printf("覆盖像素(non-zero / even-odd) = %d / %d, 差值 %+d\n", s.NonZeroOn, s.EvenOddOn, s.DeltaOn())
	fmt.Printf("任意覆盖像素 = %d / %d, 差值 %+d\n", s.NonZeroAny, s.EvenOddAny, s.DeltaAny())
	fmt.Printf("不一致样本 = %d, 不一致像素 = %d\n", s.DisagreeSamples, s.DisagreePixels)

	fmt.Println("方向统计 (交叉方向序列 -> 样本数, 绕数 W, 满足 W!=0 且 W 偶):")
	for _, w := range s.Witnesses {
		fmt.Printf("  [%s]  n=%-5d W=%+d  条件=%v\n",
			w.Pattern, s.PatternHistogram[w.Pattern], w.W, geom.DisagreesByDirections(w.Directions))
	}

	fmt.Println("不一致样例 (坐标 + 造成不一致的交点):")
	for _, w := range s.Witnesses {
		fmt.Printf("  pixel(%d,%d) sample(%d,%d) 世界坐标=(%.3f, %.3f) 方向=[%s] W=%+d\n",
			w.Col, w.Row, w.SI, w.SJ, w.P.X, w.P.Y, w.Pattern, w.W)
		fmt.Print("    射线交点: ")
		for k, c := range w.Crossings {
			if k > 0 {
				fmt.Print(", ")
			}
			fmt.Printf("(段%d 于 (%.3f,%.3f) 方向%+d)", c.Index, c.Point.X, c.Point.Y, c.Dir)
		}
		fmt.Println()
		fmt.Print("    造成不一致的交点(未配对，方向统计中被奇偶抵消的): ")
		for k, c := range w.Unpaired {
			if k > 0 {
				fmt.Print(", ")
			}
			fmt.Printf("段%d (%.3f,%.3f) 方向%+d", c.Index, c.Point.X, c.Point.Y, c.Dir)
		}
		fmt.Println()
	}
}
