// Command junctionreport 复算三边接点的扇区面积/守恒权重，逐接点核对权重和，
// 生成“除以三”反例数据、逐像素亮度台账与可视化，并把全部证据写入 artifacts/。
//
//	go run ./cmd/junctionreport
package main

import (
	"encoding/csv"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"

	"vecgo/junction"
	"vecgo/raster"
)

const artifactsDir = "artifacts"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "junctionreport 失败:", err)
		os.Exit(1)
	}
}

func run() error {
	if err := os.MkdirAll(artifactsDir, 0o755); err != nil {
		return err
	}
	opts := junction.DefaultOptions()
	classes := junction.Classes()
	all := append(append([]junction.Junction{}, classes...), junction.ExtraJunctions()...)
	all = append(all, junction.SpotCheckJunctions()...)

	classRes, err := junction.AuditAll(classes, opts)
	if err != nil {
		return err
	}
	allRes, err := junction.AuditAll(all, opts)
	if err != nil {
		return err
	}

	// 1) 汇总 CSV：三类标准接点（面积两列组 + 权重列组 + 亮度偏差列）。
	if err := writeMainCSV(classRes); err != nil {
		return err
	}
	// 2) 每个接点单独一行的权重和核对。
	if err := writeSumCheckCSV(allRes); err != nil {
		return err
	}
	// 3) “除以三”反例数据。
	if err := writeCounterexampleCSV(classRes); err != nil {
		return err
	}
	// 4) 环亮度汇总 + 逐像素明细。
	if err := writeBrightnessCSVs(classRes); err != nil {
		return err
	}
	// 5) 独立裁剪面积守恒。
	if err := writeClipCSV(allRes); err != nil {
		return err
	}
	// 6) 可视化环（PNG + PGM）。
	if err := renderRings(classRes); err != nil {
		return err
	}
	// 7) Markdown 报告。
	md := buildMarkdown(classRes, allRes)
	if err := os.WriteFile(filepath.Join(artifactsDir, "junction-report.md"), []byte(md), 0o644); err != nil {
		return err
	}

	fmt.Print(md)
	fmt.Println("\n已写出 artifacts/junction-report.md 及 junction-*.csv / junction-rings.png")
	return nil
}

func f6(x float64) string  { return fmt.Sprintf("%.6f", x) }
func f3(x float64) string  { return fmt.Sprintf("%.3f", x) }
func f2e(x float64) string { return fmt.Sprintf("%.2e", x) }

func writeMainCSV(rs []junction.Result) error {
	p := filepath.Join(artifactsDir, "junction-main.csv")
	f, err := os.Create(p)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	_ = w.Write([]string{
		"junction_id", "angles_deg",
		"area_1_px2", "area_2_px2", "area_3_px2", "area_sum",
		"weight_1", "weight_2", "weight_3", "weight_sum",
		"brightness_delta_levels", "ledger_float_delta",
	})
	for _, r := range rs {
		d := r.Junction.Degrees()
		_ = w.Write([]string{
			r.Junction.ID,
			fmt.Sprintf("%.0f/%.0f/%.0f", d[0], d[1], d[2]),
			f6(r.Areas[0]), f6(r.Areas[1]), f6(r.Areas[2]), f6(junction.SumAreas(r.Areas)),
			fmt.Sprintf("%.9f", r.Weights[0]), fmt.Sprintf("%.9f", r.Weights[1]), fmt.Sprintf("%.9f", r.Weights[2]),
			fmt.Sprintf("%.15f", r.WeightSum),
			fmt.Sprintf("%d", r.Ring.CorrectedMaxDelta),
			fmt.Sprintf("%.2e", r.Ring.LedgerMaxFloatDelta),
		})
	}
	return w.Error()
}

func writeSumCheckCSV(rs []junction.Result) error {
	p := filepath.Join(artifactsDir, "junction-sumcheck.csv")
	f, err := os.Create(p)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	_ = w.Write([]string{"junction_id", "label", "angles_deg", "w1", "w2", "w3", "weight_sum", "abs_sum_minus_1", "holds"})
	for _, r := range rs {
		d := r.Junction.Degrees()
		_ = w.Write([]string{
			r.Junction.ID, r.Junction.Name,
			fmt.Sprintf("%.1f/%.1f/%.1f", d[0], d[1], d[2]),
			fmt.Sprintf("%.12f", r.Weights[0]), fmt.Sprintf("%.12f", r.Weights[1]), fmt.Sprintf("%.12f", r.Weights[2]),
			fmt.Sprintf("%.15f", r.WeightSum), fmt.Sprintf("%.3e", r.WeightSumError),
			fmt.Sprintf("%v", r.WeightSumHolds),
		})
	}
	return w.Error()
}

