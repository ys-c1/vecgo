// clipbias 对比两种流水线顺序的边缘覆盖率偏差：
//
//	CF (clip-first): geom.Clip 裁剪多边形 -> raster.Fill 按裁剪后 bbox 布点填充
//	FC (fill-first): raster.Fill 按原始 bbox 布点填充 -> 在同一超采样网格上按裁剪框掩膜
//
// 两者使用完全相同的超采样规则（N×N 子像元中心），差别只在 bbox 归一化带来的采样位置平移。
package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"vecgo/geom"
	"vecgo/raster"
)

const (
	imgW = 64
	imgH = 64
	ssN  = 8 // 超采样 N×N，单采样灰度量子 q = 255/N^2
)

// rect 是轴对齐矩形，本实验的多边形与裁剪框均为矩形（CCW 顶点）。
type rect struct {
	x0, y0, x1, y1 float64
}

func (r rect) pts() []geom.Point {
	return []geom.Point{
		{X: r.x0, Y: r.y0},
		{X: r.x1, Y: r.y0},
		{X: r.x1, Y: r.y1},
		{X: r.x0, Y: r.y1},
	}
}

func intersectRect(a, b rect) (rect, bool) {
	r := rect{
		x0: math.Max(a.x0, b.x0),
		y0: math.Max(a.y0, b.y0),
		x1: math.Min(a.x1, b.x1),
		y1: math.Min(a.y1, b.y1),
	}
	return r, r.x1 > r.x0 && r.y1 > r.y0
}

// gridSample 与 raster.Fill 的布点公式逐项一致：
// 第 i 个像素内第 s 个超采样取子像元中心 (i+(s+0.5)/N)，再按 bbox 归一化到世界坐标。
func gridSample(b0, span float64, size, n, i, s int) float64 {
	return b0 + (float64(i)+(float64(s)+0.5)/float64(n))*span/float64(size)
}

// renderFC 填充后裁剪：固定使用原始 bbox 布点，每个超采样点同时要求在多边形与裁剪框内。
// mask == nil 时退化为纯填充（用于与 raster.Fill 做字节级对照）。
func renderFC(poly, window []geom.Point, box rect) *raster.Image {
	im, _ := raster.New(imgW, imgH)
	den := ssN * ssN
	for row := 0; row < imgH; row++ {
		for col := 0; col < imgW; col++ {
			hit := 0
			for sj := 0; sj < ssN; sj++ {
				py := gridSample(box.y0, box.y1-box.y0, imgH, ssN, row, sj)
				for si := 0; si < ssN; si++ {
					px := gridSample(box.x0, box.x1-box.x0, imgW, ssN, col, si)
					if !geom.Inside(poly, px, py) {
						continue
					}
					if window != nil && !geom.Inside(window, px, py) {
						continue
					}
					hit++
				}
			}
			im.Pix[row*imgW+col] = uint8(255 * hit / den)
		}
	}
	return im
}

// renderCF 裁剪后填充：直接走仓库原生路径 geom.Clip + raster.Fill，
// Fill 内部按裁剪后多边形的 bbox 重新归一化布点。
func renderCF(poly, window []geom.Point) *raster.Image {
	im, _ := raster.New(imgW, imgH)
	clipped, err := geom.Clip(poly, window)
	if err != nil || len(clipped) < 3 {
		return im // 裁剪结果为空：全黑图
	}
	if err := im.Fill([][]geom.Point{clipped}, ssN); err != nil {
		panic(err)
	}
	return im
}

