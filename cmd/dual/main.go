// dual 命令在同一场景上并行走整型与浮点渲染路径，按可配精度对齐中间结果，
// 逐像素比对并产出对照表、差异坐标列表和并排 PGM。
package main

import (
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"vecgo/dual"
)

func main() {
	size1 := flag.Int("size1", 32, "第一档画幅边长（方形）")
	size2 := flag.Int("size2", 128, "第二档画幅边长（方形）")
	coordBits := flag.Uint("coordbits", 16, "坐标定点小数位 Q（取整精度，可配 8..24）")
	colorBits := flag.Int("colorbits", 8, "颜色截断位深（可配 1..8）")
	ss := flag.Int("ss", 2, "超采样数")
	scale := flag.Float64("scale", 1.0, "缩放")
	rotate := flag.Float64("rotate", 23.0, "旋转角度（度）")
	gamma := flag.Float64("gamma", 1.0, "gamma")
	blur := flag.Int("blur", 0, "模糊半径")
	shading := flag.String("shading", "flat", "着色模式 flat|gradient")
	outDir := flag.String("out", "artifacts/dual", "输出目录")
	flag.Parse()

	// 同一场景：中心在原点的单位方块，旋转后产生大量非轴对齐边缘像素。
	scene := [][][2]float64{{
		{-5, -5}, {5, -5}, {5, 5}, {-5, 5},
	}}
	base := dual.Options{
		Scale:       *scale,
		Rotate:      *rotate * math.Pi / 180,
		Supersample: *ss,
		Shading:     *shading,
		Top:         1.0,
		Bottom:      0.55,
		Gamma:       *gamma,
		Blur:        *blur,
		CoordBits:   *coordBits,
		ColorBits:   *colorBits,
	}
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fail(err)
	}

	sizes := []int{*size1, *size2}
	type runSet struct {
		size                  int
		intR, floatR, nativeR *dual.Result
		alignedRep, nativeRep dual.Report
	}
	sets := make([]runSet, 0, len(sizes))
	var b strings.Builder

	fmt.Fprintf(&b, "# 双路径逐像素比对报告\n\n")
	fmt.Fprintf(&b, "场景：中心单位方块，旋转 %.1f°，scale=%v，ss=%d，shading=%s，gamma=%v，blur=%d\n",
		*rotate, *scale, *ss, *shading, *gamma, *blur)
	fmt.Fprintf(&b, "可配精度：坐标取整 Q=coordbits=%d（网格 2^-%d），颜色截断 colorbits=%d（步长 %d）\n\n",
		*coordBits, *coordBits, *colorBits, 1<<(8-*colorBits))

	for _, sz := range sizes {
		opt := base
		opt.Width, opt.Height = sz, sz
		ir, err := dual.IntEngine{}.Run(scene, opt)
		if err != nil {
			fail(err)
		}
		fr, err := dual.FloatEngine{}.Run(scene, opt)
		if err != nil {
			fail(err)
		}
		nr, err := dual.NativeFloatEngine{}.Run(scene, opt)
		if err != nil {
			fail(err)
		}
		set := runSet{
			size: sz, intR: ir, floatR: fr, nativeR: nr,
			alignedRep: dual.Compare(ir, fr),
			nativeRep:  dual.Compare(ir, nr),
		}
		sets = append(sets, set)
		writeArtifacts(*outDir, sz, ir, fr, nr)
	}

	// 对照表：两档画幅，每档整型/对齐浮点/原生浮点三行并排指标。
	b.WriteString("## 两档画幅对照表\n\n")
	b.WriteString("| 画幅 | 路径 | 亮像素(≥128) | 非零像素 | 总差值 | 最大单像素差 | 差异像素数 | 首个分歧阶段 |\n")
	b.WriteString("|---|---|---:|---:|---:|---:|---:|---|\n")
	for _, set := range sets {
		appendRow(&b, set.size, "整型 int64 定点", set.intR)
		appendRow(&b, set.size, "浮点（对齐）", set.floatR)
		appendNativeRow(&b, set.size, "浮点（原生基线）", set.nativeR, set.nativeRep)
		appendCompareRows(&b, set.size, "对齐比对(整型 vs 对齐浮点)", set.alignedRep)
		appendCompareRows(&b, set.size, "基线比对(整型 vs 原生浮点)", set.nativeRep)
	}
	b.WriteString("\n> “—”表示该行为单路径统计，差值列见紧随其后的比对行。\n\n")

	// 对齐结论与（若有）差异坐标。
	b.WriteString("## 对齐后逐像素比对（整型 vs 对齐浮点）\n\n")
	for _, set := range sets {
		fmt.Fprintf(&b, "### 画幅 %dx%d\n\n", set.size, set.size)
		writeReportBody(&b, set.alignedRep)
	}

	// 未对齐基线：展示“差一档”的差异并归因到具体步骤。
	b.WriteString("## 未对齐基线（整型 vs 原生浮点，不做中间量化）\n\n")
	for _, set := range sets {
		fmt.Fprintf(&b, "### 画幅 %dx%d\n\n", set.size, set.size)
		writeReportBody(&b, set.nativeRep)
	}

	reportPath := filepath.Join(*outDir, "report.md")
	if err := os.WriteFile(reportPath, []byte(b.String()), 0o644); err != nil {
		fail(err)
	}
	fmt.Print(b.String())
	fmt.Printf("产物目录: %s\n", *outDir)
}

