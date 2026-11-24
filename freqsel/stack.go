package freqsel

import "math"

// SigmaLevel 返回整数层 l 的等效高斯核宽（源像素标准差）：σ_l=Sigma0·2^l。
func SigmaLevel(l int) float64 {
	return Sigma0 * math.Ldexp(1, l) // Sigma0 * 2^l
}

// LevelForSigma 是 SigmaLevel 的反函数：τ=log2(σ/Sigma0)，返回连续层坐标。
func LevelForSigma(sigma float64) float64 {
	if sigma <= Sigma0 {
		return 0
	}
	return math.Log2(sigma / Sigma0)
}

// GaussianBlur 返回以边界复制方式做夹取的可分离高斯模糊结果。
// 截断半径 r=ceil(3σ)（至少 1），核权重归一化，因此不会改变平坦区亮度，
// 且冲激响应非负、频响无旁瓣——这是“不出现振铃”的结构前提。
func GaussianBlur(g *Gray, sigma float64) *Gray {
	r := int(math.Ceil(3 * sigma))
	if r < 1 {
		r = 1
	}
	kernel := make([]float64, 2*r+1)
	twoVar := 2 * sigma * sigma
	sum := 0.0
	for k := -r; k <= r; k++ {
		w := math.Exp(-float64(k*k) / twoVar)
		kernel[k+r] = w
		sum += w
	}
	for i := range kernel {
		kernel[i] /= sum
	}

	tmp := MustGray(g.W, g.H)
	out := MustGray(g.W, g.H)

	// 水平方向。
	for y := 0; y < g.H; y++ {
		for x := 0; x < g.W; x++ {
			acc := 0.0
			for k := -r; k <= r; k++ {
				acc += kernel[k+r] * g.At(x+k, y)
			}
			tmp.Pix[y*g.W+x] = acc
		}
	}
	// 垂直方向。
	for y := 0; y < g.H; y++ {
		for x := 0; x < g.W; x++ {
			acc := 0.0
			for k := -r; k <= r; k++ {
				acc += kernel[k+r] * tmp.At(x, y+k)
			}
			out.Pix[y*g.W+x] = acc
		}
	}
	return out
}

// Stack 是全分辨率尺度空间层栈：Layers[l] 为源图经 σ=SigmaLevel(l)
// 高斯预滤波的结果。保留全分辨率使“同坐标在相邻层间逐像素插值”可直接核对；
// 它与半分辨率 mip 的三线性采样在尺度空间意义下等价（核宽均以源像素计）。
type Stack struct {
	Layers []*Gray
}

// BuildStack 构造 0..maxLevel（含）的层栈，每层都直接从原图模糊到目标 σ，
// 以保证报告的核宽是精确的累计高斯宽度，而不是递推近似。
func BuildStack(g *Gray, maxLevel int) *Stack {
	if maxLevel < 0 {
		maxLevel = 0
	}
	st := &Stack{Layers: make([]*Gray, maxLevel+1)}
	for l := 0; l <= maxLevel; l++ {
		st.Layers[l] = GaussianBlur(g, SigmaLevel(l))
	}
	return st
}

// MaxLevel 返回最大整数层号。
func (s *Stack) MaxLevel() int { return len(s.Layers) - 1 }

// sampleBilinear 在任意连续源像素坐标 (sx,sy) 处双线性采样某一层，边界夹取。
func (s *Stack) sampleBilinear(level int, sx, sy float64) float64 {
	if level < 0 {
		level = 0
	}
	if level > s.MaxLevel() {
		level = s.MaxLevel()
	}
	g := s.Layers[level]
	x0 := int(math.Floor(sx))
	y0 := int(math.Floor(sy))
	fx := sx - float64(x0)
	fy := sy - float64(y0)
	v00 := g.At(x0, y0)
	v10 := g.At(x0+1, y0)
	v01 := g.At(x0, y0+1)
	v11 := g.At(x0+1, y0+1)
	a := v00*(1-fx) + v10*fx
	b := v01*(1-fx) + v11*fx
	return a*(1-fy) + b*fy
}

// SampleContinuous 在连续层坐标 tau 上做相邻两层线性插值（逐像素层间插值）。
// tau 会被夹到 [0,maxLevel]；返回插值灰度与实际使用的小数权重等信息。
func (s *Stack) SampleContinuous(tau, sx, sy float64) float64 {
	maxL := float64(s.MaxLevel())
	if tau < 0 {
		tau = 0
	}
	if tau > maxL {
		tau = maxL
	}
	l0 := int(math.Floor(tau))
	if l0 >= s.MaxLevel() {
		return s.sampleBilinear(s.MaxLevel(), sx, sy)
	}
	w := tau - float64(l0)
	lo := s.sampleBilinear(l0, sx, sy)
	hi := s.sampleBilinear(l0+1, sx, sy)
	return lo*(1-w) + hi*w
}

// SourceToOutputSize 按缩放比计算目标画幅（四舍五入，至少 1）。
func SourceToOutputSize(src int, scale float64) int {
	n := int(math.Round(float64(src) * scale))
	if n < 1 {
		n = 1
	}
	return n
}

// OutputToSource 把目标像素中心 (ox+0.5,oy+0.5) 映到连续源像素坐标（像素中心
// 约定：源像素 i 的中心在 i+0.5，反解后再减 0.5 得到双线性下标）。
func OutputToSource(ox, oy int, scale float64) (sx, sy float64) {
	sx = (float64(ox)+0.5)/scale - 0.5
	sy = (float64(oy)+0.5)/scale - 0.5
	return sx, sy
}
