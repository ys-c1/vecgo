package shadow

import (
	"errors"
	"math"
	"sort"

	"vecgo/raster"
)

// Profile 是沿 x 方向平均出的灰度剖面；X[i] 为第 i 列的像素中心坐标。
type Profile struct {
	X []float64
	G []float64
}

// ColumnProfile 把 y∈[y0,y1) 的行按列平均，得到一维灰度剖面。
// y1<=0 表示取到图像底边。多行平均可压低 8 bit 量化与采样抖动。
func ColumnProfile(im *raster.Image, y0, y1 int) (Profile, error) {
	if im == nil {
		return Profile{}, errors.New("shadow: nil 图像")
	}
	if y0 < 0 {
		y0 = 0
	}
	if y1 <= 0 || y1 > im.H {
		y1 = im.H
	}
	if y0 >= y1 {
		return Profile{}, errors.New("shadow: 行区间为空")
	}
	rows := float64(y1 - y0)
	x := make([]float64, im.W)
	g := make([]float64, im.W)
	for c := 0; c < im.W; c++ {
		sum := 0.0
		for r := y0; r < y1; r++ {
			sum += float64(im.Pix[r*im.W+c])
		}
		x[c] = float64(c) + 0.5
		g[c] = sum / rows
	}
	return Profile{X: x, G: g}, nil
}

func (p Profile) sampleAt(x float64) float64 {
	n := len(p.X)
	if n == 1 {
		return p.G[0]
	}
	if x <= p.X[0] {
		return p.G[0]
	}
	if x >= p.X[n-1] {
		return p.G[n-1]
	}
	i := int(math.Floor(x - 0.5)) // X[i] = i+0.5
	if i < 0 {
		i = 0
	}
	if i >= n-1 {
		i = n - 2
	}
	t := (x - p.X[i]) / (p.X[i+1] - p.X[i])
	return p.G[i] + t*(p.G[i+1]-p.G[i])
}

// crossing 返回灰度 g 等于 level 的亚像素 x（要求剖面在该处单调跨越）。
// startCol 给出从哪一列附近寻找，取距离最近的跨越。
func (p Profile) crossing(level, approxX, maxDist float64) (float64, bool) {
	best := math.Inf(1)
	bestX := 0.0
	found := false
	for i := 0; i+1 < len(p.X); i++ {
		a, b := p.G[i]-level, p.G[i+1]-level
		if a == 0 {
			if d := math.Abs(p.X[i] - approxX); d < best && d <= maxDist {
				best, bestX, found = d, p.X[i], true
			}
			continue
		}
		if a*b < 0 {
			t := a / (a - b) // 线性插值过零点
			x := p.X[i] + t*(p.X[i+1]-p.X[i])
			if d := math.Abs(x - approxX); d < best && d <= maxDist {
				best, bestX, found = d, x, true
			}
		}
	}
	return bestX, found
}

func percentile(vals []float64, q float64) float64 {
	cp := append([]float64(nil), vals...)
	sort.Float64s(cp)
	if len(cp) == 1 {
		return cp[0]
	}
	pos := q * float64(len(cp)-1)
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))
	if lo == hi {
		return cp[lo]
	}
	t := pos - float64(lo)
	return cp[lo]*(1-t) + cp[hi]*t
}

// EdgeMeasure 是一条影边的半影测量结果。
type EdgeMeasure struct {
	// XHalfGray 是“灰度过半”位置的亚像素 x（明暗平台的中点）。
	XHalfGray float64 `json:"x_half_gray_px"`
	// GrayDark/GrayLit 是暗、亮平台灰度（0..255）。
	GrayDark float64 `json:"gray_dark"`
	GrayLit  float64 `json:"gray_lit"`
	// SlopeAtHalf 是在半灰位置中心线性段拟合出的灰度斜率（灰度/像素）。
	SlopeAtHalf float64 `json:"slope_at_half"`
	// WidthHalfGray 是主测量值：以灰度过半位置为中心求切线，
	// 再把切线外推到整个明暗灰度差得到的半影宽（像素）。
	WidthHalfGray float64 `json:"penumbra_width_meas_px"`
	// WidthTanHalfPx 是半灰点 ±0.5 px 中心差分切线给出的宽度（对照）。
	WidthTanHalfPx float64 `json:"width_tan_halfpx_px"`
	// X10/X90/X25/X75 是各灰度分位的亚像素位置。
	X10 float64 `json:"x10_px"`
	X90 float64 `json:"x90_px"`
	X25 float64 `json:"x25_px"`
	X75 float64 `json:"x75_px"`
	// Width1090 / Width2575 是分位跨度按线性斜坡折算的总宽（对照）。
	Width1090 float64 `json:"width_10_90_px"`
	Width2575 float64 `json:"width_25_75_px"`
}

