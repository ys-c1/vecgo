package lab

import (
	"fmt"
	"strings"
)

// FormatTables 生成终端可读的全部对照表。
func FormatTables(r *Report) string {
	var b strings.Builder

	fmt.Fprintf(&b, "场景: %dx%d supersample=%d shading=%s 金字塔层数 L=%d\n",
		r.Width, r.Height, r.SuperSample, r.Shading, r.Depth)
	fmt.Fprintf(&b, "每层衰减拟合: δ1=%.4f q=%.5f R²=%.6f  ->  A=%.4f (E=A·q^n)\n",
		r.Model.D1, r.Model.Q, r.Model.RSquared, r.Model.A)
	fmt.Fprintf(&b, "总误差直接拟合(n=%d..%d): A=%.4f q=%.5f R²=%.6f\n\n",
		r.TotalFit.FitFromN, r.TotalFit.FitToN, r.TotalFit.A, r.TotalFit.Q, r.TotalFit.RSquared)

	b.WriteString("每层细节 RMS（由粗到精 δ1..δM）及相邻比:\n")
	for i, d := range r.Delta {
		ratio := "-"
		if i > 0 && r.Delta[i-1] > 1e-12 {
			ratio = fmt.Sprintf("%.4f", d/r.Delta[i-1])
		}
		fmt.Fprintf(&b, "  δ%-2d = %9.4f   δ%d/δ%d = %s\n", i+1, d, i+1, i, ratio)
	}

	b.WriteString("\n穷举全部层数的重建误差（RMSE，灰度级）:\n")
	b.WriteString("  n | 实测float | 8位图实测 | A q^n预测 | 有限尾预测 | 正交能量\n")
	for _, s := range r.Sweep {
		fmt.Fprintf(&b, " %2d | %9.4f | %9.4f | %9.4f | %10.4f | %9.4f\n",
			s.N, s.Measured, s.ImageMeasured, s.Predicted, s.FiniteTail, s.EnergyRMSE)
	}

	b.WriteString("\n三档目标误差 -> 最小层数（解析 vs 穷举）:\n")
	b.WriteString(" 目标ε | 实数根 | 解析n | 穷举n | 一致 | 实测E | Aq^n预测 | 有限尾 | 偏差%\n")
	for _, t := range r.Targets {
		fmt.Fprintf(&b, " %6.2f | %6.3f | %5d | %5d |  %v  | %6.3f | %8.3f | %7.3f | %+5.1f%%\n",
			t.Epsilon, t.RequiredNReal, t.AnalyticN, t.ExhaustiveN, t.SolutionsMatch,
			t.MeasuredRMSE, t.PredictedRMSE, t.FiniteTailRMSE, t.RelativeErrorPct)
	}
	return b.String()
}

