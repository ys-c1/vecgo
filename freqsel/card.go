package freqsel

import "math"

// Card 是一张多分区频率测试卡：正弦条纹（低/中/高）、棋盘、扫频 chirp、
// 同心环、缓变/平坦区、专用阶跃边缘与细条组。所有坐标均为源像素。
type Card struct {
	*Gray
	Tile           int
	VerticalEdge   Rect // 专用竖直阶跃
	HorizontalEdge Rect // 专用水平阶跃
	DiagonalEdge   Rect // 对角阶跃（考察几何锯齿/过渡宽度）
	DetailBars     Rect // 1/2/3/4px 细条
	Chirp          Rect // 水平扫频
	EdgeCrop       Rect // 竖边+横边+细条的连续对照裁剪区
	ChirpCrop      Rect // 扫频+条纹裁剪区
}

// NewCard 创建 4*tile 见方的测试卡（报告统一用 tile=120 → 480px）。
func NewCard(tile int) *Card {
	n := 4 * tile
	g := MustGray(n, n)
	c := &Card{Gray: g, Tile: tile}

	for cy := 0; cy < 4; cy++ {
		for cx := 0; cx < 4; cx++ {
			x0, y0 := cx*tile, cy*tile
			for v := 0; v < tile; v++ {
				for u := 0; u < tile; u++ {
					g.Pix[(y0+v)*n+(x0+u)] = tileValue(cx, cy, u, v, tile)
				}
			}
		}
	}

	R := func(cx, cy int) Rect {
		return Rect{cx * tile, cy * tile, (cx + 1) * tile, (cy + 1) * tile}
	}
	c.VerticalEdge = R(0, 3)
	c.HorizontalEdge = R(1, 3)
	c.DetailBars = R(2, 3)
	c.DiagonalEdge = R(3, 3)
	c.Chirp = R(0, 2)
	c.EdgeCrop = Rect{0, 3 * tile, 3 * tile, 4 * tile}
	c.ChirpCrop = Rect{0, 2 * tile, 2 * tile, 3 * tile}
	return c
}

// tileValue 返回第 (cx,cy) 个 tile 内 (u,v) 处的灰度。
func tileValue(cx, cy, u, v, t int) float64 {
	U, V := float64(u), float64(v)
	switch cy {
	case 0: // 竖直正弦条，频率随列升高
		freqs := [4]float64{0.03, 0.07, 0.12, 0.20}
		return 128 + 92*math.Sin(2*math.Pi*freqs[cx]*U)
	case 1:
		switch cx {
		case 0: // 水平正弦 f=.10
			return 128 + 92*math.Sin(2*math.Pi*0.10*V)
		case 1: // 6px 格棋盘（双向、低相干）
			return checker(U, V, 6, 34, 222)
		case 2: // 3px 细棋盘
			return checker(U, V, 3, 34, 222)
		default: // 对角正弦，径向 f=.10（高相干）
			return 128 + 92*math.Sin(2*math.Pi*0.10*(U+V)/math.Sqrt2)
		}
	case 2:
		switch cx {
		case 0: // 水平线性扫频 .02 -> .22
			f0, f1 := 0.02, 0.22
			ph := 2 * math.Pi * (f0*U + (f1-f0)/(2*float64(t))*U*U)
			return 128 + 92*math.Sin(ph)
		case 1: // 同心环，径向频率 .11
			dx, dy := U-float64(t)/2, V-float64(t)/2
			r := math.Hypot(dx, dy)
			return 128 + 92*math.Sin(2*math.Pi*0.11*r)
		case 2: // 平滑对角渐变（全 tile 仅一次爬坡，极低频）
			return 40 + 185*(U+V)/(2*float64(t-1))
		default: // 平坦中灰
			return 198
		}
	default: // cy==3
		switch cx {
		case 0: // 纯竖直阶跃
			if u < t/2 {
				return 42
			}
			return 218
		case 1: // 纯水平阶跃
			if v < t/2 {
				return 42
			}
			return 218
		case 2: // 1/2/3/4px 宽细条，四段各 30px
			widths := [4]int{1, 2, 3, 4}
			seg := u / (t / 4)
			if seg > 3 {
				seg = 3
			}
			w := widths[seg]
			uu := u % (t / 4)
			if (uu/w)%2 == 0 {
				return 225
			}
			return 28
		default: // 对角阶跃
			if U >= V {
				return 218
			}
			return 42
		}
	}
}

// checker 返回 c 像素格棋盘在 (u,v) 的两色之一。
func checker(U, V float64, c int, lo, hi float64) float64 {
	cf := float64(c)
	if (int(math.Floor(U/cf))+int(math.Floor(V/cf)))%2 == 0 {
		return hi
	}
	return lo
}
