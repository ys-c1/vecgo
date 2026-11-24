// 实验程序：覆盖率采样的停点分析。
//
// 回答客户问题：反复渲染同一模型为何一次比一次慢？采样数是否在偷偷增加？
// 做法：
//  1. 对同一几何模型、两个画幅，用逐像素蒙特卡洛覆盖率采样测经验方差随样本数 n 的衰减；
//  2. 从方差衰减反算边际收益，按固定阈值定停点 n*，验证 n* 只与边缘几何有关、与画幅无关；
//  3. 越过 n* 继续加样本，记录多花样本换来的方差降幅与耗时；
//  4. 用生产代码 raster.Image.Fill 做反复渲染漂移检查（固定超采样 vs 递增超采样）。
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"time"

	"vecgo/geom"
	"vecgo/raster"
)

// ---------- 确定性随机源（splitmix64，保证实验可复现） ----------

type rng struct{ s uint64 }

func (r *rng) uint64() uint64 {
	r.s += 0x9e3779b97f4a7c15
	z := r.s
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	return z ^ (z >> 31)
}

func (r *rng) float64() float64 {
	return float64(r.uint64()>>11) / float64(uint64(1)<<53)
}

// ---------- 场景：与 cmd/main.go 同类的方形，旋转 30 度，制造边缘像素 ----------

func scene() []geom.Point {
	c, s := math.Cos(math.Pi/6), math.Sin(math.Pi/6)
	unit := [4][2]float64{{-5, -5}, {5, -5}, {5, 5}, {-5, 5}}
	poly := make([]geom.Point, 4)
	for i, q := range unit {
		poly[i] = geom.Point{X: q[0]*c - q[1]*s, Y: q[0]*s + q[1]*c}
	}
	return poly
}

func bounds(poly []geom.Point) (minx, miny, maxx, maxy float64) {
	minx, miny = poly[0].X, poly[0].Y
	maxx, maxy = minx, miny
	for _, p := range poly {
		minx = math.Min(minx, p.X)
		maxx = math.Max(maxx, p.X)
		miny = math.Min(miny, p.Y)
		maxy = math.Max(maxy, p.Y)
	}
	return
}

// ---------- 逐像素蒙特卡洛覆盖率渲染 ----------

type mcRenderer struct {
	w, h                     int
	poly                     []geom.Point
	minx, miny, spanx, spany float64
	edge                     []int // 边缘像素索引（覆盖率严格介于 0、1 之间）
}

func newMC(w, h int, poly []geom.Point) *mcRenderer {
	minx, miny, maxx, maxy := bounds(poly)
	m := &mcRenderer{w: w, h: h, poly: poly, minx: minx, miny: miny, spanx: maxx - minx, spany: maxy - miny}
	// 用高样本数先跑一遍，确定哪些像素是边缘像素（方差只可能出现在这些像素上）。
	const probe = 4096
	for idx := 0; idx < w*h; idx++ {
		row, col := idx/w, idx%w
		hits := 0
		r := rng{s: 0x123456789abcdef0 ^ uint64(idx)*0x9e3779b97f4a7c15}
		for k := 0; k < probe; k++ {
			if m.sample(row, col, &r) {
				hits++
			}
		}
		p := float64(hits) / probe
		if p > 0.001 && p < 0.999 {
			m.edge = append(m.edge, idx)
		}
	}
	return m
}

func (m *mcRenderer) sample(row, col int, r *rng) bool {
	px := m.minx + (float64(col)+r.float64())*m.spanx/float64(m.w)
	py := m.miny + (float64(row)+r.float64())*m.spany/float64(m.h)
	return geom.Inside(m.poly, px, py)
}

// render 对每个边缘像素投 n 个样本，返回该次渲染中各边缘像素的覆盖率。
func (m *mcRenderer) render(n int, seed uint64) []float64 {
	cov := make([]float64, len(m.edge))
	for e, idx := range m.edge {
		row, col := idx/m.w, idx%m.w
		r := rng{s: seed ^ uint64(idx)*0x9e3779b97f4a7c15}
		hits := 0
		for k := 0; k < n; k++ {
			if m.sample(row, col, &r) {
				hits++
			}
		}
		cov[e] = float64(hits) / float64(n)
	}
	return cov
}

// groundTruth 用大样本数估计每个边缘像素的真实覆盖率 p。
func (m *mcRenderer) groundTruth() []float64 {
	return m.render(1<<16, 0xabcdef0123456789)
}

// ---------- 主流程 ----------

type varianceRow struct {
	Size       int     `json:"size"`
	N          int     `json:"n"`
	EmpVar     float64 `json:"emp_variance"`  // 边缘像素 MSE（对真实覆盖率），重复实验均值
	TheoVar    float64 `json:"theo_variance"` // mean p(1-p)/n
	EdgePixels int     `json:"edge_pixels"`
	Marginal   float64 `json:"marginal_gain"` // 相对上一个 n 的每样本方差下降（经验）
}

