package geom

import "math"

// FitToViewport 把多边形等比缩放并平移到 width×width 像素的视口内，
// 四周留 pad 像素。覆盖率误差按像素量化，因此必须在设备空间里度量。
func FitToViewport(poly []Point, width, pad float64) []Point {
	if len(poly) == 0 {
		return nil
	}
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
	s := (width - 2*pad) / math.Max(spanX, spanY)
	// 居中
	offX := pad + ((width-2*pad)-spanX*s)/2
	offY := pad + ((width-2*pad)-spanY*s)/2
	out := make([]Point, len(poly))
	for i, p := range poly {
		out[i] = Point{(p.X-minx)*s + offX, (p.Y-miny)*s + offY}
	}
	return out
}

// EllipsePolygon 用 n 个顶点内接逼近一个轴对齐椭圆。
func EllipsePolygon(a, b float64, n int) []Point {
	out := make([]Point, n)
	for k := 0; k < n; k++ {
		t := 2*math.Pi*float64(k)/float64(n) + 0.15
		out[k] = Point{a * math.Cos(t), b * math.Sin(t)}
	}
	return out
}

// StarPolygon 生成 5 角星形的简单多边形，R 为外半径，r 为内半径。
func StarPolygon(R, r float64) []Point {
	const n = 5
	out := make([]Point, 0, 2*n)
	for k := 0; k < 2*n; k++ {
		rad := R
		if k%2 == 1 {
			rad = r
		}
		t := math.Pi*float64(k)/float64(n) + 0.3
		out = append(out, Point{rad * math.Cos(t), rad * math.Sin(t)})
	}
	return out
}

// CorridorPolygon 生成一条 S 形细长走廊：中心线为幅值 amp 的正弦，
// 两侧各偏移 halfWidth，首尾用竖直边封口。这是典型的“细长三角形重灾区”。
func CorridorPolygon(amp, halfWidth, span float64, segments int) []Point {
	center := make([]Point, segments+1)
	for k := 0; k <= segments; k++ {
		u := float64(k) / float64(segments)
		center[k] = Point{
			X: u * span,
			Y: amp * math.Sin(2*math.Pi*1.25*u),
		}
	}
	top := make([]Point, 0, len(center))
	bot := make([]Point, 0, len(center))
	for i, p := range center {
		var dx, dy float64
		if i < segments {
			dx = center[i+1].X - p.X
			dy = center[i+1].Y - p.Y
		} else {
			dx = p.X - center[i-1].X
			dy = p.Y - center[i-1].Y
		}
		l := math.Hypot(dx, dy)
		nx, ny := -dy/l*halfWidth, dx/l*halfWidth
		top = append(top, Point{p.X + nx, p.Y + ny})
		bot = append(bot, Point{p.X - nx, p.Y - ny})
	}
	out := make([]Point, 0, len(top)+len(bot))
	out = append(out, top...)
	for i := len(bot) - 1; i >= 0; i-- {
		out = append(out, bot[i])
	}
	if SignedArea(out) < 0 {
		for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
			out[i], out[j] = out[j], out[i]
		}
	}
	return out
}
