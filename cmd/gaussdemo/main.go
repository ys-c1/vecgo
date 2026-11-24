// gaussdemo：三级箱式模糊逼近高斯的反解、逐级产物、实测误差与耗时对照。
//
// 用法：
//
//	go run ./cmd/gaussdemo [-size 256] [-budget 0.05] [-out artifacts/gauss]
//
// 默认对 sigma=2 与 sigma=5 两档各做一次反解与实测，产物写入 -out 目录。
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"time"

	"vecgo/post"
	"vecgo/raster"
)

type sigmaReport struct {
	Sigma         float64   `json:"sigma"`
	Budget255     float64   `json:"budget255"`
	N             int       `json:"n"`
	Widths        []int     `json:"widths3"`
	WIdeal        float64   `json:"widthIdeal"`
	WL            int       `json:"widthLow"`
	WU            int       `json:"widthUp"`
	Variance      float64   `json:"varianceCascade"`
	VarianceTgt   float64   `json:"varianceTarget"`
	L1Dist        float64   `json:"l1Dist"`
	Tail          float64   `json:"tailBound"`
	TruncK        int       `json:"truncK"`
	Bound255      float64   `json:"pointwiseBound255"`
	NStar         int       `json:"nStarForBudget"`
	WithinBudget  bool      `json:"threeStagesWithinBudget"`
	MeasuredMax   int       `json:"measuredMaxAbsDiff"`
	MeasuredMean  float64   `json:"measuredMeanAbsDiff"`
	BoundHolds    bool      `json:"measuredWithinBound"`
	TimeSep       float64   `json:"timeSeparableMs"`
	TimeNonSep    float64   `json:"timeNonSeparableMs"`
	Speedup       float64   `json:"speedupNonSepOverSep"`
	StageFiles    [3]string `json:"stageFiles"`
	FinalFile     string    `json:"finalFile"`
	ReferenceFile string    `json:"referenceFile"`
	DiffFile      string    `json:"diffFile"`
}

type fullReport struct {
	Size        int            `json:"size"`
	BudgetFrac  float64        `json:"budgetFraction"`
	Budget255   float64        `json:"budget255"`
	GeneratedAt string         `json:"generatedAt"`
	Reports     []*sigmaReport `json:"reports"`
}

