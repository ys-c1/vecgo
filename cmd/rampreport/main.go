// Command rampreport 复现客户渐变图的暗部过冲，输出约束最优切线修复前后的
// 两张图、过冲像素计数、亮部偏离量，以及三种最优解位置的数值例子。
//
//	go run ./cmd/rampreport [输出目录]
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"vecgo/ramp"
	"vecgo/raster"
)

// 客户渐变控制点（归一化行位置, 强度）。严格单调下降：亮部在左/上，暗部在
// 右/下；x=0.58 之后是一段很平的暗尾，而它前面是陡坡——朴素平均切线在这种
// 陡/缓切换处把切线给大，三次段冲出端点值域，形成“往回勾”。
var customerStops = []ramp.Stop{
	{X: 0.00, Y: 0.970},
	{X: 0.15, Y: 0.850},
	{X: 0.30, Y: 0.625},
	{X: 0.45, Y: 0.385},
	{X: 0.58, Y: 0.177},
	{X: 0.73, Y: 0.161},
	{X: 0.88, Y: 0.143},
	{X: 1.00, Y: 0.133},
}

// brightIdx 是“亮部”控制点：强度 ≥ 0.385 的前四个点。
var brightIdx = []int{0, 1, 2, 3}

type imageReport struct {
	Size             int     `json:"size"`
	OvershootPixels  int     `json:"overshoot_pixels"`
	OvershootRows    int     `json:"overshoot_rows"`
	MaxExcessLevels  int     `json:"max_excess_gray_levels"`
	DenseBadSamples  int     `json:"dense_bad_samples"`
	DenseMaxExcess   float64 `json:"dense_max_excess_intensity"`
	BrightPixelDev   int     `json:"bright_pixel_deviation_levels"`
	BrightDevDetails []int   `json:"bright_pixel_deviation_per_point"`
}

type knotReport struct {
	K     int     `json:"k"`
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	Case  string  `json:"case"`
	Lo    float64 `json:"lo"`
	Hi    float64 `json:"hi"`
	Tau   float64 `json:"tau"`
	That  float64 `json:"t_opt"`
	Naive float64 `json:"t_naive"`
}

type caseNumbers struct {
	Name        string             `json:"name"`
	Description string             `json:"description"`
	Input       map[string]float64 `json:"input"`
	Solution    ramp.OneSolution   `json:"solution"`
	CheckJ      map[string]float64 `json:"objective_at_candidates"`
}

type report struct {
	Stops           []ramp.Stop   `json:"stops"`
	SecantSlopes    []float64     `json:"secant_slopes"`
	NaiveTangents   []float64     `json:"naive_tangents"`
	OptimalTangents []float64     `json:"optimal_tangents"`
	Sweeps          int           `json:"sweeps"`
	FinalMaxChange  float64       `json:"final_max_change"`
	FCFeasible      bool          `json:"fc_feasible"`
	BrightKnotDev   float64       `json:"bright_knot_deviation_intensity"`
	BrightChordDev  float64       `json:"bright_chord_shape_deviation_intensity"`
	Before          imageReport   `json:"before"`
	After           imageReport   `json:"after"`
	Knots           []knotReport  `json:"knot_solutions"`
	ThreeCases      []caseNumbers `json:"three_cases"`
}

func render(size int, c *ramp.Curve, m []float64, path string) (*raster.Image, error) {
	im, err := raster.New(size, size)
	if err != nil {
		return nil, err
	}
	old := append([]float64{}, c.M...)
	if err := c.SetTangents(m); err != nil {
		return nil, err
	}
	if err := c.FillImage(im); err != nil {
		return nil, err
	}
	if err := im.SavePGM(path); err != nil {
		return nil, err
	}
	_ = c.SetTangents(old)
	return im, nil
}

func imageMetrics(size int, c *ramp.Curve, m []float64, im *raster.Image) imageReport {
	old := append([]float64{}, c.M...)
	_ = c.SetTangents(m)
	px, badRows, maxLev, _ := c.OvershootPixels(im)
	badDense, maxDense := c.OvershootDense(800)
	brightDev, perPoint := c.BrightPixelDeviation(im, brightIdx)
	_ = c.SetTangents(old)
	return imageReport{
		Size:             size,
		OvershootPixels:  px,
		OvershootRows:    badRows,
		MaxExcessLevels:  maxLev,
		DenseBadSamples:  badDense,
		DenseMaxExcess:   maxDense,
		BrightPixelDev:   brightDev,
		BrightDevDetails: perPoint,
	}
}

// jAt 用给定系数计算 J(t)。
func jAt(A, B, C, t float64) float64 { return A*t*t - 2*B*t + C }

