package geom

import "math"

// PixelFrame 是 FitToViewport 默认对齐的设备帧边长（像素）。
const PixelFrame = 64

// PixelSamples 是逐像素覆盖率对照所用的每轴超采样数（4×4=16 点/像素）。
const PixelSamples = 4

// PixelCoverageError 是直接的光栅化覆盖率误差：
// 在 frame×frame 的像素帧上，对每个三角形用 samples×samples 规则子像素
// 采样独立估计其覆盖面积，与 Shoelace 精确面积比较，累计
//
//	ε_pix = Σ_t |Â_t − A_t| / A(P)。
//
// 每个三角形独立计量，因此共享内边两侧的误差不会相互抵消——这正是
// “逐图元计算覆盖率”的光栅化路径。细长三角形跨越大量部分覆盖像素，
// 固定采样格对其覆盖率的量化更粗，故该指标对最差三角形的形状敏感。
//
// 坐标约定：三角形顶点已经过 FitToViewport(..., PixelFrame, ...) 变换，
// 1 个坐标单位 = 1 像素。
func PixelCoverageError(tris []Triangle, frame, samples int) float64 {
	var acc, total float64
	for _, t := range tris {
		a := t.Area()
		if a <= 0 {
			continue
		}
		acc += math.Abs(SampledTriangleArea(t, frame, samples) - a)
		total += a
	}
	if total == 0 {
		return 0
	}
	return acc / total
}

// SampledTriangleArea 用 samples×samples 子采样估计三角形覆盖的像素面积。
func SampledTriangleArea(t Triangle, frame, samples int) float64 {
	if samples < 1 {
		samples = 1
	}
	minx := math.Min(t[0].X, math.Min(t[1].X, t[2].X))
	maxx := math.Max(t[0].X, math.Max(t[1].X, t[2].X))
	miny := math.Min(t[0].Y, math.Min(t[1].Y, t[2].Y))
	maxy := math.Max(t[0].Y, math.Max(t[1].Y, t[2].Y))
	i0 := int(math.Floor(minx))
	i1 := int(math.Ceil(maxx))
	j0 := int(math.Floor(miny))
	j1 := int(math.Ceil(maxy))
	if i0 < 0 {
		i0 = 0
	}
	if j0 < 0 {
		j0 = 0
	}
	if i1 > frame-1 {
		i1 = frame - 1
	}
	if j1 > frame-1 {
		j1 = frame - 1
	}
	if i0 > i1 || j0 > j1 {
		return 0
	}
	hit := 0
	for j := j0; j <= j1; j++ {
		for i := i0; i <= i1; i++ {
			for sj := 0; sj < samples; sj++ {
				py := float64(j) + (float64(sj)+0.5)/float64(samples)
				for sx := 0; sx < samples; sx++ {
					px := float64(i) + (float64(sx)+0.5)/float64(samples)
					if pointInTriangleInclusive(Point{X: px, Y: py}, t[0], t[1], t[2]) {
						hit++
					}
				}
			}
		}
	}
	return float64(hit) / float64(samples*samples)
}
