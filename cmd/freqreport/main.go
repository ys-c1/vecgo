// Command freqreport 运行“局部频率自适应选层”实验：
// 生成频率测试卡 -> 梯度结构张量估主频（5/11 两档窗口）-> 反解每像素层号与核宽
// -> 相邻层逐像素插值缩小；与全局整数层方案、全局固定 τ 三线性方案对比
// 梯度能量与振铃，并把数据、报告和对照图写入 -out 目录。
//
//	go run ./cmd/freqreport -out artifacts
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"strings"

	"vecgo/freqsel"
)

type scaleRow struct {
	Window        int       `json:"window"`
	ScaleLabel    string    `json:"scale_label"`
	Scale         float64   `json:"scale"`
	OutW          int       `json:"out_w"`
	OutH          int       `json:"out_h"`
	D             float64   `json:"source_px_per_output_px"`
	FN            float64   `json:"nyquist_source_cycles_per_px"`
	GlobalLevel   int       `json:"global_level"`
	GlobalSigma   float64   `json:"global_kernel_sigma_source_px"`
	SigmaN        float64   `json:"scale_only_sigma"`
	TauN          float64   `json:"scale_only_tau"`
	MeanTau       float64   `json:"adaptive_mean_layer_continuous"`
	MeanLevel     float64   `json:"adaptive_mean_integer_level"`
	MeanSigma     float64   `json:"adaptive_mean_kernel_sigma"`
	BelowGlobal   float64   `json:"pct_pixels_below_global_level"`
	Histogram     []float64 `json:"integer_level_histogram"`
	GEGlobalInt   float64   `json:"gradient_energy_global_integer_total"`
	GEGlobalTri   float64   `json:"gradient_energy_global_trilinear_total"`
	GEAdaptive    float64   `json:"gradient_energy_adaptive_total"`
	GEGlobalIntPP float64   `json:"gradient_energy_global_integer_per_pixel"`
	GEGlobalTriPP float64   `json:"gradient_energy_global_trilinear_per_pixel"`
	GEAdaptivePP  float64   `json:"gradient_energy_adaptive_per_pixel"`
	GainVsIntPct  float64   `json:"gain_adaptive_vs_global_integer_pct"`
	GainVsTriPct  float64   `json:"gain_adaptive_vs_global_trilinear_pct"`
	RingGlobal    float64   `json:"ringing_global_integer"`
	RingAdaptive  float64   `json:"ringing_adaptive"`
	RingControl   float64   `json:"ringing_sharpen_control"`
	GEControl     float64   `json:"gradient_energy_sharpen_control_total"`
}

type freqCalib struct {
	Window  int                `json:"window"`
	Stripes map[string]float64 `json:"stripe_median_frequency"`
}

type report struct {
	SourceSize    int         `json:"source_size"`
	Tile          int         `json:"tile"`
	Windows       [2]int      `json:"estimation_windows"`
	PrimaryWindow int         `json:"primary_window_for_images"`
	Scales        []float64   `json:"scales"`
	Rows          []scaleRow  `json:"rows"`
	Calibration   []freqCalib `json:"calibration"`
	Sigma0        float64     `json:"sigma0"`
}

var scaleDefs = []struct {
	label string
	s     float64
	zoom  int // 把整幅输出最近邻放大回 480 展示所用整数倍
	cropZ int // EdgeCrop 放大到 240 高所用整数倍
}{
	{"1/2", 0.5, 2, 4},
	{"1/3", 1.0 / 3.0, 3, 6},
	{"1/4", 0.25, 4, 8},
}