func writeCounterexampleCSV(rs []junction.Result) error {
	p := filepath.Join(artifactsDir, "junction-counterexample.csv")
	f, err := os.Create(p)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	_ = w.Write([]string{
		"junction_id", "sector", "required_area", "uniform_thirds_area", "misalloc_plus_over_minus_gap",
		"brightness_delta_unclamped_levels",
	})
	for _, r := range rs {
		for i := 0; i < 3; i++ {
			_ = w.Write([]string{
				r.Junction.ID, fmt.Sprintf("%d", i+1),
				f6(r.Thirds.RequiredArea[i]), f6(r.Thirds.UniformArea[i]), f6(r.Thirds.MisallocArea[i]),
				f3(r.Thirds.BrightnessDelta[i]),
			})
		}
		_ = w.Write([]string{
			r.Junction.ID, "LITERAL_Si/3",
			f6(junction.SumAreas(r.Areas)), f6(junction.SumAreas(r.Areas) / 3),
			"-" + f6(r.Thirds.LiteralGapArea),
			"-" + f3(r.Thirds.LiteralDarkLevels),
		})
	}
	return w.Error()
}

func writeBrightnessCSVs(rs []junction.Result) error {
	sumPath := filepath.Join(artifactsDir, "junction-brightness-summary.csv")
	f, err := os.Create(sumPath)
	if err != nil {
		return err
	}
	w := csv.NewWriter(f)
	_ = w.Write([]string{
		"junction_id", "surround_gray", "corrected_max_delta",
		"naive_gray", "naive_overshoot",
		"thirds_literal_undershoot", "thirds_uniform_undershoot", "thirds_uniform_overshoot",
	})
	for _, r := range rs {
		_ = w.Write([]string{
			r.Junction.ID, "200", fmt.Sprintf("%d", r.Ring.CorrectedMaxDelta),
			fmt.Sprintf("%d", r.Ring.NaiveMaxGray), fmt.Sprintf("%d", r.Ring.NaiveMaxOvershoot),
			fmt.Sprintf("%d", r.Ring.ThirdsLiteralMaxUndershoot),
			fmt.Sprintf("%d", r.Ring.ThirdsUniformMaxUndershoot),
			fmt.Sprintf("%d", r.Ring.ThirdsUniformMaxOvershoot),
		})
	}
	w.Flush()
	f.Close()
	if err := w.Error(); err != nil {
		return err
	}

	pxPath := filepath.Join(artifactsDir, "junction-ring-pixels.csv")
	pf, err := os.Create(pxPath)
	if err != nil {
		return err
	}
	defer pf.Close()
	pw := csv.NewWriter(pf)
	defer pw.Flush()
	_ = pw.Write([]string{
		"junction_id", "pixel_index", "angle_deg", "sector",
		"surround", "corrected", "naive_add", "thirds_literal", "thirds_uniform_w",
	})
	for _, r := range rs {
		for _, p := range r.Ring.Pixels {
			_ = pw.Write([]string{
				r.Junction.ID, fmt.Sprintf("%d", p.Index), f3(p.AngleDeg), fmt.Sprintf("%d", p.Sector),
				fmt.Sprintf("%d", p.SurroundGray), fmt.Sprintf("%d", p.CorrectedGray),
				fmt.Sprintf("%d", p.NaiveAddGray), fmt.Sprintf("%d", p.ThirdsLiteralGray),
				fmt.Sprintf("%d", p.ThirdsUniformGray),
			})
		}
	}
	return pw.Error()
}

func writeClipCSV(rs []junction.Result) error {
	p := filepath.Join(artifactsDir, "junction-clip-conservation.csv")
	f, err := os.Create(p)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	_ = w.Write([]string{"junction_id", "clip_area_1", "clip_area_2", "clip_area_3", "sum", "abs_sum_minus_1"})
	for _, r := range rs {
		_ = w.Write([]string{
			r.Junction.ID,
			fmt.Sprintf("%.9f", r.CenterPixelAreas[0]),
			fmt.Sprintf("%.9f", r.CenterPixelAreas[1]),
			fmt.Sprintf("%.9f", r.CenterPixelAreas[2]),
			fmt.Sprintf("%.12f", junction.SumAreas(r.CenterPixelAreas)),
			fmt.Sprintf("%.3e", r.CenterConservationErr),
		})
	}
	return w.Error()
}

