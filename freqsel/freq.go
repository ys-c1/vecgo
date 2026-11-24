package freqsel

import (
	"errors"
	"math"
	"sort"
)

var errBadSize = errors.New("freqsel: 画幅必须为正")

// FreqField 是逐像素的局部频率估计，全部对齐源图分辨率。
type FreqField struct {
	W, H   int
	Win    int       // 估计窗口边长（奇数）
	F      []float64 // 主频估计，周期/源像素，已夹到 [0,0.5]
	Coh    []float64 // 梯度方向相干度 0..1；越高越接近单一方向的窄带频率
	Var    []float64 // 窗口内灰度方差（振幅²/2 的估计量）
	Energy []float64 // 窗口平均梯度能量 gx²+gy²
}

// EstimateFrequency 在每个像素边长 win（奇数，自动夹成奇数）的邻域内，
// 用梯度结构张量估计局部主频。
//
// 原理：对窄带信号 I=μ+A·cos(2π(ux+vy)+φ)，令 f=hypot(u,v)，则在覆盖若干
// 相位的窗口内 Var(I)≈A²/2、mean(gx²+gy²)≈(2πf)²A²/2，两式相除得
//
//	f = sqrt(mean(gx²+gy²)/Var(I)) / (2π)。
//
// 结构张量 J=Σ[[gx²,gxgy],[gxgy,gy²]] 的特征值还给出方向相干度，用来识别
// 棋盘/交叉条纹这类双向宽带结构（相干度低），此时选层走保守上限。
func EstimateFrequency(g *Gray, win int) *FreqField {
	if win < 1 {
		win = 1
	}
	if win%2 == 0 {
		win++
	}
	gx, gy := Gradients(g)

	gxx := make([]float64, len(g.Pix))
	gyy := make([]float64, len(g.Pix))
	gxy := make([]float64, len(g.Pix))
	g2 := make([]float64, len(g.Pix))
	for i := range g.Pix {
		a, b := gx[i], gy[i]
		gxx[i] = a * a
		gyy[i] = b * b
		gxy[i] = a * b
		g2[i] = a*a + b*b
	}
	integ := buildIntegral(g.W, g.H, g.Pix, square(g.Pix), g2, gxx, gyy, gxy)

	fld := &FreqField{
		W: g.W, H: g.H, Win: win,
		F: make([]float64, len(g.Pix)), Coh: make([]float64, len(g.Pix)),
		Var: make([]float64, len(g.Pix)), Energy: make([]float64, len(g.Pix)),
	}

	const varFlat = 16.0 // 方差 <16（灰度标准差<4）视为平坦区
	for y := 0; y < g.H; y++ {
		for x := 0; x < g.W; x++ {
			i := y*g.W + x
			s, n := integ.windowSums(x, y, win)
			sumI, sumI2, sumG2, sumGxx, sumGyy, sumGxy := s[0], s[1], s[2], s[3], s[4], s[5]
			meanI := sumI / n
			vari := sumI2/n - meanI*meanI
			if vari < 0 {
				vari = 0
			}
			meanG2 := sumG2 / n

			// 结构张量相干度：(λ1-λ2)/(λ1+λ2) = disc/tr。
			a, c, b := sumGxx, sumGyy, sumGxy
			tr := a + c
			disc := math.Hypot(a-c, 2*b) // sqrt((a-c)^2+4b^2)
			coh := 0.0
			if tr > 1e-9 {
				coh = disc / tr
				if coh > 1 {
					coh = 1
				}
			}

			f := 0.0
			if vari >= varFlat {
				raw := math.Sqrt(meanG2/vari) / (2 * math.Pi)
				// 校正中心差分在高频处的幅值衰减：raw≈f*sinc(2πf)。
				// 迭代一次反解 f，并限制校正倍数避免在奈奎斯特附近发散。
				f = correctSinc(raw)
				if f > 0.5 {
					f = 0.5
				}
			}
			fld.F[i] = f
			fld.Coh[i] = coh
			fld.Var[i] = vari
			fld.Energy[i] = meanG2
		}
	}
	return fld
}

// correctSinc 由 raw=f*sinc(2πf) 反解 f，做一次牛顿/不动点校正并夹取。
func correctSinc(raw float64) float64 {
	f := raw
	for k := 0; k < 4; k++ {
		gain := SincGain(f)
		if gain < 0.25 {
			gain = 0.25 // 防止奈奎斯特附近过校正
		}
		f = raw / gain
		if f > 0.5 {
			f = 0.5
		}
	}
	return f
}

// square 返回逐元素平方的新切片。
func square(a []float64) []float64 {
	out := make([]float64, len(a))
	for i, v := range a {
		out[i] = v * v
	}
	return out
}

// MedianFrequency 返回指定矩形内主频估计的中位数（便于标定/汇报）。
func (fld *FreqField) MedianFrequency(r Rect) float64 {
	var vals []float64
	for y := r.Y0; y < r.Y1; y++ {
		for x := r.X0; x < r.X1; x++ {
			if x < 0 || y < 0 || x >= fld.W || y >= fld.H {
				continue
			}
			vals = append(vals, fld.F[y*fld.W+x])
		}
	}
	if len(vals) == 0 {
		return 0
	}
	sort.Float64s(vals)
	return vals[len(vals)/2]
}