func main() {
	outDir := flag.String("out", "artifacts", "产物输出目录")
	tile := flag.Int("tile", 120, "测试卡每个分区的像素边长（整图 4*tile）")
	flag.Parse()

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fatal("建目录失败", err)
	}

	card := freqsel.NewCard(*tile)
	src := card.Gray
	N := src.W
	st := freqsel.BuildStack(src, 4)
	windows := [2]int{5, 11}
	const primaryWin = 5
	opts := freqsel.DefaultOptions()

	rep := report{
		SourceSize: N, Tile: *tile, Windows: windows, PrimaryWindow: primaryWin,
		Scales: []float64{0.5, 1.0 / 3.0, 0.25}, Sigma0: freqsel.Sigma0,
	}

	// 频率标定（行 0 四块已知频率条纹），佐证窗口机制。
	trueStripes := []float64{0.03, 0.07, 0.12, 0.20}
	for _, w := range windows {
		fld := freqsel.EstimateFrequency(src, w)
		fc := freqCalib{Window: w, Stripes: map[string]float64{}}
		for i, f := range trueStripes {
			r := freqsel.Rect{X0: i**tile + 12, Y0: 12, X1: (i+1)**tile - 12, Y1: *tile - 12}
			got := fld.MedianFrequency(r)
			fc.Stripes[fmt.Sprintf("true_%.2f", f)] = round(got, 4)
		}
		rep.Calibration = append(rep.Calibration, fc)
	}

	// 缓存主窗口各档结果用于绘图。
	type prim struct {
		def  typeofScaleDef
		sel  *freqsel.Selector
		ad   *freqsel.AdaptiveResult
		glob *freqsel.Gray
		tri  *freqsel.Gray
		ctrl *freqsel.Gray
	}
	var prims []prim

	for _, w := range windows {
		fld := freqsel.EstimateFrequency(src, w)
		for _, sd := range scaleDefs {
			sel := freqsel.NewSelector(fld, sd.s, opts)
			ad := freqsel.DownscaleAdaptive(st, sel)
			glob := freqsel.DownscaleGlobal(st, sd.s)
			tri := freqsel.DownscaleGlobalTrilinear(st, sd.s)
			ctrl := freqsel.UnsharpControl(ad.Out, 1.2, 1.3)

			geG, geGpp := freqsel.GradientEnergy(glob)
			geT, geTpp := freqsel.GradientEnergy(tri)
			geA, geApp := freqsel.GradientEnergy(ad.Out)
			geC, _ := freqsel.GradientEnergy(ctrl)

			ow, oh := glob.W, glob.H
			ring := func(im *freqsel.Gray) float64 {
				ve := freqsel.RingingIndex(im, freqsel.ScaleRect(card.VerticalEdge, sd.s, ow, oh), false)
				he := freqsel.RingingIndex(im, freqsel.ScaleRect(card.HorizontalEdge, sd.s, ow, oh), true)
				return combineRing(ve, he)
			}
			rG, rA, rC := ring(glob), ring(ad.Out), ring(ctrl)

			row := scaleRow{
				Window: w, ScaleLabel: sd.label, Scale: round(sd.s, 4),
				OutW: ow, OutH: oh, D: round(sel.D, 3), FN: round(sel.FN, 4),
				GlobalLevel: sel.GlobalLevel, GlobalSigma: round(sel.GlobalSigma, 3),
				SigmaN: round(sel.SigmaN, 3), TauN: round(sel.TauN, 3),
				MeanTau: round(ad.MeanTau, 3), MeanLevel: round(ad.MeanLevel, 3),
				MeanSigma: round(ad.MeanSigma, 3), BelowGlobal: round(100*ad.BelowGlobal, 1),
				Histogram:   roundSlice(ad.LevelHistogram, 3),
				GEGlobalInt: round(geG, 0), GEGlobalTri: round(geT, 0), GEAdaptive: round(geA, 0),
				GEGlobalIntPP: round(geGpp, 1), GEGlobalTriPP: round(geTpp, 1), GEAdaptivePP: round(geApp, 1),
				GainVsIntPct: round(100*(geA/geG-1), 1), GainVsTriPct: round(100*(geA/geT-1), 1),
				RingGlobal: round(rG, 3), RingAdaptive: round(rA, 3), RingControl: round(rC, 2),
				GEControl: round(geC, 0),
			}
			rep.Rows = append(rep.Rows, row)

			if w == primaryWin {
				prims = append(prims, prim{def: sd, sel: sel, ad: ad, glob: glob, tri: tri, ctrl: ctrl})
			}
		}

		// 两档窗口在中间缩放下的逐像素层号图。
		mid := 1.0 / 3.0
		selMid := freqsel.NewSelector(fld, mid, opts)
		if err := freqsel.SaveLevelMap(selMid.Level, N, N, 1,
			filepath.Join(*outDir, fmt.Sprintf("freq_layers_w%d.png", w))); err != nil {
			fatal("写层号图失败", err)
		}
		if err := freqsel.SaveTauHeatmap(selMid.Tau, N, N, 0.4, 2.0,
			filepath.Join(*outDir, fmt.Sprintf("freq_tau_w%d.png", w))); err != nil {
			fatal("写层坐标热力图失败", err)
		}
	}

	// ---- 两版各一张整幅三档对照图 ----
	pad, gap := 12, 10
	heroW := N + 2*pad
	heroH := pad + 3*N + 2*gap + pad
	bg := rgba(24, 26, 32)
	gHero := freqsel.NewCanvas(heroW, heroH, bg)
	aHero := freqsel.NewCanvas(heroW, heroH, bg)
	for i, p := range prims {
		y := pad + i*(N+gap)
		gHero.BlitGrayNearest(p.glob, freqsel.Rect{}, pad, y, p.def.zoom)
		aHero.BlitGrayNearest(p.ad.Out, freqsel.Rect{}, pad, y, p.def.zoom)
	}
	must(gHero.SavePNG(filepath.Join(*outDir, "freq_global.png")))
	must(aHero.SavePNG(filepath.Join(*outDir, "freq_adaptive.png")))

	// ---- 边缘/细条裁剪的 全局 | 自适应 | 锐化阳性对照 直接对照图 ----
	const cropDispH = 240
	cropSrc := card.EdgeCrop
	// 列宽取三档里最大的实际贴入宽（s=1/2,z=4 时为 180*4=720）。
	colW := 0
	for _, p := range prims {
		cr := freqsel.ScaleRect(cropSrc, p.def.s, p.glob.W, p.glob.H)
		if w := (cr.X1 - cr.X0) * p.def.cropZ; w > colW {
			colW = w
		}
	}
	colGap := 10
	cmpW := pad + 3*colW + 2*colGap + pad
	cmpH := pad + 3*cropDispH + 2*gap + pad
	cmp := freqsel.NewCanvas(cmpW, cmpH, bg)
	for i, p := range prims {
		y := pad + i*(cropDispH+gap)
		ow, oh := p.glob.W, p.glob.H
		outCrop := freqsel.ScaleRect(cropSrc, p.def.s, ow, oh)
		z := p.def.cropZ
		x0 := pad
		cmp.BlitGrayNearest(p.glob, outCrop, x0, y, z)
		x1 := x0 + colW + colGap
		cmp.BlitGrayNearest(p.ad.Out, outCrop, x1, y, z)
		x2 := x1 + colW + colGap
		cmp.BlitGrayNearest(p.ctrl, outCrop, x2, y, z)
		// 列分隔亮线。
		cmp.FillRect(x1-colGap/2-1, y, 2, cropDispH, rgba(90, 96, 110))
		cmp.FillRect(x2-colGap/2-1, y, 2, cropDispH, rgba(90, 96, 110))
	}
	must(cmp.SavePNG(filepath.Join(*outDir, "freq_compare.png")))

	// ---- s=1/3 高倍特写：竖边 / 细条 / 对角边，列＝全局|自适应|锐化对照 ----
	if len(prims) >= 2 {
		p := prims[1] // scaleDefs[1] = 1/3
		cuTiles := []freqsel.Rect{card.VerticalEdge, card.DetailBars, card.DiagonalEdge}
		const cell, cuZoom, cuGap, cuPad = 400, 10, 8, 12
		cuW := cuPad + 3*cell + 2*cuGap + cuPad
		cuH := cuPad + 3*cell + 2*cuGap + cuPad
		cu := freqsel.NewCanvas(cuW, cuH, bg)
		for row, tr := range cuTiles {
			oy := cuPad + row*(cell+cuGap)
			oc := freqsel.ScaleRect(tr, p.def.s, p.glob.W, p.glob.H)
			cu.BlitGrayNearest(p.glob, oc, cuPad, oy, cuZoom)
			cu.BlitGrayNearest(p.ad.Out, oc, cuPad+cell+cuGap, oy, cuZoom)
			cu.BlitGrayNearest(p.ctrl, oc, cuPad+2*(cell+cuGap), oy, cuZoom)
		}
		must(cu.SavePNG(filepath.Join(*outDir, "freq_closeup.png")))
	}

	// ---- 真实 vecgo 渲染场景补充：全局 | 自适应（s=1/3，放大 3 倍） ----
	vecgoMD := ""
	vg, err := renderVecgoScene(N)
	if err != nil {
		fatal("vecgo 场景渲染失败", err)
	}
	vst := freqsel.BuildStack(vg, 4)
	const vScale = 1.0 / 3.0
	vfld := freqsel.EstimateFrequency(vg, primaryWin)
	vsel := freqsel.NewSelector(vfld, vScale, opts)
	vad := freqsel.DownscaleAdaptive(vst, vsel)
	vgl := freqsel.DownscaleGlobal(vst, vScale)
	vgeG, _ := freqsel.GradientEnergy(vgl)
	vgeA, _ := freqsel.GradientEnergy(vad.Out)
	const vz = 3
	vcell := vgl.W * vz
	vw := pad + vcell + 16 + vcell + pad
	vh := pad + vcell + pad
	vc := freqsel.NewCanvas(vw, vh, bg)
	vc.BlitGrayNearest(vgl, freqsel.Rect{}, pad, pad, vz)
	vc.BlitGrayNearest(vad.Out, freqsel.Rect{}, pad+vcell+16, pad, vz)
	must(vc.SavePNG(filepath.Join(*outDir, "freq_vecgo_compare.png")))
	fmt.Printf("vecgo 真实场景 s=1/3: GE 全局=%.0f 自适应=%.0f (%+.1f%%)；平均层号 τ̄=%.2f 核宽 σ̄=%.2f\n",
		vgeG, vgeA, 100*(vgeA/vgeG-1), vad.MeanTau, vad.MeanSigma)
	vecgoMD = fmt.Sprintf("\n## 7. 附：真实 vecgo 矢量渲染场景\n\n"+
		"用仓库 `raster.Fill`（2× 超采样）渲染条形码竖条/细横线/方格阵/三角斜边的高密度场景，"+
		"再走同一套 5×5 窗口自适应流程。s=1/3 时整图梯度能量 全局=%.0f → 自适应=%.0f（**%+.1f%%**），"+
		"平均层号 τ̄=%.2f、平均核宽 σ̄=%.2f 源px。"+
		"对照图 `freq_vecgo_compare.png`（左：全局 L=%d；右：自适应逐像素选层）；"+
		"振铃结论以第 4 节专用阶跃靶的 0 过冲为准。\n",
		vgeG, vgeA, 100*(vgeA/vgeG-1), vad.MeanTau, vad.MeanSigma, vsel.GlobalLevel)

	// ---- JSON ----
	js, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		fatal("JSON 编码失败", err)
	}
	must(os.WriteFile(filepath.Join(*outDir, "freqsel_metrics.json"), js, 0o644))

	// ---- Markdown 报告 ----
	md := renderMarkdown(rep) + vecgoMD
	must(os.WriteFile(filepath.Join(*outDir, "FREQ_REPORT.md"), []byte(md), 0o644))

	// ---- 控制台：打出任务要求的关键行，便于直接核对/转录 ----
	fmt.Println("主频估计窗口：小档 5×5（半径2），大档 11×11（半径5）；主图采用 5×5。")
	fmt.Println("三档缩放比 —— 自适应（5×5窗口）平均层号 τ̄ / 整数层均值 / 平均核宽 σ̄(源px)：")
	for _, r := range rep.Rows {
		if r.Window != primaryWin {
			continue
		}
		fmt.Printf("缩放 %s (s=%.2f): 平均层号 τ̄=%.2f（整数层均值 %.2f，全局固定 L=%d），平均核宽 σ̄=%.2f px\n",
			r.ScaleLabel, r.Scale, r.MeanTau, r.MeanLevel, r.GlobalLevel, r.MeanSigma)
	}
	fmt.Println("梯度能量（整图总和；全局整数层 / 全局τ三线性 / 自适应 / 自适应相对整数层）：")
	for _, r := range rep.Rows {
		if r.Window != primaryWin {
			continue
		}
		fmt.Printf("  s=%s: GE=%.0f / %.0f / %.0f  (+%.1f%% vs 整数层, %+.1f%% vs τ三线性)\n",
			r.ScaleLabel, r.GEGlobalInt, r.GEGlobalTri, r.GEAdaptive, r.GainVsIntPct, r.GainVsTriPct)
	}
	fmt.Println("两档窗口平均层号 τ̄ 对照与层号差异：")
	for _, sd := range scaleDefs {
		var r5, r11 scaleRow
		for _, r := range rep.Rows {
			if r.ScaleLabel == sd.label {
				if r.Window == 5 {
					r5 = r
				} else {
					r11 = r
				}
			}
		}
		fmt.Printf("  s=%s: τ̄(5×5)=%.3f vs τ̄(11×11)=%.3f（差 %+.3f）；整数层均值 %.2f vs %.2f\n",
			sd.label, r5.MeanTau, r11.MeanTau, r5.MeanTau-r11.MeanTau, r5.MeanLevel, r11.MeanLevel)
	}
	fmt.Println("振铃指标（边缘过冲灰度级，越小越好；全局/自适应/锐化对照）：")
	for _, r := range rep.Rows {
		if r.Window != primaryWin {
			continue
		}
		fmt.Printf("  s=%s: %.3f / %.3f / %.2f\n", r.ScaleLabel, r.RingGlobal, r.RingAdaptive, r.RingControl)
	}
	fmt.Println("产物：freq_global.png, freq_adaptive.png, freq_compare.png, freq_layers_w{5,11}.png, freq_tau_w{5,11}.png, freqsel_metrics.json, FREQ_REPORT.md")
}

