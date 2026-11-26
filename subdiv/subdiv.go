// Package subdiv 实现三角形的中点（medial）细分、坐标分辨率量化、
// 面积误差上界以及固定坐标映射下的覆盖统计。
//
// 设计约定：1 个坐标单位 = 1 个像素（coverage 光栅使用 [0,W]×[0,H]
// 的固定映射，不随包围盒缩放）。坐标网格分辨率 ρ（resolution，单位 px）
// 表示相邻可表示坐标的间距；四舍五入到最近网格的单坐标误差上限为 ρ/2。
package subdiv

import (
	"math"

	"vecgo/geom"
	"vecgo/raster"
)

// Triangle 是三个顶点组成的三角形，顶点顺序决定有向面积符号。
type Triangle [3]geom.Point

// SnapCoord 把 x 量化到分辨率为 res 的坐标网格上（四舍五入，半数向上）。
// 对非负坐标与 math.Round(x/res)*res 完全一致；显式 +0.5 便于和
// 参照脚本逐位对齐。误差满足 |SnapCoord(x,res)-x| <= res/2。
func SnapCoord(x, res float64) float64 {
	if res <= 0 {
		return x
	}
	return math.Floor(x/res+0.5) * res
}

// MidChildren 返回中点细分的四个子三角形，全部使用未取整的精确中点。
// 顶点按原三角形的方向排列，四个子三角形与父三角形同向。
func MidChildren(t Triangle) [4]Triangle {
	a, b, c := t[0], t[1], t[2]
	mab := geom.Point{X: (a.X + b.X) / 2, Y: (a.Y + b.Y) / 2}
	mbc := geom.Point{X: (b.X + c.X) / 2, Y: (b.Y + c.Y) / 2}
	mca := geom.Point{X: (c.X + a.X) / 2, Y: (c.Y + a.Y) / 2}
	return [4]Triangle{
		{a, mab, mca},
		{mab, b, mbc},
		{mca, mbc, c},
		{mab, mbc, mca},
	}
}

// Subdivide 返回精确中点细分 n 次后的 4^n 个三角形。
// n=0 时返回原三角形的单元素切片。
func Subdivide(t Triangle, n int) []Triangle {
	tris := []Triangle{t}
	for k := 0; k < n; k++ {
		next := make([]Triangle, 0, len(tris)*4)
		for _, p := range tris {
			ch := MidChildren(p)
			next = append(next, ch[0], ch[1], ch[2], ch[3])
		}
		tris = next
	}
	return tris
}

// SubdivideRounded 复现“每算出一级中点就立刻按 res 取整并存回”的递归流程，
// 用于解释粗分辨率下观察到的大面积漂移。主结论使用 Subdivide + SnapAll
// （高精度细分、输出时按坐标分辨率量化一次）。
func SubdivideRounded(t Triangle, n int, res float64) []Triangle {
	tris := []Triangle{SnapTriangle(t, res)}
	for k := 0; k < n; k++ {
		next := make([]Triangle, 0, len(tris)*4)
		for _, p := range tris {
			ch := MidChildren(p)
			for _, q := range ch {
				next = append(next, SnapTriangle(q, res))
			}
		}
		tris = next
	}
	return tris
}

// SnapTriangle 量化三角形的三个顶点。
func SnapTriangle(t Triangle, res float64) Triangle {
	return Triangle{
		{X: SnapCoord(t[0].X, res), Y: SnapCoord(t[0].Y, res)},
		{X: SnapCoord(t[1].X, res), Y: SnapCoord(t[1].Y, res)},
		{X: SnapCoord(t[2].X, res), Y: SnapCoord(t[2].Y, res)},
	}
}

// SnapAll 批量量化三角形顶点。
func SnapAll(tris []Triangle, res float64) []Triangle {
	out := make([]Triangle, len(tris))
	for i, t := range tris {
		out[i] = SnapTriangle(t, res)
	}
	return out
}

// Area2 是有向两倍面积（叉积），符号表示绕序。
func Area2(t Triangle) float64 {
	uX := t[1].X - t[0].X
	uY := t[1].Y - t[0].Y
	vX := t[2].X - t[0].X
	vY := t[2].Y - t[0].Y
	return uX*vY - uY*vX
}

// Area 返回无符号面积（坐标单位的平方；坐标以 px 计时单位为 px²）。
func Area(t Triangle) float64 {
	return math.Abs(Area2(t)) / 2
}

// TotalArea 求一组三角形的面积之和。
func TotalArea(tris []Triangle) float64 {
	s := 0.0
	for _, t := range tris {
		s += Area(t)
	}
	return s
}

