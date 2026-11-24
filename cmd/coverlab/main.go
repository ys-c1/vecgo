// Command coverlab 按二项分布反解出的样本数，在两档画幅上实测覆盖率
// 估计的方差、χ²/正态预测区间覆盖率与空间样本分布均匀性。
//
// 用法：
//
//	go run ./cmd/coverlab                 # 用报告默认参数实测
//	go run ./cmd/coverlab -reps 200       # 快速冒烟
//
// 结果同时打印到 stdout（Markdown 片段）并写入 -json 指定的 JSON 文件。
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"vecgo/coverage"
	"vecgo/geom"
)

type outFile struct {
	TargetWorstVariance float64               `json:"targetWorstVariance"`
	DerivedN            int                   `json:"derivedN"`
	UsedN               int                   `json:"usedN"`
	Confidence          float64               `json:"confidence"`
	Formats             []coverage.FormatSpec `json:"formats"`
	Uniform             []*coverage.Result    `json:"uniform"`
	ClusterControl      []*coverage.Result    `json:"clusterControl"`
}

func main() {
	vTarget := flag.Float64("vtarget", 1e-4, "方差上界目标 V*，反解 n>=ceil(1/(4V*))")
	nFlag := flag.Int("n", 0, "直接指定每估计样本数；0 表示按 -vtarget 反解")
	reps := flag.Int("reps", 20000, "每档画幅的独立重复估计数 K")
	batch := flag.Int("batch", 50, "计算批次方差的每组估计数 r")
	seed := flag.Int64("seed", 20261125, "主实验固定随机种子")
	jsonPath := flag.String("json", "artifacts/coverage-measure.json", "JSON 结果输出路径；置空则不写")
	controlReps := flag.Int("control-reps", 4000, "聚类阳性对照的重复估计数")
	flag.Parse()

	derivedN, err := coverage.RequiredSamples(*vTarget)
	if err != nil {
		fmt.Fprintln(os.Stderr, "反解失败:", err)
		os.Exit(2)
	}
	n := derivedN
	if *nFlag > 0 {
		n = *nFlag
	}

	// 直角三角形：在其包围盒内恰好占一半面积，仿射映射到画幅后仍 p=1/2，
	// 因而二项预测方差 p(1-p)/n 与最坏上界 1/(4n) 完全相等。
	tri := [][]geom.Point{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}}}
	formats := []coverage.FormatSpec{
		{Name: "square-64x64", W: 64, H: 64},
		{Name: "widescreen-128x72", W: 128, H: 72},
	}

	doc := outFile{
		TargetWorstVariance: *vTarget,
		DerivedN:            derivedN,
		UsedN:               n,
		Confidence:          0.95,
		Formats:             formats,
	}

	fmt.Printf("# coverlab 实测结果（n=%d, K=%d, r=%d, seed=%d）\n\n", n, *reps, *batch, *seed)
	fmt.Printf("反解：V*=%.0e ⇒ n*=ceil(1/(4V*))=%d；实际采用 n=%d\n\n", *vTarget, derivedN, n)

	for i, f := range formats {
		r := coverage.Run(coverage.ExperimentConfig{
			Spec: f, Polys: tri, N: n, Reps: *reps, BatchSize: *batch,
			Seed: *seed + int64(i)*1000, TrueP: 0.5, Alpha: 0.05,
			BinsX: 10, BinsY: 10, TopK: 3,
		})
		doc.Uniform = append(doc.Uniform, r)
		printResult(r)
	}

	fmt.Println("\n## 阳性对照：故意聚类采样（验证“超方差→分布不均→定位”链路）")
	for i, f := range formats {
		r := coverage.Run(coverage.ExperimentConfig{
			Spec: f, Polys: tri, N: n, Reps: *controlReps, BatchSize: *batch,
			Seed: *seed + 777 + int64(i)*1000, TrueP: 0.5, Alpha: 0.05,
			BinsX: 10, BinsY: 10, TopK: 3,
			Trial: &coverage.Cluster{CentralFrac: 0.5, RadiusFrac: 0.12},
		})
		doc.ClusterControl = append(doc.ClusterControl, r)
		printResult(r)
	}

	if *jsonPath != "" {
		if err := os.MkdirAll(filepath.Dir(*jsonPath), 0o755); err != nil {
			fmt.Fprintln(os.Stderr, "建目录失败:", err)
			os.Exit(2)
		}
		b, err := json.MarshalIndent(doc, "", "  ")
		if err != nil {
			fmt.Fprintln(os.Stderr, "JSON 编码失败:", err)
			os.Exit(2)
		}
		if err := os.WriteFile(*jsonPath, b, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "写 JSON 失败:", err)
			os.Exit(2)
		}
		fmt.Printf("\nJSON 已写入 %s\n", *jsonPath)
	}
}

func printResult(r *coverage.Result) {
	fmt.Printf("\n## [%s] %dx%d sampler=%s\n", r.Name, r.W, r.H, r.Sampler)
	fmt.Printf("- p 实测均值=%.6f（偏析 %+.2e），理论 p=%.2f\n", r.PSampleMean, r.PBias, r.TrueP)
	fmt.Printf("- 预测方差 p(1-p)/n = %.6e；上界 1/(4n) = %.6e\n", r.VariancePred, r.VarianceUpper)
	fmt.Printf("- 实测方差 S² = %.6e（SD=%.6e），S²/预测 = %.4f，等效样本数 n_eff=%.1f\n",
		r.VarianceMeasured, r.StdMeasured, r.RatioToPred, r.EffectiveN)
	fmt.Printf("- 总方差 95%% χ² 预测区间 [%.6e, %.6e]，实测落入=%v，显著超出上界=%v\n",
		r.VarPILow, r.VarPIHigh, r.InVarPI, r.ExcessSignificant)
	fmt.Printf("- 单估计 95%% 正态区间 [%.4f, %.4f]，K=%d 个实测值覆盖率=%.4f（%d/%d）\n",
		r.EstPILow, r.EstPIHigh, r.Reps, r.EstCoverage, r.EstInside, r.Reps)
	fmt.Printf("- 批次方差区间（r=%d, df=%d）[%.6e, %.6e]，%d 个批次覆盖率=%.4f（%d/%d）\n",
		r.BatchSize, r.BatchSize-1, r.BatchPILow, r.BatchPIHigh,
		r.Batches, r.BatchCoverage, r.BatchesInside, r.Batches)
	if r.Bins != nil {
		b := r.Bins
		fmt.Printf("- 空间 χ²=%.1f（df=%d, p=%.4g），最大 |z|=%.2f\n", b.Chi2, b.DF, b.PValue, b.MaxAbsZ)
		for _, h := range b.Hot {
			fmt.Printf("  - 最热 %s\n", h.String())
		}
		for _, c := range b.Cold {
			fmt.Printf("  - 最冷 %s\n", c.String())
		}
	}
}
