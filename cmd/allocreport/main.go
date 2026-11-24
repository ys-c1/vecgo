// allocreport 用三档斜边斜率跑拉格朗日最优分配与固定均分对照，
// 输出逐行分配表（行=行号，列=采样数与误差贡献）、总误差并列对照，
// 并把报告写入 artifacts/。
package main

import (
	"flag"
	"fmt"
	"math"
	"os"
	"strings"

	"vecgo/alloc"
)

type tier struct {
	name  string
	s     float64 // 斜率 s=|dx/dy|，像素/行
	angle float64 // 斜边与竖直方向夹角（度），仅用于展示
}

func main() {
	budget := flag.Int("N", 80, "总采样预算")
	rowsN := flag.Int("m", 8, "扫描线行数")
	phi := flag.Float64("phi", 0.2, "斜边亚像素相位")
	trials := flag.Int("trials", 50000, "蒙特卡洛实验次数")
	out := flag.String("out", "artifacts/alloc_report.md", "报告输出路径")
	flag.Parse()

	tiers := []tier{
		{name: "陡斜边", s: 0.25, angle: math.Atan(0.25) * 180 / math.Pi},
		{name: "中等斜边", s: 0.5, angle: math.Atan(0.5) * 180 / math.Pi},
		{name: "45° 斜边", s: 1.0, angle: 45},
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "# 细长三角形逐行采样预算分配报告\n\n")
	fmt.Fprintf(&sb, "参数: 行数 m=%d, 总预算 N=%d, 亚像素相位 φ=%g, 蒙特卡洛 %d 次\n\n",
		*rowsN, *budget, *phi, *trials)
	fmt.Fprintf(&sb, "## 闭式解（拉格朗日乘子）\n\n")
	fmt.Fprintf(&sb, "模型: 行 i 覆盖率 p_i=frac(φ+s·i)，单次采样方差 v_i=p_i(1-p_i)，总方差 V=Σ v_i/n_i，约束 Σ n_i=N。\n\n")
	fmt.Fprintf(&sb, "- 最优分配: **n_i* = N·√v_i / Σⱼ√v_j**（v_i=0 的精确行不采样）\n")
	fmt.Fprintf(&sb, "- 最优总方差: **V* = (Σ √v_i)² / N**\n")
	fmt.Fprintf(&sb, "- 固定均分: n_i=N/m，总方差 V_u = (m/N)·Σ v_i\n")
	fmt.Fprintf(&sb, "- 比值 V*/V_u = (Σ√v_i)²/(m·Σv_i) ≤ 1（Cauchy–Schwarz），等号当各行 v_i 相同\n\n")

	// 汇总表数据
	type sumRow struct {
		t                                                 tier
		vStar, vOpt, vUni, mseOpt, mseUni, ratio, improve float64
	}
	sums := make([]sumRow, 0, len(tiers))

	console := []string{}
	for _, t := range tiers {
		rows, err := alloc.Coverage(t.s, *phi, *rowsN)
		if err != nil {
			fmt.Fprintln(os.Stderr, "失败:", err)
			os.Exit(1)
		}
		opt, err := alloc.Allocate(rows, *budget, alloc.Optimal)
		if err != nil {
			fmt.Fprintln(os.Stderr, "最优分配失败:", err)
			os.Exit(1)
		}
		fix, err := alloc.Allocate(rows, *budget, alloc.Fixed)
		if err != nil {
			fmt.Fprintln(os.Stderr, "固定均分失败:", err)
			os.Exit(1)
		}
		vStar := alloc.OptimalVariance(rows, float64(*budget))
		vOpt := alloc.TotalVariance(opt)
		vUni := alloc.TotalVariance(fix)
		mseOpt := alloc.Simulate(opt, *trials, 20261125)
		mseUni := alloc.Simulate(fix, *trials, 20261125)
		ratio := vOpt / vUni
		sums = append(sums, sumRow{
			t: t, vStar: vStar, vOpt: vOpt, vUni: vUni,
			mseOpt: mseOpt, mseUni: mseUni, ratio: ratio,
			improve: (1 - ratio) * 100,
		})

		title := fmt.Sprintf("## %s（s=%.2f 像素/行，与竖直夹角 %.1f°）\n", t.name, t.s, t.angle)
		sb.WriteString(title + "\n")
		sb.WriteString("| 行号 | 覆盖率 p_i | 单次方差 v_i | 采样数 n_i(最优) | 误差贡献(最优) | 采样数 n_i(均分) | 误差贡献(均分) |\n")
		sb.WriteString("|---:|---:|---:|---:|---:|---:|---:|\n")
		for i := 0; i < len(rows); i++ {
			fmt.Fprintf(&sb, "| %d | %.4f | %.5f | %d | %.5f | %d | %.5f |\n",
				rows[i].Index, rows[i].P, rows[i].V,
				opt[i].N, opt[i].Err, fix[i].N, fix[i].Err)
		}
		fmt.Fprintf(&sb, "| **合计** |  |  | **%d** | **%.5f** | **%d** | **%.5f** |\n\n",
			alloc.TotalSamples(opt), vOpt, alloc.TotalSamples(fix), vUni)

		// 控制台简表
		line := []string{fmt.Sprintf("-- %s s=%.2f --", t.name, t.s)}
		line = append(line, fmt.Sprintf("%-4s %6s %8s %6s %9s %6s %9s",
			"行", "p", "v", "n_opt", "err_opt", "n_eq", "err_eq"))
		for i := 0; i < len(rows); i++ {
			line = append(line, fmt.Sprintf("%-4d %6.3f %8.5f %6d %9.5f %6d %9.5f",
				rows[i].Index, rows[i].P, rows[i].V,
				opt[i].N, opt[i].Err, fix[i].N, fix[i].Err))
		}
		line = append(line,
			fmt.Sprintf("合计样本 opt=%d eq=%d", alloc.TotalSamples(opt), alloc.TotalSamples(fix)),
			fmt.Sprintf("总方差 opt=%.5f (闭式V*=%.5f) 均分=%.5f 比值=%.4f 降方差=%.2f%%",
				vOpt, vStar, vUni, ratio, (1-ratio)*100),
			fmt.Sprintf("MC均方误差 opt=%.5f 均分=%.5f\n", mseOpt, mseUni))
		console = append(console, strings.Join(line, "\n"))
	}

	sb.WriteString("## 三档斜率汇总（误差最优与均分并列）\n\n")
	sb.WriteString("| 斜率档位 | s (像素/行) | 夹角 | 闭式 V* | 总误差(最优,整数) | 总误差(均分) | 比值 最优/均分 | 降方差 | MC-MSE 最优 | MC-MSE 均分 |\n")
	sb.WriteString("|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|\n")
	for _, r := range sums {
		fmt.Fprintf(&sb, "| %s | %.2f | %.1f° | %.5f | %.5f | %.5f | %.4f | %.2f%% | %.5f | %.5f |\n",
			r.t.name, r.t.s, r.t.angle, r.vStar, r.vOpt, r.vUni,
			r.ratio, r.improve, r.mseOpt, r.mseUni)
	}
	sb.WriteString("\n说明: 固定均分路径保留在 alloc.Allocate(..., alloc.Fixed)，可随时与 alloc.Optimal 切换；两条路径预算守恒均为 N。\n")

	fmt.Println(strings.Join(console, "\n"))
	fmt.Println("== 汇总 ==")
	fmt.Printf("%-8s %8s %10s %10s %10s %8s %10s %10s\n",
		"档位", "s", "V*", "V_opt", "V_eq", "比值", "MSE_opt", "MSE_eq")
	for _, r := range sums {
		fmt.Printf("%-8s %8.2f %10.5f %10.5f %10.5f %8.4f %10.5f %10.5f\n",
			r.t.name, r.t.s, r.vStar, r.vOpt, r.vUni, r.ratio, r.mseOpt, r.mseUni)
	}

	if err := os.WriteFile(*out, []byte(sb.String()), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "写报告失败:", err)
		os.Exit(2)
	}
	fmt.Println("\n报告已写入:", *out)
}