// LongestEdge 返回三角形最长边的欧氏长度。
func LongestEdge(t Triangle) float64 {
	d01 := math.Hypot(t[1].X-t[0].X, t[1].Y-t[0].Y)
	d12 := math.Hypot(t[2].X-t[1].X, t[2].Y-t[1].Y)
	d20 := math.Hypot(t[0].X-t[2].X, t[0].Y-t[2].Y)
	return math.Max(d01, math.Max(d12, d20))
}

// MaxLongestEdge 返回所有三角形中的最大边长。
func MaxLongestEdge(tris []Triangle) float64 {
	l := 0.0
	for _, t := range tris {
		if e := LongestEdge(t); e > l {
			l = e
		}
	}
	return l
}

// PerTriangleBound 是单个三角形坐标量化后的面积误差上界：
//
//	|ΔA| <= 3 * res * L
//
// 其中 res 是坐标网格分辨率 ρ，L 是该三角形最长边。
// 严格推导中还有 +ρ² 项：|ΔA| <= 2ρL + ρ²；当 ρ <= L 时
// ρ² <= ρL，故合并为 3ρL。调用方需保证 res <= L。
// 更紧的常数为 1+√2（约 2.414），这里取 3 留余量。
func PerTriangleBound(res, L float64) float64 {
	return 3 * res * L
}

// EdgeCap 返回量化网格下任意一级叶子最长边的统一保守上限。
// 精确中点边不超过初始最长边 L0；一个边的两个端点量化后，
// 每个端点的欧氏位移 ≤ ρ/√2，边长扰动 ≤ √2ρ < 2ρ，因此
//
//	L_leaf ≤ L0 + 2ρ = EdgeCap(L0, ρ)
//
// 对 n=0 与任意更深层都无条件成立。
func EdgeCap(L0, res float64) float64 {
	return L0 + 2*res
}

// TotalBound 是细分 n 次后全部 4^n 个叶子的面积误差总上界。
// 传入的 Lcap 应是所有叶子最长边的统一上限（用 EdgeCap(L0,res)）：
//
//	B(n) = 3 * res * Lcap * 4^n = B(0) * 4^n。
func TotalBound(n int, res, Lcap float64) float64 {
	return PerTriangleBound(res, Lcap) * math.Pow(4, float64(n))
}

// AcceptableN 返回满足 TotalBound(n,res,L0) <= budget 的最大非负细分次数。
// 若连 n=0 都不满足，返回 -1（预算比单三角形上界还小）。
func AcceptableN(res, L0, budget float64) int {
	if TotalBound(0, res, L0) > budget {
		return -1
	}
	n := 0
	for TotalBound(n+1, res, L0) <= budget {
		n++
	}
	return n
}

// Coverage 使用固定的坐标→像素 1:1 映射生成覆盖灰度图：
// 像素 (col,row) 覆盖坐标矩形 [col,col+1)×[row,row+1)，
// 内部做 ss×ss 超采样，灰度 = 255 * 命中子样本数 / ss²。
// 同一映射用于所有细分层级，因此覆盖差异只可能来自几何/量化。
func Coverage(tris []Triangle, w, h, ss int) (*raster.Image, error) {
	im, err := raster.New(w, h)
	if err != nil {
		return nil, err
	}
	if ss < 1 {
		ss = 1
	}
	den := ss * ss
	poly := make([]geom.Point, 3)
	for row := 0; row < h; row++ {
		for col := 0; col < w; col++ {
			hit := 0
			for sj := 0; sj < ss; sj++ {
				for si := 0; si < ss; si++ {
					px := float64(col) + (float64(si)+0.5)/float64(ss)
					py := float64(row) + (float64(sj)+0.5)/float64(ss)
					for _, t := range tris {
						poly[0], poly[1], poly[2] = t[0], t[1], t[2]
						if geom.Inside(poly, px, py) {
							hit++
							break
						}
					}
				}
			}
			im.Pix[row*w+col] = uint8(255 * hit / den)
		}
	}
	return im, nil
}

// BitmapDifferentPixels 返回两张同尺寸灰度图中像素值不同的格数。
func BitmapDifferentPixels(a, b *raster.Image) int {
	if a.W != b.W || a.H != b.H || len(a.Pix) != len(b.Pix) {
		return -1
	}
	d := 0
	for i := range a.Pix {
		if a.Pix[i] != b.Pix[i] {
			d++
		}
	}
	return d
}