// renderSameGridCF 对照实验：仍用裁剪后的多边形，但把它放到 FC 的固定网格上判内外。
// 若裁剪数学本身无偏，它应与 FC 掩膜结果字节一致。
func renderSameGridCF(poly, window []geom.Point, box rect) *raster.Image {
	clipped, err := geom.Clip(poly, window)
	if err != nil || len(clipped) < 3 {
		return renderFC(poly, window, box) // 空裁剪时两者都应全黑
	}
	im, _ := raster.New(imgW, imgH)
	den := ssN * ssN
	for row := 0; row < imgH; row++ {
		for col := 0; col < imgW; col++ {
			hit := 0
			for sj := 0; sj < ssN; sj++ {
				py := gridSample(box.y0, box.y1-box.y0, imgH, ssN, row, sj)
				for si := 0; si < ssN; si++ {
					px := gridSample(box.x0, box.x1-box.x0, imgW, ssN, col, si)
					if geom.Inside(clipped, px, py) {
						hit++
					}
				}
			}
			im.Pix[row*imgW+col] = uint8(255 * hit / den)
		}
	}
	return im
}

// axisCounts 解析计数：在给定 bbox 布点规则下，每个像素轴向上落入区间 [lo,hi] 的超采样数。
// 不调用 geom.Inside，与实测路径（射线法）相互独立。
func axisCounts(b0, span, lo, hi float64, size int) []int {
	out := make([]int, size)
	for i := 0; i < size; i++ {
		cnt := 0
		for s := 0; s < ssN; s++ {
			p := gridSample(b0, span, size, ssN, i, s)
			if p >= lo && p <= hi {
				cnt++
			}
		}
		out[i] = cnt
	}
	return out
}

// predict 解析预测：矩形区域覆盖率可分离为 nx(c)*ny(r)/N^2，
// 分别在 FC 网格（原始 bbox）与 CF 网格（裁剪后 bbox）上计数后求灰度差矩阵。
func predict(region, fcBox, cfBox rect) [][]int {
	nxF := axisCounts(fcBox.x0, fcBox.x1-fcBox.x0, region.x0, region.x1, imgW)
	nyF := axisCounts(fcBox.y0, fcBox.y1-fcBox.y0, region.y0, region.y1, imgH)
	nxC := axisCounts(cfBox.x0, cfBox.x1-cfBox.x0, region.x0, region.x1, imgW)
	nyC := axisCounts(cfBox.y0, cfBox.y1-cfBox.y0, region.y0, region.y1, imgH)
	den := ssN * ssN
	diff := make([][]int, imgH)
	for r := 0; r < imgH; r++ {
		diff[r] = make([]int, imgW)
		for c := 0; c < imgW; c++ {
			// 与 raster.Fill 的 uint8(255*hit/den) 末端截断严格一致，不预截断量子
			gFC := 255 * nxF[c] * nyF[r] / den
			gCF := 255 * nxC[c] * nyC[r] / den
			diff[r][c] = absInt(gCF - gFC)
		}
	}
	return diff
}

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func minOrDash(x int) string {
	if x > 255 {
		return "无"
	}
	return fmt.Sprintf("%d", x)
}

type cell struct {
	row, col int
	fc, cf   int
}

type caseReport struct {
	name      string
	window    rect
	region    rect
	hasRegion bool
	fcImg     *raster.Image
	cfImg     *raster.Image
	cells     []cell
	maxGray   int
	predDiff  [][]int
	predCount int
	predMax   int
	mismatch  []string
	// 暗部统计（暗部定义：FC 灰度 <255，即未被亮部饱和拉平的差异像素）
	darkCells       []cell
	minDarkFC       int
	minDarkCF       int
	gapMean, gapVar float64
	rowGapMean      float64
	rowGapVar       float64
	// bbox 重映射导致的最大采样平移，以 FC 网格像素间距 h=S/W 为单位。
	// Δ(i,s) 同时含 b0 平移与 span 缩放两项，随像素位置变化，故在全网格取最大绝对值。
	shiftMaxX, shiftMaxY float64
}

// boundaryKind 判定像素最近的边界属于多边形边还是裁剪框边。
func boundaryKind(region, poly rect, row, col int, fcBox rect) string {
	x := gridSample(fcBox.x0, fcBox.x1-fcBox.x0, imgW, ssN, col, ssN/2)
	y := gridSample(fcBox.y0, fcBox.y1-fcBox.y0, imgH, ssN, row, ssN/2)
	type cand struct {
		d    float64
		kind string
	}
	cands := []cand{
		{math.Abs(x - region.x0), edgeKind(region.x0, poly.x0, poly.x1)},
		{math.Abs(x - region.x1), edgeKind(region.x1, poly.x0, poly.x1)},
		{math.Abs(y - region.y0), edgeKind(region.y1, poly.y0, poly.y1)},
		{math.Abs(y - region.y1), edgeKind(region.y1, poly.y0, poly.y1)},
	}
	sort.Slice(cands, func(i, j int) bool { return cands[i].d < cands[j].d })
	return cands[0].kind
}

