// Package freqsel 实现“按局部频率自适应”的缩小选层。
//
// 背景：高斯/ mip 金字塔把图像预滤波成若干层，层号越大核越宽、越模糊。
// 旧的全局方案只看缩放比 s：D=1/s 个源像素对应一个目标像素，按
// L=ceil(log2 D) 固定选一层。取整（尤其向上取整到 2 的幂）会在平坦区与
// 中低频纹理上过度模糊，缩小后细节丢失、梯度能量偏低。
//
// 本包在每个像素的邻域内用梯度结构张量估计局部主频 f（周期/源像素），
// 以输出奈奎斯特频率 fN=s/2 为基准反解该像素所需的连续层坐标 τ 与高斯
// 核宽 σ，再在相邻两个整数层之间按小数权重逐像素线性插值（等价于逐像素
// 偏置的三线性 mip 采样）。核宽处处不低于安全下限，且高斯核无负瓣，因此
// 在比全局整数层方案更锐利（梯度能量更高）的同时不会产生过冲/振铃。
package freqsel

import (
	"math"
)

// Sigma0 是第 0 层的等效高斯标准差（源像素）。
//
// 取 Sigma0=2/π≈0.6366：高斯频率响应 H(f)=exp(-2π²σ²f²)，在输出奈奎斯特
// 频率 fN=s/2=1/(2D) 处，若 σ=Sigma0*2^log2D=Sigma0*D=2D/π，则
// 2π²σ²fN²=2，H(fN)=e^{-2}≈0.135，是统一的“奈奎斯特增益 e^-2”口径。
const Sigma0 = 2.0 / math.Pi

// Gray 是 8-bit 灰度的浮点工作缓冲，取值约定 0..255。
type Gray struct {
	W, H int
	Pix  []float64
}

// Rect 是源/目标像素坐标系下的半开矩形。
type Rect struct {
	X0, Y0, X1, Y1 int
}

// NewGray 创建 w*h 的零灰度图。
func NewGray(w, h int) (*Gray, error) {
	if w <= 0 || h <= 0 {
		return nil, errBadSize
	}
	return &Gray{W: w, H: h, Pix: make([]float64, w*h)}, nil
}

// MustGray 同 NewGray，非法尺寸时 panic（仅用于已知合法的构造）。
func MustGray(w, h int) *Gray {
	g, err := NewGray(w, h)
	if err != nil {
		panic(err)
	}
	return g
}

// FromUint8 用 0..255 字节构造浮点灰度。
func FromUint8(w, h int, p []uint8) *Gray {
	g := MustGray(w, h)
	n := w * h
	if len(p) < n {
		n = len(p)
	}
	for i := 0; i < n; i++ {
		g.Pix[i] = float64(p[i])
	}
	return g
}

// At 带边界夹取地读取像素。
func (g *Gray) At(x, y int) float64 {
	if x < 0 {
		x = 0
	} else if x >= g.W {
		x = g.W - 1
	}
	if y < 0 {
		y = 0
	} else if y >= g.H {
		y = g.H - 1
	}
	return g.Pix[y*g.W+x]
}

// Clamp255 把浮点值夹到 0..255 并四舍五入为字节。
func Clamp255(v float64) uint8 {
	if v <= 0 {
		return 0
	}
	if v >= 255 {
		return 255
	}
	return uint8(math.Round(v))
}

// ToUint8 输出夹取取整后的字节缓冲。
func (g *Gray) ToUint8() []uint8 {
	out := make([]uint8, len(g.Pix))
	for i, v := range g.Pix {
		out[i] = Clamp255(v)
	}
	return out
}

// clone 返回深拷贝。
func (g *Gray) clone() *Gray {
	c := MustGray(g.W, g.H)
	copy(c.Pix, g.Pix)
	return c
}

// Gradients 用中心差分（边界复制夹取）计算 gx, gy。
// 中心差分对频率 θ=2πf 的正弦分量的幅值增益为 sinθ（理想导数为 θ），
// 这一已知偏差在频率反解处用 SincGain 做一阶校正。
func Gradients(g *Gray) (gx, gy []float64) {
	gx = make([]float64, g.W*g.H)
	gy = make([]float64, g.W*g.H)
	for y := 0; y < g.H; y++ {
		for x := 0; x < g.W; x++ {
			i := y*g.W + x
			gx[i] = (g.At(x+1, y) - g.At(x-1, y)) * 0.5
			gy[i] = (g.At(x, y+1) - g.At(x, y-1)) * 0.5
		}
	}
	return gx, gy
}

// SincGain 是中心差分在指定方向、指定频率上的幅值/理想导数比：sin(2πf)/(2πf)。
// 频率接近奈奎斯特时趋零，无法可靠反演，因此只做温和校正并夹取。
func SincGain(f float64) float64 {
	theta := 2 * math.Pi * f
	if theta < 1e-6 {
		return 1
	}
	return math.Sin(theta) / theta
}

// integral 是若干通道的积分图（左上含 1 行 1 列零边），用于 O(1) 邻域求和。
type integral struct {
	w, h int
	d    [][]float64 // 每个通道一张 (w+1)*(h+1) 积分图
}

// buildIntegral 对 channels（各长 w*h）构造联合积分图。
func buildIntegral(w, h int, channels ...[]float64) *integral {
	nch := len(channels)
	I := &integral{w: w, h: h, d: make([][]float64, nch)}
	stride := w + 1
	for c := 0; c < nch; c++ {
		I.d[c] = make([]float64, (w+1)*(h+1))
		for y := 1; y <= h; y++ {
			rowSum := 0.0
			for x := 1; x <= w; x++ {
				rowSum += channels[c][(y-1)*w+(x-1)]
				I.d[c][y*stride+x] = I.d[c][(y-1)*stride+x] + rowSum
			}
		}
	}
	return I
}

// windowSums 返回以 (cx,cy) 为中心、边长 win 的夹取窗口内各通道之和与实际像素数。
func (I *integral) windowSums(cx, cy, win int) (sums []float64, n float64) {
	r := win / 2
	x0 := maxInt(cx-r, 0)
	y0 := maxInt(cy-r, 0)
	x1 := minInt(cx+r+1, I.w) // 半开
	y1 := minInt(cy+r+1, I.h)
	stride := I.w + 1
	sums = make([]float64, len(I.d))
	for c := range I.d {
		d := I.d[c]
		sums[c] = d[y1*stride+x1] - d[y0*stride+x1] - d[y1*stride+x0] + d[y0*stride+x0]
	}
	return sums, float64((x1 - x0) * (y1 - y0))
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
