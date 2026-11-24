// Package texture 提供贴图采样与越界时的衰减带权外插。
//
// 传统做法在采样坐标越过 [0,W-1]×[0,H-1] 的纹素中心范围时直接截断到边界
// (clamp-to-edge)，边界上的同一行/列纹素被无限拉伸到整片越界区域。
// 本包按“越界距离”构造一个随越界程度单调衰减的权重，并以该权重把外插
// 结果与基线底色混合：越界越远，外插贡献越低。
package texture

import "math"

// RGB 是归一化到 [0,1] 的一个颜色（本包只做数值线性插值，不做 gamma 校正）。
type RGB struct {
	R, G, B float64
}

// Add 逐通道相加。
func (c RGB) Add(o RGB) RGB { return RGB{R: c.R + o.R, G: c.G + o.G, B: c.B + o.B} }

// Sub 逐通道相减。
func (c RGB) Sub(o RGB) RGB { return RGB{R: c.R - o.R, G: c.G - o.G, B: c.B - o.B} }

// Scale 逐通道乘标量。
func (c RGB) Scale(a float64) RGB { return RGB{R: c.R * a, G: c.G * a, B: c.B * a} }

// Clip 把各通道夹到 [0,1]。
func (c RGB) Clip() RGB {
	return RGB{R: clip01(c.R), G: clip01(c.G), B: clip01(c.B)}
}

func clip01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// DiffEuclid255 返回两个归一化颜色在 0..255 RGB 空间中的欧氏色差。
func DiffEuclid255(a, b RGB) float64 {
	dr := (a.R - b.R) * 255
	dg := (a.G - b.G) * 255
	db := (a.B - b.B) * 255
	return math.Sqrt(dr*dr + dg*dg + db*db)
}

// DiffMAE255 返回三通道绝对差的平均值（0..255）。
func DiffMAE255(a, b RGB) float64 {
	return (math.Abs(a.R-b.R) + math.Abs(a.G-b.G) + math.Abs(a.B-b.B)) / 3 * 255
}

// Tex 是一张行优先存储的 RGB 贴图。
type Tex struct {
	W, H int
	Pix  []RGB
}

// New 创建 w×h 的空贴图。
func New(w, h int) (*Tex, error) {
	if w <= 0 || h <= 0 {
		return nil, errBadSize
	}
	return &Tex{W: w, H: h, Pix: make([]RGB, w*h)}, nil
}

func (t *Tex) at(x, y int) RGB {
	switch {
	case x < 0:
		x = 0
	case x >= t.W:
		x = t.W - 1
	}
	switch {
	case y < 0:
		y = 0
	case y >= t.H:
		y = t.H - 1
	}
	return t.Pix[y*t.W+x]
}

// TexelCoord 把归一化 UV 坐标换算为连续纹素中心坐标：
// 纹素 i 的中心恰好在 x=i，即 x = u*W - 0.5。
func TexelCoord(u float64, size int) float64 {
	return u*float64(size) - 0.5
}

// AxisOvershoot 返回坐标 x 越过闭区间 [0,maxIdx] 的程度（纹素单位）。
// 在区间内（含边界）返回 0；向左/右越界分别返回 -x 与 x-maxIdx。
func AxisOvershoot(x, maxIdx float64) float64 {
	if x < 0 {
		return -x
	}
	if x > maxIdx {
		return x - maxIdx
	}
	return 0
}

// Overshoot 返回 x/y 两轴各自的越界量以及欧氏越界距离 d=hypot(ox,oy)。
func Overshoot(x, y, maxX, maxY float64) (ox, oy, d float64) {
	ox = AxisOvershoot(x, maxX)
	oy = AxisOvershoot(y, maxY)
	return ox, oy, math.Hypot(ox, oy)
}

// OutOfBounds 判断连续纹素坐标是否严格落到纹素中心范围之外。
func OutOfBounds(x, y, maxX, maxY float64) bool {
	return x < 0 || x > maxX || y < 0 || y > maxY
}

// DecayWeight 是外插权重的衰减函数：
//
//	w(d; λ) = exp(-λ d), w(0)=1，且随越界距离 d 单调下降、d→∞ 时趋于 0。
//
// 推导：设权重对越界距离的相对衰减率为常数 λ，即 dw/dd = -λw，
// 边界处仍完全采信外插结果 w(0)=1，解此一阶齐次方程即得指数式。
// λ 的单位是 1/纹素；若距离以输出像素计（1 像素 = k 个源纹素），
// 则该档系数 β = kλ。
func DecayWeight(d, lambda float64) float64 {
	if lambda <= 0 {
		return 1
	}
	if d <= 0 {
		return 1
	}
	return math.Exp(-lambda * d)
}

// Sampler 在贴图之上缓存每纹素梯度，供一阶外插使用。
type Sampler struct {
	T  *Tex
	gx []RGB
	gy []RGB
}

