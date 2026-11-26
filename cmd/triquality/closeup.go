package main

// 最差三角形“证件照”：把单个最差三角形用统一比例尺（x/y 同尺度，
// 绝不各轴独立拉伸，否则细条会被拉成等边三角形）居中放大，
// 叠在 1 设备像素的浅网格上，直观看细长程度。
//
// 输出 tri-N-{ear,opt}-closeup.png，与全多边形对照图互补。

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"

	"vecgo/geom"
)

const (
	closeW    = 760
	closeH    = 460
	closePad  = 56.0
	closeSS   = 3
	arcRadius = 34.0
)

var colGrid = color.RGBA{223, 219, 210, 255}

func saveWorstCloseupPNG(path string, t geom.Triangle, accent color.RGBA) error {
	img := image.NewRGBA(image.Rect(0, 0, closeW, closeH))
	for y := 0; y < closeH; y++ {
		for x := 0; x < closeW; x++ {
			img.SetRGBA(x, y, colBG)
		}
	}

	tf := newCloseTransform(t)
	drawPixelGrid(img, t, tf)

	a := tf.apply(t[0])
	b := tf.apply(t[1])
	c := tf.apply(t[2])

	fillCloseTriangle(img, a, b, c, accent, 0.22)
	strokeCloseLine(img, a, b, accent, 0.95, 1.7)
	strokeCloseLine(img, b, c, accent, 0.95, 1.7)
	strokeCloseLine(img, c, a, accent, 0.95, 1.7)

	// 标出最小角：顶点圆点 + 角度弧，弧的张角就是该三角形的最小角。
	vi := minAngleVertex(t)
	v := tf.apply(t[vi])
	p := tf.apply(t[(vi+1)%3])
	q := tf.apply(t[(vi+2)%3])
	drawAngleArc(img, v, p, q, accent)
	fillCloseDot(img, v, 4.2, accent)
	fillCloseDot(img, a, 2.6, colOutline)
	fillCloseDot(img, b, 2.6, colOutline)
	fillCloseDot(img, c, 2.6, colOutline)

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

func newCloseTransform(t geom.Triangle) pngTransform {
	minx := math.Min(t[0].X, math.Min(t[1].X, t[2].X))
	maxx := math.Max(t[0].X, math.Max(t[1].X, t[2].X))
	miny := math.Min(t[0].Y, math.Min(t[1].Y, t[2].Y))
	maxy := math.Max(t[0].Y, math.Max(t[1].Y, t[2].Y))
	w := math.Max(maxx-minx, 1e-12)
	h := math.Max(maxy-miny, 1e-12)
	s := math.Min((closeW-2*closePad)/w, (closeH-2*closePad)/h)
	dx := closePad + ((closeW-2*closePad)-w*s)/2
	dy := closePad + ((closeH-2*closePad)-h*s)/2
	return pngTransform{scale: s, minx: minx, miny: miny, maxy: maxy, dx: dx, dy: dy}
}

// drawPixelGrid 在三角形包围盒范围内画整数设备坐标线（1 单位=1 像素）。
// 太密（<5px/格）时跳过，避免糊成一片。
func drawPixelGrid(img *image.RGBA, t geom.Triangle, tf pngTransform) {
	minx := math.Min(t[0].X, math.Min(t[1].X, t[2].X))
	maxx := math.Max(t[0].X, math.Max(t[1].X, t[2].X))
	miny := math.Min(t[0].Y, math.Min(t[1].Y, t[2].Y))
	maxy := math.Max(t[0].Y, math.Max(t[1].Y, t[2].Y))

	if tf.scale >= 5 {
		for gx := math.Ceil(minx); gx <= math.Floor(maxx); gx++ {
			x := tf.apply(geom.Point{X: gx, Y: miny}).X
			strokeCloseLine(img, geom.Point{X: x, Y: 0}, geom.Point{X: x, Y: closeH}, colGrid, 0.9, 0.5)
		}
		for gy := math.Ceil(miny); gy <= math.Floor(maxy); gy++ {
			y := tf.apply(geom.Point{X: minx, Y: gy}).Y
			strokeCloseLine(img, geom.Point{X: 0, Y: y}, geom.Point{X: closeW, Y: y}, colGrid, 0.9, 0.5)
		}
	}
}

func minAngleVertex(t geom.Triangle) int {
	ang := [3]float64{
		angleAtClose(t[0], t[1], t[2]),
		angleAtClose(t[1], t[2], t[0]),
		angleAtClose(t[2], t[0], t[1]),
	}
	if ang[0] <= ang[1] && ang[0] <= ang[2] {
		return 0
	}
	if ang[1] <= ang[2] {
		return 1
	}
	return 2
}

func angleAtClose(v, a, b geom.Point) float64 {
	ux, uy := a.X-v.X, a.Y-v.Y
	wx, wy := b.X-v.X, b.Y-v.Y
	nu := math.Hypot(ux, uy)
	nw := math.Hypot(wx, wy)
	if nu == 0 || nw == 0 {
		return 0
	}
	c := (ux*wx + uy*wy) / (nu * nw)
	return math.Acos(math.Max(-1, math.Min(1, c)))
}

func drawAngleArc(img *image.RGBA, v, p, q geom.Point, c color.RGBA) {
	a0 := math.Atan2(p.Y-v.Y, p.X-v.X)
	a1 := math.Atan2(q.Y-v.Y, q.X-v.X)
	d := a1 - a0
	for d > math.Pi {
		d -= 2 * math.Pi
	}
	for d < -math.Pi {
		d += 2 * math.Pi
	}
	const steps = 28
	var prev geom.Point
	for i := 0; i <= steps; i++ {
		ang := a0 + d*float64(i)/steps
		cur := geom.Point{X: v.X + arcRadius*math.Cos(ang), Y: v.Y + arcRadius*math.Sin(ang)}
		if i > 0 {
			strokeCloseLine(img, prev, cur, c, 0.9, 1.3)
		}
		prev = cur
	}
}

// ---- 自包含的抗锯齿绘制（按 closeW×closeH 边界裁剪） ----

func blendClose(img *image.RGBA, x, y int, c color.RGBA, a float64) {
	b := img.Bounds()
	if x < b.Min.X || y < b.Min.Y || x >= b.Max.X || y >= b.Max.Y || a <= 0 {
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

func strokeCloseLine(img *image.RGBA, a, b geom.Point, c color.RGBA, alpha, halfWidth float64) {
	pad := int(math.Ceil(halfWidth)) + 1
	minx := int(math.Floor(math.Min(a.X, b.X))) - pad
	maxx := int(math.Ceil(math.Max(a.X, b.X))) + pad
	miny := int(math.Floor(math.Min(a.Y, b.Y))) - pad
	maxy := int(math.Ceil(math.Max(a.Y, b.Y))) + pad
	dx, dy := b.X-a.X, b.Y-a.Y
	len2 := dx*dx + dy*dy
	if len2 == 0 {
		return
	}
	for y := miny; y <= maxy; y++ {
		for x := minx; x <= maxx; x++ {
			hit := 0
			for sy := 0; sy < closeSS; sy++ {
				py := float64(y) + (float64(sy)+0.5)/closeSS
				for sx := 0; sx < closeSS; sx++ {
					px := float64(x) + (float64(sx)+0.5)/closeSS
					t := ((px-a.X)*dx + (py-a.Y)*dy) / len2
					t = math.Max(0, math.Min(1, t))
					qx, qy := a.X+t*dx, a.Y+t*dy
					if math.Hypot(px-qx, py-qy) <= halfWidth {
						hit++
					}
				}
			}
			if hit > 0 {
				blendClose(img, x, y, c, alpha*float64(hit)/(closeSS*closeSS))
			}
		}
	}
}

func fillCloseTriangle(img *image.RGBA, a, b, c geom.Point, col color.RGBA, alpha float64) {
	minx := int(math.Floor(math.Min(a.X, math.Min(b.X, c.X))))
	maxx := int(math.Ceil(math.Max(a.X, math.Max(b.X, c.X))))
	miny := int(math.Floor(math.Min(a.Y, math.Min(b.Y, c.Y))))
	maxy := int(math.Ceil(math.Max(a.Y, math.Max(b.Y, c.Y))))
	for y := miny; y <= maxy; y++ {
		for x := minx; x <= maxx; x++ {
			hit := 0
			for sy := 0; sy < closeSS; sy++ {
				py := float64(y) + (float64(sy)+0.5)/closeSS
				for sx := 0; sx < closeSS; sx++ {
					px := float64(x) + (float64(sx)+0.5)/closeSS
					if insideTri(geom.Point{X: px, Y: py}, a, b, c) {
						hit++
					}
				}
			}
			if hit > 0 {
				blendClose(img, x, y, col, alpha*float64(hit)/(closeSS*closeSS))
			}
		}
	}
}

func fillCloseDot(img *image.RGBA, center geom.Point, radius float64, col color.RGBA) {
	r := int(math.Ceil(radius)) + 1
	cx, cy := int(math.Round(center.X)), int(math.Round(center.Y))
	for y := cy - r; y <= cy+r; y++ {
		for x := cx - r; x <= cx+r; x++ {
			hit := 0
			for sy := 0; sy < closeSS; sy++ {
				py := float64(y) + (float64(sy)+0.5)/closeSS
				for sx := 0; sx < closeSS; sx++ {
					px := float64(x) + (float64(sx)+0.5)/closeSS
					if math.Hypot(px-center.X, py-center.Y) <= radius {
						hit++
					}
				}
			}
			if hit > 0 {
				blendClose(img, x, y, col, float64(hit)/(closeSS*closeSS))
			}
		}
	}
}