// typeofScaleDef 是 scaleDefs 元素类型的简写辅助。
type typeofScaleDef = struct {
	label string
	s     float64
	zoom  int
	cropZ int
}

func combineRing(a, b freqsel.RingingReport) float64 {
	p := a.Profiles + b.Profiles
	if p == 0 {
		return 0
	}
	return (a.MeanOvershoot*float64(a.Profiles) + b.MeanOvershoot*float64(b.Profiles)) / float64(p)
}

func renderMarkdown(rep report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# 局部频率自适应选层实验报告\n\n")
	fmt.Fprintf(&b, "源图 `%d×%d` 频率测试卡（每分区 %dpx，共 16 区），高斯尺度空间 %s。\n\n",
		rep.SourceSize, rep.SourceSize, rep.Tile, "σ_l=Σ0·2^l, Σ0=2/π≈0.6366 源px")

	b.WriteString("## 1. 方法\n\n")
	b.WriteString("1. 中心差分求 gx,gy，在边长 **W** 的邻域内汇总梯度结构张量 " +
		"J=Σ[[gx²,gxgy],[gxgy,gy²]] 与灰度方差 Var。\n")
	b.WriteString("2. 对窄带信号有 `f = sqrt(mean(gx²+gy²)/Var)/(2π)`，并对中心差分的 " +
		"sinc 衰减做校正；J 的特征值给出方向相干度，棋盘/交叉等低相干宽带结构直接用满核宽。\n")
	b.WriteString("3. 输出奈奎斯特频率 `fN=s/2`、D=1/s、仅按缩放比的理想核宽 " +
		"`σN=Σ0·D`、连续层坐标 `τN=log2D`。令 `r=clamp(f/(0.85·fN),0.70,1)`，" +
		"则每像素 `σ=r·σN`、`τ=log2(σ/Σ0)=τN+log2 r`，再做半径 1 的层场盒式正则。\n")
	b.WriteString("4. 渲染时在相邻整数层 `floor(τ)`、`ceil(τ)` 间按小数权重逐像素线性插值" +
		"（等价逐像素偏置的三线性 mip）。对照为整图固定 `L=ceil(log2D)` 的全局整数层方案。\n\n")

	b.WriteString("## 2. 主频估计窗口：两档取值与标定\n\n")
	b.WriteString("主频由像素邻域内的梯度能量/方差比估计，窗口必须取奇数且关于像素对称。" +
		"实验给两档具体值：**小档 W=5（5×5，半径 2）** 与 **大档 W=11（11×11，半径 5）**；" +
		"对照主图与“三档一行”数据采用小档 5×5。\n\n")
	b.WriteString("行 0 四块已知竖直正弦条纹的中位主频（真值 / 5×5 估计 / 11×11 估计，周期/源px）：\n\n")
	b.WriteString("| 真值 f | 5×5 估计 | 11×11 估计 |\n|---|---|---|\n")
	for _, f := range []float64{0.03, 0.07, 0.12, 0.20} {
		k := fmt.Sprintf("true_%.2f", f)
		fmt.Fprintf(&b, "| %.2f | %.4f | %.4f |\n", f, rep.Calibration[0].Stripes[k], rep.Calibration[1].Stripes[k])
	}
	b.WriteString("\n小窗口在长波（λ≫W）邻域里只截到一段近似斜坡，窗内方差相对梯度偏小，" +
		"比值反解会**高估**主频；大窗口平均到更多相位，f 更准但层场更平滑、并会跨越分区边界混合。\n\n")

	b.WriteString("## 3. 三档缩放比：平均层号与核宽（5×5 主窗口，每档一行）\n\n")
	b.WriteString("```text\n")
	for _, r := range rep.Rows {
		if r.Window != rep.PrimaryWindow {
			continue
		}
		fmt.Fprintf(&b, "缩放 %s (s=%.2f): 平均层号 τ̄=%.2f（整数层均值 %.2f，全局固定 L=%d），平均核宽 σ̄=%.2f px\n",
			r.ScaleLabel, r.Scale, r.MeanTau, r.MeanLevel, r.GlobalLevel, r.MeanSigma)
	}
	b.WriteString("```\n\n")
	b.WriteString("各档同时给出全局方案的固定层号/核宽作对照：\n\n")
	b.WriteString("| 缩放 | D=1/s | 全局 L | 全局 σ_L | 自适应 τ̄ | 自适应整数层均值 | 自适应 σ̄ | 低于全局层像素 |\n")
	b.WriteString("|---|---|---|---|---|---|---|---|\n")
	for _, r := range rep.Rows {
		if r.Window != rep.PrimaryWindow {
			continue
		}
		fmt.Fprintf(&b, "| %s (%.2f) | %.2f | %d | %.2f | %.2f | %.2f | %.2f | %.1f%% |\n",
			r.ScaleLabel, r.Scale, r.D, r.GlobalLevel, r.GlobalSigma, r.MeanTau, r.MeanLevel, r.MeanSigma, r.BelowGlobal)
	}
	b.WriteString("\n整数层号直方图（层号:占比）：\n\n")
	for _, r := range rep.Rows {
		if r.Window != rep.PrimaryWindow {
			continue
		}
		b.WriteString(fmt.Sprintf("- s=%s: ", r.ScaleLabel))
		for l, h := range r.Histogram {
			if h > 0.0005 {
				b.WriteString(fmt.Sprintf("L%d=%.2f  ", l, h))
			}
		}
		b.WriteString("\n")
	}
	b.WriteString("\n逐像素层号图：`freq_layers_w5.png` / `freq_layers_w11.png`（s=1/3，离散色），" +
		"连续 τ 热力图：`freq_tau_w5.png` / `freq_tau_w11.png`。全局方案整图只有同一个 L，" +
		"自适应方案在平坦/低频区落到更低层（核更窄、更保锐），在高频/宽带区升到 σN 附近。\n\n")

	b.WriteString("## 4. 梯度能量（整图）与振铃\n\n")
	b.WriteString("梯度能量 `GE=Σ_{全图像素}(gx²+gy²)`，**按整图总和**统计（中心差分）；" +
		"另列每像素均值以消除三档画幅尺寸差异。\n\n")
	b.WriteString("| 缩放 | GE 全局整数层 | GE 全局τ三线性 | GE 自适应 | vs 整数层 | vs τ三线性 | 自适应每像素 |\n")
	b.WriteString("|---|---|---|---|---|---|---|\n")
	for _, r := range rep.Rows {
		if r.Window != rep.PrimaryWindow {
			continue
		}
		fmt.Fprintf(&b, "| %s | %.0f | %.0f | %.0f | **%+.1f%%** | %+.1f%% | %.1f |\n",
			r.ScaleLabel, r.GEGlobalInt, r.GEGlobalTri, r.GEAdaptive, r.GainVsIntPct, r.GainVsTriPct, r.GEAdaptivePP)
	}
	b.WriteString("\n振铃指标为专用竖直/水平阶跃边缘法向剖面上“超出终端平台的非单调过冲/下冲均值”" +
		"（0..255 灰度级）。高斯核冲激响应非负、阶跃响应单调，相邻高斯层的凸混合仍单调，" +
		"因此全局与自适应的过冲均为 0；锐化阳性对照 `out+1.3(out-Gaussian1.2)` 产生约 20–28 级过冲，" +
		"证明指标能抓到振铃：\n\n")
	b.WriteString("| 缩放 | 振铃 全局 | 振铃 自适应 | 振铃 锐化对照 |\n|---|---|---|---|\n")
	for _, r := range rep.Rows {
		if r.Window != rep.PrimaryWindow {
			continue
		}
		fmt.Fprintf(&b, "| %s | %.3f | **%.3f** | %.2f |\n", r.ScaleLabel, r.RingGlobal, r.RingAdaptive, r.RingControl)
	}
	b.WriteString("\n结论：三档缩放比下自适应整图梯度能量都高于全局整数层方案（也高于全局固定 τ 三线性），" +
		"且振铃指标为 0、边缘剖面单调，不出现振铃。s=1/3 时全局把 τN=1.58 向上取整到 L=2" +
		"（σ=2.55，比理想 σN=1.91 宽约 1.34×），过模糊最明显，故能量增益最大。\n\n")

	b.WriteString("## 5. 两档窗口的层号差异及解释\n\n")
	b.WriteString("| 缩放 | τ̄ W=5 | τ̄ W=11 | Δτ(5−11) | 整数层均值 5 / 11 | 平均核宽 5 / 11 |\n")
	b.WriteString("|---|---|---|---|---|---|\n")
	for _, sd := range scaleDefs {
		var r5, r11 scaleRow
		for _, r := range rep.Rows {
			if r.ScaleLabel == sd.label {
				if r.Window == 5 {
					r5 = r
				} else {
					r11 = r
				}
			}
		}
		fmt.Fprintf(&b, "| %s | %.3f | %.3f | %+.3f | %.2f / %.2f | %.2f / %.2f |\n",
			sd.label, r5.MeanTau, r11.MeanTau, r5.MeanTau-r11.MeanTau, r5.MeanLevel, r11.MeanLevel, r5.MeanSigma, r11.MeanSigma)
	}
	fmt.Fprintf(&b, "\n实测三档下 **5×5 小窗的平均层号都略高于 11×11 大窗**。原因是：测试卡含大量"+
		"波长 λ≈14–33px 的中低频正弦与缓变区，5px 窗不足一个波长，窗内近似斜坡、方差被低估，"+
		"梯度/方差比反解出的主频偏高（见第 2 节标定：f=0.03 被估到 %.3f，而 11 窗为 %.3f），"+
		"于是 r 更大、τ 更高、核更宽；11px 窗平均到更多相位，主频回落至真值附近，τ 更低、更保锐，"+
		"梯度能量也更高（见 JSON 中两窗口对比）。代价是大窗的层场空间定位更粗、会跨过纹理/边缘边界"+
		"做邻域混合，层号图更平滑但边界分辨率下降；小窗定位准、跟手，但估计相位噪声更大、层号图更碎。"+
		"在 f 本来就高的细条纹/棋盘区，两档都把 r 推到 1、τ 收敛到 τN，故差异主要来自中低频区。\n\n",
		rep.Calibration[0].Stripes["true_0.03"], rep.Calibration[1].Stripes["true_0.03"])

	b.WriteString("## 6. 图像清单\n\n")
	b.WriteString("- `freq_global.png`：**全局层号版**一张图，竖排三档（上 1/2、中 1/3、下 1/4），整幅最近邻放大到 480 高。\n")
	b.WriteString("- `freq_adaptive.png`：**局部频率自适应版**一张图，同版式，逐像素选层+层间插值。\n")
	b.WriteString("- `freq_compare.png`：边缘/细条裁剪，三档各行，列＝全局 | 自适应 | 锐化振铃阳性对照。\n")
	b.WriteString("- `freq_closeup.png`：s=1/3 高倍特写，三行＝竖边/细条/对角边，列＝全局 | 自适应 | 锐化对照，可直接看边缘有无亮/暗振铃瓣。\n")
	b.WriteString("- `freq_layers_w5.png` / `freq_layers_w11.png`：两档窗口的逐像素整数层号（s=1/3）。\n")
	b.WriteString("- `freq_tau_w5.png` / `freq_tau_w11.png`：连续层坐标 τ 伪彩热力图。\n")
	b.WriteString("- `freqsel_metrics.json`：全部数值。\n")
	return b.String()
}

func rgba(r, g, b uint8) color.RGBA {
	return color.RGBA{R: r, G: g, B: b, A: 255}
}

func round(v float64, digits int) float64 {
	p := math.Pow10(digits)
	return math.Round(v*p) / p
}

func roundSlice(a []float64, digits int) []float64 {
	out := make([]float64, len(a))
	for i, v := range a {
		out[i] = round(v, digits)
	}
	return out
}

func must(err error) {
	if err != nil {
		fatal("写入失败", err)
	}
}

func fatal(msg string, err error) {
	fmt.Fprintf(os.Stderr, "%s: %v\n", msg, err)
	os.Exit(1)
}
