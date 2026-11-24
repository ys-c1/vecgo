package post

import (
	"errors"
	"math"

	"vecgo/raster"
)

// sRGB 电光转换曲线（线性 -> sRGB 编码），定义域 [0,1]。
// 0.0031308 以下为线性段（斜率 12.92），以上为 1/2.4 次幂区。
const srgbToe = 0.0031308

func srgbEncode(x float64) float64 {
	if x <= 0 {
		return 0
	}
	if x <= srgbToe {
		return 12.92 * x
	}
	return 1.055*math.Pow(x, 1.0/2.4) - 0.055
}

// SRGBEncode 是精确的 sRGB 电光转换（线性 -> sRGB 编码），供外部对照使用。
func SRGBEncode(x float64) float64 { return srgbEncode(x) }

// chordErr 返回 sRGB 曲线在 [a,b] 上与连接两端点的弦之间的最大偏差。
// 幂区内 f 为凹函数，偏差峰值在 f'(x) = 弦斜率 处；
// 线性段内偏差关于 x 线性，峰值只可能出现在折点 srgbToe。
func chordErr(a, b float64) float64 {
	fa, fb := srgbEncode(a), srgbEncode(b)
	m := (fb - fa) / (b - a)
	dev := func(x float64) float64 {
		return srgbEncode(x) - (fa + m*(x-a))
	}
	best := 0.0
	if srgbToe > a && srgbToe < b {
		if d := dev(srgbToe); d > best {
			best = d
		}
	}
	if b > srgbToe && m > 0 {
		// 幂区 f'(x) = 1.055*(5/12)*x^(-7/12) = m
		// 解得驻点 x* = (1.055*(5/12)/m)^(12/7)
		x := math.Pow(1.055*(5.0/12.0)/m, 12.0/7.0)
		if x < srgbToe {
			x = srgbToe
		}
		if x < a {
			x = a
		}
		if x > b {
			x = b
		}
		if d := dev(x); d > best {
			best = d
		}
	}
	return best
}

// SolveSRGBBreakpoints 贪心反解分段线性逼近的断点：
// 每段从 a 出发，二分找到使弦偏差不超过 eps 的最大 b。
// 返回含 0 和 1 的断点序列，段数 = len-1。
func SolveSRGBBreakpoints(eps float64) ([]float64, error) {
	if eps <= 0 {
		return nil, errors.New("eps 必须为正")
	}
	bps := []float64{0}
	a := 0.0
	for a < 1 {
		if chordErr(a, 1) <= eps {
			bps = append(bps, 1)
			break
		}
		lo, hi := a, 1.0
		for i := 0; i < 80; i++ {
			mid := (lo + hi) / 2
			if chordErr(a, mid) <= eps {
				lo = mid
			} else {
				hi = mid
			}
		}
		if lo <= a { // 不应发生：b->a 时偏差趋于 0
			return nil, errors.New("断点求解不收敛")
		}
		bps = append(bps, lo)
		a = lo
	}
	return bps, nil
}

// SRGBPiecewise 是 sRGB 曲线的分段线性逼近。
type SRGBPiecewise struct {
	Xs, Ys []float64
}

func NewSRGBPiecewise(eps float64) (*SRGBPiecewise, error) {
	xs, err := SolveSRGBBreakpoints(eps)
	if err != nil {
		return nil, err
	}
	ys := make([]float64, len(xs))
	for i, x := range xs {
		ys[i] = srgbEncode(x)
	}
	return &SRGBPiecewise{Xs: xs, Ys: ys}, nil
}

// At 求分段线性逼近在 x 处的值，x 截断到 [0,1]。
func (p *SRGBPiecewise) At(x float64) float64 {
	if x <= 0 {
		return p.Ys[0]
	}
	if x >= 1 {
		return p.Ys[len(p.Ys)-1]
	}
	lo, hi := 0, len(p.Xs)-1
	for hi-lo > 1 {
		mid := (lo + hi) / 2
		if p.Xs[mid] <= x {
			lo = mid
		} else {
			hi = mid
		}
	}
	t := (x - p.Xs[lo]) / (p.Xs[hi] - p.Xs[lo])
	return p.Ys[lo] + t*(p.Ys[hi]-p.Ys[lo])
}

// MaxDev 在稠密采样上测量逼近与精确曲线的最大偏差（用于出报告）。
func (p *SRGBPiecewise) MaxDev(samples int) (x, dev float64) {
	for i := 0; i <= samples; i++ {
		xi := float64(i) / float64(samples)
		if d := math.Abs(p.At(xi) - srgbEncode(xi)); d > dev {
			dev, x = d, xi
		}
	}
	return x, dev
}

// SRGBTable 精确 sRGB 编码的 8-bit 查找表（四舍五入量化）。
func SRGBTable() [256]uint8 {
	var t [256]uint8
	for i := 0; i < 256; i++ {
		t[i] = uint8(255*srgbEncode(float64(i)/255) + 0.5)
	}
	return t
}

// Table 分段线性逼近的 8-bit 查找表（同一量化方式，便于逐位对比）。
func (p *SRGBPiecewise) Table() [256]uint8 {
	var t [256]uint8
	for i := 0; i < 256; i++ {
		t[i] = uint8(255*p.At(float64(i)/255) + 0.5)
	}
	return t
}

// ApplyTable 用查找表重映射全图像素。
func ApplyTable(im *raster.Image, table [256]uint8) {
	for i := range im.Pix {
		im.Pix[i] = table[im.Pix[i]]
	}
}
