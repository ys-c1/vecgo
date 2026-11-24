// Package coverage 提供覆盖率估计的二项分布统计口径、可配样本采样器，
// 以及实测方差对理论预测的校验工具。
//
// 核心模型：在画幅内独立均匀抽取 n 个样本点，命中几何覆盖区域的次数为
// X，则 X~Bin(n,p)，覆盖率估计 p̂=X/n 满足
//
//	E[p̂]=p,  Var(p̂)=p(1-p)/n ≤ 1/(4n)。
package coverage

import (
	"errors"
	"math"
)

// Variance 是覆盖率估计 p̂=X/n 的二项分布理论方差 p(1-p)/n。
// n 必须为正，p 会被截断到 [0,1]。
func Variance(p float64, n int) float64 {
	if n <= 0 {
		panic("coverage: n 必须为正")
	}
	if p < 0 {
		p = 0
	} else if p > 1 {
		p = 1
	}
	return p * (1 - p) / float64(n)
}

// WorstVariance 是方差上界 max_p p(1-p)/n = 1/(4n)，在 p=1/2 时取到。
func WorstVariance(n int) float64 {
	if n <= 0 {
		panic("coverage: n 必须为正")
	}
	return 1.0 / (4.0 * float64(n))
}

// RequiredSamples 按“方差上界不超过 vMax”反解样本数：
//
//	1/(4n) ≤ vMax  ⇒  n ≥ ceil(1/(4 vMax))。
//
// vMax 必须为正且不超过 1/4（否则 n=1 已满足，这里对 >1/4 放行返回 1，
// 对非正或 NaN 报错）。
func RequiredSamples(vMax float64) (int, error) {
	if math.IsNaN(vMax) || vMax <= 0 {
		return 0, errors.New("coverage: 目标方差必须为正")
	}
	n := int(math.Ceil(1.0 / (4.0 * vMax)))
	if n < 1 {
		n = 1
	}
	return n, nil
}

// RequiredSamplesSE 是按最坏情况标准误上界 seMax 反解样本数的便捷入口：
// 1/(2√n) ≤ seMax ⇒ n ≥ ceil(1/(4 seMax²))。
func RequiredSamplesSE(seMax float64) (int, error) {
	if math.IsNaN(seMax) || seMax <= 0 {
		return 0, errors.New("coverage: 标准误上界必须为正")
	}
	return RequiredSamples(seMax * seMax)
}

// Mean 计算算术平均。
func Mean(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := 0.0
	for _, x := range xs {
		s += x
	}
	return s / float64(len(xs))
}

// SampleVariance 返回无偏样本方差 1/(m-1) Σ(x-x̄)²；样本少于 2 时返回 0。
func SampleVariance(xs []float64) float64 {
	m := len(xs)
	if m < 2 {
		return 0
	}
	mean := Mean(xs)
	s := 0.0
	for _, x := range xs {
		d := x - mean
		s += d * d
	}
	return s / float64(m-1)
}

// Welford 是单遍稳定的均值/方差累加器。
type Welford struct {
	N    int
	mean float64
	m2   float64
}

// Add 加入一个观测值。
func (w *Welford) Add(x float64) {
	w.N++
	d := x - w.mean
	w.mean += d / float64(w.N)
	d2 := x - w.mean
	w.m2 += d * d2
}

// Mean 返回当前均值。
func (w *Welford) Mean() float64 { return w.mean }

// Variance 返回无偏样本方差；样本少于 2 时为 0。
func (w *Welford) Variance() float64 {
	if w.N < 2 {
		return 0
	}
	return w.m2 / float64(w.N-1)
}

// VariancePredictionInterval 在 iid 正态近似下给出样本方差 S² 的 1-α
// 预测区间。若单次覆盖率估计 p̂ 近似正态，则 (m-1)S²/σ² ~ χ²_{m-1}，故
//
//	S² ∈ [σ² q_{α/2}/(m-1), σ² q_{1-α/2}/(m-1)]。
//
// m 是用于计算 S² 的重复估计个数（自由度 m-1）。
func VariancePredictionInterval(sigma2 float64, m int, alpha float64) (lo, hi float64) {
	df := float64(m - 1)
	lo = sigma2 * ChiSquareQuantile(alpha/2, df) / df
	hi = sigma2 * ChiSquareQuantile(1-alpha/2, df) / df
	return lo, hi
}

// EstimatePredictionInterval 给出单个覆盖率估计 p̂ 的正态预测区间
// p ± z_{1-α/2} σ。
func EstimatePredictionInterval(p, sigma float64, alpha float64) (lo, hi float64) {
	z := NormalQuantile(1 - alpha/2)
	return p - z*sigma, p + z*sigma
}

