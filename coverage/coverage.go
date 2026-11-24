// Package coverage 给出直边裁剪下像素覆盖率关于裁剪位置的闭式函数，
// 以及与规则超采样对照所需的参考实现和加权误差工具。
//
// 坐标约定：像素 col 覆盖 x 区间 [col, col+1]，row 覆盖 y 区间
// [row, row+1]；采样约定与 raster.Image.Fill 一致，使用中点采样
// (i+0.5)/n。
package coverage

import (
	"math"

	"vecgo/geom"
)

// Clamp01 把标量截断到 [0,1]。
func Clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// VerticalFraction 是竖直直边 x=c、保留 x<=c 一侧时，矩形区间
// [x0,x1] 上的覆盖宽度分数（与 y 无关）。
//
// 闭式函数：
//
//	alpha(c) = clamp((c-x0)/(x1-x0), 0, 1)
//
// 三段分别对应完全未覆盖、边界穿过像素、完全覆盖。
func VerticalFraction(c, x0, x1 float64) float64 {
	w := x1 - x0
	if w <= 0 {
		return 0
	}
	return Clamp01((c - x0) / w)
}

// HorizontalFraction 是水平直边 y=c、保留 y<=c 一侧时，区间 [y0,y1]
// 上的覆盖高度分数。闭式同 VerticalFraction。
func HorizontalFraction(c, y0, y1 float64) float64 {
	h := y1 - y0
	if h <= 0 {
		return 0
	}
	return Clamp01((c - y0) / h)
}

// UnitVertical 是单位像素 [i,i+1] 被竖直边 x=c（保留左侧）覆盖的分数：
//
//	alpha_i(c) = clamp(c-i, 0, 1)
//
// 当 c 与 i 均为整数时，c-i 为整数，截断结果只能是 0 或 1。
func UnitVertical(c float64, i int) float64 {
	return Clamp01(c - float64(i))
}

// UnitHorizontal 是单位像素 [j,j+1] 被水平边 y=c（保留 y<=c 侧）覆盖的分数。
func UnitHorizontal(c float64, j int) float64 {
	return Clamp01(c - float64(j))
}

// VerticalArea 返回矩形 R=[x0,x1]×[y0,y1] 被 x<=c 覆盖的面积 A_R(c)，
// 量纲为长度平方：
//
//	A_R(c) = (y1-y0) * clamp(c-x0, 0, x1-x0)
func VerticalArea(c, x0, y0, x1, y1 float64) float64 {
	h := y1 - y0
	if h < 0 {
		h = -h
	}
	return h * math.Min(math.Max(c-x0, 0), x1-x0)
}

// VerticalDensity 是已覆盖面积关于裁剪位置的导数 dA_R/dc，即裁剪线
// 在像素内的截面长度 rho_R(c)，量纲为长度：
//
//	rho_R(c) = (y1-y0) * 1_[x0,x1](c)   （几乎处处）
//
// 端点取 0 还是 h 不影响积分。按左闭右开约定返回 h。
// 对位置积分即得像素面积：
//
//	int_-inf^+inf rho_R(c) dc = (y1-y0)(x1-x0) = Area(R)
func VerticalDensity(c, x0, y0, x1, y1 float64) float64 {
	if c < x0 || c >= x1 {
		return 0
	}
	return y1 - y0
}

// Gray 把 [0,1] 覆盖率按四舍五入映射到 8 位灰度 [0,255]。
// 新的解析路径统一使用这一映射，避免与旧路径的整数截断混淆。
func Gray(alpha float64) uint8 {
	return uint8(math.Round(255 * Clamp01(alpha)))
}

// SuperSampleVertical 在矩形 R 内做 n×n 中点规则超采样，统计满足
// x<=c 的样本数。返回命中数、总样本数与覆盖率 hits/total。
//
// 采样位置与 raster.Image.Fill 相同：
//
//	px = x0 + (x1-x0)*(si+0.5)/n
//	py = y0 + (y1-y0)*(sj+0.5)/n
func SuperSampleVertical(c, x0, y0, x1, y1 float64, n int) (hits, total int, fraction float64) {
	if n < 1 {
		n = 1
	}
	w, h := x1-x0, y1-y0
	for sj := 0; sj < n; sj++ {
		py := y0 + h*(float64(sj)+0.5)/float64(n)
		for si := 0; si < n; si++ {
			px := x0 + w*(float64(si)+0.5)/float64(n)
			if px <= c && py >= y0 && py <= y1 {
				hits++
			}
		}
	}
	total = n * n
	return hits, total, float64(hits) / float64(total)
}

// SuperSampleHalfPlane 在矩形 R 内做 n×n 中点超采样，统计满足
// nx*x+ny*y <= d 的样本比例，作为任意方向直边的超采样对照。
func SuperSampleHalfPlane(x0, y0, x1, y1, nx, ny, d float64, n int) (hits, total int, fraction float64) {
	if n < 1 {
		n = 1
	}
	w, h := x1-x0, y1-y0
	for sj := 0; sj < n; sj++ {
		py := y0 + h*(float64(sj)+0.5)/float64(n)
		for si := 0; si < n; si++ {
			px := x0 + w*(float64(si)+0.5)/float64(n)
			if nx*px+ny*py <= d {
				hits++
			}
		}
	}
	total = n * n
	return hits, total, float64(hits) / float64(total)
}

// HalfPlaneArea 精确计算凸多边形 pixel 被半平面 nx*x+ny*y <= d
// 覆盖的面积。做法是单次 Sutherland-Hodgman 直边裁剪后用鞋带公式，
// 等价于任意方向直边覆盖率的分段二次闭式解的精确数值实现。
//
// 对轴对齐直边应优先使用 VerticalArea / HorizontalFraction。
func HalfPlaneArea(pixel []geom.Point, nx, ny, d float64) float64 {
	if len(pixel) < 3 {
		return 0
	}
	val := func(p geom.Point) float64 { return nx*p.X + ny*p.Y - d }
	out := make([]geom.Point, 0, len(pixel)+1)
	for i := range pixel {
		cur := pixel[i]
		prv := pixel[(i-1+len(pixel))%len(pixel)]
		fc, fp := val(cur), val(prv)
		ci, pi := fc <= 0, fp <= 0
		switch {
		case ci && pi:
			out = append(out, cur)
		case ci && !pi:
			out = append(out, halfPlaneIntersect(prv, cur, fp, fc), cur)
		case !ci && pi:
			out = append(out, halfPlaneIntersect(prv, cur, fp, fc))
		}
	}
	if len(out) < 3 {
		return 0
	}
	twice := 0.0
	for i := range out {
		a := out[i]
		b := out[(i+1)%len(out)]
		twice += a.X*b.Y - a.Y*b.X
	}
	return math.Abs(twice) / 2
}

// halfPlaneIntersect 求线段 prv->cur 与 f(p)=0 的交点，
// 其中 fp=f(prv)、fc=f(cur)。
func halfPlaneIntersect(prv, cur geom.Point, fp, fc float64) geom.Point {
	t := fp / (fp - fc)
	return geom.Point{
		X: prv.X + (cur.X-prv.X)*t,
		Y: prv.Y + (cur.Y-prv.Y)*t,
	}
}
