package post

import (
	"errors"
	"math"

	"vecgo/raster"
)

// 三级箱式模糊逼近高斯：
//
// 一维宽度为 w（奇数）的箱核 b_w 在整数点上的方差为 (w^2-1)/12。
// n 个箱核串联（可分离，先横后纵）的方差为各箱核方差之和，
// 方差匹配要求 sum_i (w_i^2-1)/12 = sigma^2。
//
// 连续域等宽反解给出 w_ideal = sqrt(12 sigma^2/n + 1)（Kovesi 取法），
// 取相邻奇数 wl/wu 并挑选使实际方差最接近 sigma^2 的组合。
// n=3 时穷举全部奇数三元组可验证：Kovesi 取法同时是离散 L1 最优三元组。
//
// 逐像素误差上界（Young 卷积不等式，任意边界处理均成立）：
//
//	|(G_sigma * f - B * f)(x)| <= sup|f| * ||G_sigma - B||_1
//
// 高斯核在 [-K,K] 处截断的尾部满足 sum_{|k|>K} G(k) <= erfc(K/(sqrt2 sigma))，
// 故 uint8 图像（sup|f|=255）的逐像素上界为
//
//	255 * (L1(级联核, 截断高斯核) + 尾部界) + 0.5（取整误差）

// GaussPlan 是一次反解的完整参数。
type GaussPlan struct {
	Sigma    float64 // 目标高斯 sigma（像素）
	N        int     // 实际采用的串联级数
	Widths   []int   // 每级箱宽（奇数），表中的取法
	WIdeal   float64 // 连续域等宽反解宽度
	WL, WU   int     // 相邻奇数档
	Variance float64 // 级联核实际方差
	L1Dist   float64 // 级联核与截断离散高斯核的 L1 距离
	Tail     float64 // 高斯截断尾部上界 erfc(...)
	TruncK   int     // 高斯核截断半径
	// Bound255 是 uint8 图像的逐像素误差上界（含 0.5 取整误差）。
	Bound255 float64
	// Budget255 是要求的逐像素误差预算；0 表示未给预算。
	Budget255 float64
	// NStar 是满足预算所需的最小级数；NStar>N 即说明当前级数不够。
	NStar int
}

// gaussKernel 构造截断并归一化的离散一维高斯核，键为整数偏移。
func gaussKernel(sigma float64, k int) map[int]float64 {
	g := make(map[int]float64, 2*k+1)
	z := 0.0
	for i := -k; i <= k; i++ {
		v := math.Exp(-float64(i*i) / (2 * sigma * sigma))
		g[i] = v
		z += v
	}
	for i := range g {
		g[i] /= z
	}
	return g
}

func boxKernel(w int) map[int]float64 {
	r := w / 2
	k := make(map[int]float64, w)
	v := 1.0 / float64(w)
	for i := -r; i <= r; i++ {
		k[i] = v
	}
	return k
}

func convolve1D(a, b map[int]float64) map[int]float64 {
	out := make(map[int]float64, len(a)*len(b))
	for ka, va := range a {
		for kb, vb := range b {
			out[ka+kb] += va * vb
		}
	}
	return out
}

// cascadeKernel 返回 n 级箱核串联后的一维离散核。
func cascadeKernel(widths []int) map[int]float64 {
	k := map[int]float64{0: 1}
	for _, w := range widths {
		k = convolve1D(k, boxKernel(w))
	}
	return k
}

// oddFloor 返回不大于 x 的最大奇数（至少为 1）。
func oddFloor(x float64) int {
	w := int(math.Floor(x))
	if w%2 == 0 {
		w--
	}
	if w < 1 {
		w = 1
	}
	return w
}

// kovesiWidths 按方差匹配反解 n 级奇数箱宽：
// m 级取 wu，其余取 wl，选择使实际方差最贴近 sigma^2 的 m。
func kovesiWidths(sigma float64, n int) (widths []int, wIdeal float64, wl, wu, mBest int) {
	wIdeal = math.Sqrt(12*sigma*sigma/float64(n) + 1)
	wl = oddFloor(wIdeal)
	wu = wl + 2
	target := sigma * sigma
	best := math.Inf(1)
	for m := 0; m <= n; m++ {
		variance := 0.0
		for i := 0; i < n; i++ {
			w := wl
			if i < m {
				w = wu
			}
			variance += float64(w*w-1) / 12
		}
		if d := math.Abs(variance - target); d < best {
			best = d
			mBest = m
		}
	}
	widths = make([]int, n)
	for i := range widths {
		if i < mBest {
			widths[i] = wu
		} else {
			widths[i] = wl
		}
	}
	return widths, wIdeal, wl, wu, mBest
}

