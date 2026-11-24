package freqsel

import (
	"image"
	"image/color"
	"image/png"
	"os"
)

// SaveGrayPNG 把灰度图夹取取整后写为 8-bit PNG；zoom>1 时按最近邻放大 zoom 倍，
// 便于在报告里看清小尺寸缩小结果的像素与边缘。
func SaveGrayPNG(g *Gray, path string, zoom int) error {
	if zoom < 1 {
		zoom = 1
	}
	img := image.NewGray(image.Rect(0, 0, g.W*zoom, g.H*zoom))
	for y := 0; y < g.H; y++ {
		for x := 0; x < g.W; x++ {
			c := color.Gray{Y: Clamp255(g.Pix[y*g.W+x])}
			if zoom == 1 {
				img.SetGray(x, y, c)
				continue
			}
			for zy := 0; zy < zoom; zy++ {
				for zx := 0; zx < zoom; zx++ {
					img.SetGray(x*zoom+zx, y*zoom+zy, c)
				}
			}
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

// Canvas 是最简单的 RGB 拼版画布，用于把多档缩放结果拼成“每版一张图”。
type Canvas struct {
	Img *image.RGBA
}

// NewCanvas 创建指定尺寸、给定底色的画布。
func NewCanvas(w, h int, bg color.RGBA) *Canvas {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i+0] = bg.R
		img.Pix[i+1] = bg.G
		img.Pix[i+2] = bg.B
		img.Pix[i+3] = 255
	}
	return &Canvas{Img: img}
}

// FillRect 填充一个实心矩形（用于分隔线/标注框）。
func (c *Canvas) FillRect(x0, y0, w, h int, col color.RGBA) {
	for y := y0; y < y0+h; y++ {
		for x := x0; x < x0+w; x++ {
			c.Img.SetRGBA(x, y, col)
		}
	}
}

// BlitGrayNearest 把 g 的 src 区域（零值 Rect 表示整图）最近邻放大 zoom 倍，
// 贴到画布 (dx,dy)。返回贴入块的宽高。
func (c *Canvas) BlitGrayNearest(g *Gray, src Rect, dx, dy, zoom int) (int, int) {
	if zoom < 1 {
		zoom = 1
	}
	if src.X1 <= src.X0 || src.Y1 <= src.Y0 {
		src = Rect{0, 0, g.W, g.H}
	}
	rw, rh := src.X1-src.X0, src.Y1-src.Y0
	for y := 0; y < rh; y++ {
		for x := 0; x < rw; x++ {
			v := g.At(src.X0+x, src.Y0+y)
			gc := color.RGBA{R: Clamp255(v), G: Clamp255(v), B: Clamp255(v), A: 255}
			for zy := 0; zy < zoom; zy++ {
				for zx := 0; zx < zoom; zx++ {
					c.Img.SetRGBA(dx+x*zoom+zx, dy+y*zoom+zy, gc)
				}
			}
		}
	}
	return rw * zoom, rh * zoom
}

// SavePNG 编码保存画布。
func (c *Canvas) SavePNG(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, c.Img)
}

// heatStops 是黑蓝—青—绿—黄—红的多停点伪彩。
var heatStops = []struct {
	t float64
	c [3]uint8
}{
	{0.00, [3]uint8{8, 12, 40}},
	{0.20, [3]uint8{32, 80, 200}},
	{0.40, [3]uint8{20, 180, 220}},
	{0.60, [3]uint8{40, 200, 90}},
	{0.80, [3]uint8{240, 220, 40}},
	{1.00, [3]uint8{220, 40, 30}},
}

// HeatColor 把 t∈[0,1] 映成伪彩色。
func HeatColor(t float64) color.RGBA {
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	for i := 1; i < len(heatStops); i++ {
		if t <= heatStops[i].t {
			a, b := heatStops[i-1], heatStops[i]
			k := (t - a.t) / (b.t - a.t)
			return color.RGBA{
				R: uint8(float64(a.c[0]) + k*float64(b.c[0]-a.c[0])),
				G: uint8(float64(a.c[1]) + k*float64(b.c[1]-a.c[1])),
				B: uint8(float64(a.c[2]) + k*float64(b.c[2]-a.c[2])),
				A: 255,
			}
		}
	}
	return color.RGBA{R: 220, G: 40, B: 30, A: 255}
}

// SaveTauHeatmap 把连续层坐标 tau∈[tauMin,tauMax] 伪彩保存为原尺寸 PNG。
func SaveTauHeatmap(tau []float64, w, h int, tauMin, tauMax float64, path string) error {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	rng := tauMax - tauMin
	if rng <= 1e-9 {
		rng = 1
	}
	for i, t := range tau {
		img.SetRGBA(i%w, i/w, HeatColor((t-tauMin)/rng))
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

// LevelPalette 给整数层号一组可区分的离散颜色（0..5，超出循环）。
var LevelPalette = []color.RGBA{
	{R: 35, G: 120, B: 220, A: 255}, // 0 蓝（最细核）
	{R: 30, G: 190, B: 170, A: 255}, // 1 青
	{R: 120, G: 205, B: 70, A: 255}, // 2 绿
	{R: 245, G: 200, B: 40, A: 255}, // 3 黄
	{R: 240, G: 120, B: 40, A: 255}, // 4 橙
	{R: 215, G: 45, B: 45, A: 255},  // 5 红（最宽核）
}

// SaveLevelMap 把逐像素整数层号保存为离散色 PNG，可 zoom 放大。
func SaveLevelMap(level []int, w, h, zoom int, path string) error {
	if zoom < 1 {
		zoom = 1
	}
	img := image.NewRGBA(image.Rect(0, 0, w*zoom, h*zoom))
	for i, l := range level {
		col := LevelPalette[l%len(LevelPalette)]
		x, y := i%w, i/w
		for zy := 0; zy < zoom; zy++ {
			for zx := 0; zx < zoom; zx++ {
				img.SetRGBA((x*zoom + zx), y*zoom+zy, col)
			}
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
