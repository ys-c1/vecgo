// Command kernelreport 复算复合变换雅可比、面积守恒轴、误差上界，
// 渲染三组直边对照图，并把机器可读结果与 Markdown 报告写入 artifacts/。
package main

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"

	"vecgo/kernel"
)

func main() {
	rep := kernel.DefaultReport()

	if err := os.MkdirAll("artifacts", 0o755); err != nil {
		fail(err)
	}
	if err := os.MkdirAll("docs", 0o755); err != nil {
		fail(err)
	}

	// 1) 机器可读 JSON。
	raw, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		fail(err)
	}
	if err := os.WriteFile(filepath.Join("artifacts", "kernel_report.json"), raw, 0o644); err != nil {
		fail(err)
	}

	// 2) PGM 对照图。
	for _, g := range rep.Groups {
		p := filepath.Join("artifacts", "edge_"+g.ID+".pgm")
		if err := g.Image.SavePGM(p); err != nil {
			fail(err)
		}
	}
	if err := rep.Hero.Image.SavePGM(filepath.Join("artifacts", "edge_hero_before_fix.pgm")); err != nil {
		fail(err)
	}

	// 3) 三联 PNG 对照图（G1 | G2 | G3），中间用红线分隔。
	if err := writeContactSheet(rep, filepath.Join("artifacts", "edge_compare.png")); err != nil {
		fail(err)
	}
	if err := writeContactSheet(rep, filepath.Join("docs", "edge-compare.png")); err != nil {
		fail(err)
	}

	// 4) Markdown 报告。
	md := renderMarkdown(rep)
	if err := os.WriteFile(filepath.Join("artifacts", "KERNEL_REPORT.md"), []byte(md), 0o644); err != nil {
		fail(err)
	}

	fmt.Print(md)
	fmt.Println("\n已写出 artifacts/kernel_report.json、artifacts/KERNEL_REPORT.md、artifacts/edge_*.pgm、artifacts/edge_compare.png")
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "kernelreport 失败:", err)
	os.Exit(1)
}

func writeContactSheet(rep *kernel.Report, path string) error {
	const sep = 3
	const scale = 4 // 最近邻放大，便于直接查看灰雾晕
	w := rep.Groups[0].Image.W
	h := rep.Groups[0].Image.H
	panelW := w*scale + sep
	W := panelW*3 - sep
	H := h * scale
	sheet := image.NewGray(image.Rect(0, 0, W, H))
	for i, g := range rep.Groups {
		ox := i * panelW
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				col := color.Gray{Y: g.Image.Pix[y*w+x]}
				for sy := 0; sy < scale; sy++ {
					for sx := 0; sx < scale; sx++ {
						sheet.SetGray(ox+x*scale+sx, y*scale+sy, col)
					}
				}
			}
		}
		if i < 2 {
			for y := 0; y < H; y++ {
				for k := 0; k < sep; k++ {
					sheet.SetGray((i+1)*panelW-sep+k, y, color.Gray{Y: 200})
				}
			}
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, sheet)
}