// kernelVariance 返回以 0 为中心的离散核的二阶矩。
func kernelVariance(k map[int]float64) float64 {
	v := 0.0
	for i, p := range k {
		v += float64(i*i) * p
	}
	return v
}

// l1AndTail 计算级联核与截断高斯核的精确离散 L1 距离，
// 以及截断尾部的解析上界 erfc(K/(sqrt2 sigma))。
func l1AndTail(widths []int, sigma float64) (l1, tail float64, truncK int) {
	c := cascadeKernel(widths)
	support := 0
	for _, w := range widths {
		support += w / 2
	}
	truncK = support + 4*int(math.Ceil(sigma))
	g := gaussKernel(sigma, truncK)
	seen := make(map[int]bool, len(c)+len(g))
	for i := range c {
		seen[i] = true
	}
	for i := range g {
		seen[i] = true
	}
	for i := range seen {
		l1 += math.Abs(c[i] - g[i])
	}
	tail = math.Erfc(float64(truncK) / (math.Sqrt2 * sigma))
	return l1, tail, truncK
}

// Invert 按目标逐像素误差上界 budget255（0..255 量纲）反解箱宽与级数。
// 固定产出 n=3 的方案；若三级不满足预算，NStar 给出所需最小级数（maxN 封顶）。
func Invert(sigma float64, budget255 float64, maxN int) (*GaussPlan, error) {
	if sigma <= 0 {
		return nil, errors.New("sigma 必须为正")
	}
	if maxN < 3 {
		maxN = 3
	}
	plan := &GaussPlan{Sigma: sigma, Budget255: budget255}
	widths, wIdeal, wl, wu, _ := kovesiWidths(sigma, 3)
	fillPlan(plan, 3, widths, wIdeal, wl, wu)
	if budget255 > 0 {
		for n := 1; n <= maxN; n++ {
			ws, wi, lo, up, _ := kovesiWidths(sigma, n)
			p := &GaussPlan{Sigma: sigma}
			fillPlan(p, n, ws, wi, lo, up)
			if p.Bound255 <= budget255 {
				plan.NStar = n
				break
			}
			plan.NStar = -1 // maxN 内无解
		}
	}
	return plan, nil
}

func fillPlan(p *GaussPlan, n int, widths []int, wIdeal float64, wl, wu int) {
	p.N = n
	p.Widths = widths
	p.WIdeal = wIdeal
	p.WL, p.WU = wl, wu
	p.Variance = kernelVariance(cascadeKernel(widths))
	p.L1Dist, p.Tail, p.TruncK = l1AndTail(widths, p.Sigma)
	p.Bound255 = 255*(p.L1Dist+p.Tail) + 0.5
}

// GaussBlur3 以固定三级（先横后纵、每级两个一维 pass）箱式模糊逼近高斯。
// stages 非 nil 时依次写入每一级（横向+纵向完成后）的浮点输出，便于逐级核。
func GaussBlur3(im *raster.Image, widths [3]int, stages *[3][]float64) (*raster.Image, error) {
	for _, w := range widths {
		if w < 1 || w%2 == 0 {
			return nil, errors.New("箱宽必须为正奇数")
		}
	}
	cur := pixToFloat(im.Pix)
	for s, w := range widths {
		cur = boxPassH(cur, im.W, im.H, w)
		cur = boxPassV(cur, im.W, im.H, w)
		if stages != nil {
			(*stages)[s] = append([]float64(nil), cur...)
		}
	}
	out, _ := raster.New(im.W, im.H)
	for i, v := range cur {
		out.Pix[i] = roundUint8(v)
	}
	return out, nil
}

// GaussBlur2DNonSeparable 是对照组：同样三级箱宽，但每级做完整二维方窗卷积，
// 不做行列分离，用于实测分离带来的加速。
func GaussBlur2DNonSeparable(im *raster.Image, widths [3]int) (*raster.Image, error) {
	for _, w := range widths {
		if w < 1 || w%2 == 0 {
			return nil, errors.New("箱宽必须为正奇数")
		}
	}
	cur := pixToFloat(im.Pix)
	for _, w := range widths {
		cur = boxPass2D(cur, im.W, im.H, w)
	}
	out, _ := raster.New(im.W, im.H)
	for i, v := range cur {
		out.Pix[i] = roundUint8(v)
	}
	return out, nil
}