type timingRow struct {
	Size int     `json:"size"`
	N    int     `json:"n"`
	MS   float64 `json:"median_ms"`
}

func main() {
	outDir := flag.String("out", "artifacts/exp", "输出目录")
	reps := flag.Int("reps", 40, "每个 n 的独立重复渲染次数")
	timedRuns := flag.Int("timed-runs", 15, "每个 n 的计时次数（含预热）")
	flag.Parse()

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fail(err)
	}
	poly := scene()
	canvasSizes := []int{64, 256}
	ns := []int{1, 2, 4, 8, 10, 12, 16, 24, 32, 48, 64, 100, 128, 160, 200, 256}

	type sizeData struct {
		mc       *mcRenderer
		truth    []float64
		meanPQ   float64
		variance []varianceRow
		timing   []timingRow
	}
	data := map[int]*sizeData{}

	for _, size := range canvasSizes {
		mc := newMC(size, size, poly)
		truth := mc.groundTruth()
		sumPQ := 0.0
		for _, p := range truth {
			sumPQ += p * (1 - p)
		}
		meanPQ := sumPQ / float64(len(truth))
		d := &sizeData{mc: mc, truth: truth, meanPQ: meanPQ}
		fmt.Printf("画幅 %3dx%-3d 边缘像素 %d 个，mean p(1-p)=%.5f\n", size, size, len(mc.edge), meanPQ)

		// ---- 方差实验：每次重复使用独立随机流，避免序列相关 ----
		var prevVar float64
		for _, n := range ns {
			seSum := 0.0
			for rep := 0; rep < *reps; rep++ {
				seed := uint64(rep+1)*0x9e3779b97f4a7c15 ^ uint64(n)*0xd1b54a32d192ed03
				cov := mc.render(n, seed)
				se := 0.0
				for e, est := range cov {
					diff := est - truth[e]
					se += diff * diff
				}
				seSum += se / float64(len(cov))
			}
			emp := seSum / float64(*reps)
			theo := meanPQ / float64(n)
			row := varianceRow{Size: size, N: n, EmpVar: emp, TheoVar: theo, EdgePixels: len(mc.edge)}
			if n > 1 {
				row.Marginal = (prevVar - emp) / float64(n-prevN(ns, n))
			} else {
				row.Marginal = emp // 首样本的"边际收益"即其全部方差量级
			}
			prevVar = emp
			d.variance = append(d.variance, row)
		}

		// ---- 计时实验：预热后取中位数 ----
		for _, n := range ns {
			mc.render(n, 1) // 预热
			durs := make([]time.Duration, 0, *timedRuns)
			for k := 0; k < *timedRuns; k++ {
				start := time.Now()
				mc.render(n, uint64(k+7)*0x9e3779b97f4a7c15)
				durs = append(durs, time.Since(start))
			}
			sort.Slice(durs, func(i, j int) bool { return durs[i] < durs[j] })
			med := durs[len(durs)/2]
			d.timing = append(d.timing, timingRow{Size: size, N: n, MS: float64(med.Microseconds()) / 1000.0})
		}
		data[size] = d
	}

	// ---- 停点：边际收益阈值 ε（相对首样本边际收益）----
	const epsilon = 0.01
	// 理论反算：v(n)=C/n，每样本边际收益 ≈ C/n²；n=1 时为 C。
	// C/n² <= εC  =>  n >= 1/sqrt(ε)。C=mean p(1-p) 只含边缘几何，不含画幅。
	nStar := int(math.Ceil(1.0 / math.Sqrt(epsilon)))

	writeCSV(filepath.Join(*outDir, "variance.csv"), func() [][]string {
		rows := [][]string{{"size", "n", "edge_pixels", "emp_variance", "theo_variance", "marginal_gain"}}
		for _, size := range canvasSizes {
			for _, r := range data[size].variance {
				rows = append(rows, fmtFields(r.Size, r.N, r.EdgePixels, r.EmpVar, r.TheoVar, r.Marginal))
			}
		}
		return rows
	}())
	writeCSV(filepath.Join(*outDir, "timing.csv"), func() [][]string {
		rows := [][]string{{"size", "n", "median_ms"}}
		for _, size := range canvasSizes {
			for _, r := range data[size].timing {
				rows = append(rows, fmtFields(r.Size, r.N, r.MS))
			}
		}
		return rows
	}())

	// ---- 越过停点的代价：以 n* 为基准 ----
	type overshoot struct {
		Size         int     `json:"size"`
		N            int     `json:"n"`
		ExtraSamples int     `json:"extra_samples"`
		VarDropPct   float64 `json:"variance_drop_pct_vs_nstar"`
		TimeMS       float64 `json:"time_ms"`
		TimeOverPct  float64 `json:"time_overhead_pct_vs_nstar"`
	}
	var overshoots []overshoot
	for _, size := range canvasSizes {
		d := data[size]
		baseVar := lookupVar(d.variance, nStar)
		baseTime := lookupTime(d.timing, nStar)
		for _, n := range []int{16, 32, 64, 128, 160, 200, 256} {
			v := lookupVar(d.variance, n)
			t := lookupTime(d.timing, n)
			overshoots = append(overshoots, overshoot{
				Size:         size,
				N:            n,
				ExtraSamples: n - nStar,
				VarDropPct:   100 * (baseVar - v) / baseVar,
				TimeMS:       t,
				TimeOverPct:  100 * (t - baseTime) / baseTime,
			})
		}
	}

	// ---- 漂移检查：生产代码 raster.Image.Fill 反复渲染 ----
	type driftRow struct {
		Call        int     `json:"call"`
		Super       int     `json:"supersample"`
		FixedMS     float64 `json:"fixed_s2_ms"`
		RampedMS    float64 `json:"ramped_ms"`
		RampedSuper int     `json:"ramped_supersample"`
	}
	var drift []driftRow
	supers := []int{2, 2, 2, 2, 2, 3, 4, 5, 6, 7, 8}
	fillOnce := func(w, h, super int) float64 {
		im, err := raster.New(w, h)
		if err != nil {
			fail(err)
		}
		start := time.Now()
		if err := im.Fill([][]geom.Point{poly}, super); err != nil {
			fail(err)
		}
		return float64(time.Since(start).Microseconds()) / 1000.0
	}
	// 预热
	for k := 0; k < 3; k++ {
		fillOnce(256, 256, 2)
	}
	for i, super := range supers {
		// 固定组：始终 supersample=2
		fixed := medianFloat(fillRepeated(5, func() float64 { return fillOnce(256, 256, 2) }))
		// 递增组：按 supers 序列逐次加大
		ramped := fillOnce(256, 256, super)
		drift = append(drift, driftRow{Call: i + 1, Super: 2, FixedMS: fixed, RampedMS: ramped, RampedSuper: super})
	}
	writeCSV(filepath.Join(*outDir, "drift.csv"), func() [][]string {
		rows := [][]string{{"call", "fixed_supersample", "fixed_ms", "ramped_supersample", "ramped_ms"}}
		for _, r := range drift {
			rows = append(rows, fmtFields(r.Call, r.Super, r.FixedMS, r.RampedSuper, r.RampedMS))
		}
		return rows
	}())

	summary := map[string]any{
		"scene":         "square rotated 30deg, world bbox 10x10 units",
		"canvas_sizes":  canvasSizes,
		"replicates":    *reps,
		"epsilon":       epsilon,
		"n_star_theory": nStar,
		"edge_pixels":   map[int]int{64: len(data[64].mc.edge), 256: len(data[256].mc.edge)},
		"mean_p1p":      map[int]float64{64: data[64].meanPQ, 256: data[256].meanPQ},
		"overshoot":     overshoots,
		"ns":            ns,
	}
	buf, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		fail(fmt.Errorf("summary 序列化失败: %w", err))
	}
	if err := os.WriteFile(filepath.Join(*outDir, "summary.json"), buf, 0o644); err != nil {
		fail(err)
	}
	fmt.Printf("n*（理论，ε=%.2f）= %d；结果写入 %s\n", epsilon, nStar, *outDir)
}