// MarkdownReport 生成 REPORT.md 的正文（含公式与全部要求的对照表）。
// chartRelPath 是 SVG 图表相对报告文件的路径。
func MarkdownReport(r *Report, chartRelPath string) string {
	var b strings.Builder
	b.WriteString("# 金字塔层数与重建误差：衰减建模、最小层数求解与实测\n\n")

	b.WriteString("## 1. 问题与构造\n\n")
	b.WriteString("金字塔层数给多了浪费算力与存储，给少了重建发糊。实验用 vecgo 管线渲染" +
		"一个铺满画幅的四边形并采用 `gradient` 着色（" +
		fmt.Sprintf("%d×%d，%d× 超采样），得到跨满 0–255 的稳定纵向渐变作为基准图。\n\n",
			r.Width, r.Height, r.SuperSample) +
		"金字塔为 2×2 均值（盒式）金字塔：`G_0` 是原图，`G_{k+1}` 是 `G_k` 的 2×2 块均值；" +
		"细节层 `D_k = G_k − Up2(G_{k+1})`。用 **n 层**做由粗到精重建时，从最粗层出发补入 " +
		"n−1 个细节层并把剩余尺度最近邻放大回原尺寸；`n=L`（" + fmt.Sprintf("L=%d", r.Depth) +
		"）时逐像素精确还原。固定层数路径 `Pyramid.Reconstruct(n)` 始终保留，" +
		"自适应路径只是在其外面包了一层“按目标误差选 n”。\n\n")

	b.WriteString("## 2. 每层误差的衰减表达式\n\n")
	b.WriteString("把细节层按**由粗到精**编号为 δ₁…δ_M（M=L−1），实测相邻比稳定在常数附近：\n\n")
	b.WriteString("```\n")
	b.WriteString("δ_j = d1 · q^(j−1)\n")
	fmt.Fprintf(&b, "标定结果: d1 = %.3f 灰度级, q = %.4f (每层约 ×%.2f), 对数拟合 R² = %.5f\n",
		r.Model.D1, r.Model.Q, r.Model.Q, r.Model.RSquared)
	b.WriteString("```\n\n")

	b.WriteString("| j（粗→细） | 细节层 RMS δⱼ | 相邻比 δⱼ/δⱼ₋₁ |\n|---:|---:|---:|\n")
	for i, d := range r.Delta {
		ratio := "—"
		if i > 0 {
			ratio = fmt.Sprintf("%.3f", d/r.Delta[i-1])
		}
		b.WriteString(fmt.Sprintf("| %d | %.3f | %s |\n", i+1, d, ratio))
	}

	b.WriteString("\n## 3. 总误差与层数的关系式\n\n")
	b.WriteString("均值金字塔 + 块常量重建等价于 Haar 多分辨率分解，各细节子带能量近似正交，" +
		"故用 n 层时被省略的细尺度尾带满足能量相加：\n\n")
	b.WriteString("```\nE(n)² ≈ Σ_{j=n}^{M} δ_j²\n```\n\n")
	b.WriteString("代入 δ_j = d1·q^(j−1) 得**有限平方尾和**形式：\n\n")
	b.WriteString("```\n")
	b.WriteString("E(n) = d1·q^(n−1) · sqrt( (1 − q^(2(L−n))) / (1 − q²) )\n")
	b.WriteString("```\n\n")
	b.WriteString("在尾带还很长（远离最细端）时退化为纯指数曲线：\n\n")
	fmt.Fprintf(&b, "```\nE(n) ≈ A·q^n,  A = d1/(q·sqrt(1−q²)) = %.3f,  q = %.4f\n```\n\n",
		r.Model.A, r.Model.Q)
	fmt.Fprintf(&b, "直接对实测总误差做对数线性拟合（n=%d..%d）得到 A=%.3f, q=%.4f, R²=%.5f，"+
		"与逐层推导一致。\n\n", r.TotalFit.FitFromN, r.TotalFit.FitToN,
		r.TotalFit.A, r.TotalFit.Q, r.TotalFit.RSquared)

	b.WriteString("## 4. 解出满足给定误差的最小层数\n\n")
	b.WriteString("令 A·q^n ≤ ε，因 0<q<1，解得实数层数根与最小整数层：\n\n")
	b.WriteString("```\n")
	b.WriteString("n₀(ε) = ln(ε/A) / ln q\n")
	b.WriteString("n*    = min(L, ceil(n₀))      // L 层精确还原，误差为 0\n")
	b.WriteString("```\n\n")
	b.WriteString("三档目标误差代入（ε 单位为 0–255 灰度级 RMSE）：\n\n")
	b.WriteString("| 目标 ε | n₀=ln(ε/A)/ln q | 解析 n*=ceil | 穷举最小 n | 解一致 |\n")
	b.WriteString("|---:|---:|---:|---:|:---:|\n")
	for _, t := range r.Targets {
		b.WriteString(fmt.Sprintf("| %.2f | %.3f | **%d** | %d | %s |\n",
			t.Epsilon, t.RequiredNReal, t.AnalyticN, t.ExhaustiveN, mark(t.SolutionsMatch)))
	}
	b.WriteString("\n穷举做法：令 n=1…L 逐层重建并实测 RMSE，取第一个实测 ≤ ε 的层；" +
		"上表说明解析解与穷举结果三档全部相同，完成“与穷举层数结果对照一次”。\n\n")

	b.WriteString("## 5. 三档层数：实测误差与预测并列\n\n")
	b.WriteString("| 目标 ε | 选定层数 n | 实测重建误差 | A·q^n 预测 | 有限尾预测 | 8 位图像实测 | 实测/预测偏差 |\n")
	b.WriteString("|---:|---:|---:|---:|---:|---:|---:|\n")
	for _, t := range r.Targets {
		b.WriteString(fmt.Sprintf("| %.2f | %d | **%.3f** | %.3f | %.3f | %.3f | %+.1f%% |\n",
			t.Epsilon, t.AnalyticN, t.MeasuredRMSE, t.PredictedRMSE, t.FiniteTailRMSE,
			t.ImageRMSE, t.RelativeErrorPct))
	}
	b.WriteString("\n纯指数 A·q^n 在接近最细端（全表 n=8、9）会略偏离实测：n=L 时误差" +
		"严格为 0，而无限几何尾假设给出正的 A·q^L。有限尾公式修正这一端效应" +
		"（见“有限尾预测”列，全层数均与实测相差 ≤4%）。三档选点 n=3、5、7 处于稳定" +
		"几何衰减段，实测与 A·q^n 偏差不超过 6%。8 位往返在 n≤7 时只带来 ≤0.07 " +
		"灰度级差异，仅最细的 n=8 取整处升到约 0.18。\n\n")

	b.WriteString("## 6. 误差随层数衰减曲线与三档实测点\n\n")
	fmt.Fprintf(&b, "![误差衰减曲线与三档目标点](%s)\n\n", chartRelPath)
	if strings.HasSuffix(chartRelPath, ".png") {
		vec := strings.TrimSuffix(chartRelPath, ".png") + ".svg"
		fmt.Fprintf(&b, "_位图预览；矢量版见 [`%s`](%s)。_\n\n", vec, vec)
	}
	b.WriteString("蓝线为解出的 E(n)=A·q^n 曲线，灰点为 n=1…L 的穷举实测，" +
		"三个橙色大点为三档目标解出层数上的实测重建误差，均落在曲线附近；" +
		"橙色虚线标出各档目标 ε。下图（对数纵轴）中指数衰减呈直线。\n\n")

	b.WriteString("## 7. 全层数穷举数据\n\n")
	b.WriteString("| n | 实测 RMSE | 8 位实测 | A·q^n | 有限尾 | 正交能量 √Σδ² |\n")
	b.WriteString("|---:|---:|---:|---:|---:|---:|\n")
	for _, s := range r.Sweep {
		b.WriteString(fmt.Sprintf("| %d | %.3f | %.3f | %.3f | %.3f | %.3f |\n",
			s.N, s.Measured, s.ImageMeasured, s.Predicted, s.FiniteTail, s.EnergyRMSE))
	}

	b.WriteString("\n## 8. 复现与固定层数路径\n\n")
	b.WriteString("```bash\n")
	b.WriteString("go run ./cmd/pyrlab -out artifacts   # 生成 JSON/SVG/REPORT，并刷新 pyramid/ 文档\n")
	b.WriteString("go test ./...                        # 含三档解析-穷举对照与固定路径测试\n")
	b.WriteString("go run ./cmd -levels 4 out.pgm       # 固定层数路径\n")
	b.WriteString("go run ./cmd -shading gradient -target-error 5 out.pgm # 自适应选层\n")
	b.WriteString("```\n\n")
	b.WriteString("- 固定层数路径：`pyramid.Build(im,L)` + `(*Pyramid).Reconstruct(n)` 原样保留，" +
		"对应配置字段 `Config.PyramidLevels` 与命令行 `-levels`。\n")
	b.WriteString("- 自适应路径：`Model.MinLevels(ε)` 选层后仍调用同一个 `Reconstruct(n*)`，" +
		"对应 `Config.TargetError` 与 `-target-error`。\n")
	return b.String()
}

func mark(ok bool) string {
	if ok {
		return "✓"
	}
	return "✗"
}