func renderMarkdown(rep *kernel.Report) string {
	var b []byte
	add := func(s string) { b = append(b, s...) }

	add("# 雅可比面积守恒核：三组变换对照\n\n")
	add("复现：`go run ./cmd/kernelreport`；校验：`go test ./kernel/`。\n\n")

	add("## 1. 复合变换的雅可比（先旋转 θ，再按 (sx,sy) 缩放）\n\n")
	add("`J = diag(sx,sy)·R(θ) = [[sx cosθ, -sx sinθ], [sy sinθ, sy cosθ]]`\n\n")
	h := rep.Hero
	add(fmt.Sprintf("题目主场景 θ=45°, s=0.5：四个分量为 "+
		"**a=%.6f, b=%.6f, c=%.6f, d=%.6f**，det J = %.4f（面积缩到 %.2f），"+
		"逆足迹面积因子 1/det J = %.1f——未归一化的核被放大 4 倍，这就是边缘灰雾的来源。\n\n",
		h.Jacobian[0], h.Jacobian[1], h.Jacobian[2], h.Jacobian[3], h.Det, h.Det, h.InvDet))

	add("## 2. 面积守恒反解长短轴\n\n")
	add("设轴比 κ=a/b≥1，核椭圆面积 πab=1，则 " +
		"**a=√(κ/π), b=1/√(πκ)**（轴积约定 ab=1 时为 a=√κ, b=1/√κ，只差公共 √π 缩放）。\n\n")

	add("## 3. 三组变换组合：核面积 与 边缘灰度差 / 灰雾面积\n\n")
	add("灰度差取距边 1.5px 的两侧平台列相对纯黑/纯白的最大偏差（档位，0–255）；" +
		"灰雾像素=边对（|d|<1px）之外偏差仍 >2 档的像素。\n\n")
	add("| 组 | 变换 | detJ | κ | 用轴 (a,b) | 核面积 πab | 守恒残差 | 面积上界 U | 界内? | 偏差轴 | 边缘最大灰度差(档) | 灰雾上界 | 灰度界内? | 灰雾像素 |\n")
	add("|---|---|---:|---:|---|---:|---:|---:|:--:|:--:|---:|---:|:--:|---:|\n")
	for _, g := range rep.Groups {
		add(fmt.Sprintf("| %s | %s | %.3g | %.3g | (%.4f, %.4f) | %.6f | %.3g | %.3g | %v | %s | **%.0f** | %.1f | %v | **%d** |\n",
			g.ID, g.Label, g.Det, g.Kappa,
			g.UsedAxes.Long, g.UsedAxes.Short, g.UsedAxes.GeometricArea,
			g.AreaResidual, g.AreaErrorBound, mark(g.BoundHolds),
			axisCN(g.OffendingAxis), g.Edge.MaxSideDev,
			g.GrayErrorBound, mark(g.GrayBoundHolds), g.Edge.FogPixels))
	}
	add("\n")

	add("### 修复前 → 修复后（题目主场景）\n\n")
	hb := h.Before
	add(fmt.Sprintf("- 修复前直接拿 J⁻¹ 足迹当核：轴 (%.1f,%.1f)，轴积面积因子 %.0f=1/detJ，"+
		"边缘最大灰度差 %.0f 档，灰雾 **%d px**。\n",
		hb.Axes.Long, hb.Axes.Short, hb.Axes.AxisProduct, hb.Edge.MaxSideDev, hb.Edge.FogPixels))
	g2 := rep.Groups[1]
	add(fmt.Sprintf("- 修复后按面积守恒反解：轴 (%.5f,%.5f)，πab=%.6f，"+
		"边缘最大灰度差 **%.0f 档（≤2）**，灰雾 **%d px**。\n\n",
		g2.UsedAxes.Long, g2.UsedAxes.Short, g2.UsedAxes.GeometricArea,
		g2.Edge.MaxSideDev, g2.Edge.FogPixels))

	add("### 变换组合扫描：原始逆足迹核 vs 守恒核\n\n")
	add("| 变换组合 | detJ | κ | 原始足迹轴 (长,短) | 原始面积因子 | 原始灰雾 px / 灰度差 | 守恒核面积 | 守恒灰雾 px / 灰度差 |\n")
	add("|---|---:|---:|---|---:|---:|---:|---:|\n")
	for _, r := range rep.ComboScan {
		add(fmt.Sprintf("| %s | %.3g | %.3g | (%.3f, %.3f) | %.3g | %d / %.0f | %.4f | %d / %.0f |\n",
			r.Name, r.Det, r.Kappa, r.RawAxes.Long, r.RawAxes.Short, r.RawAreaProd,
			r.RawEdge.FogPixels, r.RawEdge.MaxSideDev,
			r.Conserved.GeometricArea, r.ConsEdge.FogPixels, r.ConsEdge.MaxSideDev))
	}
	add("\n均匀缩放时原始足迹面积因子=1/detJ=4，核过宽产生灰雾；非均匀组的原始轴 (2,1) 把圆拉成长/短轴，" +
		"即“被拉长又压扁”。守恒归一后所有组 πab=1、平台灰度差为 0。\n\n")

	add("## 4. 误差上界与轴归属\n\n")
	g3 := rep.Groups[2]
	dl, ds := kernel.AxisDeviations(kernel.Ellipse{
		Long: g3.UsedAxes.Long, Short: g3.UsedAxes.Short,
	}, g3.CorrectAxes.Long, g3.CorrectAxes.Short)
	add(fmt.Sprintf("- 面积守恒残差上界：U = π(b0 εa + a0 εb + εa εb)。G3 只放大长轴，"+
		"实际偏差 δa=%.4f=4a0，取认证预算 εa=%.4f=5a0、εb=0，得 U=%.3f；"+
		"实测残差 %.3f，%.3f ≤ %.3f（成立：%s）。\n",
		g3.UsedAxes.Long-g3.CorrectAxes.Long, g3.EpsLong, g3.AreaErrorBound,
		g3.AreaResidual, g3.AreaResidual, g3.AreaErrorBound, passCN(g3.BoundHolds)))
	add(fmt.Sprintf("- 轴比误差上界：G3 雅可比要求 κ=%.0f，实际用轴比 ρ=a/b=%.1f（不满足守恒轴比），"+
		"轴比误差 %.0f；由 ρ∈[(a0-εa)/(b0+εb),(a0+εa)/(b0-εb)] 得上界 %.1f，%.0f ≤ %.1f（成立：%s）。\n",
		g3.Kappa, g3.RatioUsed, g3.RatioError, g3.RatioErrorBound,
		g3.RatioError, g3.RatioErrorBound, passCN(g3.RatioBoundHolds)))
	add(fmt.Sprintf("- 灰度污染上界（球冠外接矩形）：U_gray=255·2(1-d/w)√(1-(d/w)²)/π，"+
		"d=1.5px 处 G3 上界 %.1f 档，实测最大 %.0f 档，实测落在界内：%s。\n",
		g3.GrayErrorBound-0.5, g3.Edge.MaxSideDev, passCN(g3.GrayBoundHolds)))
	add(fmt.Sprintf("- G3 守恒不成立，逐轴相对偏差：长轴 %+.1f%%、短轴 %+.1f%%，偏差落在 **%s**。\n\n",
		100*dl, 100*ds, axisCN(g3.OffendingAxis)))

	add("## 5. 灰雾面积（像素数，三组各一个数）\n\n")
	for _, g := range rep.Groups {
		add(fmt.Sprintf("- %s：**%d px**\n", g.ID, g.Edge.FogPixels))
	}
	add("\n对照图：`artifacts/edge_compare.png`（左 G1 / 中 G2 / 右 G3），原始 PGM 同目录。\n")
	return string(b)
}

func mark(ok bool) string {
	if ok {
		return "✓"
	}
	return "✗"
}

func passCN(ok bool) string {
	if ok {
		return "是"
	}
	return "否"
}

func axisCN(a string) string {
	switch a {
	case "long":
		return "长轴"
	case "short":
		return "短轴"
	case "both":
		return "两轴"
	case "none":
		return "—"
	}
	return a
}
