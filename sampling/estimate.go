package sampling

import (
	"math/rand"

	"vecgo/geom"
)

// Coverage 用点集 pts 估计形状 s 在单位像素内的覆盖率 P̂ = (1/n)·Σ 1{s(xᵢ)}。
func Coverage(s Shape, pts []geom.Point) float64 {
	hit := 0
	for _, p := range pts {
		if s.Contains(p) {
			hit++
		}
	}
	return float64(hit) / float64(len(pts))
}

// CRB 返回 iid 伯努利模型下覆盖率估计量的 Cramér–Rao 方差下界 p(1-p)/n。
func CRB(p, n float64) float64 {
	return p * (1 - p) / n
}

// BenchResult 汇总单个采样器在固定样本数下的一次基准结果。
type BenchResult struct {
	Sampler Sampler
	N       int
	// MSE 实测均方误差 E[(P̂-p)²]，对无偏估计量即方差。
	MSE float64
	// BiasMean 平均有符号误差 E[P̂-p]，用于检验无偏性。
	BiasMean float64
	// CRBMean 各次试验 p(1-p)/n 的均值，即 iid 模型下界的总体版本。
	CRBMean float64
}

// Ratio 返回实测方差与下界之比，1.0 表示恰好达到下界。
func (r BenchResult) Ratio() float64 {
	return r.MSE / r.CRBMean
}

// BenchmarkCoverage 在同一组随机形状上运行采样器，估计方差并与下界并列。
// shapes 对所有采样器共用（公共随机数），保证比较公平；
// rng 只对含随机性的采样器（IID）有意义，网格与 Halton 的点集确定，
// 其随机性来自形状的随机平移，等价于 Cranley–Patterson 随机化。
func BenchmarkCoverage(sampler Sampler, n int, shapes []Shape, rng *rand.Rand) BenchResult {
	var se, bias, crbSum float64
	for _, shape := range shapes {
		pts := sampler.Sample(n, rng)
		est := Coverage(shape, pts)
		p := shape.Area()
		err := est - p
		se += err * err
		bias += err
		crbSum += CRB(p, float64(n))
	}
	t := float64(len(shapes))
	return BenchResult{
		Sampler:  sampler,
		N:        n,
		MSE:      se / t,
		BiasMean: bias / t,
		CRBMean:  crbSum / t,
	}
}