func appendRow(b *strings.Builder, size int, name string, r *dual.Result) {
	on, nz := countStats(r.Final)
	fmt.Fprintf(b, "| %dx%d | %s | %d | %d | — | — | — | 见下方比对 |\n",
		size, size, name, on, nz)
}

func appendCompareRows(b *strings.Builder, size int, name string, rep dual.Report) {
	stage := rep.FirstStage
	if stage == "" {
		stage = "无（逐像素相同）"
	}
	fmt.Fprintf(b, "| %dx%d | %s | — | — | %d | %d | %d | %s |\n",
		size, size, name, rep.Total(), rep.MaxDelta, rep.DiffCount, stage)
}

func appendNativeRow(b *strings.Builder, size int, name string, r *dual.Result, rep dual.Report) {
	on, nz := countStats(r.Final)
	stage := rep.FirstStage
	if stage == "" {
		stage = "无（逐像素相同）"
	}
	fmt.Fprintf(b, "| %dx%d | %s | %d | %d | %d | %d | %d | %s |\n",
		size, size, name, on, nz, rep.Total(), rep.MaxDelta, rep.DiffCount, stage)
}

func writeReportBody(b *strings.Builder, rep dual.Report) {
	if rep.Identical() {
		fmt.Fprintf(b, "结论：**逐像素相同**，%d 个像素全部一致。\n\n", rep.W*rep.H)
		return
	}
	fmt.Fprintf(b, "结论：存在 %d 个差异像素（占 %d 的 %.2f%%），最大单像素差 %d，首个分歧阶段 `%s`。\n\n",
		rep.DiffCount, rep.W*rep.H, 100*float64(rep.DiffCount)/float64(rep.W*rep.H),
		rep.MaxDelta, stageName(rep.FirstStage))
	b.WriteString("按差异大小排序，前 20 个差异坐标（x,y）：\n\n")
	b.WriteString("| 排名 | 坐标 (x,y) | 整型值 | 另一路径值 | 差值 | 分歧阶段 |\n")
	b.WriteString("|---:|---|---:|---:|---:|---|\n")
	for i, d := range rep.Diffs {
		fmt.Fprintf(b, "| %d | (%d,%d) | %d | %d | %d | %s |\n",
			i+1, d.X, d.Y, d.A, d.B, d.Delta, stageName(d.Stage))
	}
	b.WriteString("\n精度归因说明：\n")
	b.WriteString(explain(rep.FirstStage))
	b.WriteString("\n")
}

func stageName(s string) string {
	if s == "" {
		return "无"
	}
	return s
}

func explain(stage string) string {
	switch stage {
	case "coverage":
		return "- 分歧首次出现在 `coverage`（光栅覆盖度）：采样坐标或点在多边形内判定的精度不足。\n" +
			"  整型路径的采样点被量化到 2^-Q 网格，原生浮点保留全精度，边缘子采样命中数差一档。\n" +
			"  调大 coordbits 可收窄该差异；对齐浮点路径因采样点量化到同一网格而无此分歧。\n"
	case "shaded":
		return "- 分歧首次出现在 `shaded`（着色）：插值/强度的舍入方向或颜色位深不足。\n" +
			"  两条路径需对同一强度做同向截断，并统一 colorbits。\n"
	case "gamma":
		return "- 分歧首次出现在 `gamma`：gamma 查找表的表项截断位深不一致。\n"
	case "blur":
		return "- 分歧首次出现在 `blur`：邻域均值的整数除法与浮点除法舍入方向不同。\n"
	default:
		return "- 全阶段无分歧。\n"
	}
}

func writeArtifacts(dir string, size int, ir, fr, nr *dual.Result) {
	tag := fmt.Sprintf("%d", size)
	must(dual.SavePGM(ir.Final, filepath.Join(dir, "int_"+tag+".pgm")))
	must(dual.SavePGM(fr.Final, filepath.Join(dir, "float_aligned_"+tag+".pgm")))
	must(dual.SavePGM(nr.Final, filepath.Join(dir, "float_native_"+tag+".pgm")))
	must(dual.SavePGM(dual.SideBySide(ir.Final, fr.Final),
		filepath.Join(dir, "side_by_side_aligned_"+tag+".pgm")))
	must(dual.SavePGM(dual.SideBySide(ir.Final, nr.Final),
		filepath.Join(dir, "side_by_side_native_"+tag+".pgm")))
	must(dual.SavePGM(dual.DiffMap(ir.Final, fr.Final),
		filepath.Join(dir, "diff_aligned_"+tag+".pgm")))
	must(dual.SavePGM(dual.DiffMap(ir.Final, nr.Final),
		filepath.Join(dir, "diff_native_"+tag+".pgm")))
}

func countStats(p dual.Pix) (on, nz int) {
	for _, v := range p.V {
		if v >= 128 {
			on++
		}
		if v != 0 {
			nz++
		}
	}
	return
}

func must(err error) {
	if err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "失败:", err)
	os.Exit(1)
}
