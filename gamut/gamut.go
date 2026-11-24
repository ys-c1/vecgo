// Package gamut 在线性光 sRGB 空间（域为 [0,1]^3 单位立方体）内做色域映射。
//
// 超域像素沿等明度（Rec.709 luma）方向按色域边界推出的压缩曲线回压，
// 替代逐通道硬截断（Clip），避免饱和色被压成一块死色。
package gamut

import "math"

// RGB 是线性光 sRGB 三元组，[0,1] 为域内，任一通道 >1 即超域。
type RGB struct {
	R, G, B float64
}

// Rec.709 luma 权重，和为 1。
const (
	wR = 0.2126
	wG = 0.7152
	wB = 0.0722
)

// 压缩曲线参数：
//
//	y0  恒等段拐点（内移到色域边界以内，归一化越界量 y<=y0 时像素原样保留）
//	A   远场渐近线，A<1 保证任意越界量都被压回域内且给远场留出层次余量
//
// 软膝宽度 w=A-y0 由拐点处 C1 连续条件反解（见 curve）。
const (
	y0 = 0.85
	A  = 0.95
)

// w 是软膝宽度，构造时保证 y0+w=A。
var w = A - y0

var (
	lumaW   = RGB{R: wR, G: wG, B: wB}
	grayOne = RGB{R: 1, G: 1, B: 1} // 等明度灰点方向：灰像素即 (L,L,L)
)

func dot(a, b RGB) float64 { return a.R*b.R + a.G*b.G + a.B*b.B }

func scale(a RGB, k float64) RGB {
	return RGB{R: a.R * k, G: a.G * k, B: a.B * k}
}

func add(a, b RGB) RGB {
	return RGB{R: a.R + b.R, G: a.G + b.G, B: a.B + b.B}
}

// Luma 返回 Rec.709 明度。等明度灰点为 g=(L,L,L)。
func Luma(c RGB) float64 { return dot(c, lumaW) }

// 灰轴正交补平面（w·d=0）内的一组标准正交基，用于定义与压缩方向
// 严格共面的色相角；init 中对种子向量做 Gram–Schmidt 构造。
var (
	e1 RGB
	e2 RGB
)

func init() {
	n := math.Sqrt(dot(lumaW, lumaW))
	u := scale(lumaW, 1/n) // 灰轴单位向量
	seed := RGB{R: 1, G: 0, B: 0}
	if math.Abs(dot(seed, u)) > 0.9 {
		seed = RGB{R: 0, G: 1, B: 0}
	}
	e1 = add(seed, scale(u, -dot(seed, u)))
	e1 = scale(e1, 1/math.Sqrt(dot(e1, e1)))
	e2 = RGB{
		R: u.G*e1.B - u.B*e1.G,
		G: u.B*e1.R - u.R*e1.B,
		B: u.R*e1.G - u.G*e1.R,
	}
}

// HueDeg 返回压缩工作空间内的色相角（度）。
// 压缩只对偏离向量 d 做统一标量缩放，故该角在数学上严格不变。
func HueDeg(c RGB) float64 {
	L := Luma(c)
	d := add(c, scale(grayOne, -L)) // g=(L,L,L)，且 w·d=0
	return math.Atan2(dot(d, e2), dot(d, e1)) * 180 / math.Pi
}

// Boundary 返回像素的明度 L 与归一化越界量 y：
//
//	c = g + d，g=(L,L,L)，w·d=0；同射线色域边界点为 g + yb*d，
//	归一化后 y = 1/yb：y<=1 域内，y==1 恰在边界，y>1 超域。
//
// 边界倍率 yb 是射线首次碰到立方体任一界面的距离，上下界面都取：
//
//	上界面（通道=1）：yb = (1-L)/d_i，d_i>0
//	下界面（通道=0）：yb = -L/d_i，  d_i<0
//
// 灰像素（d≈0）返回 y=0。
func Boundary(c RGB) (L, y float64) {
	L = Luma(c)
	d := add(c, scale(grayOne, -L))
	yb := math.Inf(1)
	for i := 0; i < 3; i++ {
		var di float64
		switch i {
		case 0:
			di = d.R
		case 1:
			di = d.G
		default:
			di = d.B
		}
		switch {
		case di > 0:
			if v := (1 - L) / di; v < yb {
				yb = v
			}
		case di < 0:
			if v := -L / di; v < yb {
				yb = v
			}
		}
	}
	if math.IsInf(yb, 1) {
		return L, 0
	}
	return L, 1 / yb
}

