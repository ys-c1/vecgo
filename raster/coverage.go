package raster

import (
	"errors"
	"math"
	"sort"

	"vecgo/geom"
)

// Coverage 保存每个像素被多边形并集覆盖的比例（0..1），以及像素到世界坐标的映射。
// 多边形包围盒映射到整个画幅，与 Image.Fill 的映射一致；因此 Polys 的并集完全落在
// 画幅内，所有像素覆盖率之和乘像素面积即为并集的精确面积。
type Coverage struct {
	W, H                       int
	MinX, MinY, PixelW, PixelH float64
	Cov                        []float64
}

type covEdge struct {
	a, b geom.Point
	poly int
}

// Rasterize 用精确的像素覆盖面积填充画幅。
// 每个像素的覆盖率由该像素矩形与多边形并集的交面积除以像素面积得到：
// 把矩形按所有边端点、边与边交点、边与矩形竖边的交点切成若干水平带，
// 每条带内的覆盖区间端点对 y 线性，梯形积分即可得到交面积的精确值。
func Rasterize(polys [][]geom.Point, w, h int) (*Coverage, error) {
	if w <= 0 || h <= 0 {
		return nil, errors.New("画幅必须为正")
	}
	if len(polys) == 0 {
		return nil, errors.New("至少需要一个多边形")
	}
	for _, p := range polys {
		if len(p) < 3 {
			return nil, errors.New("多边形至少三个顶点")
		}
	}
	minx, miny := polys[0][0].X, polys[0][0].Y
	maxx, maxy := minx, miny
	for _, p := range polys {
		for _, q := range p {
			minx = math.Min(minx, q.X)
			maxx = math.Max(maxx, q.X)
			miny = math.Min(miny, q.Y)
			maxy = math.Max(maxy, q.Y)
		}
	}
	spanx, spany := maxx-minx, maxy-miny
	if spanx == 0 {
		spanx = 1
	}
	if spany == 0 {
		spany = 1
	}
	cv := &Coverage{
		W: w, H: h,
		MinX: minx, MinY: miny,
		PixelW: spanx / float64(w),
		PixelH: spany / float64(h),
		Cov:    make([]float64, w*h),
	}
	edges := make([]covEdge, 0, 8)
	for pi, p := range polys {
		n := len(p)
		for i := 0; i < n; i++ {
			edges = append(edges, covEdge{a: p[i], b: p[(i+1)%n], poly: pi})
		}
	}
	for row := 0; row < h; row++ {
		y0 := miny + float64(row)*cv.PixelH
		y1 := y0 + cv.PixelH
		for col := 0; col < w; col++ {
			x0 := minx + float64(col)*cv.PixelW
			x1 := x0 + cv.PixelW
			v := cellCoverage(polys, edges, x0, y0, x1, y1)
			if v < 0 {
				v = 0
			}
			if v > 1 {
				v = 1
			}
			cv.Cov[row*w+col] = v
		}
	}
	return cv, nil
}

func cellCoverage(polys [][]geom.Point, all []covEdge, x0, y0, x1, y1 float64) float64 {
	local := make([]covEdge, 0, len(all))
	for _, e := range all {
		if e.a.Y == e.b.Y {
			continue // 水平边不产生横向交点
		}
		if math.Min(e.a.Y, e.b.Y) > y1 || math.Max(e.a.Y, e.b.Y) < y0 {
			continue
		}
		local = append(local, e)
	}
	if len(local) == 0 {
		if anyInside(polys, (x0+x1)/2, (y0+y1)/2) {
			return 1
		}
		return 0
	}
	ys := []float64{y0, y1}
	addY := func(y float64) {
		if y0 < y && y < y1 {
			ys = append(ys, y)
		}
	}
	for _, e := range local {
		addY(e.a.Y)
		addY(e.b.Y)
		// 边穿过矩形左右竖边的位置：裁剪后的区间长度在此处折线化。
		for _, x := range [2]float64{x0, x1} {
			if e.a.X == e.b.X {
				continue
			}
			t := (x - e.a.X) / (e.b.X - e.a.X)
			if t > 0 && t < 1 {
				addY(e.a.Y + t*(e.b.Y-e.a.Y))
			}
		}
	}
	for i := 0; i < len(local); i++ {
		for j := i + 1; j < len(local); j++ {
			if y, ok := crossY(local[i], local[j]); ok {
				addY(y)
			}
		}
	}
	sort.Float64s(ys)
	uniq := ys[:1]
	for _, y := range ys[1:] {
		last := uniq[len(uniq)-1]
		if math.Abs(y-last) > 1e-12*(1+math.Abs(y)) {
			uniq = append(uniq, y)
		}
	}
	area := 0.0
	for i := 0; i+1 < len(uniq); i++ {
		ya, yb := uniq[i], uniq[i+1]
		if yb <= ya {
			continue
		}
		la := bandLength(local, ya, x0, x1, true)
		lb := bandLength(local, yb, x0, x1, false)
		area += 0.5 * (la + lb) * (yb - ya)
	}
	cw, ch := x1-x0, y1-y0
	return area / (cw * ch)
}

type crossing struct {
	poly int
	x    float64
}

