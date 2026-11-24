package main

import (
	"fmt"
	"os"

	"vecgo/raster"
)

// cmd/variance 打印“三角度 × 两档样本数”的网格 vs 低差异方差对照表。
// 复现方式：go run ./cmd/variance（结果由固定种子决定，逐次运行一致）。
func main() {
	rows := raster.CoverageVarianceTable()
	fmt.Printf("覆盖率估计方差对照（每格 %d 次重复，主种子 seed=%d，边相位 h=%.3f）\n",
		raster.VarianceTrials, raster.VarianceSeed, raster.EdgePhase)
	fmt.Println("分布规则: p_i=(i/N, phi_2(i)); N=16=2^4 -> (i/16,phi2(i)); N=64=2^6 -> (i/64,phi2(i));")
	fmt.Println("重复时叠加 Cranley-Patterson 平移 p'_i=frac(p_i+(ux,uy))，平移由 SplitMix64(seed) 产生。")
	fmt.Println()
	fmt.Printf("%-12s %-7s %-14s %-14s %-9s %-10s %-10s\n",
		"角度", "N", "网格方差", "低差异方差", "LD/网格", "网格均值", "LD均值")
	for _, r := range rows {
		label := fmt.Sprintf("%.3f°", r.AngleDeg)
		if r.AngleDeg == 0 {
			label = "0°(水平)"
		} else if r.AngleDeg == 45 {
			label = "45°(对角)"
		}
		fmt.Printf("%-12s %-7d %-14.6e %-14.6e %-9.4f %-10.5f %-10.5f\n",
			label, r.Samples, r.GridVariance, r.LDVariance, r.Ratio(), r.GridMean, r.LDMean)
	}
	fmt.Println()
	for _, r := range rows {
		if !(r.LDVariance < r.GridVariance) {
			fmt.Fprintf(os.Stderr, "未达预期: 角度 %.3f N=%d LD 方差 %.6e >= 网格 %.6e\n",
				r.AngleDeg, r.Samples, r.LDVariance, r.GridVariance)
			os.Exit(1)
		}
	}
	fmt.Println("全部 6 个对照格均满足: 低差异方差 < 均匀网格方差。")
}
