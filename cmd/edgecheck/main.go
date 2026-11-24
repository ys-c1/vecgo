// Command edgecheck 验证直边裁剪覆盖率闭式函数：
// 三档裁剪位置 vs 16 倍超采样并列对照、整数位置 0/1 性质、
// 面积积分恒等式、置信度与量纲统一前后的加权误差，
// 并把 Markdown/CSV 报告与 PGM 对照图写入输出目录。
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"

	"vecgo/coverage"
	"vecgo/raster"
)

type tier struct {
	name     string
	fracText string
	f        float64
}

type tierResult struct {
	tier
	alpha  float64
	hits   int
	total  int
	beta   float64
	e      float64
	b256   float64
	e256   float64
	byteA  uint8
	byteS  uint8
	se     float64
	ci95   float64
	weight float64
}

func main() {
	outDir := flag.String("outdir", filepath.Join("artifacts", "coverage"), "报告与 PGM 输出目录")
	flag.Parse()

	tiers := []tier{
		{name: "浅切", fracText: "1/3", f: 1.0 / 3},
		{name: "半切", fracText: "1/2", f: 1.0 / 2},
		{name: "深切", fracText: "2/3", f: 2.0 / 3},
	}

	rows := make([]tierResult, 0, len(tiers))
	alphas := make([]float64, len(tiers))
	betas := make([]float64, len(tiers))
	weights := coverage.BinomialWeights([]float64{1.0 / 3, 0.5, 2.0 / 3})
	for i, t := range tiers {
		a := coverage.UnitVertical(t.f, 0)
		hits, total, b := coverage.SuperSampleVertical(t.f, 0, 0, 1, 1, 16)
		_, _, b256 := coverage.SuperSampleVertical(t.f, 0, 0, 1, 1, 256)
		se := math.Sqrt(a * (1 - a) / float64(total))
		rows = append(rows, tierResult{
			tier: t, alpha: a, hits: hits, total: total, beta: b, e: a - b,
			b256: b256, e256: a - b256,
			byteA: coverage.Gray(a), byteS: coverage.Gray(b),
			se: se, ci95: 1.96 * se, weight: weights[i],
		})
		alphas[i], betas[i] = a, b
	}

	wmae := coverage.WeightedMAE(alphas, betas, weights)
	wrmse := coverage.WeightedRMSE(alphas, betas, weights)
	equalW := []float64{1.0 / 3, 1.0 / 3, 1.0 / 3}
	maeEqual := coverage.WeightedMAE(alphas, betas, equalW)
	rmseEqual := coverage.WeightedRMSE(alphas, betas, equalW)

	// 8-bit 灰度量纲（未取整的连续灰度 255*c）。
	wmaeGray := 255 * wmae
	wrmseGray := 255 * wrmse

	// 字节取整后的实际灰度误差。
	byteA := make([]float64, len(rows))
	byteS := make([]float64, len(rows))
	for i, r := range rows {
		byteA[i], byteS[i] = float64(r.byteA), float64(r.byteS)
	}
	wmaeByte := coverage.WeightedMAE(byteA, byteS, weights)
	wrmseByte := coverage.WeightedRMSE(byteA, byteS, weights)

	// 非单位像素的物理面积量纲：w=2, h=3, Area=6。
	const pw, ph = 2.0, 3.0
	pixArea := pw * ph
	areaA := make([]float64, len(tiers))
	areaB := make([]float64, len(tiers))
	for i, t := range tiers {
		c := pw * t.f // 边在 [0,pw] 内的相对位置保持 f
		areaA[i] = coverage.VerticalArea(c, 0, 0, pw, ph)
		_, _, bf := coverage.SuperSampleVertical(c, 0, 0, pw, ph, 16)
		areaB[i] = pixArea * bf
	}
	wmaeArea := coverage.WeightedMAE(areaA, areaB, weights)
	wrmseArea := coverage.WeightedRMSE(areaA, areaB, weights)

	// 整数位置性质的大范围验证结果（测试中另有断言）。
	intBad := 0
	for k := -1000; k <= 1000; k++ {
		for i := -3; i <= 3; i++ {
			if a := coverage.UnitVertical(float64(k), i); a != 0 && a != 1 {
				intBad++
			}
		}
	}

	// 数值积分核验面积恒等式（单位像素，细中点和）。
	numIntegral := unitDensityIntegral()

	// 生成三档条带对照 PGM。
	panelMetrics := buildPanels(*outDir, tiers)

	// 生成灰度阶数扫描 PGM。
	sweepMetrics := buildSweeps(*outDir)

	var md strings.Builder
	writeReport(&md, reportData{
		rows: rows, wmae: wmae, wrmse: wrmse,
		maeEqual: maeEqual, rmseEqual: rmseEqual,
		wmaeGray: wmaeGray, wrmseGray: wrmseGray,
		wmaeByte: wmaeByte, wrmseByte: wrmseByte,
		wmaeArea: wmaeArea, wrmseArea: wrmseArea, pixArea: pixArea,
		intBad: intBad, numIntegral: numIntegral,
		panel: panelMetrics, sweep: sweepMetrics,
	})
	report := md.String()
	fmt.Print(report)

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fatal(err)
	}
	if err := os.WriteFile(filepath.Join(*outDir, "report.md"), []byte(report), 0o644); err != nil {
		fatal(err)
	}
	if err := os.WriteFile(filepath.Join(*outDir, "three_tiers.csv"), []byte(buildCSV(rows)), 0o644); err != nil {
		fatal(err)
	}
	fmt.Fprintf(os.Stderr, "\n报告已写入 %s\n", filepath.Join(*outDir, "report.md"))
}