// bandLength 返回 y 高度处、被并集覆盖且落在 [x0,x1] 内的总长度。
// strict 为 true 时顶点等于 y 按“在上方”计，为 false 时按“在下方”计，
// 这样带的上下边界各取单侧极限，梯形积分不会重复计入顶点。
func bandLength(local []covEdge, y, x0, x1 float64, strict bool) float64 {
	crs := make([]crossing, 0, len(local))
	for _, e := range local {
		var aboveA, aboveB bool
		if strict {
			aboveA, aboveB = e.a.Y > y, e.b.Y > y
		} else {
			aboveA, aboveB = e.a.Y >= y, e.b.Y >= y
		}
		if aboveA == aboveB {
			continue
		}
		t := (y - e.a.Y) / (e.b.Y - e.a.Y)
		crs = append(crs, crossing{poly: e.poly, x: e.a.X + (e.b.X-e.a.X)*t})
	}
	sort.Slice(crs, func(i, j int) bool {
		if crs[i].poly != crs[j].poly {
			return crs[i].poly < crs[j].poly
		}
		return crs[i].x < crs[j].x
	})
	type iv struct{ lo, hi float64 }
	ivs := make([]iv, 0, len(crs)/2)
	for i := 0; i < len(crs); {
		j := i
		for j < len(crs) && crs[j].poly == crs[i].poly {
			j++
		}
		for k := i; k+1 < j; k += 2 {
			if crs[k+1].x > crs[k].x {
				ivs = append(ivs, iv{lo: crs[k].x, hi: crs[k+1].x})
			}
		}
		i = j
	}
	sort.Slice(ivs, func(i, j int) bool { return ivs[i].lo < ivs[j].lo })
	total := 0.0
	curLo, curHi, have := 0.0, 0.0, false
	for _, s := range ivs {
		lo := math.Max(s.lo, x0)
		hi := math.Min(s.hi, x1)
		if hi <= lo {
			continue
		}
		if !have {
			curLo, curHi, have = lo, hi, true
			continue
		}
		if lo <= curHi {
			if hi > curHi {
				curHi = hi
			}
			continue
		}
		total += curHi - curLo
		curLo, curHi = lo, hi
	}
	if have {
		total += curHi - curLo
	}
	return total
}

func crossY(e1, e2 covEdge) (float64, bool) {
	den := (e1.b.X-e1.a.X)*(e2.b.Y-e2.a.Y) - (e1.b.Y-e1.a.Y)*(e2.b.X-e2.a.X)
	if math.Abs(den) < 1e-15 {
		return 0, false
	}
	t := ((e2.a.X-e1.a.X)*(e2.b.Y-e2.a.Y) - (e2.a.Y-e1.a.Y)*(e2.b.X-e2.a.X)) / den
	u := ((e2.a.X-e1.a.X)*(e1.b.Y-e1.a.Y) - (e2.a.Y-e1.a.Y)*(e1.b.X-e1.a.X)) / den
	if t <= 0 || t >= 1 || u <= 0 || u >= 1 {
		return 0, false
	}
	return e1.a.Y + t*(e1.b.Y-e1.a.Y), true
}

func anyInside(polys [][]geom.Point, x, y float64) bool {
	for _, p := range polys {
		if geom.Inside(p, x, y) {
			return true
		}
	}
	return false
}

// PixelArea 返回单个像素在世界坐标中的面积。
func (cv *Coverage) PixelArea() float64 { return cv.PixelW * cv.PixelH }

// WeightedArea 返回按覆盖率加权的面积：Σ 覆盖率 × 像素面积。
func (cv *Coverage) WeightedArea() float64 {
	total := 0.0
	for _, v := range cv.Cov {
		total += v
	}
	return total * cv.PixelArea()
}

// Image 返回 8 位灰度图，像素值 = round(255 × 覆盖率)。
func (cv *Coverage) Image() *Image {
	im := &Image{W: cv.W, H: cv.H, Pix: make([]uint8, len(cv.Cov))}
	for i, v := range cv.Cov {
		im.Pix[i] = uint8(math.Round(math.Max(0, math.Min(1, v)) * 255))
	}
	return im
}

// BinaryImage 按阈值二值化：覆盖率 ≥ threshold 记 255，否则 0。
func (cv *Coverage) BinaryImage(threshold float64) *Image {
	im := &Image{W: cv.W, H: cv.H, Pix: make([]uint8, len(cv.Cov))}
	for i, v := range cv.Cov {
		if v >= threshold {
			im.Pix[i] = 255
		}
	}
	return im
}

// BinaryStats 汇总二值图与加权图的差异。
//
//	BinArea    二值面积 = 亮像素数 × 像素面积
//	L1         逐像素差之和 × 像素面积（差异的 L1 度量）
//	Bound      先验上界：max(t,1-t) × 像素面积 × 部分覆盖像素数（0 < 覆盖率 < 1）
//	Partial    部分覆盖像素数
//	Diff       二值面积与加权面积之差的绝对值
//
// 因 |BinArea − WeightedArea| ≤ L1 ≤ Bound，Bound 同时也是 Diff 的严格上界；
// 取 t=0.5 时 Bound = 0.5 × 像素面积 × 部分覆盖像素数。
func (cv *Coverage) BinaryStats(threshold float64) (binArea, l1, bound, diff float64, partial int) {
	if threshold < 0 {
		threshold = 0
	}
	if threshold > 1 {
		threshold = 1
	}
	px := cv.PixelArea()
	for _, v := range cv.Cov {
		var bin float64
		if v >= threshold {
			bin = 1
		}
		binArea += bin
		l1 += math.Abs(bin - v)
		if v > 0 && v < 1 {
			partial++
		}
	}
	binArea *= px
	l1 *= px
	diff = math.Abs(binArea - cv.WeightedArea())
	worst := math.Max(threshold, 1-threshold)
	bound = worst * px * float64(partial)
	return
}
