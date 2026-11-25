package shading

import (
	"image"
	"math"

	"vecgo/geom"
)

// RGB 使用归一化通道，合法范围为 [0,1]。
type RGB struct {
	R, G, B float64
}

func clampUnit(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func clampByte(v float64) uint8 {
	v = math.Round(v)
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}

// Clamp 把颜色投影回 8-bit 量程对应的归一化立方体。
func (c RGB) Clamp() RGB {
	return RGB{R: clampUnit(c.R), G: clampUnit(c.G), B: clampUnit(c.B)}
}

// ParameterField 是一维渐变参数 t(P) 与参数域边界有符号距离的共同接口。
// 距离为正表示在合法参数域内侧，为负表示 t 已越界。
type ParameterField interface {
	Parameter(p geom.Point) float64
	SignedDistance(p geom.Point) float64
}

// RadialGradient 的参数为 t=|P-C|/R，合法域是圆盘 |P-C|<=R。
type RadialGradient struct {
	Center geom.Point
	Radius float64
	Inner  RGB // t=0
	Outer  RGB // t=1
}

func (g RadialGradient) Parameter(p geom.Point) float64 {
	return math.Hypot(p.X-g.Center.X, p.Y-g.Center.Y) / g.Radius
}

// SignedDistance 是点到圆 |P-C|=R 的有符号距离；内侧为正。
func (g RadialGradient) SignedDistance(p geom.Point) float64 {
	return g.Radius - math.Hypot(p.X-g.Center.X, p.Y-g.Center.Y)
}

// At 按线性渐变求值。t 不做截断，因此 t<0 或 t>1 时得到外推颜色。
func (g RadialGradient) At(t float64) RGB {
	return RGB{
		R: g.Inner.R + (g.Outer.R-g.Inner.R)*t,
		G: g.Inner.G + (g.Outer.G-g.Inner.G)*t,
		B: g.Inner.B + (g.Outer.B-g.Inner.B)*t,
	}
}

// LinearGradient 是线段端点 A、B 定义的线性渐变。
// t=((P-A)·(B-A))/|B-A|^2。
type LinearGradient struct {
	A, B  geom.Point
	Inner RGB
	Outer RGB
}

func (g LinearGradient) length() float64 {
	return math.Hypot(g.B.X-g.A.X, g.B.Y-g.A.Y)
}

func (g LinearGradient) unit() geom.Point {
	l := g.length()
	return geom.Point{X: (g.B.X - g.A.X) / l, Y: (g.B.Y - g.A.Y) / l}
}

func (g LinearGradient) Parameter(p geom.Point) float64 {
	dx, dy := g.B.X-g.A.X, g.B.Y-g.A.Y
	l2 := dx*dx + dy*dy
	return ((p.X-g.A.X)*dx + (p.Y-g.A.Y)*dy) / l2
}

// SignedDistance 返回 min(s0,s1)：s0 是到 t=0 直线的内侧有符号距离，
// s1 是到 t=1 直线的内侧有符号距离。
func (g LinearGradient) SignedDistance(p geom.Point) float64 {
	u := g.unit()
	s0 := (p.X-g.A.X)*u.X + (p.Y-g.A.Y)*u.Y
	s1 := (g.B.X-p.X)*u.X + (g.B.Y-p.Y)*u.Y
	return math.Min(s0, s1)
}

func (g LinearGradient) At(t float64) RGB {
	return RGB{
		R: g.Inner.R + (g.Outer.R-g.Inner.R)*t,
		G: g.Inner.G + (g.Outer.G-g.Inner.G)*t,
		B: g.Inner.B + (g.Outer.B-g.Inner.B)*t,
	}
}

// Overshoot 返回参数伸出 [0,1] 的长度 e(P)=max(-t,t-1,0)。
func Overshoot(f ParameterField, p geom.Point) float64 {
	t := f.Parameter(p)
	return math.Max(math.Max(-t, t-1), 0)
}

// OutOfRange 按“有符号距离<0”的等价判据扫描。
func OutOfRange(f ParameterField, p geom.Point) bool {
	return f.SignedDistance(p) < 0
}

// RenderRadial 渲染一个多边形内的径向渐变。
// truncate=false 复现“参数不截断、最终颜色硬饱和到量程”的旧路径；
// truncate=true 先把 t 截断到 [0,1]，因此不会产生外推导致的死白边。
//
// 超采样样本按覆盖率合成到黑色背景；这与 PGM 实验的黑底一致。
func RenderRadial(poly []geom.Point, g RadialGradient, w, h, ss int, truncate bool) *image.RGBA {
	if ss < 1 {
		ss = 1
	}
	im := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 3; i < len(im.Pix); i += 4 {
		im.Pix[i] = 255
	}
	n := float64(ss * ss)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var sum [3]float64
			for sj := 0; sj < ss; sj++ {
				for si := 0; si < ss; si++ {
					p := geom.Point{
						X: float64(x) + (float64(si)+0.5)/float64(ss),
						Y: float64(y) + (float64(sj)+0.5)/float64(ss),
					}
					if !geom.Inside(poly, p.X, p.Y) {
						continue // 黑底贡献为 0
					}
					t := g.Parameter(p)
					if truncate {
						t = clampUnit(t)
					}
					c := g.At(t)
					if !truncate {
						c = c.Clamp() // 旧路径在输出端硬饱和，溢出通道全部成 255
					}
					sum[0] += 255 * c.R
					sum[1] += 255 * c.G
					sum[2] += 255 * c.B
				}
			}
			off := im.PixOffset(x, y)
			im.Pix[off+0] = clampByte(sum[0] / n)
			im.Pix[off+1] = clampByte(sum[1] / n)
			im.Pix[off+2] = clampByte(sum[2] / n)
			im.Pix[off+3] = 255
		}
	}
	return im
}