// curve 是压缩曲线 R(y)：压缩后偏离向量的归一化越界量。
//
// 恒等段：y<=y0 时 R(y)=y（倍率 h=R/y=1）。
// 越界段令 z=y-y0，取 R(y)=A-w·e^{-z/w}，实际倍率 h(y)=R(y)/y。
// z=0 时 R(y0)=A-w=y0、R'(y0)=1，故 h(y0)=1、h'(y0)=0，与恒等段 C1 衔接；
// z→∞ 时 R 以指数速度趋于 A=0.95<1，对一切有限 y 有 R(y)<A，
// 超域像素必然严格压回域内；对数式远场（R'=e^{-z/w}）缓慢衰减，
// 在 [y0,A] 整段保留单调层次，不会像有理曲线那样快速贴近平顶。
func curve(y float64) (R, h float64) {
	if y <= y0 {
		return y, 1
	}
	z := y - y0
	R = A - w*math.Exp(-z/w)
	return R, R / y
}

// Compress 按色域边界压缩曲线回压一个（可能超域的）线性 RGB 像素。
// 明度 L 与色相方向 d/|d| 保持不变，仅缩小偏离等明度灰点的幅度。
func Compress(c RGB) RGB {
	L, y := Boundary(c)
	if y == 0 {
		return c
	}
	d := add(c, scale(grayOne, -L))
	_, h := curve(y)
	return add(scale(grayOne, L), scale(d, h))
}

// Clip 是原截断路径：逐通道硬夹到 [0,1]，保留以备对照。
func Clip(c RGB) RGB {
	return RGB{R: clamp(c.R), G: clamp(c.G), B: clamp(c.B)}
}

func clamp(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// OutOfGamut 报告像素是否超域（任一通道超出 [0,1]）。
func OutOfGamut(c RGB) bool {
	return c.R < 0 || c.R > 1 || c.G < 0 || c.G > 1 || c.B < 0 || c.B > 1
}

// EncodeSRGB 把线性光值编码为 sRGB 传递函数后的 8 位值，用于导出 PNG。
func EncodeSRGB(v float64) uint8 {
	v = clamp(v)
	var e float64
	if v <= 0.0031308 {
		e = 12.92 * v
	} else {
		e = 1.055*math.Pow(v, 1/2.4) - 0.055
	}
	return uint8(math.Round(e * 255))
}

// OklabDeg 把线性 sRGB 像素转入 Oklab，返回 Oklab 色相角（度）。
// 作为感知色相的独立度量，用来报同色区域压缩前后的实际色相偏移上界。
func OklabDeg(c RGB) float64 {
	l := 0.4122214708*c.R + 0.5363325363*c.G + 0.0514459929*c.B
	m := 0.2119034982*c.R + 0.6806995451*c.G + 0.1073969566*c.B
	s := 0.0883024619*c.R + 0.2817188376*c.G + 0.6299787005*c.B
	l, m, s = math.Cbrt(l), math.Cbrt(m), math.Cbrt(s)
	a := 1.9779984951*l - 2.4285922050*m + 0.4505937099*s
	b := 0.0259040371*l + 0.7827717662*m - 0.8086757660*s
	return math.Atan2(b, a) * 180 / math.Pi
}

// HueDistance 返回两个色相角之间的夹角，单位度，范围 [0,180]。
func HueDistance(h1, h2 float64) float64 {
	d := math.Abs(math.Mod(h1-h2+540, 360) - 180)
	if d > 180 {
		d = 360 - d
	}
	return d
}