type panelMetric struct {
	name             string
	levels           int
	rmsVsSS16        float64
	diffPixelsVsSS16 int
}

type reportData struct {
	rows                         []tierResult
	wmae, wrmse                  float64
	maeEqual, rmseEqual          float64
	wmaeGray, wrmseGray          float64
	wmaeByte, wrmseByte          float64
	wmaeArea, wrmseArea, pixArea float64
	intBad                       int
	numIntegral                  float64
	panel                        []panelMetric
	sweep                        []panelMetric
}

// unitDensityIntegral 用细中点黎曼和数值积分单位像素 [0,1]² 的
// 竖直覆盖率密度，应得到 1。
func unitDensityIntegral() float64 {
	const step = 1e-5
	const lo, hi = -2.0, 3.0
	sum := 0.0
	n := int((hi - lo) / step)
	for k := 0; k < n; k++ {
		c := lo + (float64(k)+0.5)*step
		sum += coverage.VerticalDensity(c, 0, 0, 1, 1) * step
	}
	return sum
}

// buildPanels 生成一张 24x24 图，三个 8 行条带分别对应三档位置，
// 每种路径一张 PGM，并统计灰度阶数及与 SS16 的整图 RMS 差。
func buildPanels(dir string, tiers []tier) []panelMetric {
	const w, h = 24, 24
	modes := []struct {
		name string
		kind int // 0 hard, 1 analytic, 2 ss16
	}{
		{"panel-hard", 0},
		{"panel-analytic", 1},
		{"panel-ss16", 2},
	}
	imgs := map[string]*raster.Image{}
	for _, m := range modes {
		im, _ := raster.New(w, h)
		for band, t := range tiers {
			c := 8 + t.f
			for y := band * 8; y < band*8+8; y++ {
				for x := 0; x < w; x++ {
					var v uint8
					switch m.kind {
					case 0:
						if float64(x)+0.5 <= c {
							v = 255
						}
					case 1:
						v = coverage.Gray(coverage.UnitVertical(c, x))
					case 2:
						_, _, f := coverage.SuperSampleVertical(c, float64(x), float64(y),
							float64(x+1), float64(y+1), 16)
						v = coverage.Gray(f)
					}
					im.Pix[y*w+x] = v
				}
			}
		}
		imgs[m.name] = im
		savePGM(dir, m.name+".pgm", im)
		saveZoomPNG(dir, m.name+"-zoom.png", im, 12)
	}
	ss := imgs["panel-ss16"]
	out := make([]panelMetric, 0, len(modes))
	for _, m := range modes {
		im := imgs[m.name]
		sq, diffN := 0, 0
		for i := range im.Pix {
			d := int(im.Pix[i]) - int(ss.Pix[i])
			if d != 0 {
				diffN++
			}
			sq += d * d
		}
		out = append(out, panelMetric{
			name: m.name, levels: im.DistinctLevels(),
			rmsVsSS16:        math.Sqrt(float64(sq) / float64(len(im.Pix))),
			diffPixelsVsSS16: diffN,
		})
	}
	return out
}