// GaussReference 是截断离散真高斯（可分离）参考实现，用于逐像素实测误差。
func GaussReference(im *raster.Image, sigma float64, truncK int) *raster.Image {
	g := gaussKernel(sigma, truncK)
	taps := make([]struct {
		off int
		w   float64
	}, 0, len(g))
	for i, v := range g {
		taps = append(taps, struct {
			off int
			w   float64
		}{i, v})
	}
	cur := pixToFloat(im.Pix)
	cur = separableTaps(cur, im.W, im.H, taps, true)
	cur = separableTaps(cur, im.W, im.H, taps, false)
	out, _ := raster.New(im.W, im.H)
	for i, v := range cur {
		out.Pix[i] = roundUint8(v)
	}
	return out
}

// MaxAbsDiff 返回两幅同尺寸图像的最大逐像素差与平均绝对差。
func MaxAbsDiff(a, b *raster.Image) (maxD int, mean float64) {
	sum := 0
	for i := range a.Pix {
		d := int(a.Pix[i]) - int(b.Pix[i])
		if d < 0 {
			d = -d
		}
		if d > maxD {
			maxD = d
		}
		sum += d
	}
	return maxD, float64(sum) / float64(len(a.Pix))
}

func pixToFloat(pix []uint8) []float64 {
	out := make([]float64, len(pix))
	for i, v := range pix {
		out[i] = float64(v)
	}
	return out
}

func roundUint8(v float64) uint8 {
	r := math.Round(v)
	if r < 0 {
		return 0
	}
	if r > 255 {
		return 255
	}
	return uint8(r)
}

func clamp(i, n int) int {
	if i < 0 {
		return 0
	}
	if i >= n {
		return n - 1
	}
	return i
}

// boxPassH 横向一维箱核（clamp-to-edge），滑动求和，O(WH)。
func boxPassH(src []float64, w, h, bw int) []float64 {
	r := bw / 2
	dst := make([]float64, len(src))
	for y := 0; y < h; y++ {
		row := y * w
		sum := 0.0
		for x := -r; x <= r; x++ {
			sum += src[row+clamp(x, w)]
		}
		for x := 0; x < w; x++ {
			dst[row+x] = sum / float64(bw)
			lo := clamp(x-r, w)
			hi := clamp(x+r+1, w)
			sum += src[row+hi] - src[row+lo]
		}
	}
	return dst
}

// boxPassV 纵向一维箱核（clamp-to-edge），滑动求和，O(WH)。
func boxPassV(src []float64, w, h, bw int) []float64 {
	r := bw / 2
	dst := make([]float64, len(src))
	for x := 0; x < w; x++ {
		sum := 0.0
		for y := -r; y <= r; y++ {
			sum += src[clamp(y, h)*w+x]
		}
		for y := 0; y < h; y++ {
			dst[y*w+x] = sum / float64(bw)
			lo := clamp(y-r, h) * w
			hi := clamp(y+r+1, h) * w
			sum += src[hi+x] - src[lo+x]
		}
	}
	return dst
}

// boxPass2D 不分离的二维方窗卷积，O(WH*w^2)。
func boxPass2D(src []float64, w, h, bw int) []float64 {
	r := bw / 2
	dst := make([]float64, len(src))
	norm := float64(bw * bw)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			sum := 0.0
			for dy := -r; dy <= r; dy++ {
				row := clamp(y+dy, h) * w
				for dx := -r; dx <= r; dx++ {
					sum += src[row+clamp(x+dx, w)]
				}
			}
			dst[y*w+x] = sum / norm
		}
	}
	return dst
}

func separableTaps(src []float64, w, h int, taps []struct {
	off int
	w   float64
}, horizontal bool) []float64 {
	dst := make([]float64, len(src))
	if horizontal {
		for y := 0; y < h; y++ {
			row := y * w
			for x := 0; x < w; x++ {
				sum := 0.0
				for _, t := range taps {
					sum += t.w * src[row+clamp(x+t.off, w)]
				}
				dst[row+x] = sum
			}
		}
	} else {
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				sum := 0.0
				for _, t := range taps {
					sum += t.w * src[clamp(y+t.off, h)*w+x]
				}
				dst[y*w+x] = sum
			}
		}
	}
	return dst
}