// MeasureEdge 在 approxX 附近（搜索半径 searchPx）测量一条影边。
//
// “半影宽度按灰度过半的位置量”的具体操作定义：
//  1. 用剖面的低/高分位确定暗、亮平台，取中点灰度 g50；
//  2. 线性插值找到 g=g50 的亚像素位置 x50；
//  3. 取灰度落在 20%~80% 之间、以 x50 为中心的线性段做最小二乘切线 m；
//  4. 半影宽 = (g_lit-g_dark)/|m|，即切线从暗平台走到亮平台的水平跨度。
//
// 理想面光源（均匀线/方形光源）直边的灰度剖面是线性斜坡，
// 因此该宽度与相似三角形预测的 w 严格一致。
func MeasureEdge(p Profile, approxX, searchPx float64) (EdgeMeasure, error) {
	if len(p.X) < 2 {
		return EdgeMeasure{}, errors.New("shadow: 剖面至少需要两列")
	}
	dark := percentile(p.G, 0.02)
	lit := percentile(p.G, 0.98)
	delta := lit - dark
	if delta < 8 {
		return EdgeMeasure{}, errors.New("shadow: 明暗灰度差过小，无法测边")
	}
	g50 := dark + delta/2
	x50, ok := p.crossing(g50, approxX, searchPx)
	if !ok {
		return EdgeMeasure{}, errors.New("shadow: 在给定位置附近找不到灰度过半点")
	}

	// 半灰点 ±0.5 px 的中心差分（最贴近“就在过半位置量”的定义）。
	gm := p.sampleAt(x50 - 0.5)
	gp := p.sampleAt(x50 + 0.5)
	mHalf := gp - gm // 分母 1 px
	wHalfTan := delta / math.Max(math.Abs(mHalf), 1e-9)

	m := fitCentralSlope(p, x50, searchPx, dark, delta, mHalf)
	w := delta / math.Abs(m)

	frac := func(f float64) float64 {
		x, _ := p.crossing(dark+delta*f, x50, searchPx)
		return x
	}
	x10, x90 := frac(0.10), frac(0.90)
	x25, x75 := frac(0.25), frac(0.75)

	return EdgeMeasure{
		XHalfGray:      x50,
		GrayDark:       dark,
		GrayLit:        lit,
		SlopeAtHalf:    m,
		WidthHalfGray:  w,
		WidthTanHalfPx: wHalfTan,
		X10:            x10,
		X90:            x90,
		X25:            x25,
		X75:            x75,
		Width1090:      (x90 - x10) / 0.80,
		Width2575:      (x75 - x25) / 0.50,
	}, nil
}

// fitCentralSlope 在 x50 附近（半径 radiusPx，确保只取本条边）挑选灰度处于
// 20%~80% 的中心线性段列做 OLS：g = m x + b。
// 点光源硬边没有中间列时，退化为给定的半灰点中心差分 mHalf。
func fitCentralSlope(p Profile, x50, radiusPx, dark, delta, mHalf float64) float64 {
	lo := dark + 0.20*delta
	hi := dark + 0.80*delta
	var sx, sy, sxx, sxy, n float64
	for i, x := range p.X {
		if math.Abs(x-x50) > radiusPx {
			continue
		}
		g := p.G[i]
		if g < lo || g > hi {
			continue
		}
		n++
		sx += x
		sy += g
		sxx += x * x
		sxy += x * g
	}
	if n >= 2 {
		den := n*sxx - sx*sx
		if math.Abs(den) > 1e-12 {
			m := (n*sxy - sx*sy) / den
			if math.Abs(m) > 1e-9 {
				return m
			}
		}
	}
	return mHalf
}

// HalfGraySpan 返回两条影边半灰位置之间的距离（像素）。
// 对有限遮挡物，它是“半灰轮廓宽”，其解析值是中心投影宽度，
// 与半影过渡宽不是同一个几何量，仅作对照列出。
func HalfGraySpan(a, b EdgeMeasure) float64 {
	return math.Abs(a.XHalfGray - b.XHalfGray)
}
