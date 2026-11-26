package main

// 输出改进前后最差三角形的 PNG 对照图。
// 图里画出整个网格，并把面积比最小的那个三角形高亮，
// 红=耳切基线，绿=最坏情况最优。

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"

	"vecgo/geom"
)

const pngSize = 640

var (
	colBG      = color.RGBA{251, 250, 247, 255}
	colMesh    = color.RGBA{138, 143, 150, 255}
	colOutline = color.RGBA{44, 62, 80, 255}
	colEar     = color.RGBA{176, 58, 46, 255}
	colOpt     = color.RGBA{30, 107, 60, 255}
)

func saveWorstPNG(path string, poly []geom.Point, tris []geom.Triangle, worst geom.Triangle, accent color.RGBA) error {
	img := image.NewRGBA(image.Rect(0, 0, pngSize, pngSize))
	for y := 0; y < pngSize; y++ {
		for x := 0; x < pngSize; x++ {
			img.SetRGBA(x, y, colBG)
		}
	}

	tf := newPNGTransform(poly, pngSize, 16)
	for _, t := range tris {
		for e := 0; e < 3; e++ {
			drawLine(img, tf.apply(t[e]), tf.apply(t[(e+1)%3]), colMesh, 0.15)
		}
	}
	// 高亮最差三角形
	fillTriangle(img, tf.apply(worst[0]), tf.apply(worst[1]), tf.apply(worst[2]), accent, 0.35)
	for e := 0; e < 3; e++ {
		drawLine(img, tf.apply(worst[e]), tf.apply(worst[(e+1)%3]), accent, 0.85)
	}
	// 多边形轮廓
	for i := 0; i < len(poly); i++ {
		drawLine(img, tf.apply(poly[i]), tf.apply(poly[(i+1)%len(poly)]), colOutline, 0.55)
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

type pngTransform struct {
	scale            float64
	minx, miny, maxy float64
	dx, dy           float64
}

func newPNGTransform(poly []geom.Point, size, pad float64) pngTransform {
	minx, miny := math.Inf(1), math.Inf(1)
	maxx, maxy := math.Inf(-1), math.Inf(-1)
	for _, p := range poly {
		minx = math.Min(minx, p.X)
		miny = math.Min(miny, p.Y)
		maxx = math.Max(maxx, p.X)
		maxy = math.Max(maxy, p.Y)
	}
	spanX := math.Max(maxx-minx, 1e-12)
	spanY := math.Max(maxy-miny, 1e-12)
	s := math.Min((size-2*pad)/spanX, (size-2*pad)/spanY)
	dx := pad + ((size-2*pad)-spanX*s)/2
	dy := pad + ((size-2*pad)-spanY*s)/2
	return pngTransform{scale: s, minx: minx, miny: miny, maxy: maxy, dx: dx, dy: dy}
}

func (t pngTransform) apply(p geom.Point) geom.Point {
	return geom.Point{X: t.dx + (p.X-t.minx)*t.scale, Y: t.dy + (t.maxy-p.Y)*t.scale}
}

// drawLine 用 3x3 超采样的线段覆盖来抗锯齿。
func drawLine(img *image.RGBA, a, b geom.Point, c color.RGBA, alpha float64) {
	const ss = 3
	minx := int(math.Floor(math.Min(a.X, b.X))) - 1
	maxx := int(math.Ceil(math.Max(a.X, b.X))) + 1
	miny := int(math.Floor(math.Min(a.Y, b.Y))) - 1
	maxy := int(math.Ceil(math.Max(a.Y, b.Y))) + 1
	dx, dy := b.X-a.X, b.Y-a.Y
	len2 := dx*dx + dy*dy
	if len2 == 0 {
		return
	}
	for y := miny; y <= maxy; y++ {
		for x := minx; x <= maxx; x++ {
			hit := 0
			for sy := 0; sy < ss; sy++ {
				py := float64(y) + (float64(sy)+0.5)/ss
				for sx := 0; sx < ss; sx++ {
					px := float64(x) + (float64(sx)+0.5)/ss
					t := ((px-a.X)*dx + (py-a.Y)*dy) / len2
					t = math.Max(0, math.Min(1, t))
					qx, qy := a.X+t*dx, a.Y+t*dy
					if math.Hypot(px-qx, py-qy) <= 0.55 {
						hit++
					}
				}
			}
			if hit > 0 {
				blend(img, x, y, c, alpha*float64(hit)/(ss*ss))
			}
		}
	}
}

func fillTriangle(img *image.RGBA, a, b, c geom.Point, col color.RGBA, alpha float64) {
	const ss = 3
	minx := int(math.Floor(math.Min(a.X, math.Min(b.X, c.X))))
	maxx := int(math.Ceil(math.Max(a.X, math.Max(b.X, c.X))))
	miny := int(math.Floor(math.Min(a.Y, math.Min(b.Y, c.Y))))
	maxy := int(math.Ceil(math.Max(a.Y, math.Max(b.Y, c.Y))))
	for y := miny; y <= maxy; y++ {
		for x := minx; x <= maxx; x++ {
			hit := 0
			for sy := 0; sy < ss; sy++ {
				py := float64(y) + (float64(sy)+0.5)/ss
				for sx := 0; sx < ss; sx++ {
					px := float64(x) + (float64(sx)+0.5)/ss
					if insideTri(geom.Point{X: px, Y: py}, a, b, c) {
						hit++
					}
				}
			}
			if hit > 0 {
				blend(img, x, y, col, alpha*float64(hit)/(ss*ss))
			}
		}
	}
}

func insideTri(p, a, b, c geom.Point) bool {
	d := func(o, u, v geom.Point) float64 {
		return (u.X-o.X)*(v.Y-o.Y) - (u.Y-o.Y)*(v.X-o.X)
	}
	s := d(a, b, c)
	if s == 0 {
		return false
	}
	x1 := d(a, b, p) * s
	x2 := d(b, c, p) * s
	x3 := d(c, a, p) * s
	return x1 >= 0 && x2 >= 0 && x3 >= 0
}

func blend(img *image.RGBA, x, y int, c color.RGBA, a float64) {
	if x < 0 || y < 0 || x >= pngSize || y >= pngSize || a <= 0 {
		return
	}
	if a > 1 {
		a = 1
	}
	cur := img.RGBAAt(x, y)
	mix := func(u, v uint8) uint8 {
		return uint8(math.Round(float64(u)*(1-a) + float64(v)*a))
	}
	img.SetRGBA(x, y, color.RGBA{mix(cur.R, c.R), mix(cur.G, c.G), mix(cur.B, c.B), 255})
}