func main() {
	size := flag.Int("size", 256, "方形测试图边长")
	budgetFrac := flag.Float64("budget", 0.05, "目标逐像素误差上界（占满量程比例）")
	outDir := flag.String("out", filepath.Join("artifacts", "gauss"), "产物目录")
	flag.Parse()

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fail(err)
	}
	im := buildTestImage(*size)
	if err := im.SavePGM(filepath.Join(*outDir, "input.pgm")); err != nil {
		fail(err)
	}

	budget255 := *budgetFrac * 255
	rep := &fullReport{
		Size:        *size,
		BudgetFrac:  *budgetFrac,
		Budget255:   budget255,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
	}

	for _, sigma := range []float64{2.0, 5.0} {
		plan, err := post.Invert(sigma, budget255, 8)
		if err != nil {
			fail(err)
		}
		widths := [3]int{plan.Widths[0], plan.Widths[1], plan.Widths[2]}

		var stages [3][]float64
		var sep, nonSep *raster.Image
		sepMs := timeMedian(7, func() {
			var err error
			sep, err = post.GaussBlur3(im, widths, &stages)
			if err != nil {
				fail(err)
			}
		}).Seconds() * 1000

		nonSepMs := timeMedian(7, func() {
			var err error
			nonSep, err = post.GaussBlur2DNonSeparable(im, widths)
			if err != nil {
				fail(err)
			}
		}).Seconds() * 1000

		ref := post.GaussReference(im, sigma, plan.TruncK)
		maxD, meanD := post.MaxAbsDiff(sep, ref)
		eqMax, _ := post.MaxAbsDiff(sep, nonSep)

		r := &sigmaReport{
			Sigma:         sigma,
			Budget255:     budget255,
			N:             3,
			Widths:        append([]int(nil), plan.Widths...),
			WIdeal:        plan.WIdeal,
			WL:            plan.WL,
			WU:            plan.WU,
			Variance:      plan.Variance,
			VarianceTgt:   sigma * sigma,
			L1Dist:        plan.L1Dist,
			Tail:          plan.Tail,
			TruncK:        plan.TruncK,
			Bound255:      plan.Bound255,
			NStar:         plan.NStar,
			WithinBudget:  plan.Bound255 <= budget255,
			MeasuredMax:   maxD,
			MeasuredMean:  meanD,
			BoundHolds:    float64(maxD) <= plan.Bound255,
			TimeSep:       sepMs,
			TimeNonSep:    nonSepMs,
			Speedup:       nonSepMs / sepMs,
			FinalFile:     fmt.Sprintf("sigma%d_final.pgm", int(sigma)),
			ReferenceFile: fmt.Sprintf("sigma%d_reference.pgm", int(sigma)),
			DiffFile:      fmt.Sprintf("sigma%d_diff.pgm", int(sigma)),
		}
		for s := 0; s < 3; s++ {
			r.StageFiles[s] = fmt.Sprintf("sigma%d_stage%d_w%d.pgm", int(sigma), s+1, plan.Widths[s])
		}
		rep.Reports = append(rep.Reports, r)

		for s := 0; s < 3; s++ {
			stageImg, _ := raster.New(im.W, im.H)
			for i, v := range stages[s] {
				stageImg.Pix[i] = clampU8(v)
			}
			if err := stageImg.SavePGM(filepath.Join(*outDir, r.StageFiles[s])); err != nil {
				fail(err)
			}
			if err := os.WriteFile(
				filepath.Join(*outDir, fmt.Sprintf("sigma%d_stage%d_w%d.f64", int(sigma), s+1, plan.Widths[s])),
				float64Bytes(stages[s]), 0o644); err != nil {
				fail(err)
			}
		}
		if err := sep.SavePGM(filepath.Join(*outDir, r.FinalFile)); err != nil {
			fail(err)
		}
		if err := ref.SavePGM(filepath.Join(*outDir, r.ReferenceFile)); err != nil {
			fail(err)
		}
		diffImg, _ := raster.New(im.W, im.H)
		for i := range diffImg.Pix {
			d := int(sep.Pix[i]) - int(ref.Pix[i])
			if d < 0 {
				d = -d
			}
			diffImg.Pix[i] = clampU8(float64(d * 16)) // 放大 16 倍便于目视
		}
		if err := diffImg.SavePGM(filepath.Join(*outDir, r.DiffFile)); err != nil {
			fail(err)
		}

		fmt.Printf("sigma=%v 三级箱宽=%v (wIdeal=%.4f, wl=%d, wu=%d)\n",
			sigma, plan.Widths, plan.WIdeal, plan.WL, plan.WU)
		fmt.Printf("  方差: 级联=%.4f 目标=%.4f | L1=%.6f 尾部界=%.2e K=%d\n",
			plan.Variance, sigma*sigma, plan.L1Dist, plan.Tail, plan.TruncK)
		fmt.Printf("  逐像素上界=%.3f/255 预算=%.2f/255 达标=%v NStar=%d\n",
			plan.Bound255, budget255, r.WithinBudget, plan.NStar)
		fmt.Printf("  实测 max=%d mean=%.4f 上界成立=%v | 分离/不分离一致差=%d 级\n",
			maxD, meanD, r.BoundHolds, eqMax)
		fmt.Printf("  耗时: 分离=%.3fms 不分离=%.3fms 加速比=%.2fx\n\n",
			sepMs, nonSepMs, r.Speedup)
	}

	jsonBytes, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		fail(err)
	}
	if err := os.WriteFile(filepath.Join(*outDir, "report.json"), jsonBytes, 0o644); err != nil {
		fail(err)
	}
	if err := os.WriteFile(filepath.Join(*outDir, "report.md"),
		[]byte(renderMarkdown(rep)), 0o644); err != nil {
		fail(err)
	}
	fmt.Printf("产物目录: %s\n", *outDir)
}

