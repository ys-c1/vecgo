package freqsel

import "math"

// SelectOptions 控制局部频率到层号/核宽的反解策略。
type SelectOptions struct {
	// MinRatio 是局部核宽相对“缩放比奈奎斯特核宽”σN=Sigma0/scale 的最小比例，
	// 构成处处生效的抗锯齿/边缘过渡下限（约 0.7 时硬边 10–90 过渡宽仍 ≥D）。
	MinRatio float64
	// CritFrac 把达到满核宽的临界频率设为 CritFrac*fN（略低于奈奎斯特，保守）。
	CritFrac float64
	// CohMin 低于该相干度且确有方差时判为双向/宽带结构，强制走满核宽。
	CohMin float64
	// Smooth 是对连续层坐标 τ 做盒式正则化的半径（源像素），抑制选层椒盐与接缝。
	Smooth int
}

// DefaultOptions 返回稳健的默认反解参数。
func DefaultOptions() SelectOptions {
	return SelectOptions{MinRatio: 0.70, CritFrac: 0.85, CohMin: 0.35, Smooth: 1}
}

// Selector 保存一次“某缩放比 + 某估计窗口”下的逐像素选层结果（源分辨率）。
type Selector struct {
	Scale       float64
	D           float64 // 1/scale：每个目标像素对应的源像素步长
	FN          float64 // 输出奈奎斯特频率（周期/源像素）= scale/2
	SigmaN      float64 // 仅按缩放比的连续理想核宽 Sigma0*D
	TauN        float64 // log2 D
	GlobalLevel int     // ceil(log2 D)：全局整数层方案固定选择的层
	GlobalSigma float64 // SigmaLevel(GlobalLevel)：全局方案实际核宽
	Opts        SelectOptions
	Field       *FreqField
	Tau         []float64 // 逐源像素连续层坐标（正则化后）
	Level       []int     // 逐源像素名义整数层号 round(Tau)
	Sigma       []float64 // 逐源像素核宽 Sigma0*2^Tau
}

// NewSelector 依据频率场与缩放比反解每像素层号与核宽。
func NewSelector(fld *FreqField, scale float64, opt SelectOptions) *Selector {
	if opt.MinRatio <= 0 {
		opt = DefaultOptions()
	}
	D := 1.0 / scale
	fN := scale / 2.0
	tauN := math.Log2(D)
	globalLevel := int(math.Ceil(tauN - 1e-9))
	if globalLevel < 0 {
		globalLevel = 0
	}
	sigmaN := Sigma0 * D

	sel := &Selector{
		Scale: scale, D: D, FN: fN, SigmaN: sigmaN, TauN: tauN,
		GlobalLevel: globalLevel, GlobalSigma: SigmaLevel(globalLevel),
		Opts: opt, Field: fld,
		Tau: make([]float64, fld.W*fld.H), Level: make([]int, fld.W*fld.H),
		Sigma: make([]float64, fld.W*fld.H),
	}

	tauRaw := make([]float64, len(sel.Tau))
	fCrit := opt.CritFrac * fN
	for i := range fld.F {
		f := fld.F[i]
		r := f / fCrit
		if r < opt.MinRatio {
			r = opt.MinRatio
		}
		if r > 1 {
			r = 1
		}
		// 低相干 + 有真实反差 => 棋盘/交叉/噪点等宽带信号，单一主频不可信，
		// 用满 σN 做保守抗混叠，避免欠滤波产生伪影。
		if fld.Var[i] >= 16 && fld.Coh[i] < opt.CohMin {
			r = 1
		}
		tau := tauN + math.Log2(r) // = log2(D*r)，r∈[MinRatio,1]
		if tau < 0 {
			tau = 0
		}
		tauRaw[i] = tau
	}

	sel.Tau = boxSmoothField(tauRaw, fld.W, fld.H, opt.Smooth)
	maxL := float64(globalLevel + 1) // 允许名义层比全局高 1 的安全余量（实际不会超过 tauN）
	for i, tau := range sel.Tau {
		if tau > maxL {
			tau = maxL
			sel.Tau[i] = tau
		}
		sel.Sigma[i] = Sigma0 * math.Exp2(tau)
		l := int(math.Round(tau))
		if l < 0 {
			l = 0
		}
		sel.Level[i] = l
	}
	return sel
}

// boxSmoothField 对二维标量场做夹取边界的盒式均值；radius<=0 时原样返回。
func boxSmoothField(a []float64, w, h, radius int) []float64 {
	if radius <= 0 {
		out := make([]float64, len(a))
		copy(out, a)
		return out
	}
	out := make([]float64, len(a))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			sum, n := 0.0, 0.0
			for dy := -radius; dy <= radius; dy++ {
				yy := y + dy
				if yy < 0 || yy >= h {
					continue
				}
				for dx := -radius; dx <= radius; dx++ {
					xx := x + dx
					if xx < 0 || xx >= w {
						continue
					}
					sum += a[yy*w+xx]
					n++
				}
			}
			out[y*w+x] = sum / n
		}
	}
	return out
}