// ---------- 可视化 ----------

const (
	tileN  = 121
	pad    = 8
	cols   = 4
	rowsN  = 3
	ringR  = 27.0
	ringW  = 1.7
	bgGray = uint8(200)
)

func renderRings(rs []junction.Result) error {
	W := pad + cols*tileN + (cols-1)*pad + pad
	H := pad + rowsN*tileN + (rowsN-1)*pad + pad
	gray := image.NewGray(image.Rect(0, 0, W, H))
	for i := range gray.Pix {
		gray.Pix[i] = 245 // 画板底
	}
	rim, err := raster.New(W, H)
	if err != nil {
		return err
	}

	for row, r := range rs {
		ox := pad
		oy := pad + row*(tileN+pad)
		ws := r.Weights
		for col := 0; col < cols; col++ {
			drawTile(gray, ox, oy, col, r.Junction.Angles, ws)
			ox += tileN + pad
		}
	}

	// PNG（纳入版本管理/便于查看）。
	pngPath := filepath.Join(artifactsDir, "junction-rings.png")
	pf, err := os.Create(pngPath)
	if err != nil {
		return err
	}
	if err := png.Encode(pf, gray); err != nil {
		pf.Close()
		return err
	}
	if err := pf.Close(); err != nil {
		return err
	}

	// PGM（走 vecgo/raster 自己的导出器；*.pgm 被 .gitignore 忽略，仅本地查看）。
	for i, v := range gray.Pix {
		rim.Pix[i] = v
	}
	return rim.SavePGM(filepath.Join(artifactsDir, "junction-rings.pgm"))
}

func drawTile(img *image.Gray, ox, oy, mode int, ang junction.Angles, w junction.Weights) {
	jm := []junction.RingMode{
		junction.ModeWeighted, junction.ModeNaiveAdd,
		junction.ModeThirdsLiteral, junction.ModeThirdsUniform,
	}[mode]
	c := float64(tileN) / 2
	for y := 0; y < tileN; y++ {
		for x := 0; x < tileN; x++ {
			g := bgGray
			dx := float64(x) + 0.5 - c
			dy := -(float64(y) + 0.5 - c) // 翻 y，使角度按数学逆时针
			d := math.Hypot(dx, dy)
			if math.Abs(d-ringR) <= ringW {
				phi := math.Atan2(dy, dx)
				if phi < 0 {
					phi += junction.Tau
				}
				sec := junction.SectorAt(ang, phi)
				g = junction.RingGray(jm, sec, w, float64(bgGray))
			}
			// 1px 黑边框。
			if x == 0 || y == 0 || x == tileN-1 || y == tileN-1 {
				g = 0
			}
			img.SetGray(ox+x, oy+y, color.Gray{Y: g})
		}
	}
}

// ---------- Markdown ----------