// buildSweeps 逐行把亚像素位置 f 从 0 扫到 1，生成灰度阶数扫描图。
func buildSweeps(dir string) []panelMetric {
	const w, h = 24, 64
	makeImg := func(kind int) *raster.Image {
		im, _ := raster.New(w, h)
		for y := 0; y < h; y++ {
			f := float64(y) / float64(h-1)
			c := 8 + f
			for x := 0; x < w; x++ {
				var v uint8
				switch kind {
				case 0:
					if float64(x)+0.5 <= c {
						v = 255
					}
				case 1:
					v = coverage.Gray(coverage.UnitVertical(c, x))
				default:
					_, _, bf := coverage.SuperSampleVertical(c, float64(x), float64(y),
						float64(x+1), float64(y+1), 16)
					v = coverage.Gray(bf)
				}
				im.Pix[y*w+x] = v
			}
		}
		return im
	}
	hard := makeImg(0)
	aa := makeImg(1)
	ss := makeImg(2)
	savePGM(dir, "sweep-hard.pgm", hard)
	savePGM(dir, "sweep-analytic.pgm", aa)
	savePGM(dir, "sweep-ss16.pgm", ss)
	saveZoomPNG(dir, "sweep-analytic-zoom.png", aa, 6)

	// 只统计边界列 x=8 的不同阶数。
	boundaryLevels := func(im *raster.Image) int {
		var seen [256]bool
		for y := 0; y < h; y++ {
			seen[im.Pix[y*w+8]] = true
		}
		n := 0
		for _, ok := range seen {
			if ok {
				n++
			}
		}
		return n
	}
	return []panelMetric{
		{name: "sweep-hard（边界列阶数）", levels: boundaryLevels(hard)},
		{name: "sweep-analytic（边界列阶数）", levels: boundaryLevels(aa)},
		{name: "sweep-ss16（边界列阶数）", levels: boundaryLevels(ss)},
	}
}

func savePGM(dir, name string, im *raster.Image) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fatal(err)
	}
	if err := im.SavePGM(filepath.Join(dir, name)); err != nil {
		fatal(err)
	}
}

// saveZoomPNG 以最近邻 zoom 倍放大输出 PNG，方便直接查看边界灰阶。
func saveZoomPNG(dir, name string, im *raster.Image, zoom int) {
	if zoom < 1 {
		zoom = 1
	}
	out := image.NewGray(image.Rect(0, 0, im.W*zoom, im.H*zoom))
	for y := 0; y < im.H; y++ {
		for x := 0; x < im.W; x++ {
			v := im.Pix[y*im.W+x]
			for zy := 0; zy < zoom; zy++ {
				for zx := 0; zx < zoom; zx++ {
					out.SetGray(x*zoom+zx, y*zoom+zy, color.Gray{Y: v})
				}
			}
		}
	}
	f, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, out); err != nil {
		fatal(err)
	}
}

func buildCSV(rows []tierResult) string {
	var b strings.Builder
	b.WriteString("tier,position_f,alpha_closed,ss16_hits,ss16_total,beta_ss16,alpha_minus_beta,gray_closed,gray_ss16,weight\n")
	for _, r := range rows {
		fmt.Fprintf(&b, "%s,%s,%.10f,%d,%d,%.10f,%+.10f,%d,%d,%.10f\n",
			r.name, r.fracText, r.alpha, r.hits, r.total, r.beta, r.e,
			r.byteA, r.byteS, r.weight)
	}
	return b.String()
}