// Metrics 是一组渐变的整幅扫描结果。所有计数都来自 W×H 全像素循环。
type Metrics struct {
	ID                      string       `json:"id"`
	Label                   string       `json:"label"`
	Width                   int          `json:"width"`
	Height                  int          `json:"height"`
	SuperSample             int          `json:"supersample"`
	Polygon                 []geom.Point `json:"polygon"`
	Center                  geom.Point   `json:"center"`
	Radius                  float64      `json:"radius"`
	Inner                   RGB          `json:"inner"`
	Outer                   RGB          `json:"outer"`
	ParameterOverflowPixels int          `json:"parameter_overflow_pixels"`
	OverflowSamples         int          `json:"overflow_samples"`
	InsidePixels            int          `json:"inside_pixel_centers"`
	ChangedPixels           int          `json:"changed_pixels"`
	WhitePixelsBefore       int          `json:"white_pixels_before"`
	WhitePixelsAfter        int          `json:"white_pixels_after"`
	MaxPixelDelta           int          `json:"max_pixel_delta"`
	MaxPixelDeltaAt         [2]int       `json:"max_pixel_delta_at"`
	MaxOvershoot            float64      `json:"max_overshoot"`
	ColorLipschitz          float64      `json:"color_lipschitz_inf"`
	Bound8Bit               int          `json:"bound_8bit"`
	TightWhiteBound         int          `json:"tight_white_bound,omitempty"`
}

