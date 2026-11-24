// Command lightreport 生成“感知亮度守恒权重”三组强度对照、非线性求解的
// 迭代次数/残差，以及覆盖率—位置采样点；把 results.json 与
// table.generated.md 写到 -out 目录（默认 artifacts/light）。
//
//	go run ./cmd/lightreport -out artifacts/light
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"vecgo/light"
)

// Group 是一组两灯强度的命名验收组。
type Group struct {
	Name string
	I1   float64
	I2   float64
}

// Report 是落盘 results.json 的顶层结构。
type Report struct {
	Definition  string                `json:"definition"`
	Equation    string                `json:"equation"`
	Criterion   string                `json:"criterion"`
	Groups      []light.Result        `json:"groups"`
	Coverage    []light.CoveragePoint `json:"coverage_points"`
	CoverageSS  []CoverageCrossCheck  `json:"coverage_supersample"`
	GammaChecks []GammaCheck          `json:"gamma_crosscheck"`
}

// CoverageCrossCheck 记录闭式覆盖率与 n×n 中点超采样的对照。
type CoverageCrossCheck struct {
	X        float64 `json:"x"`
	N        int     `json:"n"`
	Hits     int     `json:"hits"`
	Total    int     `json:"total"`
	Sampled  float64 `json:"sampled"`
	Analytic float64 `json:"analytic"`
	AbsDiff  float64 `json:"abs_diff"`
}

// GammaCheck 是幂律感知定义 P=I^(1/2.2) 下同一守恒方程的闭式交叉核对。
type GammaCheck struct {
	Name   string  `json:"name"`
	Weight float64 `json:"gamma_weight_closed_form"`
}