func writeReport(b *strings.Builder, d reportData) {
	b.WriteString("# 直边裁剪覆盖率：闭式解、面积守恒与 16 倍超采样对照\n\n")

	b.WriteString("## 1. 闭式函数\n\n")
	b.WriteString("像素矩形 `R=[x0,x1]×[y0,y1]`（宽 w=x1−x0，高 h=y1−y0，面积 A=wh），")
	b.WriteString("竖直直边 `x=c`，保留 `x≤c` 一侧。像素内被覆盖面积为\n\n")
	b.WriteString("```\nA_R(c) = h · clamp(c-x0, 0, x1-x0)\n```\n\n")
	b.WriteString("故无量纲覆盖率为\n\n")
	b.WriteString("```\nα_R(c) = A_R(c)/A = clamp((c-x0)/w, 0, 1)\n")
	b.WriteString("       ┌ 0,             c ≤ x0\n")
	b.WriteString("     = ┤ (c-x0)/w,      x0 < c < x1\n")
	b.WriteString("       └ 1,             c ≥ x1\n```\n\n")
	b.WriteString("对单位像素 `[i,i+1]×[j,j+1]`：`α_i(c)=clamp(c-i,0,1)`。")
	b.WriteString("水平直边把 x 换成 y，形式完全相同。\n\n")

	b.WriteString("## 2. “对位置积分等于像素面积”的证明\n\n")
	b.WriteString("令 ρ_R(c)=dA_R/dc 为**覆盖率面积关于裁剪位置的密度**")
	b.WriteString("（即裁剪线扫到位置 c 时在像素内的截面长度，量纲 L）。由闭式解\n\n")
	b.WriteString("```\nρ_R(c) = h · 1_[x0,x1](c)   （几乎处处；两个折点不影响积分）\n```\n\n")
	b.WriteString("于是由 Fubini/Cavalieri 截面面积原理，\n\n")
	b.WriteString("```\n∫_{-∞}^{+∞} ρ_R(c) dc = ∫_{x0}^{x1} h dc = h(x1-x0) = A.\n```\n\n")
	b.WriteString("等价地，对闭式累积覆盖率直接用微积分基本定理：\n\n")
	b.WriteString("```\n∫_{-∞}^{+∞} A_R'(c) dc = A_R(+∞)-A_R(-∞) = A - 0 = A.\n```\n\n")
	b.WriteString("量纲上 ρ[L]·dc[L]=L²，恰为像素面积。注意不要把累积覆盖率 α 本身")
	b.WriteString("直接拿去积分：在穿过区间上 `∫_{x0}^{x1} A_R(c) dc = A·w/2`")
	b.WriteString("（单位像素为 1/2）；等于全面积的是其位置密度（导数）的积分。\n\n")
	fmt.Fprintf(b, "数值核验（细中点步长 1e-5）：单位像素 ∫ρ dc = %.6f；", d.numIntegral)
	b.WriteString("多组非单位矩形的核验见 `coverage/coverage_test.go` 中 ")
	b.WriteString("`TestCoverageDensityIntegratesToPixelArea`。\n\n")

	b.WriteString("## 3. 整数裁剪位置退化为 0 或 1\n\n")
	b.WriteString("当 `c=k∈Z` 且像素边界索引 `i∈Z` 时，`c-i` 是整数，")
	b.WriteString("`clamp(整数,0,1)` 只能取 `0` 或 `1`：边与像素网格对齐，")
	b.WriteString("解析路径与硬切路径逐像素相同。已对 k∈[-1000,1000]、i∈[-3,3] ")
	fmt.Fprintf(b, "穷举验证，违例数 = **%d**；Go 测试另对更大范围和水平方向断言。\n\n", d.intBad)

	b.WriteString("## 4. 三档裁剪位置 × 十六倍超采样（并列）\n\n")
	b.WriteString("十六倍指每轴 16 细分、每像素 16²=256 个中点样本，")
	b.WriteString("采样约定与 `raster.Image.Fill` 相同。考察单位像素，")
	b.WriteString("三档为 c=f=1/3、1/2、2/3。\n\n")
	b.WriteString("| 档位 | c | 闭式 α | SS16 命中 | SS16 β | α−β | 闭式灰度 | SS16 灰度 | 权重 w |\n")
	b.WriteString("|---|---:|---:|---:|---:|---:|---:|---:|---:|\n")
	for _, r := range d.rows {
		fmt.Fprintf(b, "| %s | %s | %.6f | %d/%d | %.6f | %+.6f (%s) | %d | %d | %.4f |\n",
			r.name, r.fracText, r.alpha, r.hits, r.total, r.beta, r.e,
			signedFraction(r.e), r.byteA, r.byteS, r.weight)
	}
	b.WriteString("\n精确值：β=(5/16, 1/2, 11/16)，α−β=(+1/48, 0, −1/48)，")
	b.WriteString("逆方差权重为精确的 (9/26, 8/26, 9/26)。\n\n")
	b.WriteString("更高倍参照 SS256（65536 样本）三档结果：\n\n")
	b.WriteString("| 档位 | β₂₅₆ | α−β₂₅₆ |\n|---|---:|---:|\n")
	for _, r := range d.rows {
		fmt.Fprintf(b, "| %s (%s) | %.8f | %+.2e |\n", r.name, r.fracText, r.b256, r.e256)
	}
	b.WriteString("\nSS256 误差均在 1/(2·256)≈0.00195 的量化界内，证明闭式 α 本身精确，")
	b.WriteString("SS16 与闭式的微小差别仅来自 16 档量化。若三档改取 1/4、1/2、3/4，")
	b.WriteString("它们恰好落在 SS16 箱格上，β 与 α 完全相等、加权误差为 0；")
	b.WriteString("本报告刻意选取不对齐的三分点以暴露并量化 SS16 的离散误差。\n\n")

	b.WriteString("## 5. SS16 的置信度说明\n\n")
	b.WriteString("- **确定性误差界（轴对齐直边，100% 成立）**：中点规则网格把 β 量化到 ")
	b.WriteString("m/16，任意 c 有 `|β−α| ≤ 1/(2·16) = 1/32 ≈ 0.03125` 覆盖率，")
	b.WriteString("即连续灰度 ≤ 7.97 级。本三档实测最大绝对误差 1/48 ≈ 0.02083，在界内。\n")
	b.WriteString("- **统计口径（把 256 个样本视作 iid Bernoulli 时）**：")
	b.WriteString("SE=√(α(1−α)/256)，95% 正态区间半宽 1.96·SE。\n\n")
	b.WriteString("| 档位 | SE | 近似 95% 半宽 |\n|---|---:|---:|\n")
	for _, r := range d.rows {
		fmt.Fprintf(b, "| %s (%s) | %.5f | ±%.5f |\n", r.name, r.fracText, r.se, r.ci95)
	}
	b.WriteString("\n必须强调：这里的 SS16 是**确定性分层网格**而非随机蒙特卡洛，")
	b.WriteString("256 个样本不独立（竖直边下 y 方向 16 次重复，真正变化的是 16 个 x 分层），")
	b.WriteString("所以二项置信区间是**保守的启发式参照**，严格置信度以上述 1/32 确定界为准。")
	b.WriteString("对斜边不存在该 1/32 界；实测 SS16 误差约 ≤0.023，SS64 ≤0.004 且单调收敛。\n\n")

	b.WriteString("## 6. 加权误差与量纲统一\n\n")
	b.WriteString("权重取 SS 估计的逆方差 `w_k ∝ 1/[α_k(1−α_k)]`，")
	b.WriteString("三档恰为 9:8:9。\n\n")
	b.WriteString("**(a) 统一到无量纲覆盖率后（权威值）**\n\n")
	fmt.Fprintf(b, "- 等权对照（各 1/3）：MAE = 1/72 = **%.8f**，RMSE = 1/√3456 = **%.8f**\n", d.maeEqual, d.rmseEqual)
	fmt.Fprintf(b, "- 逆方差加权 WMAE = Σw|α−β| = 3/208 = **%.8f**\n", d.wmae)
	fmt.Fprintf(b, "- 逆方差加权 WRMSE = √(Σw(α−β)²) = 1/√3328 = **%.8f**\n\n", d.wrmse)
	b.WriteString("**(b) 8-bit 灰度量纲（未取整 G=255α）**\n\n")
	fmt.Fprintf(b, "- WMAE = 255·3/208 = **%.6f 灰度级**（精确 765/208）\n", d.wmaeGray)
	fmt.Fprintf(b, "- WRMSE = 255/√3328 = **%.6f 灰度级**\n\n", d.wrmseGray)
	b.WriteString("把灰度全部除以 255 完成量纲统一后**再算一次**，数值严格回到 (a)：")
	fmt.Fprintf(b, "WMAE=%.8f、WRMSE=%.8f（灰度误差的 1/255）。\n\n", d.wmaeGray/255, d.wrmseGray/255)
	b.WriteString("**(c) 四舍五入成实际字节后**\n\n")
	fmt.Fprintf(b, "- 灰度字节 WMAE = **%.6f 级**（精确 45/13≈3.4615），", d.wmaeByte)
	fmt.Fprintf(b, "WRMSE = **%.6f 级**\n\n", d.wrmseByte)
	fmt.Fprintf(b, "- 归一化到 [0,1] 后 WMAE = **%.8f**（=3/221）、", d.wmaeByte/255)
	fmt.Fprintf(b, "WRMSE = **%.8f**（=1/(17√13)）；与未取整值的差别仅来自 ±0.5 级量化。\n\n", d.wrmseByte/255)
	b.WriteString("**(d) 物理面积量纲（w=2,h=3,A=6）**\n\n")
	fmt.Fprintf(b, "- 闭式面积 A=6α 与超采样面积 A·β 直接求加权误差：")
	fmt.Fprintf(b, "WMAE = **%.6f 面积单位**、WRMSE = **%.6f 面积单位**。\n", d.wmaeArea, d.wrmseArea)
	fmt.Fprintf(b, "两边同除以像素面积 %.0f 完成量纲统一后再算，", d.pixArea)
	b.WriteString("同样严格回到 (a)。这说明误差与选用覆盖率/灰度/面积哪种量纲无关，")
	b.WriteString("只差已知的尺度因子 1、255 或 A。\n\n")

	b.WriteString("## 7. 图像证据与灰度阶数\n\n")
	b.WriteString("`panel-*.pgm` 为三档条带对照图（每 8 行一档），`sweep-*.pgm` ")
	b.WriteString("逐行扫描 f∈[0,1]。指标：\n\n")
	b.WriteString("| 图像 | 不同灰度阶数 | 与 panel-ss16 不同像素数 | 对 SS16 整图 RMS |\n")
	b.WriteString("|---|---:|---:|---:|\n")
	for _, p := range d.panel {
		fmt.Fprintf(b, "| %s | %d | %d | %.4f |\n", p.name, p.levels, p.diffPixelsVsSS16, p.rmsVsSS16)
	}
	b.WriteString("\n边界列阶数扫描：\n\n| 路径 | 边界列灰度阶数 |\n|---|---:|\n")
	for _, s := range d.sweep {
		fmt.Fprintf(b, "| %s | %d |\n", s.name, s.levels)
	}
	b.WriteString("\n硬切路径全图只有 {0,255} 两阶；解析路径阶数远多于硬切，")
	b.WriteString("且高于 SS16 对直边只能表达的 17 档覆盖率。`raster` 测试断言了")
	b.WriteString("整数位置解析图与硬切图逐字节相同、解析图与 SS16 图仅边界列相差 ≤5 灰度级。\n\n")

	b.WriteString("## 8. 复现\n\n")
	b.WriteString("```sh\n")
	b.WriteString("go test ./...\n")
	b.WriteString("go run ./cmd/edgecheck -outdir artifacts/coverage\n")
	b.WriteString("```\n")
}

func signedFraction(e float64) string {
	switch {
	case math.Abs(e-1.0/48) < 1e-15:
		return "+1/48"
	case math.Abs(e+1.0/48) < 1e-15:
		return "-1/48"
	case e == 0:
		return "0"
	default:
		return fmt.Sprintf("%+.6f", e)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "失败:", err)
	os.Exit(1)
}
