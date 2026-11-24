package light

import "math"

// CoverageAt 是直边 x=c（保留 x≤c 一侧）穿过矩形像素区间 [x0,x1] 时的
// 一维覆盖率闭式函数，与 raster.Image.Fill 的覆盖率口径一致（面积分数）：
//
//	C(c;x0,x1) = clamp((c-x0)/(x1-x0), 0, 1)
//
// 三段分别是完全未覆盖、边界穿过像素、完全覆盖。
func CoverageAt(c, x0, x1 float64) float64 {
	if x1 <= x0 {
		return 0
	}
	v := (c - x0) / (x1 - x0)
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// UnitEdgeCoverage 是单位像素 [0,1] 被竖直边 x=c（保留左侧）覆盖的分数：
//
//	C(c)=clamp(c,0,1)。
//
// 当 c 取整数（0 或 1）时结果只能是 0 或 1，不会出现中间灰。
func UnitEdgeCoverage(c float64) float64 {
	return CoverageAt(c, 0, 1)
}

// CoveragePoint 是“覆盖率—位置”函数上的一个采样点。
type CoveragePoint struct {
	X        float64 `json:"x"`
	Coverage float64 `json:"coverage"`
	// GrayTrunc 按本仓 raster.Fill 的整数截断口径给 8 位灰：uint8(255*C)。
	// 注意 C=0.5 时为 127（255*0.5=127.5 被截断），不是四舍五入的 128。
	GrayTrunc uint8 `json:"gray_trunc"`
}

// SampleUnitEdge 在给定位置上取 CoveragePoint。
func SampleUnitEdge(xs ...float64) []CoveragePoint {
	out := make([]CoveragePoint, 0, len(xs))
	for _, x := range xs {
		c := UnitEdgeCoverage(x)
		out = append(out, CoveragePoint{
			X:         x,
			Coverage:  c,
			GrayTrunc: uint8(255 * c),
		})
	}
	return out
}

// SuperSampleUnitEdge 用 n×n 中点规则独立统计单位像素 [0,1]² 内 x≤c 的
// 命中率，采样位置与 raster.Image.Fill 完全相同：
//
//	px=(si+0.5)/n, py=(sj+0.5)/n
//
// 返回命中数、总样本数与覆盖率 hits/total，用来和闭式 C(c) 对照。
// 竖直边与 y 无关，n 为偶数且 c=0.5 时精确得到 0.5。
func SuperSampleUnitEdge(c float64, n int) (hits, total int, fraction float64) {
	if n < 1 {
		n = 1
	}
	for sj := 0; sj < n; sj++ {
		for si := 0; si < n; si++ {
			px := (float64(si) + 0.5) / float64(n)
			py := (float64(sj) + 0.5) / float64(n)
			_ = py
			if px <= c {
				hits++
			}
			total++
		}
	}
	fraction = float64(hits) / float64(total)
	return hits, total, fraction
}

// EffectiveWithCoverage 给出两灯在某像素位置、共享同一边覆盖率 C(x) 时的
// 有效线性强度与感知亮度，把“权重 × 覆盖率 × 位置”串起来：
//
//	E(x) = w * C(x) * (I1+I2)。
//
// w 取感知守恒解；C=1 时回到守恒输出，C<1 时整体按面积分数衰减。
func EffectiveWithCoverage(r Result, coverage float64) (linear, perceived float64) {
	coverage = math.Min(1, math.Max(0, coverage))
	linear = r.Weight * coverage * r.SumLinear
	perceived = Perceived(linear)
	return linear, perceived
}