func prevN(ns []int, n int) int {
	for i := range ns {
		if ns[i] == n && i > 0 {
			return ns[i-1]
		}
	}
	return n - 1
}

func lookupVar(rows []varianceRow, n int) float64 {
	for _, r := range rows {
		if r.N == n {
			return r.EmpVar
		}
	}
	return math.NaN()
}

func lookupTime(rows []timingRow, n int) float64 {
	for _, r := range rows {
		if r.N == n {
			return r.MS
		}
	}
	return math.NaN()
}

func fillRepeated(k int, fn func() float64) []float64 {
	out := make([]float64, k)
	for i := range out {
		out[i] = fn()
	}
	return out
}

func medianFloat(xs []float64) float64 {
	sort.Float64s(xs)
	return xs[len(xs)/2]
}

func fmtFields(vals ...any) []string {
	out := make([]string, len(vals))
	for i, v := range vals {
		switch t := v.(type) {
		case int:
			out[i] = fmt.Sprintf("%d", t)
		case float64:
			out[i] = fmt.Sprintf("%.6g", t)
		default:
			out[i] = fmt.Sprint(t)
		}
	}
	return out
}

func writeCSV(path string, rows [][]string) {
	f, err := os.Create(path)
	if err != nil {
		fail(err)
	}
	defer f.Close()
	for _, r := range rows {
		for i, c := range r {
			if i > 0 {
				fmt.Fprint(f, ",")
			}
			fmt.Fprint(f, c)
		}
		fmt.Fprintln(f)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "实验失败:", err)
	os.Exit(1)
}