func edgeKind(v, p0, p1 float64) string {
	if math.Abs(v-p0) < 1e-9 || math.Abs(v-p1) < 1e-9 {
		return "多边形边界像素"
	}
	return "裁剪框边界像素"
}

func variance(xs []float64) (mean, v float64, ok bool) {
	if len(xs) < 2 {
		return 0, 0, false
	}
	for _, x := range xs {
		mean += x
	}
	mean /= float64(len(xs))
	for _, x := range xs {
		v += (x - mean) * (x - mean)
	}
	v /= float64(len(xs) - 1) // 样本方差
	return mean, v, true
}

func runCase(name string, poly, window rect, outDir string) caseReport {
	rep := caseReport{name: name, window: window}
	polyPts := poly.pts()
	winPts := window.pts()

	rep.fcImg = renderFC(polyPts, winPts, poly)
	rep.cfImg = renderCF(polyPts, winPts)

	region, ok := intersectRect(poly, window)
	rep.region, rep.hasRegion = region, ok
	hx := (poly.x1 - poly.x0) / float64(imgW)
	hy := (poly.y1 - poly.y0) / float64(imgH)
	if ok {
		for i := 0; i < imgW; i++ {
			for s := 0; s < ssN; s++ {
				d := math.Abs(gridSample(region.x0, region.x1-region.x0, imgW, ssN, i, s)-
					gridSample(poly.x0, poly.x1-poly.x0, imgW, ssN, i, s)) / hx
				if d > rep.shiftMaxX {
					rep.shiftMaxX = d
				}
			}
		}
		for i := 0; i < imgH; i++ {
			for s := 0; s < ssN; s++ {
				d := math.Abs(gridSample(region.y0, region.y1-region.y0, imgH, ssN, i, s)-
					gridSample(poly.y0, poly.y1-poly.y0, imgH, ssN, i, s)) / hy
				if d > rep.shiftMaxY {
					rep.shiftMaxY = d
				}
			}
		}
		// CF 的 bbox 即裁剪后多边形顶点包络 = 交集矩形
		rep.predDiff = predict(region, poly, region)
		for r := 0; r < imgH; r++ {
			for c := 0; c < imgW; c++ {
				if rep.predDiff[r][c] > 0 {
					rep.predCount++
					if rep.predDiff[r][c] > rep.predMax {
						rep.predMax = rep.predDiff[r][c]
					}
				}
			}
		}
	}

	for r := 0; r < imgH; r++ {
		for c := 0; c < imgW; c++ {
			fc := int(rep.fcImg.Pix[r*imgW+c])
			cf := int(rep.cfImg.Pix[r*imgW+c])
			if fc != cf {
				rep.cells = append(rep.cells, cell{row: r, col: c, fc: fc, cf: cf})
				d := absInt(fc - cf)
				if d > rep.maxGray {
					rep.maxGray = d
				}
			}
			// 预测与实测不一致的像素，归类到最近边界类型
			pd := 0
			if rep.predDiff != nil {
				pd = rep.predDiff[r][c]
			}
			measuredDiffer := fc != cf
			predDiffer := pd > 0
			if measuredDiffer != predDiffer || (measuredDiffer && absInt(absInt(fc-cf)-pd) > 255/(ssN*ssN)+1) {
				kind := "无交集区域"
				if ok {
					kind = boundaryKind(region, poly, r, c, poly)
				}
				rep.mismatch = append(rep.mismatch,
					fmt.Sprintf("(%d,%d) 实测差=%d 预测差=%d %s", r, c, absInt(fc-cf), pd, kind))
			}
		}
	}

	// 暗部差异像素：FC 灰度 <255（未被亮部饱和拉平）。
	rep.minDarkFC, rep.minDarkCF = 256, 256
	for _, cl := range rep.cells {
		if cl.fc < 255 {
			rep.darkCells = append(rep.darkCells, cl)
			if cl.fc < rep.minDarkFC {
				rep.minDarkFC = cl.fc
			}
			if cl.cf < rep.minDarkCF {
				rep.minDarkCF = cl.cf
			}
		}
	}
	// 行主序间距
	idx := make([]int, 0, len(rep.darkCells))
	for _, cl := range rep.darkCells {
		idx = append(idx, cl.row*imgW+cl.col)
	}
	sort.Ints(idx)
	gaps := make([]float64, 0, len(idx))
	for i := 1; i < len(idx); i++ {
		gaps = append(gaps, float64(idx[i]-idx[i-1]))
	}
	if m, v, ok2 := variance(gaps); ok2 {
		rep.gapMean, rep.gapVar = m, v
	}
	// 行内水平间距（按行汇总）
	var rowGaps []float64
	byRow := map[int][]int{}
	for _, cl := range rep.darkCells {
		byRow[cl.row] = append(byRow[cl.row], cl.col)
	}
	for _, cols := range byRow {
		sort.Ints(cols)
		for i := 1; i < len(cols); i++ {
			rowGaps = append(rowGaps, float64(cols[i]-cols[i-1]))
		}
	}
	if m, v, ok2 := variance(rowGaps); ok2 {
		rep.rowGapMean, rep.rowGapVar = m, v
	}

	_ = rep.cfImg.SavePGM(filepath.Join(outDir, "clip_first_"+name+".pgm"))
	_ = rep.fcImg.SavePGM(filepath.Join(outDir, "fill_first_"+name+".pgm"))
	diffIm, _ := raster.New(imgW, imgH)
	for r := 0; r < imgH; r++ {
		for c := 0; c < imgW; c++ {
			d := absInt(int(rep.fcImg.Pix[r*imgW+c]) - int(rep.cfImg.Pix[r*imgW+c]))
			if d*8 > 255 {
				d = 255
			} else {
				d *= 8
			}
			diffIm.Pix[r*imgW+c] = uint8(d)
		}
	}
	_ = diffIm.SavePGM(filepath.Join(outDir, "diff8x_"+name+".pgm"))
	return rep
}