// buildTestImage 构造含实心方块、水平渐变与顶部条纹的测试图。
func buildTestImage(n int) *raster.Image {
	im, _ := raster.New(n, n)
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			v := uint8((x * 2) % 256)
			if x >= n/5 && x < 4*n/5 && y >= n/3 && y < 2*n/3 {
				v = 240
			}
			if (x/8)%2 == 0 && y < n/8 {
				v = 255 - v
			}
			if x == n/2 && y > n/8 {
				v = 64
			}
			im.Pix[y*n+x] = v
		}
	}
	return im
}

func renderMarkdown(rep *fullReport) string {
	s := "# 三级箱式模糊逼近高斯：反解与实测报告\n\n"
	s += fmt.Sprintf("- 测试图: %dx%d，预算: %.1f%%（%.2f/255），生成于 %s\n\n",
		rep.Size, rep.Size, rep.BudgetFrac*100, rep.Budget255, rep.GeneratedAt)
	s += "## 三级箱宽反解表（可复算）\n\n"
	s += "| sigma | wIdeal=sqrt(12σ²/3+1) | wl（奇） | wu（奇） | 采用三级箱宽 | 级联方差 | 目标方差 |\n"
	s += "|---:|---:|---:|---:|:---|---:|---:|\n"
	for _, r := range rep.Reports {
		s += fmt.Sprintf("| %g | %.4f | %d | %d | %v | %.4f | %.4f |\n",
			r.Sigma, r.WIdeal, r.WL, r.WU, r.Widths, r.Variance, r.VarianceTgt)
	}
	s += "\n## 逐像素误差上界与实测\n\n"
	s += "上界 = 255×(L1(级联核, 截断高斯核) + erfc(K/(√2σ))) + 0.5\n\n"
	s += "| sigma | L1 距离 | 尾部界 | 截断 K | 上界(/255) | 预算(/255) | 三级达标 | 反解 NStar | 实测 max | 实测 mean | 上界成立 |\n"
	s += "|---:|---:|---:|---:|---:|---:|:---:|---:|---:|---:|:---:|\n"
	for _, r := range rep.Reports {
		s += fmt.Sprintf("| %g | %.6f | %.2e | %d | %.3f | %.2f | %v | %d | %d | %.4f | %v |\n",
			r.Sigma, r.L1Dist, r.Tail, r.TruncK, r.Bound255, r.Budget255,
			r.WithinBudget, r.NStar, r.MeasuredMax, r.MeasuredMean, r.BoundHolds)
	}
	s += "\n## 耗时对照（同图同箱宽，ms）\n\n"
	s += "| sigma | 分离（6 个一维 pass） | 不分离（3 次二维方窗） | 加速比 |\n"
	s += "|---:|---:|---:|---:|\n"
	for _, r := range rep.Reports {
		s += fmt.Sprintf("| %g | %.3f | %.3f | %.2fx |\n",
			r.Sigma, r.TimeSep, r.TimeNonSep, r.Speedup)
	}
	s += "\n## 逐级产物\n\n"
	for _, r := range rep.Reports {
		s += fmt.Sprintf("- sigma=%g: %v（另附同名 .f64 浮点原始值），最终 %s，真高斯参考 %s，差值 %s\n",
			r.Sigma, r.StageFiles, r.FinalFile, r.ReferenceFile, r.DiffFile)
	}
	return s
}

func clampU8(v float64) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v + 0.5)
}

func float64Bytes(f []float64) []byte {
	b := make([]byte, len(f)*8)
	for i, v := range f {
		bits := math.Float64bits(v)
		for j := 0; j < 8; j++ {
			b[i*8+j] = byte(bits >> (8 * j))
		}
	}
	return b
}

// timeMedian 跑 warmup + reps 轮，返回后 reps 轮的中位数耗时。
func timeMedian(reps int, fn func()) time.Duration {
	fn() // warmup，摊平分配与缓存影响
	durations := make([]time.Duration, reps)
	for i := range durations {
		t := time.Now()
		fn()
		durations[i] = time.Since(t)
	}
	for i := 0; i < reps; i++ {
		for j := i + 1; j < reps; j++ {
			if durations[j] < durations[i] {
				durations[i], durations[j] = durations[j], durations[i]
			}
		}
	}
	return durations[reps/2]
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "失败:", err)
	os.Exit(1)
}
