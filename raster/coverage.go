package raster

import (
	"errors"
	"math"

	"vecgo/geom"
)

// Map 是几何坐标到像素坐标的仿射映射，与 Image.Fill 内部使用的
// 包围盒映射完全一致：像素格 (col,row) 对应几何区间
// [MinX+col*SX, MinX+(col+1)*SX] × [MinY+row*SY, MinY+(row+1)*SY]。
type Map struct {
	MinX, MinY float64
	SX, SY     float64 // 单个像素在几何坐标中的宽、高
}

// BoundsMap 按所有多边形的联合包围盒构造映射；零跨度保护沿用 Fill 的约定。
func BoundsMap(polys [][]geom.Point, w, h int) (Map, error) {
	if w <= 0 || h <= 0 {
		return Map{}, errors.New("画幅必须为正")
	}
	if len(polys) == 0 || len(polys[0]) == 0 {
		return Map{}, errors.New("至少需要一个多边形")
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
	return Map{MinX: minx, MinY: miny, SX: spanx / float64(w), SY: spany / float64(h)}, nil
}

// ToPixel 把几何点换算到像素坐标。
func (m Map) ToPixel(p geom.Point) geom.Point {
	return geom.Point{X: (p.X - m.MinX) / m.SX, Y: (p.Y - m.MinY) / m.SY}
}

// MapPoly 把整条多边形换算到像素坐标。
func (m Map) MapPoly(poly []geom.Point) []geom.Point {
	out := make([]geom.Point, len(poly))
	for i, p := range poly {
		out[i] = m.ToPixel(p)
	}
	return out
}

// PixelRect 返回像素 (col,row) 的单位正方形，顶点逆时针：
// 上边 y=row+1、下边 y=row、左 x=col、右 x=col+1。
func PixelRect(col, row int) []geom.Point {
	c, r := float64(col), float64(row)
	return []geom.Point{
		{X: c, Y: r},
		{X: c + 1, Y: r},
		{X: c + 1, Y: r + 1},
		{X: c, Y: r + 1},
	}
}

// ClipCoveragePx 对“已经处于像素坐标”的多边形做面积裁切，返回该像素的
// 覆盖率（单位正方形上即交叠面积）以及裁切后保留的多边形顶点，便于核账。
//
// 面积裁切三步：
//  1. 以像素四边为裁剪窗，用 Sutherland-Hodgman 逐半平面裁切多边形；
//  2. 裁后顶点走鞋带公式 A=1/2|Σ(x_i y_{i+1}-x_{i+1} y_i)|；
//  3. 像素面积为 1，覆盖率 c=clamp(A,0,1)。
//
// 交叠判断：裁剪边 A→B 逆时针定向，点 P 在窗内当且仅当叉积
// cross(B-A, P-A)=(Bx-Ax)(Py-Ay)-(By-Ay)(Px-Ax) >= 0（左侧半平面）；
// 当前点与前一点一内一外时，求线段与裁剪直线的交点并插入。
func ClipCoveragePx(polyPx []geom.Point, col, row int) (coverage float64, clipped []geom.Point, err error) {
	if len(polyPx) < 3 {
		return 0, nil, errors.New("多边形至少三个顶点")
	}
	clipped, err = geom.Clip(polyPx, PixelRect(col, row))
	if err != nil {
		return 0, nil, err
	}
	area := geom.Area(clipped)
	// 浮点误差可能给出 -1e-16 或 1+1e-16，收进 [0,1]。
	coverage = math.Min(1, math.Max(0, area))
	return coverage, clipped, nil
}

// SubsampleRatioPx 复刻 Image.Fill 的规则子点计数：ss×ss 网格、子点位于
// 各子格中心，返回命中比例 hit/(ss*ss)。ss 必须为正。
func SubsampleRatioPx(polyPx []geom.Point, col, row, ss int) (float64, int) {
	hit := 0
	for sj := 0; sj < ss; sj++ {
		for si := 0; si < ss; si++ {
			x := float64(col) + (float64(si)+0.5)/float64(ss)
			y := float64(row) + (float64(sj)+0.5)/float64(ss)
			if geom.Inside(polyPx, x, y) {
				hit++
			}
		}
	}
	return float64(hit) / float64(ss*ss), hit
}

// FillCoverage 用解析面积覆盖率填充单个多边形（多个多边形的精确并集面积
// 未实现；对比验证场景均为单个多边形）。灰度量化与 Fill 完全一致，采用
// 截断 uint8(255*c)（因此半覆盖为 127 而不是 128）。与 Fill 相同，
// 多边形包围盒被映射到整幅图像。
func (im *Image) FillCoverage(poly []geom.Point) error {
	m, err := BoundsMap([][]geom.Point{poly}, im.W, im.H)
	if err != nil {
		return err
	}
	polyPx := m.MapPoly(poly)
	for row := 0; row < im.H; row++ {
		for col := 0; col < im.W; col++ {
			c, _, err := ClipCoveragePx(polyPx, col, row)
			if err != nil {
				return err
			}
			im.Pix[row*im.W+col] = uint8(255 * c)
		}
	}
	return nil
}