func sameGridIdentity(poly, window rect) (int, int) {
	a := renderFC(poly.pts(), window.pts(), poly)
	b := renderSameGridCF(poly.pts(), window.pts(), poly)
	n, maxd := 0, 0
	for i := range a.Pix {
		if a.Pix[i] != b.Pix[i] {
			n++
			if d := absInt(int(a.Pix[i]) - int(b.Pix[i])); d > maxd {
				maxd = d
			}
		}
	}
	return n, maxd
}

func fcMatchesLibrary(poly rect) (int, int) {
	im, _ := raster.New(imgW, imgH)
	if err := im.Fill([][]geom.Point{poly.pts()}, ssN); err != nil {
		panic(err)
	}
	fc := renderFC(poly.pts(), nil, poly)
	n, maxd := 0, 0
	for i := range im.Pix {
		if im.Pix[i] != fc.Pix[i] {
			n++
		}
		if d := absInt(int(im.Pix[i]) - int(fc.Pix[i])); d > maxd {
			maxd = d
		}
	}
	return n, maxd
}

func writeReport(path string, reps []caseReport, identity [][2]int, libMatch [2]int) {
	var b strings.Builder
	b.WriteString("# 裁剪-填充顺序边缘覆盖率偏差实验\n\n")
	b.WriteString(fmt.Sprintf("画幅 %dx%d，超采样 N=%d（N×N=%d 点/像素）。raster.Fill 末端截断存 uint8：g=255·hit/%d，相邻 hit 的灰度步长约 %.4f（非整数，逐级四舍五入，不存在固定为 3 的步长）。\n\n",
		imgW, imgH, ssN, ssN*ssN, ssN*ssN, 255.0/(ssN*ssN)))
	b.WriteString("## 1. 采样位置与偏差式（两行）\n\n")
	b.WriteString("超采样点取每个像素的 N×N 子像元中心：轴向第 i 像素、第 s 个采样的归一化坐标为 i+(s+1/2)/N。\n\n")
	b.WriteString("**第一行（采样位置）**：FC 用原始 bbox (b0,S)，CF 用裁剪后 bbox (b0',S')，W 为画幅像素数：\n\n")
	b.WriteString("```\n")
	b.WriteString("P_FC(i,s) = b0 + (i+(s+1/2)/N)·S/W      P_CF(i,s) = b0' + (i+(s+1/2)/N)·S'/W\n")
	b.WriteString("Δ(i,s) = P_CF - P_FC = (b0'-b0) + (i+(s+1/2)/N)·(S'-S)/W\n")
	b.WriteString("```\n\n")
	b.WriteString("**第二行（像素覆盖率偏差）**：R 为多边形∩裁剪框，1_R 为内外指示函数，k_c 为该像素内因平移发生内外翻转的采样数：\n\n")
	b.WriteString("```\n")
	b.WriteString("Δα(c) = (1/N²)·Σ_s [1_R(P_CF,s) - 1_R(P_FC,s)] = k_c/N²      灰度差 Δg = 255·k_c/N²\n")
	b.WriteString(fmt.Sprintf("k_c 的分层上界（h=S/W 为像素间距，δ 为两套网格的采样平移，角点像素按两轴独立翻转合并）：\n"))
	b.WriteString(fmt.Sprintf("  · 小位移 |δ|≤h/N：单轴翻转 ≤1 采样，单边 |Δg|≤255/N=%.1f；角点两轴各翻 1：|Δg|≤255(2N-1)/N²≈%.1f。\n",
		255.0/ssN, float64(255*(2*ssN-1))/(ssN*ssN)))
	b.WriteString(fmt.Sprintf("  · 一般位移 |δ|≤h：单轴翻转 ≤N 采样即 |Δg|_轴≤255；角点两轴同时翻转时 k_c≤N²，逐像素硬上界 |Δg|≤255。\n"))
	b.WriteString("偏差像素只可能出现在边界在两套网格中映射到的列/行上（支撑集为两套边界列行的并集）。\n")
	b.WriteString("```\n\n")
	b.WriteString("偏差像素数量级：每条边在 FC、CF 网格各映射到一列（行），偏差像素为这些列行的并集，量级 O(边界周长像素数)，上界约为单网格边界像素数的 2 倍。矩形场景下用分离轴解析计数给出精确预测。\n\n")
	b.WriteString("## 2. 对照实验（排除裁剪数学本身的嫌疑）\n\n")
	b.WriteString(fmt.Sprintf("- FC 无掩膜渲染 vs 仓库 raster.Fill 字节差异：%d 像素，最大 %d 灰度（应为 0,0，证明两条渲染路径布点完全相同）。\n", libMatch[0], libMatch[1]))
	for i, rep := range reps {
		b.WriteString(fmt.Sprintf("- 同网格下「裁剪多边形判内外」vs「FC 双掩膜」(%s)：%d 像素差异，最大 %d 灰度（应为 0,0：同一采样网格上两种顺序数学恒等，偏差只来自 bbox 重映射导致的采样平移）。\n",
			rep.name, identity[i][0], identity[i][1]))
	}
	b.WriteString("\n## 3. 三类裁剪框实测 vs 预测\n\n")
	for _, rep := range reps {
		b.WriteString(fmt.Sprintf("### 裁剪框 %s：window=[%.2f,%.2f]×[%.2f,%.2f]\n\n", rep.name, rep.window.x0, rep.window.x1, rep.window.y0, rep.window.y1))
		if rep.hasRegion {
			b.WriteString(fmt.Sprintf("- 交集区域 R=[%.2f,%.2f]×[%.2f,%.2f]；网格最大平移 |δx|=%.4fh, |δy|=%.4fh（h=S/64 为 FC 像素间距，小位移阈值 h/N=%.4fh）\n",
				rep.region.x0, rep.region.x1, rep.region.y0, rep.region.y1,
				rep.shiftMaxX, rep.shiftMaxY, 1.0/ssN))
		} else {
			b.WriteString("- 交集为空（裁剪框完全在外），无网格可重映射\n")
		}
		b.WriteString(fmt.Sprintf("- 解析预测：偏差像素 %d 个，最大灰度差 %d\n", rep.predCount, rep.predMax))
		b.WriteString(fmt.Sprintf("- 实测：偏差像素 %d 个，最大灰度差 %d\n", len(rep.cells), rep.maxGray))
		ratio := "同量级 ✓"
		if rep.predCount > 0 {
			r := float64(len(rep.cells)) / float64(rep.predCount)
			if r < 0.5 || r > 2 {
				ratio = fmt.Sprintf("超出同量级[0.5,2]：比值 %.3f ✗", r)
			}
		} else if len(rep.cells) != 0 {
			ratio = "预测为 0 却有实测偏差 ✗"
		}
		b.WriteString(fmt.Sprintf("- 同量级判定：%s\n", ratio))
		if len(rep.mismatch) == 0 {
			b.WriteString("- 超限/错判像素：无（逐像素预测与实测均落在 1 个灰度量子容差内）\n")
		} else {
			b.WriteString("- 超限像素边界归类：\n")
			for _, m := range rep.mismatch {
				b.WriteString("  - " + m + "\n")
			}
		}
		b.WriteString("\n")
	}
	b.WriteString("## 4. 差异位置按行列出（格式：列号 FC灰度→CF灰度）\n\n")
	for _, rep := range reps {
		b.WriteString(fmt.Sprintf("### %s\n\n", rep.name))
		if len(rep.cells) == 0 {
			b.WriteString("无差异像素。\n\n")
			continue
		}
		byRow := map[int][]cell{}
		rows := make([]int, 0)
		for _, cl := range rep.cells {
			if _, ok := byRow[cl.row]; !ok {
				rows = append(rows, cl.row)
			}
			byRow[cl.row] = append(byRow[cl.row], cl)
		}
		sort.Ints(rows)
		// 行签名：完全相同的连续行合并为区间，差异仍逐行可查
		sig := make([]string, len(rows))
		for i, r := range rows {
			cs := byRow[r]
			sort.Slice(cs, func(a, c int) bool { return cs[a].col < cs[c].col })
			parts := make([]string, 0, len(cs))
			for _, cl := range cs {
				parts = append(parts, fmt.Sprintf("c%d(%d→%d)", cl.col, cl.fc, cl.cf))
			}
			sig[i] = strings.Join(parts, " ")
		}
		runStart, runSig := 0, sig[0]
		flush := func(end int) {
			if end == runStart {
				b.WriteString(fmt.Sprintf("- 行 %d：%s\n", rows[runStart], runSig))
			} else {
				b.WriteString(fmt.Sprintf("- 行 %d–%d（共 %d 行，内容相同）：%s\n",
					rows[runStart], rows[end], rows[end]-rows[runStart]+1, runSig))
			}
		}
		for i := 1; i < len(rows); i++ {
			if sig[i] != runSig {
				flush(i - 1)
				runStart, runSig = i, sig[i]
			}
		}
		flush(len(rows) - 1)
		b.WriteString("\n")
	}
	b.WriteString("## 5. 暗部区域间距方差\n\n")
	b.WriteString("暗部定义：FC 图灰度 <255 的差异像素（未被亮部饱和拉平；亮部像素两图均为 255 时无差异可测）。间距为差异像素行主序一维下标之差（单位：像素），样本方差 s²=Σ(g-ḡ)²/(m-1)；另给行内水平间距。\n\n")
	for _, rep := range reps {
		b.WriteString(fmt.Sprintf("### %s\n\n", rep.name))
		b.WriteString(fmt.Sprintf("- 暗部差异像素（FC<255）：%d 个，占全部差异 %d 个；暗部中 FC 最低灰度=%s、对应 CF 灰度最低=%s；亮部（FC=255）差异像素：%d 个\n",
			len(rep.darkCells), len(rep.cells), minOrDash(rep.minDarkFC), minOrDash(rep.minDarkCF), len(rep.cells)-len(rep.darkCells)))
		if len(rep.darkCells) < 2 {
			b.WriteString("- 暗部差异像素不足 2 个，间距方差无定义。\n\n")
			continue
		}
		b.WriteString(fmt.Sprintf("- 行主序间距：均值 %.2f，方差 %.2f\n", rep.gapMean, rep.gapVar))
		if rep.rowGapVar == 0 && rep.rowGapMean == 0 {
			b.WriteString("- 行内水平间距：无任何一行含 2 个以上暗部差异像素，无定义（差异均为每行 1 个的边线像素；此时行主序方差仅反映行间距的规则程度，方差=0 表示差异严格按等间距网格排列）。\n")
		} else {
			b.WriteString(fmt.Sprintf("- 行内水平间距：均值 %.2f，方差 %.2f\n", rep.rowGapMean, rep.rowGapVar))
		}
		b.WriteString("\n")
	}
	b.WriteString("## 6. 输出文件\n\n")
	b.WriteString("每类裁剪框各两张图（clip_first_* 为裁剪后填充，fill_first_* 为填充后裁剪），diff8x_* 为放大 8 倍的差异图。\n")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		panic(err)
	}
}

