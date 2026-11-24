// Package level 推导“色阶”在渐变参数轴上的分布，并统计暗部相邻色阶的实际间距方差。
//
// 核心假设（感知均匀）：在中性灰、稳态适应、同尺寸同空间频率的观察条件下，
// 相邻色阶的差异可用一个一维视觉响应坐标 P(t) 上的距离度量；感知均匀即要求
// P(t_{i+1})-P(t_i) 为常数。该假设只适用于明视觉灰阶/低饱和渐变，不适用于
// HDR、彩色对立通道、暗视觉或传递函数与假设不符的显示链路（详见
// artifacts/perceptual/REPORT.md 的“适用性”一节）。
package level

import (
	"errors"
	"math"
)

// Dist 是色阶沿参数轴 t∈[0,1] 的分布模型。
type Dist string

const (
	// Linear 默认线性分布：t_i = i/(N-1)。产物与历史版本逐字节一致。
	Linear Dist = "linear"
	// Perceptual 感知均匀（幂律）分布：P(t)=t^Alpha，等视觉差反解
	// t_i = (i/(N-1))^(1/Alpha)。Alpha==0 时取 DefaultAlpha（=2，闭式 sqrt）。
	Perceptual Dist = "perceptual"
	// Lab 用 CIE L* 明度作为视觉坐标的精确感知均匀分布（经 sRGB 传递函数），
	// 作为幂律假设的标准交叉验证，不参与默认路径。
	Lab Dist = "lab"

	// DefaultAlpha 是幂律假设的缺省指数。取 2 可得到 sqrt 闭式与整数奇数间距
	// 的闭式方差预测；一般 Stevens/gamma≈2.2 只需替换该指数。
	DefaultAlpha = 2.0
)

// Normalize 把配置字符串解析成 Dist；空串视为默认 Linear。
func Normalize(s string) (Dist, error) {
	switch s {
	case "", "linear":
		return Linear, nil
	case "perceptual", "power":
		return Perceptual, nil
	case "lab", "cielab", "lstar":
		return Lab, nil
	default:
		return "", errors.New("未知色阶分布: " + s + "（可选 linear|perceptual|lab）")
	}
}

// CleanAlpha 把零值替换为缺省指数，并拒绝非法指数。
func CleanAlpha(a float64) (float64, error) {
	if a == 0 {
		return DefaultAlpha, nil
	}
	if math.IsNaN(a) || math.IsInf(a, 0) || a < 0.2 || a > 8 {
		return 0, errors.New("alpha 必须落在 [0.2,8] 且有限")
	}
	return a, nil
}

// Position 返回 N 个色阶中第 i 阶（由暗到亮，i=0 最暗）在参数轴上的位置 t∈[0,1]。
func Position(i, n int, d Dist, alpha float64) (float64, error) {
	ts, err := Positions(n, d, alpha)
	if err != nil {
		return 0, err
	}
	return ts[i], nil
}

// Positions 返回长度 N 的参数轴分布，顺序由暗(t=0)到亮(t=1)。
func Positions(n int, d Dist, alpha float64) ([]float64, error) {
	if n < 1 {
		return nil, errors.New("色阶数必须为正")
	}
	a, err := CleanAlpha(alpha)
	if err != nil {
		return nil, err
	}
	if _, err := Normalize(string(d)); err != nil {
		return nil, err
	}
	out := make([]float64, n)
	if n == 1 {
		out[0] = 0
		return out, nil
	}
	for i := 0; i < n; i++ {
		x := float64(i) / float64(n-1)
		switch d {
		case Linear:
			out[i] = x
		case Perceptual:
			out[i] = math.Pow(x, 1/a)
		case Lab:
			out[i] = LabEncode(LfromStar(100 * x))
		}
	}
	return out, nil
}