// NewSampler 创建采样器并预计算中心差分梯度（边界用单侧差分）。
func NewSampler(t *Tex) *Sampler {
	s := &Sampler{T: t, gx: make([]RGB, len(t.Pix)), gy: make([]RGB, len(t.Pix))}
	for y := 0; y < t.H; y++ {
		for x := 0; x < t.W; x++ {
			i := y*t.W + x
			switch {
			case x == 0:
				s.gx[i] = t.at(1, y).Sub(t.at(0, y))
			case x == t.W-1:
				s.gx[i] = t.at(t.W-1, y).Sub(t.at(t.W-2, y))
			default:
				s.gx[i] = t.at(x+1, y).Sub(t.at(x-1, y)).Scale(0.5)
			}
			switch {
			case y == 0:
				s.gy[i] = t.at(x, 1).Sub(t.at(x, 0))
			case y == t.H-1:
				s.gy[i] = t.at(x, t.H-1).Sub(t.at(x, t.H-2))
			default:
				s.gy[i] = t.at(x, y+1).Sub(t.at(x, y-1)).Scale(0.5)
			}
		}
	}
	return s
}

// bilinear 在给定数据层（颜色或梯度）上做双线性插值；
// 调用前 x/y 必须已截断到合法范围。
func (s *Sampler) bilinear(layer []RGB, x, y float64) RGB {
	x0 := int(math.Floor(x))
	y0 := int(math.Floor(y))
	x1 := x0 + 1
	y1 := y0 + 1
	if x1 >= s.T.W {
		x1 = s.T.W - 1
	}
	if y1 >= s.T.H {
		y1 = s.T.H - 1
	}
	fx := x - math.Floor(x)
	fy := y - math.Floor(y)
	w00 := (1 - fx) * (1 - fy)
	w10 := fx * (1 - fy)
	w01 := (1 - fx) * fy
	w11 := fx * fy
	at := func(arr []RGB, xx, yy int) RGB { return arr[yy*s.T.W+xx] }
	c00 := at(layer, x0, y0)
	c10 := at(layer, x1, y0)
	c01 := at(layer, x0, y1)
	c11 := at(layer, x1, y1)
	return RGB{
		R: c00.R*w00 + c10.R*w10 + c01.R*w01 + c11.R*w11,
		G: c00.G*w00 + c10.G*w10 + c01.G*w01 + c11.G*w11,
		B: c00.B*w00 + c10.B*w10 + c01.B*w01 + c11.B*w11,
	}
}

// Clamp 是“截断到边界”的双线性采样：坐标先夹到纹素中心范围。
// 越界时整片区域重复同一边界纹素，即颜色被拉伸的现状。
func (s *Sampler) Clamp(x, y float64) RGB {
	cx := math.Min(math.Max(x, 0), float64(s.T.W-1))
	cy := math.Min(math.Max(y, 0), float64(s.T.H-1))
	return s.bilinear(s.T.Pix, cx, cy)
}

// SampleClampUV 是直接按归一化 UV 进行的截断采样便捷入口。
func (s *Sampler) SampleClampUV(u, v float64) RGB {
	return s.Clamp(TexelCoord(u, s.T.W), TexelCoord(v, s.T.H))
}

// Weighted 实现越界衰减带权外插。
//
//	δ  = 欧氏越界距离（纹素）
//	w  = exp(-λδ)
//	E  = order==0 时为边界常量外插（=截断采样值）；
//	     order>=1 时再叠加边界梯度的一阶线性外插
//	out = fallback + w·(E-fallback)
//
// 在界内 δ=0、w=1、越界量为 0，结果与普通双线性采样完全一致。
func (s *Sampler) Weighted(x, y, lambda float64, fallback RGB, order int) RGB {
	maxX := float64(s.T.W - 1)
	maxY := float64(s.T.H - 1)
	ox, oy, d := Overshoot(x, y, maxX, maxY)

	cx := math.Min(math.Max(x, 0), maxX)
	cy := math.Min(math.Max(y, 0), maxY)
	e := s.bilinear(s.T.Pix, cx, cy)

	if order >= 1 && (ox > 0 || oy > 0) {
		gx := s.bilinear(s.gx, cx, cy)
		gy := s.bilinear(s.gy, cx, cy)
		sx := 0.0
		if x < 0 {
			sx = -1
		} else if x > maxX {
			sx = 1
		}
		sy := 0.0
		if y < 0 {
			sy = -1
		} else if y > maxY {
			sy = 1
		}
		e = e.Add(gx.Scale(sx * ox)).Add(gy.Scale(sy * oy))
	}

	w := DecayWeight(d, lambda)
	return fallback.Add(e.Sub(fallback).Scale(w))
}

// SampleWeightedUV 是按归一化 UV 进行带权外插的便捷入口。
func (s *Sampler) SampleWeightedUV(u, v, lambda float64, fallback RGB, order int) RGB {
	return s.Weighted(TexelCoord(u, s.T.W), TexelCoord(v, s.T.H), lambda, fallback, order)
}
