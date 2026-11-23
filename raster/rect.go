package raster

// Rect 是像素空间中的半开矩形 [X0,X1) × [Y0,Y1)。
// 所有增量渲染的脏矩形都用这个类型表示。
type Rect struct {
	X0, Y0, X1, Y1 int
}

func (r Rect) Empty() bool { return r.X1 <= r.X0 || r.Y1 <= r.Y0 }

func (r Rect) W() int { return r.X1 - r.X0 }

func (r Rect) H() int { return r.Y1 - r.Y0 }

func (r Rect) Area() int {
	if r.Empty() {
		return 0
	}
	return r.W() * r.H()
}

// Valid 表示矩形本身形状合法（左上角严格小于右下角）。
func (r Rect) Valid() bool { return r.X0 < r.X1 && r.Y0 < r.Y1 }

// InBounds 表示矩形完全落在 w×h 的画幅内。
func (r Rect) InBounds(w, h int) bool {
	return r.Valid() && r.X0 >= 0 && r.Y0 >= 0 && r.X1 <= w && r.Y1 <= h
}

// Clamp 把矩形裁到画幅内，可能得到空矩形。
func (r Rect) Clamp(w, h int) Rect {
	if r.X0 < 0 {
		r.X0 = 0
	}
	if r.Y0 < 0 {
		r.Y0 = 0
	}
	if r.X1 > w {
		r.X1 = w
	}
	if r.Y1 > h {
		r.Y1 = h
	}
	return r
}

func (r Rect) Union(o Rect) Rect {
	if r.Empty() {
		return o
	}
	if o.Empty() {
		return r
	}
	return Rect{
		X0: minInt(r.X0, o.X0),
		Y0: minInt(r.Y0, o.Y0),
		X1: maxInt(r.X1, o.X1),
		Y1: maxInt(r.Y1, o.Y1),
	}
}

// Overlaps 表示两个矩形有面积重叠（仅共边不算）。
func (r Rect) Overlaps(o Rect) bool {
	return r.X0 < o.X1 && o.X0 < r.X1 && r.Y0 < o.Y1 && o.Y0 < r.Y1
}

// Touches 表示两个矩形相交或共边/共角，可以合并成一个矩形而不留缝隙。
func (r Rect) Touches(o Rect) bool {
	return r.X0 <= o.X1 && o.X0 <= r.X1 && r.Y0 <= o.Y1 && o.Y0 <= r.Y1
}

// Expand 在四个方向各扩 d 像素，再裁到画幅内。
func (r Rect) Expand(d, w, h int) Rect {
	return Rect{X0: r.X0 - d, Y0: r.Y0 - d, X1: r.X1 + d, Y1: r.Y1 + d}.Clamp(w, h)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