// sampleFieldBilinear 双线性采样一个与源图同分辨率的标量场。
func sampleFieldBilinear(a []float64, w, h int, sx, sy float64) float64 {
	x0 := int(math.Floor(sx))
	y0 := int(math.Floor(sy))
	fx := sx - float64(x0)
	fy := sy - float64(y0)
	get := func(x, y int) float64 {
		if x < 0 {
			x = 0
		} else if x >= w {
			x = w - 1
		}
		if y < 0 {
			y = 0
		} else if y >= h {
			y = h - 1
		}
		return a[y*w+x]
	}
	v00 := get(x0, y0)
	v10 := get(x0+1, y0)
	v01 := get(x0, y0+1)
	v11 := get(x0+1, y0+1)
	return (v00*(1-fx)+v10*fx)*(1-fy) + (v01*(1-fx)+v11*fx)*fy
}

// DownscaleGlobal 是对照方案：整图按缩放比固定选 GlobalLevel 一个整数层，
// 不做层间插值，仅在层内双线性空间采样。
func DownscaleGlobal(st *Stack, scale float64) *Gray {
	ow := SourceToOutputSize(st.Layers[0].W, scale)
	oh := SourceToOutputSize(st.Layers[0].H, scale)
	out := MustGray(ow, oh)
	lvl := int(math.Ceil(math.Log2(1/scale) - 1e-9))
	if lvl < 0 {
		lvl = 0
	}
	if lvl > st.MaxLevel() {
		lvl = st.MaxLevel()
	}
	for oy := 0; oy < oh; oy++ {
		for ox := 0; ox < ow; ox++ {
			sx, sy := OutputToSource(ox, oy, scale)
			out.Pix[oy*ow+ox] = st.sampleBilinear(lvl, sx, sy)
		}
	}
	return out
}

// GlobalTau 返回仅按缩放比反解的连续理想层坐标 τN=log2(1/scale) 与核宽 Sigma0/scale。
func GlobalTau(scale float64) (tau, sigma float64) {
	D := 1 / scale
	return math.Log2(D), Sigma0 * D
}

// DownscaleGlobalTrilinear 是更强的全局对照：所有目标像素共用同一个连续层坐标
// τN，并在相邻整数层间做全局固定权重插值（标准三线性 mip 的层间混合）。
// 它消除了“向上取整到 2 的幂”的过模糊，但仍不做任何空间自适应。
func DownscaleGlobalTrilinear(st *Stack, scale float64) *Gray {
	tau, _ := GlobalTau(scale)
	ow := SourceToOutputSize(st.Layers[0].W, scale)
	oh := SourceToOutputSize(st.Layers[0].H, scale)
	out := MustGray(ow, oh)
	for oy := 0; oy < oh; oy++ {
		for ox := 0; ox < ow; ox++ {
			sx, sy := OutputToSource(ox, oy, scale)
			out.Pix[oy*ow+ox] = st.SampleContinuous(tau, sx, sy)
		}
	}
	return out
}

// AdaptiveResult 携带自适应缩小图以及逐目标像素的选层量，供均值/直方图统计。
type AdaptiveResult struct {
	Out            *Gray
	Tau            []float64
	Level          []int
	Sigma          []float64
	LevelHistogram []float64 // 名义层号占比（长度到最大层+1）
	MeanTau        float64
	MeanLevel      float64
	MeanSigma      float64
	BelowGlobal    float64 // 名义层号低于全局层的目标像素比例
}

// DownscaleAdaptive 逐目标像素双线性取局部连续层坐标 τ，在相邻层间按小数权重
// 插值后采样，得到自适应缩小图，并汇总平均层号/核宽与层号直方图。
func DownscaleAdaptive(st *Stack, sel *Selector) *AdaptiveResult {
	scale := sel.Scale
	ow := SourceToOutputSize(sel.Field.W, scale)
	oh := SourceToOutputSize(sel.Field.H, scale)
	out := MustGray(ow, oh)
	res := &AdaptiveResult{
		Out: out, Tau: make([]float64, ow*oh), Level: make([]int, ow*oh),
		Sigma: make([]float64, ow*oh),
	}
	w, h := sel.Field.W, sel.Field.H
	histMax := sel.GlobalLevel + 2
	hist := make([]int, histMax+1)
	var sumTau, sumLevel, sumSigma float64
	below := 0

	for oy := 0; oy < oh; oy++ {
		for ox := 0; ox < ow; ox++ {
			k := oy*ow + ox
			sx, sy := OutputToSource(ox, oy, scale)
			tau := sampleFieldBilinear(sel.Tau, w, h, sx, sy)
			v := st.SampleContinuous(tau, sx, sy)
			out.Pix[k] = v
			res.Tau[k] = tau
			l := int(math.Round(tau))
			if l < 0 {
				l = 0
			}
			res.Level[k] = l
			sig := Sigma0 * math.Exp2(tau)
			res.Sigma[k] = sig

			histIdx := l
			if histIdx >= len(hist) {
				extra := make([]int, histIdx+1)
				copy(extra, hist)
				hist = extra
			}
			hist[histIdx]++
			sumTau += tau
			sumLevel += float64(l)
			sumSigma += sig
			if l < sel.GlobalLevel {
				below++
			}
		}
	}
	n := float64(ow * oh)
	res.MeanTau = sumTau / n
	res.MeanLevel = sumLevel / n
	res.MeanSigma = sumSigma / n
	res.BelowGlobal = float64(below) / n
	maxIdx := 0
	for i, c := range hist {
		if c > 0 {
			maxIdx = i
		}
	}
	res.LevelHistogram = make([]float64, maxIdx+1)
	for i := 0; i <= maxIdx; i++ {
		res.LevelHistogram[i] = float64(hist[i]) / n
	}
	return res
}