func main() {
	outDir := "artifacts"
	if len(os.Args) > 1 {
		outDir = os.Args[1]
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "建目录失败:", err)
		os.Exit(1)
	}

	c, err := ramp.New(customerStops)
	if err != nil {
		panic(err)
	}
	naive := append([]float64{}, c.M...)

	// ---- 修复前 ----
	before256Img, err := render(256, c, naive, filepath.Join(outDir, "gradient_before_256.pgm"))
	if err != nil {
		panic(err)
	}
	before64Img, err := render(64, c, naive, filepath.Join(outDir, "gradient_before_64.pgm"))
	if err != nil {
		panic(err)
	}
	before := imageMetrics(256, c, naive, before256Img)
	before64 := imageMetrics(64, c, naive, before64Img)

	// ---- 约束最优扫描修复 ----
	sweeps, maxCh := c.Optimize(2000, 1e-14)
	optimal := append([]float64{}, c.M...)
	fcOK := c.FCFeasible(1e-8)

	after256Img, err := render(256, c, optimal, filepath.Join(outDir, "gradient_after_256.pgm"))
	if err != nil {
		panic(err)
	}
	after64Img, err := render(64, c, optimal, filepath.Join(outDir, "gradient_after_64.pgm"))
	if err != nil {
		panic(err)
	}
	after := imageMetrics(256, c, optimal, after256Img)
	after64 := imageMetrics(64, c, optimal, after64Img)

	brightKnotDev := c.KnotDeviation(brightIdx)
	brightChordDev := c.ChordDeviation([]int{0, 1, 2}, 1600)

	// 每个内部节点的求解归类（用最终切线作为邻切线）。
	knots := []knotReport{}
	for k := 1; k+1 < len(c.X); k++ {
		sol, err := c.SolveKnot(k)
		if err != nil {
			panic(err)
		}
		knots = append(knots, knotReport{
			K: k, X: c.X[k], Y: c.Y[k], Case: sol.Case,
			Lo: sol.Lo, Hi: sol.Hi, Tau: sol.Tau, That: sol.That,
			Naive: naive[k],
		})
	}

	// ---- 三种情形：同一组 h=d=1、下降弦 dL=dR=-1，只改两个固定邻切线 ----
	mkCase := func(name, desc string, a, b float64) caseNumbers {
		const hL, hR, dL, dR = 1.0, 1.0, -1.0, -1.0
		sol := ramp.SolveOne(hL, hR, dL, dR, a, b)
		return caseNumbers{
			Name: name, Description: desc,
			Input: map[string]float64{
				"hL": hL, "hR": hR, "dL": dL, "dR": dR,
				"m_left_fixed_a": a, "m_right_fixed_b": b,
			},
			Solution: sol,
			CheckJ: map[string]float64{
				"J_at_lo":      jAt(sol.A, sol.B, sol.C, sol.Lo),
				"J_at_tau":     jAt(sol.A, sol.B, sol.C, sol.Tau),
				"J_at_hi":      jAt(sol.A, sol.B, sol.C, sol.Hi),
				"J_at_optimum": sol.JHat,
			},
		}
	}
	cases := []caseNumbers{
		mkCase("left_endpoint_lower",
			"驻点 τ=-2.05 落在区间 [-1.8,0] 左侧之外，最优解=左（幅值）端点 -1.8。",
			-2.4, -2.4),
		mkCase("right_endpoint_upper",
			"固定邻切线符号错误（+1），无约束驻点 τ=+0.5 落在 [-2.8284,0] 右侧之外，同号约束把最优解钉在右端点 0。",
			+1.0, +1.0),
		mkCase("interior_stationary",
			"驻点 τ=-1 位于区间 [-2.8284,0] 内部，J'(-1)=0 且 J''=2A>0，最优解就是内部驻点 -1。",
			-1.0, -1.0),
	}

	rep := report{
		Stops: customerStops, SecantSlopes: c.D,
		NaiveTangents: naive, OptimalTangents: optimal,
		Sweeps: sweeps, FinalMaxChange: maxCh, FCFeasible: fcOK,
		BrightKnotDev:  brightKnotDev,
		BrightChordDev: brightChordDev,
		Before:         before, After: after,
		Knots: knots, ThreeCases: cases,
	}

	jb, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "ramp_report.json"), jb, 0o644); err != nil {
		panic(err)
	}
	// 64×64 的数据单独写一个附加 JSON，避免主报告结构膨胀。
	mb, _ := json.MarshalIndent(map[string]imageReport{"before_64": before64, "after_64": after64}, "", "  ")
	_ = os.WriteFile(filepath.Join(outDir, "ramp_report_64.json"), mb, 0o644)

	fmt.Println("== 客户渐变：朴素平均切线（修复前） ==")
	fmt.Printf("256² 过冲像素=%d 问题行=%d 最大超出=%d 灰阶；64² 过冲像素=%d\n",
		before.OvershootPixels, before.OvershootRows, before.MaxExcessLevels, before64.OvershootPixels)
	fmt.Println("== 约束最优切线（修复后） ==")
	fmt.Printf("扫描=%d 轮 末轮最大变动=%.2e FC可行=%v\n", sweeps, maxCh, fcOK)
	fmt.Printf("256² 过冲像素=%d 连续密采坏点=%d 最大越界=%.3e；64² 过冲像素=%d\n",
		after.OvershootPixels, after.DenseBadSamples, after.DenseMaxExcess, after64.OvershootPixels)
	fmt.Printf("亮部控制点偏离量 δ=%.3e（强度单位），亮部离控制折线最大形状偏离=%.6f，8-bit 256² 最近行最大灰阶偏离=%d\n",
		brightKnotDev, brightChordDev, after.BrightPixelDev)
	fmt.Println("== 三种情形 ==")
	for _, cs := range cases {
		s := cs.Solution
		fmt.Printf("%-22s case=%-10s interval=[%.6g,%.6g] tau=%.6g t*=%.6g J*=%.9g\n",
			cs.Name, s.Case, s.Lo, s.Hi, s.Tau, s.That, s.JHat)
	}
	fmt.Println("报告已写入", filepath.Join(outDir, "ramp_report.json"))
}