// NormalQuantile 是标准正态分位数函数（Peter Acklam 有理逼近，
// 绝对误差约 1.15e-9，足够构造预测区间）。
func NormalQuantile(prob float64) float64 {
	if prob <= 0 || prob >= 1 {
		panic("coverage: NormalQuantile 的 prob 必须在 (0,1)")
	}
	const (
		a1 = -3.969683028665376e+01
		a2 = 2.209460984245205e+02
		a3 = -2.759285104469687e+02
		a4 = 1.383577518672690e+02
		a5 = -3.066479806614716e+01
		a6 = 2.506628277459239e+00
		b1 = -5.447609879822406e+01
		b2 = 1.615858368580409e+02
		b3 = -1.556989798598866e+02
		b4 = 6.680131188771972e+01
		b5 = -1.328068155288572e+01
		b6 = 1.000000000000000e+00
		c1 = -7.784894002430293e-03
		c2 = -3.223964580411365e-01
		c3 = -2.400758277161838e+00
		c4 = -2.549732539343734e+00
		c5 = 4.374664141464968e+00
		c6 = 2.938163982698783e+00
		d1 = 7.784695709041462e-03
		d2 = 3.224671290700398e-01
		d3 = 2.445134137142996e+00
		d4 = 3.754408661907416e+00
	)
	plow := 0.02425
	phigh := 1 - plow
	if prob < plow {
		q := math.Sqrt(-2 * math.Log(prob))
		return (((((c1*q+c2)*q+c3)*q+c4)*q+c5)*q + c6) /
			((((d1*q+d2)*q+d3)*q+d4)*q + 1)
	}
	if prob <= phigh {
		q := prob - 0.5
		r := q * q
		return (((((a1*r+a2)*r+a3)*r+a4)*r+a5)*r + a6) * q /
			(((((b1*r+b2)*r+b3)*r+b4)*r+b5)*r + b6)
	}
	q := math.Sqrt(-2 * math.Log(1-prob))
	return -(((((c1*q+c2)*q+c3)*q+c4)*q+c5)*q + c6) /
		((((d1*q+d2)*q+d3)*q+d4)*q + 1)
}

// ChiSquareQuantile 返回自由度 df 的 χ² 分布下侧 p 分位数。
// 通过正则化下不完全伽马函数 P(df/2, x/2) 二分反解，df 必须为正。
func ChiSquareQuantile(p, df float64) float64 {
	if df <= 0 {
		panic("coverage: chi-square df 必须为正")
	}
	if p <= 0 || p >= 1 || math.IsNaN(p) {
		panic("coverage: chi-square 分位概率必须在 (0,1)")
	}
	a := df / 2
	target := p
	lo, hi := 0.0, math.Max(a, 1)
	for RegularizedGammaP(a, hi) < target {
		hi *= 2
	}
	var mid float64
	for i := 0; i < 120; i++ {
		mid = (lo + hi) / 2
		if RegularizedGammaP(a, mid) < target {
			lo = mid
		} else {
			hi = mid
		}
	}
	return 2 * mid
}

// RegularizedGammaP 返回正则化下不完全伽马函数 P(a,x)=γ(a,x)/Γ(a)。
// 实现为 Numerical Recipes 的级数/连分式版本。
func RegularizedGammaP(a, x float64) float64 {
	if a <= 0 {
		panic("coverage: gamma a 必须为正")
	}
	if x <= 0 {
		return 0
	}
	if x < a+1 {
		return gammaSeries(a, x)
	}
	return 1 - gammaContinuedFractionQ(a, x)
}

const gammaEps = 3e-14
const gammaIter = 1000

func gammaSeries(a, x float64) float64 {
	ap := a
	del := 1 / a
	sum := del
	for n := 0; n < gammaIter; n++ {
		ap++
		del *= x / ap
		sum += del
		if math.Abs(del) < math.Abs(sum)*gammaEps {
			break
		}
	}
	return sum * math.Exp(-x+a*math.Log(x)-lgamma(a))
}

func gammaContinuedFractionQ(a, x float64) float64 {
	b := x + 1 - a
	c := 1e300
	d := 1 / b
	h := d
	for i := 1; i <= gammaIter; i++ {
		an := -float64(i) * (float64(i) - a)
		b += 2
		d = an*d + b
		if math.Abs(d) < 1e-300 {
			d = 1e-300
		}
		c = b + an/c
		if math.Abs(c) < 1e-300 {
			c = 1e-300
		}
		d = 1 / d
		delta := d * c
		h *= delta
		if math.Abs(delta-1) < gammaEps {
			break
		}
	}
	return math.Exp(-x+a*math.Log(x)-lgamma(a)) * h
}

func lgamma(a float64) float64 {
	lg, _ := math.Lgamma(a)
	return lg
}
