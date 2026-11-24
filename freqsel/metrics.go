package freqsel

import "math"

// GradientEnergy 计算整图梯度能量 E=Σ_{y,x}(gx²+gy²)（中心差分），
// 同时返回按像素平均的能量，便于不同画幅横向比较。明确按整图统计，不取局部 ROI。
func GradientEnergy(g *Gray) (total, mean float64) {
	for y := 0; y < g.H; y++ {
		for x := 0; x < g.W; x++ {
			gx := (g.At(x+1, y) - g.At(x-1, y)) * 0.5
			gy := (g.At(x, y+1) - g.At(x, y-1)) * 0.5
			total += gx*gx + gy*gy
		}
	}
	return total, total / float64(g.W*g.H)
}

// RingingReport 是强阶跃边缘旁“过冲/下冲”振铃量的统计。
type RingingReport struct {
	MeanOvershoot float64 // 每条边缘剖面的平均过冲+下冲幅度（0..255 灰度级）
	MaxOvershoot  float64 // 单条剖面最大值
	MeanContrast  float64 // 纳入统计剖面的平均两端反差
	Profiles      int     // 有效强边缘剖面数
	NormalizedPct float64 // MeanOvershoot/MeanContrast*100%
}

// RingingIndex 在给定输出图矩形内寻找强阶跃边缘并量测其法向剖面的过冲/下冲。
// horizontal=false：边缘近似竖直（沿 x 扫描剖面，逐行统计）；
// horizontal=true ：边缘近似水平（沿 y 扫描剖面，逐列统计）。
//
// 高斯/层间线性插值的核冲激响应非负，阶跃响应单调，理论过冲为 0；
// 锐化或理想低通会在平台外侧产生超出两端平台的峰/谷，被该指标捕获。
func RingingIndex(g *Gray, r Rect, horizontal bool) RingingReport {
	const reach = 4 // 剖面半长（目标像素）
	var rep RingingReport
	contrastSum := 0.0

	scan := func(fixed int) {
		// 在搜索带内找梯度最大的边缘位置 e。
		e, best := -1, -1.0
		if !horizontal {
			for x := r.X0 + 2; x < r.X1-2; x++ {
				d := math.Abs(g.At(x+1, fixed) - g.At(x-1, fixed))
				if d > best {
					best, e = d, x
				}
			}
		} else {
			for y := r.Y0 + 2; y < r.Y1-2; y++ {
				d := math.Abs(g.At(fixed, y+1) - g.At(fixed, y-1))
				if d > best {
					best, e = d, y
				}
			}
		}
		if e < 0 || e-reach < 0 || e+reach >= dim(g, horizontal) {
			return
		}
		at := func(d int) float64 {
			if !horizontal {
				return g.At(e+d, fixed)
			}
			return g.At(fixed, e+d)
		}
		leftMean := (at(-reach) + at(-reach+1) + at(-reach+2)) / 3
		rightMean := (at(reach-2) + at(reach-1) + at(reach)) / 3
		contrast := math.Abs(rightMean - leftMean)
		if contrast < 40 { // 只统计高对比阶跃，避免纹理/噪声污染
			return
		}
		// 以剖面两端最外侧三点的极值作为“终端平台”参考。对任意单调阶跃响应
		// （高斯/层间插值正是如此），全剖面极值就在终端，over/under 恒为 0；
		// 只有超出终端值的非单调峰/谷（真正的振铃瓣）才会被计入。
		leftRefLo := math.Min(math.Min(at(-reach), at(-reach+1)), at(-reach+2))
		rightRefHi := math.Max(math.Max(at(reach-2), at(reach-1)), at(reach))
		var over float64
		if leftMean <= rightMean { // 上升沿：左端为低平台，右端为高平台
			loRef := leftRefLo
			hiRef := rightRefHi
			mn, mx := math.MaxFloat64, -math.MaxFloat64
			for d := -reach; d <= 0; d++ {
				mn = math.Min(mn, at(d))
			}
			for d := 0; d <= reach; d++ {
				mx = math.Max(mx, at(d))
			}
			over = math.Max(0, loRef-mn) + math.Max(0, mx-hiRef)
		} else { // 下降沿：左端高、右端低
			hiRef := math.Max(math.Max(at(-reach), at(-reach+1)), at(-reach+2))
			loRef := math.Min(at(reach-2), math.Min(at(reach-1), at(reach)))
			mx, mn := -math.MaxFloat64, math.MaxFloat64
			for d := -reach; d <= 0; d++ {
				mx = math.Max(mx, at(d))
			}
			for d := 0; d <= reach; d++ {
				mn = math.Min(mn, at(d))
			}
			over = math.Max(0, mx-hiRef) + math.Max(0, loRef-mn)
		}
		rep.Profiles++
		rep.MeanOvershoot += over
		contrastSum += contrast
		if over > rep.MaxOvershoot {
			rep.MaxOvershoot = over
		}
	}

	if !horizontal {
		for y := r.Y0 + 2; y < r.Y1-2; y++ {
			scan(y)
		}
	} else {
		for x := r.X0 + 2; x < r.X1-2; x++ {
			scan(x)
		}
	}
	if rep.Profiles > 0 {
		rep.MeanOvershoot /= float64(rep.Profiles)
		rep.MeanContrast = contrastSum / float64(rep.Profiles)
		if rep.MeanContrast > 1e-9 {
			rep.NormalizedPct = 100 * rep.MeanOvershoot / rep.MeanContrast
		}
	}
	return rep
}

// dim 返回沿剖面方向的图像长度：竖直边缘沿 x（W），水平边缘沿 y（H）。
func dim(g *Gray, horizontal bool) int {
	if horizontal {
		return g.H
	}
	return g.W
}

// UnsharpControl 生成“靠锐化硬提梯度”的阳性对照：out=g+amount*(g-Gaussian(g,sigma))。
// 返回未夹取的浮点结果，使过冲/振铃可被 RingingIndex 如实量到；存图时再夹取。
func UnsharpControl(g *Gray, sigma, amount float64) *Gray {
	blur := GaussianBlur(g, sigma)
	out := MustGray(g.W, g.H)
	for i := range g.Pix {
		out.Pix[i] = g.Pix[i] + amount*(g.Pix[i]-blur.Pix[i])
	}
	return out
}

// ScaleRect 把源像素矩形按 scale 映到目标像素矩形（边界向下/向上取整并夹取）。
func ScaleRect(r Rect, scale float64, ow, oh int) Rect {
	sr := Rect{
		X0: int(math.Floor(float64(r.X0) * scale)),
		Y0: int(math.Floor(float64(r.Y0) * scale)),
		X1: int(math.Ceil(float64(r.X1) * scale)),
		Y1: int(math.Ceil(float64(r.Y1) * scale)),
	}
	if sr.X0 < 0 {
		sr.X0 = 0
	}
	if sr.Y0 < 0 {
		sr.Y0 = 0
	}
	if sr.X1 > ow {
		sr.X1 = ow
	}
	if sr.Y1 > oh {
		sr.Y1 = oh
	}
	return sr
}