// HalfDarkGaps 取由暗端起约一半的相邻间隔作为“暗部相邻色阶”。
// 对 N 阶序列共 N-1 个间隔，取前 m=(N-1)/2（整数向下取整）个。
func HalfDarkGaps(ts []float64, d Dist, alpha float64) ([]float64, error) {
	a, err := CleanAlpha(alpha)
	if err != nil {
		return nil, err
	}
	if _, err := Normalize(string(d)); err != nil {
		return nil, err
	}
	if len(ts) < 2 {
		return nil, errors.New("至少需要两个色阶")
	}
	m := HalfDarkCount(len(ts))
	gaps := make([]float64, 0, m)
	for i := 0; i < m; i++ {
		switch d {
		case Lab:
			// ts 已经是 sRGB 编码值，Lstar 内部会解码一次，切勿再 LabDecode。
			gaps = append(gaps, Lstar(ts[i+1])-Lstar(ts[i]))
		default:
			gaps = append(gaps, math.Pow(ts[i+1], a)-math.Pow(ts[i], a))
		}
	}
	return gaps, nil
}

// HalfDarkCount 返回 N 阶序列暗半部的间隔数 m=(N-1)/2。
func HalfDarkCount(n int) int {
	if n < 2 {
		return 0
	}
	return (n - 1) / 2
}

// Variance 返回总体方差（除以 k，而非 k-1）；空序列返回 0。
func Variance(xs []float64) float64 {
	k := len(xs)
	if k == 0 {
		return 0
	}
	s := 0.0
	for _, x := range xs {
		s += x
	}
	mean := s / float64(k)
	ss := 0.0
	for _, x := range xs {
		d := x - mean
		ss += d * d
	}
	return ss / float64(k)
}

// Mean 返回算术平均；空序列返回 0。
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

// DarkVariance 是一步到位的统计：构造 N 阶分布并返回暗半部视觉间距及其总体方差。
func DarkVariance(n int, d Dist, alpha float64) (m int, mean, variance float64, err error) {
	ts, err := Positions(n, d, alpha)
	if err != nil {
		return 0, 0, 0, err
	}
	gaps, err := HalfDarkGaps(ts, d, alpha)
	if err != nil {
		return 0, 0, 0, err
	}
	return len(gaps), Mean(gaps), Variance(gaps), nil
}

// LinearGap 给出默认线性分布 t_i=i/(N-1) 在幂律视觉坐标 P(t)=t^Alpha 上的第 i 个间距：
//
//	g_i = ((i+1)^Alpha - i^Alpha) / (N-1)^Alpha
func LinearGap(i, n int, alpha float64) (float64, error) {
	if n < 2 {
		return 0, errors.New("色阶数至少为 2")
	}
	a, err := CleanAlpha(alpha)
	if err != nil {
		return 0, err
	}
	den := math.Pow(float64(n-1), a)
	return (math.Pow(float64(i+1), a) - math.Pow(float64(i), a)) / den, nil
}

// PredictLinearDarkVariance 用与测量路径独立的整数下标闭式求和，预测默认线性分布
// 暗半部 m=HalfDarkCount(N) 个间距的均值与总体方差。
func PredictLinearDarkVariance(n int, alpha float64) (m int, mean, variance float64, err error) {
	m = HalfDarkCount(n)
	if m == 0 {
		return 0, 0, 0, nil
	}
	gaps := make([]float64, m)
	for i := 0; i < m; i++ {
		g, e := LinearGap(i, n, alpha)
		if e != nil {
			return 0, 0, 0, e
		}
		gaps[i] = g
	}
	return m, Mean(gaps), Variance(gaps), nil
}

// PredictLinearDarkVarianceSq 是 Alpha=2 时的真·闭式预测。
//
// 此时 g_i=(2i+1)/(N-1)^2，前 m 个奇数的均值为 m/(N-1)^2，
// 总体方差为 (m^2-1)/(3 (N-1)^4)。
func PredictLinearDarkVarianceSq(n int) (m int, mean, variance float64) {
	m = HalfDarkCount(n)
	if m == 0 || n < 2 {
		return m, 0, 0
	}
	d := float64(n - 1)
	mean = float64(m) / (d * d)
	variance = (float64(m*m) - 1) / (3 * d * d * d * d)
	return m, mean, variance
}

// IdealPerceptualVariance 是“相邻视觉差严格相等”假设下的理想间距方差：恒为 0。
// 实现分布因浮点/8-bit 量化只会在其附近有残差。
func IdealPerceptualVariance() float64 { return 0 }