func buildMarkdown(classRes, allRes []junction.Result) string {
	var b strings.Builder
	b.WriteString("# 三边接点抗锯齿：扇区面积守恒权重核验\n\n")
	b.WriteString("由 `go run ./cmd/junctionreport` 重新生成；原始数据见同目录 `junction-*.csv` 与 `junction-ring-pixels.csv`。\n\n")

	b.WriteString("## 1. 模型与反解\n\n")
	b.WriteString("三条边（射线）把接点邻域切成夹角 θ₀,θ₁,θ₂ 的三个扇形，Σθ=2π。半径 r 的扇区面积与守恒权重：\n\n")
	b.WriteString("```\nA_i = 1/2 r² θ_i,   ΣA_i = πr²\nw_i · πr² = A_i  ⇒  w_i = A_i/ΣA = θ_i/(2π),   Σw_i = 1\n```\n\n")
	b.WriteString("`Σw_i=1` 是配分不变量：接点处三份覆盖索赔加权后总亮度不被放大，因而与周围普通像素连续。\n\n")

	// 主表：面积列、权重列、亮度偏差列（第三列）。
	b.WriteString("## 2. 三类接点：扇区面积、权重与亮度偏差\n\n")
	b.WriteString("r=1 像素（面积单位 px²），参考灰度 ink=200/255。亮度偏差为面积权重修正后环上 720 个像素\n")
	b.WriteString("逐个与周围灰度比较的最大绝对偏差（0–255 阶）。\n\n")
	b.WriteString("| 接点角度 | 扇区面积 A₁,A₂,A₃ (px²) | 权重 w₁,w₂,w₃ | 亮度偏差 ΔB（第三列） |\n")
	b.WriteString("|---|---|---|---:|\n")
	for _, r := range classRes {
		d := r.Junction.Degrees()
		fmt.Fprintf(&b, "| %s **%.0f/%.0f/%.0f°** | %s, %s, %s | %.6f, %.6f, %.6f | **%d 阶**；台账 %.2e；裁剪守恒残差 %.1e |\n",
			r.Junction.ID, d[0], d[1], d[2],
			f6(r.Areas[0]), f6(r.Areas[1]), f6(r.Areas[2]),
			r.Weights[0], r.Weights[1], r.Weights[2],
			r.Ring.CorrectedMaxDelta, r.Ring.LedgerMaxFloatDelta, r.CenterConservationErr)
	}
	b.WriteString("\n面积和均为 π=" + f6(math.Pi) + " px²，权重和均为 1.000000000000000。\n\n")

	// 逐接点权重和核对。
	b.WriteString("## 3. “权重之和恒为一”逐接点单独核对\n\n")
	b.WriteString("每一行是一个独立接点，各自判定，不做平均。\n\n")
	b.WriteString("| 接点 | 角度(度) | w₁ | w₂ | w₃ | Σw | \\|Σw−1\\| | 判定 |\n")
	b.WriteString("|---|---|---:|---:|---:|---:|---:|:---:|\n")
	for _, r := range allRes {
		d := r.Junction.Degrees()
		verdict := "✅"
		if !r.WeightSumHolds {
			verdict = "❌"
		}
		fmt.Fprintf(&b, "| %s | %.1f/%.1f/%.1f | %.9f | %.9f | %.9f | %.15f | %.2e | %s |\n",
			r.Junction.ID, d[0], d[1], d[2],
			r.Weights[0], r.Weights[1], r.Weights[2], r.WeightSum, r.WeightSumError, verdict)
	}
	b.WriteString("\n")

	// 除以三反例。
	b.WriteString("## 4. 反例数据：只把每个扇区除以 3 会留下缺口\n\n")
	b.WriteString("统一取 w=1/3 作用在守恒总预算 πr² 上时，扇区 i 只分到 πr²/3，而守恒要求 A_i。\n")
	b.WriteString("错配 `πr²/3 − A_i` 为负即覆盖缺口。亮度为未截断的相对灰度偏差（ink=255 口径）。\n\n")
	b.WriteString("| 接点 | 扇区 | 守恒面积 A_i | “除以3”面积 | 错配(+过/−缺口) | 亮度偏差(阶) |\n")
	b.WriteString("|---|---:|---:|---:|---:|---:|\n")
	for _, r := range classRes {
		for i := 0; i < 3; i++ {
			fmt.Fprintf(&b, "| %s | %d | %.6f | %.6f | %+.6f | %+.1f |\n",
				r.Junction.ID, i+1, r.Thirds.RequiredArea[i], r.Thirds.UniformArea[i],
				r.Thirds.MisallocArea[i], r.Thirds.BrightnessDelta[i])
		}
		fmt.Fprintf(&b, "| %s | 字面 A_i/3 | 总 %.6f | 保留 %.6f | **缺口 −%.6f（2/3 总面积）** | **%.1f 阶暗缺口** |\n",
			r.Junction.ID, junction.TotalArea(r.Radius), junction.TotalArea(r.Radius)/3,
			r.Thirds.LiteralGapArea, r.Thirds.LiteralDarkLevels)
	}
	b.WriteString("\n关键反例（亮度均为满墨 255 口径的未截断相对偏差）：\n\n")
	tRes := classRes[1]
	aRes := classRes[2]
	fmt.Fprintf(&b, "- **T 接 90/90/180°**：180° 大扇区真实权重 1/2，除以三只给 1/3，面积缺口 π/2−π/3 = **%.6f px²（π/6）**，"+
		"该扇区欠亮 **%.1f 阶**；两个 90° 小扇区反而各多占 π/12、过亮 %.1f 阶。\n",
		tRes.Thirds.MaxDeficitArea, tRes.Thirds.WorstDarkLevels, tRes.Thirds.WorstBrightLevels)
	fmt.Fprintf(&b, "- **锐角偏置 60/90/210°**：210° 优角扇区缺口 **%.6f px²（π/4）**、欠亮约 **%.1f 阶**；"+
		"60° 小扇区相对过亮 %+.0f%%（线性值 +%.0f 阶，截断后全白）。\n",
		aRes.Thirds.MaxDeficitArea, aRes.Thirds.WorstDarkLevels,
		(aRes.Thirds.BrightnessDelta[0]/255)*100, aRes.Thirds.BrightnessDelta[0])
	b.WriteString("- **等角 120/120/120° 对照组**：w_i 恰好都是 1/3，错配为 0——这正是“都除以 3”看似可行的原因，" +
		"但它只在三等分接点成立，不能推广。\n")
	fmt.Fprintf(&b, "- **字面把每个扇区面积各自除以 3**：总覆盖从 πr² 掉到 πr²/3，任何角度下都丢掉 2/3，"+
		"满墨口径整环暗 **%.1f 阶**；换成第 5 节 ink=200 并做 8-bit 量化后为 **133 阶**（200−round(66.67)）。\n\n",
		classRes[0].Thirds.LiteralDarkLevels)

	// 亮度汇总。
	b.WriteString("## 5. 环像素亮度核对汇总（ink=200，截断到 0..255）\n\n")
	b.WriteString("| 接点 | 周围灰度 | 面积权重 ΔBmax | 朴素相加灰度/过冲 | 字面÷3 欠亮 | 统一1/3 欠亮/过亮 |\n")
	b.WriteString("|---|---:|---:|---:|---:|---:|\n")
	for _, r := range classRes {
		fmt.Fprintf(&b, "| %s | %d | **%d** | %d / +%d 阶 | −%d 阶 | −%d / +%d 阶 |\n",
			r.Junction.ID, r.Ring.Pixels[0].SurroundGray, r.Ring.CorrectedMaxDelta,
			r.Ring.NaiveMaxGray, r.Ring.NaiveMaxOvershoot,
			r.Ring.ThirdsLiteralMaxUndershoot,
			r.Ring.ThirdsUniformMaxUndershoot, r.Ring.ThirdsUniformMaxOvershoot)
	}
	b.WriteString("\n逐像素 720×3 行见 `junction-ring-pixels.csv`。可视化 `junction-rings.png` 每行一个接点，" +
		"四列依次为：面积权重修正（环消失，与背景同为 200）、朴素三倍相加（白亮环）、" +
		"字面÷3（暗环）、统一 1/3 错配（宽扇区暗/窄扇区亮）。\n\n")

	// 独立几何核验。
	b.WriteString("## 6. 独立面积裁切核验\n\n")
	b.WriteString("用仓库自己的 `geom.Clip`（Sutherland–Hodgman）把三个扇形与接点中心单位像素正方形求交、" +
		"`geom.Area`（鞋带公式）求和：三扇区恰好铺满平面，故每像素 Σa_i=1。\n\n")
	b.WriteString("| 接点 | 裁切面积 a₁,a₂,a₃ | Σa | \\|Σa−1\\| |\n")
	b.WriteString("|---|---|---:|---:|\n")
	for _, r := range classRes {
		fmt.Fprintf(&b, "| %s | %.6f, %.6f, %.6f | %.12f | %.2e |\n",
			r.Junction.ID, r.CenterPixelAreas[0], r.CenterPixelAreas[1], r.CenterPixelAreas[2],
			junction.SumAreas(r.CenterPixelAreas), r.CenterConservationErr)
	}
	b.WriteString("\n注意中心像素是正方形而非圆盘，裁切面积不必等于 ½r²θ 的圆弧扇形面积" +
		"（如等角接点为 0.322/0.356/0.322），但三者之和严格为 1，这正是逐像素面积守恒。\n\n")

	b.WriteString("## 7. 复现与测试\n\n")
	b.WriteString("```sh\n")
	b.WriteString("go test ./...\n")
	b.WriteString("go run ./cmd/junctionreport\n")
	b.WriteString("```\n\n")
	b.WriteString("- `junction/junction_test.go` 对 11 个接点逐个断言 Σw=1；\n")
	b.WriteString("- `TestEqualThirdsLeavesGapCounterexample` 锁定 π/6、π/4、85 与 109.286 阶等反例数值；\n")
	b.WriteString("- `TestBrightnessContinuity` 逐像素断言修正灰度==周围灰度，并锁定朴素 +55、字面÷3 −133 阶；\n")
	b.WriteString("- 默认 `raster.Image.Fill` 路径未改动，既有产物与测试保持不变。\n")
	return b.String()
}
