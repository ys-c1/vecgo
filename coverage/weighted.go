package coverage

import "math"

// BinomialWeights 按超采样估计的二项方差给出逆方差权重：
//
//	sigma_k^2 = alpha_k*(1-alpha_k)/N,  w_k ∝ 1/sigma_k^2
//
// 公共因子 N 消去，因此直接使用 1/(alpha*(1-alpha)) 并归一化。
// 边界 alpha=0/1 处方差为 0，权重无界；这类档位误差也必为 0，
// 故权重取 0、由其余档位归一化。
func BinomialWeights(alphas []float64) []float64 {
	w := make([]float64, len(alphas))
	sum := 0.0
	for i, a := range alphas {
		a = Clamp01(a)
		v := a * (1 - a)
		if v > 0 {
			w[i] = 1 / v
			sum += w[i]
		}
	}
	if sum == 0 {
		// 全部落在 0/1：退化为等权，避免除零。
		for i := range w {
			w[i] = 1 / float64(len(w))
		}
		return w
	}
	for i := range w {
		w[i] /= sum
	}
	return w
}

// WeightedMAE 加权平均绝对误差 sum(w_i*|a_i-b_i|)。
func WeightedMAE(a, b, w []float64) float64 {
	s := 0.0
	for i := range a {
		s += w[i] * math.Abs(a[i]-b[i])
	}
	return s
}

// WeightedRMSE 加权均方根误差 sqrt(sum(w_i*(a_i-b_i)^2))，
// 权重应已归一化到和为 1。
func WeightedRMSE(a, b, w []float64) float64 {
	s := 0.0
	for i := range a {
		d := a[i] - b[i]
		s += w[i] * d * d
	}
	return math.Sqrt(s)
}