func main() {
	outDir := "artifacts/clipbias"
	if len(os.Args) > 1 {
		outDir = os.Args[1]
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		panic(err)
	}
	poly := rect{x0: 0, y0: 0, x1: 10, y1: 10}
	cases := []struct {
		name string
		win  rect
	}{
		{"contain", rect{x0: -1, y0: -1, x1: 11, y1: 11}},       // 裁剪框完全包含多边形
		{"intersect", rect{x0: -1, y0: -1, x1: 9.99, y1: 9.99}}, // 相交（小位移）：右/上各咬入 ε=0.01，|δ|≈0.064h < h/N
		{"outside", rect{x0: 20, y0: 20, x1: 30, y1: 30}},       // 裁剪框完全在外
		// 辅助案例（一般位移区间，|δ|≈0.64h > h/N）：单边窄咬入 0.10，
		// 交集在 FC 网格边缘列只覆盖 3 个超采样，FC 灰度 95 落在暗部(<128)，
		// 用于在暗区上统计间距方差（主案例的边缘列偏亮）。
		{"intersect_dark", rect{x0: -1, y0: -1, x1: 9.90, y1: 11}},
	}
	reps := make([]caseReport, 0, len(cases))
	identity := make([][2]int, 0, len(cases))
	for _, cs := range cases {
		reps = append(reps, runCase(cs.name, poly, cs.win, outDir))
		n, d := sameGridIdentity(poly, cs.win)
		identity = append(identity, [2]int{n, d})
	}
	libN, libD := fcMatchesLibrary(poly)
	writeReport(filepath.Join(outDir, "report.md"), reps, identity, [2]int{libN, libD})

	fmt.Printf("q=255/%d=%.4f  library-match diffs=%d max=%d\n", ssN*ssN, 255.0/(ssN*ssN), libN, libD)
	for i, rep := range reps {
		fmt.Printf("[%s] same-grid identity: diffs=%d max=%d | predicted count=%d max=%d | measured count=%d max=%d | mismatches=%d | dark=%d minFC=%s gapVar=%.2f | shift=(%.4f,%.4f)h\n",
			rep.name, identity[i][0], identity[i][1],
			rep.predCount, rep.predMax, len(rep.cells), rep.maxGray, len(rep.mismatch),
			len(rep.darkCells), minOrDash(rep.minDarkFC), rep.gapVar,
			rep.shiftMaxX, rep.shiftMaxY)
	}
	fmt.Println("report:", filepath.Join(outDir, "report.md"))
}