func main() {
	out := flag.String("out", "artifacts/light", "报告输出目录")
	flag.Parse()

	groups := []Group{
		{Name: "G1 等高暗调", I1: 0.10, I2: 0.10},
		{Name: "G2 强主弱辅(3:1)", I1: 0.18, I2: 0.06},
		{Name: "G3 深暗跨线性趾", I1: 0.012, I2: 0.004},
	}
	opts := light.Options{Trace: true} // 全零容差字段 → WithDefaults；保留逐轮轨迹

	results := make([]light.Result, 0, len(groups))
	var md strings.Builder

	md.WriteString("# 感知守恒权重：机器生成对照表\n\n")
	md.WriteString("感知亮度 P(Y)=L*(Y)/100（CIE 1976 L*，Y 为 Rec.709 线性亮度；灰度下 Y=I）。\n")
	md.WriteString("收敛判据：|F|≤1e-13 且 |Δw|≤1e-14(1+|w|)，Newton+二分保护，上限 50 次。\n\n")

	md.WriteString("## 三组强度：线性归一 vs 感知归一（两种亮度值并列）\n\n")
	md.WriteString("| 组 | I1 | I2 | 线性归一权重 | **线性归一亮度 L\\*** | 感知守恒权重 w | **感知归一亮度 L\\*** | 感知 P 值 |\n")
	md.WriteString("|---|---:|---:|---:|---:|---:|---:|---:|\n")

	gammaChecks := make([]GammaCheck, 0, len(groups))
	for _, g := range groups {
		r, err := light.ConservationWeight(g.I1, g.I2, opts)
		if err != nil {
			fmt.Fprintln(os.Stderr, "求解失败:", err)
			os.Exit(1)
		}
		results = append(results, r)
		fmt.Fprintf(&md, "| %s | %.3f | %.3f | %.3f | **%.4f** | %.9f | **%.4f** | %.9f |\n",
			g.Name, g.I1, g.I2, r.LinearWeight, r.LstarLinear, r.Weight, r.Lstar, r.Brightness)

		// gamma 2.2 闭式交叉核对：w = (I1^(1/γ)+I2^(1/γ))^γ / (I1+I2)
		const gamma = 2.2
		tg := light.GammaBrightness(g.I1, gamma) + light.GammaBrightness(g.I2, gamma)
		wg := light.InvGammaBrightness(tg, gamma) / (g.I1 + g.I2)
		gammaChecks = append(gammaChecks, GammaCheck{Name: g.Name, Weight: wg})
	}

	// 开场现象：权重 1 的原始线性相加 S=I1+I2（等高灯恰为单灯两倍能量），
	// 但感知亮度 P(S) 因 P 是凹函数而低于 2P(I)。
	md.WriteString("\n### 对照：不归一的原始线性相加（w=1，S=I1+I2）\n\n")
	md.WriteString("| 组 | 单灯 L*(I1) | 单灯 L*(I2) | 线性和 S（等高时=2I） | 原始相加 L*(S) | 主观“两份”目标 L*(I1)+L*(I2) |\n")
	md.WriteString("|---|---:|---:|---:|---:|---:|\n")
	for i, g := range groups {
		r := results[i]
		md.WriteString(fmt.Sprintf("| %s | %.4f | %.4f | %.3f | %.4f | %.4f |\n",
			g.Name, light.Lightness(g.I1), light.Lightness(g.I2),
			r.SumLinear, light.Lightness(r.SumLinear), r.TargetLstar))
	}

	md.WriteString("\n## 非线性求解：迭代次数与残差\n\n")
	md.WriteString("| 组 | 迭代次数 | 末残差 \\|F(w)\\| | 收敛 | 求解路径 | 闭式权重(核对) | 与闭式相对差 |\n")
	md.WriteString("|---|---:|---:|:---:|---|---:|---:|\n")
	for i, r := range results {
		rel := abs(r.Weight-r.ClosedFormWeight) / r.ClosedFormWeight
		md.WriteString(fmt.Sprintf("| %s | **%d** | %.3e | %v | %s | %.9f | %.2e |\n",
			groups[i].Name, r.Iterations, r.Residual, r.Converged, r.Solver,
			r.ClosedFormWeight, rel))
	}

	// G1 的逐轮 Newton 轨迹，展示残差如何降到机器地板。
	md.WriteString("\n### G1 逐轮迭代轨迹（初值 w0=1）\n\n")
	md.WriteString("| k | w_k | \\|F(w_k)\\|（判定残差） | 步长 \\|Δw\\| | 二分保护 |\n")
	md.WriteString("|---:|---:|---:|---:|:---:|\n")
	for _, it := range results[0].Trace {
		md.WriteString(fmt.Sprintf("| %d | %.12f | %.3e | %.3e | %v |\n",
			it.K, it.W, it.AbsF, it.Step, it.Guarded))
	}
	md.WriteString(fmt.Sprintf("\n收敛后最终点残差 = %.3e（表中最后一行是判定前残差）。\n", results[0].Residual))

	md.WriteString("\n## 覆盖率—位置函数 C(x)=clamp(x,0,1)（单位像素，直边 x=c 保留左侧）\n\n")
	md.WriteString("| 位置 x | 覆盖率 C(x) | 本仓截断灰度 uint8(255C) |\n")
	md.WriteString("|---:|---:|---:|\n")
	cps := light.SampleUnitEdge(0.25, 0.5, 0.75)
	for _, p := range cps {
		md.WriteString(fmt.Sprintf("| %.2f | %.2f | %d |\n", p.X, p.Coverage, p.GrayTrunc))
	}
	md.WriteString("\n整数位置核对：C(0)=0、C(1)=1（整数位置只取 0/1）。\n\n")

	// 超采样对照（16×16 中点）。
	xs := []float64{0.25, 0.5, 0.75}
	cross := make([]CoverageCrossCheck, 0, 3)
	md.WriteString("### 与 16×16 中点超采样对照\n\n")
	md.WriteString("| x | 闭式 C | 16×SS 命中/总数 | SS 覆盖率 | 绝对差 |\n")
	md.WriteString("|---:|---:|---|---:|---:|\n")
	for _, x := range xs {
		hits, total, frac := light.SuperSampleUnitEdge(x, 16)
		a := light.UnitEdgeCoverage(x)
		d := abs(frac - a)
		cross = append(cross, CoverageCrossCheck{X: x, N: 16, Hits: hits, Total: total, Sampled: frac, Analytic: a, AbsDiff: d})
		md.WriteString(fmt.Sprintf("| %.2f | %.4f | %d/%d | %.4f | %.2e |\n", x, a, hits, total, frac, d))
	}

	// 权重 × 覆盖率 × 位置 的有效亮度（用 G1 的解）。
	md.WriteString(fmt.Sprintf("\n### 权重落到覆盖率/位置上的有效亮度（G1，w=%.6f）\n\n", results[0].Weight))
	md.WriteString("| x | C(x) | 有效线性 E=w·C·S | 有效感知亮度 L*(E) |\n")
	md.WriteString("|---:|---:|---:|---:|\n")
	for _, p := range cps {
		lin, perc := light.EffectiveWithCoverage(results[0], p.Coverage)
		md.WriteString(fmt.Sprintf("| %.2f | %.2f | %.6f | %.4f |\n", p.X, p.Coverage, lin, 100*perc))
	}

	fmt.Print(md.String())

	rep := Report{
		Definition:  "P(Y)=L*(Y)/100, L*=116 f(Y)-16; f=cbrt(Y) if Y>(6/29)^3 else Y/(3*(6/29)^2)+4/29; Y=0.2126R_lin+0.7152G_lin+0.0722B_lin, grayscale Y=I.",
		Equation:    "P(w*(I1+I2)) = P(I1)+P(I2); not w1+w2=1 / linear-energy equality.",
		Criterion:   "|F(w)|<=1e-13 AND |w_next-w|<=1e-14*(1+|w|); max 50; Newton with bisection guard.",
		Groups:      results,
		Coverage:    cps,
		CoverageSS:  cross,
		GammaChecks: gammaChecks,
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "建目录失败:", err)
		os.Exit(1)
	}
	writeJSON(filepath.Join(*out, "results.json"), rep)
	writeFile(filepath.Join(*out, "table.generated.md"), []byte(md.String()))

	var csv strings.Builder
	csv.WriteString("x,coverage,gray_trunc\n")
	for _, p := range cps {
		csv.WriteString(fmt.Sprintf("%.2f,%.2f,%d\n", p.X, p.Coverage, p.GrayTrunc))
	}
	writeFile(filepath.Join(*out, "coverage_points.csv"), []byte(csv.String()))

	fmt.Fprintf(os.Stderr, "已写出 %s/{results.json,table.generated.md,coverage_points.csv}\n", *out)
}

func writeJSON(path string, v any) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, "JSON 编码失败:", err)
		os.Exit(1)
	}
	writeFile(path, append(b, '\n'))
}

func writeFile(path string, b []byte) {
	if err := os.WriteFile(path, b, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "写文件失败:", path, err)
		os.Exit(1)
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
