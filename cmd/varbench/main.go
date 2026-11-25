// varbench 实测三种采样方式的覆盖率估计方差，并与 iid 无偏估计的
// Cramér–Rao 下界 p(1-p)/n 并列；同时给出星号 L2 偏差的衰减阶，
// 用于对照 Roth 下界（网格达不到、低差异序列逼近）。
package main

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"text/tabwriter"

	"vecgo/geom"
	"vecgo/sampling"
)

func main() {
	const trials = 50000

	shapeRNG := rand.New(rand.NewSource(20261126))
	shapes := make([]sampling.Shape, trials)
	for i := range shapes {
		shapes[i] = sampling.RandomShape(shapeRNG)
	}

	ns := []int{16, 64, 256}
	samplers := []sampling.Sampler{sampling.IID{}, sampling.Grid{}, sampling.Halton{}}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Println("== 表 1：覆盖率估计方差 vs Cramér–Rao 下界（trials =", trials, "，混合半平面/细条带）==")
	fmt.Fprintln(w, "采样方式\tn\t实测方差(MSE)\tCRB 下界 p(1-p)/n\t比值 MSE/CRB")
	for _, n := range ns {
		for _, s := range samplers {
			// 每个采样器用独立的随机流；网格/Halton 的点集不依赖它。
			rng := rand.New(rand.NewSource(int64(n*1000) + 7))
			r := sampling.BenchmarkCoverage(s, n, shapes, rng)
			fmt.Fprintf(w, "%s\t%d\t%.3e\t%.3e\t%.4f\n", s.Name(), n, r.MSE, r.CRBMean, r.Ratio())
		}
	}
	w.Flush()

	fmt.Println()
	fmt.Println("== 无偏性校验：平均有符号误差 E[P̂-p]（应全部接近 0）==")
	w2 := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w2, "采样方式\tn\t平均偏差")
	for _, n := range ns {
		for _, s := range samplers {
			rng := rand.New(rand.NewSource(int64(n*1000) + 7))
			r := sampling.BenchmarkCoverage(s, n, shapes, rng)
			fmt.Fprintf(w2, "%s\t%d\t%.2e\n", s.Name(), n, r.BiasMean)
		}
	}
	w2.Flush()

	fmt.Println()
	fmt.Println("== 表 2：Cranley–Patterson 随机平移下的星号 L2 偏差（200 次平移均值）==")
	fmt.Println("   参照：Roth 下界 D₂ ≳ c·√log n/n（2D）；网格 Θ(n^-1/2)，低差异 Θ(log n/n)")
	w3 := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w3, "采样方式\tn\tD₂²(平移均值)\tD₂\tn·D₂")
	discRNG := rand.New(rand.NewSource(31337))
	const shifts = 200
	discRows := []struct {
		name string
		base func(n int) []geom.Point
	}{
		{"iid-随机", func(n int) []geom.Point { return sampling.IID{}.Sample(n, discRNG) }},
		{"网格", sampling.GridBase},
		{"Halton-低差异", sampling.HaltonBase},
	}
	for _, n := range ns {
		for _, r := range discRows {
			base := r.base(n)
			d2sq := sampling.MeanShiftedL2(base, shifts, discRNG)
			d2 := math.Sqrt(d2sq)
			fmt.Fprintf(w3, "%s\t%d\t%.3e\t%.3e\t%.4f\n", r.name, n, d2sq, d2, float64(n)*d2)
		}
	}
	w3.Flush()
}