// MeasureRadial 全幅扫描参数溢出，并逐像素比较截断前后的 RGBA 图。
func MeasureRadial(id, label string, poly []geom.Point, g RadialGradient, before, after *image.RGBA, ss int) Metrics {
	bounds := before.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	m := Metrics{
		ID: id, Label: label, Width: w, Height: h, SuperSample: ss,
		Polygon: poly,
		Center:  g.Center, Radius: g.Radius, Inner: g.Inner, Outer: g.Outer,
		MaxPixelDeltaAt: [2]int{-1, -1},
	}
	m.ColorLipschitz = math.Max(math.Abs(g.Outer.R-g.Inner.R),
		math.Max(math.Abs(g.Outer.G-g.Inner.G), math.Abs(g.Outer.B-g.Inner.B)))

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			center := geom.Point{X: float64(x) + 0.5, Y: float64(y) + 0.5}
			if geom.Inside(poly, center.X, center.Y) {
				m.InsidePixels++
				if OutOfRange(g, center) {
					m.ParameterOverflowPixels++
				}
			}

			for sj := 0; sj < ss; sj++ {
				for si := 0; si < ss; si++ {
					p := geom.Point{
						X: float64(x) + (float64(si)+0.5)/float64(ss),
						Y: float64(y) + (float64(sj)+0.5)/float64(ss),
					}
					if !geom.Inside(poly, p.X, p.Y) {
						continue
					}
					if OutOfRange(g, p) {
						m.OverflowSamples++
					}
					if e := Overshoot(g, p); e > m.MaxOvershoot {
						m.MaxOvershoot = e
					}
				}
			}

			bo := before.PixOffset(x, y)
			ao := after.PixOffset(x, y)
			pixelChanged := false
			for k := 0; k < 3; k++ {
				d := int(before.Pix[bo+k]) - int(after.Pix[ao+k])
				if d < 0 {
					d = -d
				}
				if d > m.MaxPixelDelta {
					m.MaxPixelDelta = d
					m.MaxPixelDeltaAt = [2]int{x, y}
				}
				if d != 0 {
					pixelChanged = true
				}
			}
			if pixelChanged {
				m.ChangedPixels++
			}
			if isWhitePixel(before, bo) {
				m.WhitePixelsBefore++
			}
			if isWhitePixel(after, ao) {
				m.WhitePixelsAfter++
			}
		}
	}

	m.Bound8Bit = int(math.Ceil(255*m.ColorLipschitz*m.MaxOvershoot)) + 1
	minOuter := math.Min(g.Outer.R, math.Min(g.Outer.G, g.Outer.B))
	m.TightWhiteBound = 255 - int(math.Round(255*minOuter))
	return m
}

func isWhitePixel(im *image.RGBA, off int) bool {
	return im.Pix[off+0] == 255 && im.Pix[off+1] == 255 && im.Pix[off+2] == 255
}

// RenderOverflow 把整幅扫描出的参数越界像素标成红色，合法多边形区域保留 after 图。
func RenderOverflow(poly []geom.Point, g RadialGradient, after *image.RGBA) *image.RGBA {
	out := image.NewRGBA(after.Bounds())
	copy(out.Pix, after.Pix)
	b := after.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			p := geom.Point{X: float64(x) + 0.5, Y: float64(y) + 0.5}
			if !geom.Inside(poly, p.X, p.Y) {
				continue
			}
			off := out.PixOffset(x, y)
			if OutOfRange(g, p) {
				out.Pix[off+0] = 255
				out.Pix[off+1] = 72
				out.Pix[off+2] = 72
			}
		}
	}
	return out
}

// DiffHeatmap 放大截断前后的通道差；差值越大越偏品红。
func DiffHeatmap(before, after *image.RGBA) *image.RGBA {
	out := image.NewRGBA(before.Bounds())
	b := before.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			bo := before.PixOffset(x, y)
			ao := after.PixOffset(x, y)
			d := 0
			for k := 0; k < 3; k++ {
				v := int(before.Pix[bo+k]) - int(after.Pix[ao+k])
				if v < 0 {
					v = -v
				}
				if v > d {
					d = v
				}
			}
			v := uint8(0)
			if d > 0 {
				v = uint8(80 + 175*d/255)
			}
			off := out.PixOffset(x, y)
			out.Pix[off+0] = v
			out.Pix[off+1] = 20
			out.Pix[off+2] = v
			out.Pix[off+3] = 255
		}
	}
	return out
}
