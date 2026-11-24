package raster

import (
	"math"

	"vecgo/geom"
)

// EdgePhase 是对照实验中边缘相对像素中心的固定有向偏移（像素单位）。
// 不取 0：真实图元的边几乎不会恰好穿过像素中心；一般相位下均匀网格的
// 覆盖率会随平移在相邻量化档之间跳变，这正是“排得太整齐”的方差来源。
const EdgePhase = 0.073

// VarianceRow 是一个“角度×样本数”格子的对照结果。
type VarianceRow struct {
	AngleDeg     float64 // 边缘倾角（相对 x 轴，度）
	Samples      int     // 每像素样本数 N
	Trials       int     // 重复出图次数
	Seed         int64   // 该格子随机流的派生种子（主种子 VarianceSeed）
	GridVariance float64 // 均匀网格 + CP 平移的覆盖率估计方差
	LDVariance   float64 // Hammersley + CP 平移的覆盖率估计方差
	GridMean     float64 // 网格估计均值（CP 平移下应接近真实覆盖率）
	LDMean       float64 // 低差异估计均值
}

// Ratio 返回 LD 方差 / 网格方差；小于 1 即低差异更稳。
func (r VarianceRow) Ratio() float64 { return r.LDVariance / r.GridVariance }

// edgePolygon 构造单位像素 [0,1]² 内的半平面 n·(p-c) <= phase，
// 用一个向内侧延伸 L 的凸四边形表示，以便直接走 geom.Inside 判定。
// alphaDeg 是边缘走向与 x 轴的夹角；法向 n=(-sin α, cos α)。
func edgePolygon(alphaDeg, phase, length float64) []geom.Point {
	alpha := alphaDeg * math.Pi / 180
	dx, dy := math.Cos(alpha), math.Sin(alpha) // 边缘切向
	nx, ny := -dy, dx                          // 指向“外侧”的法向
	cx, cy := 0.5, 0.5
	bx, by := cx+nx*phase, cy+ny*phase // 边缘上的基点
	return []geom.Point{
		{X: bx + dx*length, Y: by + dy*length},
		{X: bx - dx*length, Y: by - dy*length},
		{X: bx - dx*length - nx*length, Y: by - dy*length - ny*length},
		{X: bx + dx*length - nx*length, Y: by + dy*length - ny*length},
	}
}

// coverageOnce 用指定分布与 CP 平移估计一个像素的覆盖率。
func coverageOnce(poly []geom.Point, kind SampleKind, n int, ux, uy float64) float64 {
	m := 0
	if kind == SampleGrid {
		m = int(math.Sqrt(float64(n)))
	}
	hit := 0
	for i := 0; i < n; i++ {
		var sx, sy float64
		if kind == SampleGrid {
			sx, sy = GridSample(m, i/m, i%m)
		} else {
			sx, sy = HammersleySample(n, i)
		}
		sx, sy = ShiftCP(sx, sy, ux, uy)
		if geom.Inside(poly, sx, sy) {
			hit++
		}
	}
	return float64(hit) / float64(n)
}

// welford 累积样本方差（数值稳定的单遍算法）。
type welford struct {
	n    int
	mean float64
	m2   float64
}

func (w *welford) add(x float64) {
	w.n++
	d := x - w.mean
	w.mean += d / float64(w.n)
	w.m2 += d * (x - w.mean)
}

// variance 返回无偏样本方差（ddof=1）。
func (w *welford) variance() float64 {
	if w.n < 2 {
		return 0
	}
	return w.m2 / float64(w.n-1)
}

// CoverageVarianceTable 用固定种子的可复现实验，给出三种角度、两档样本数下
// “均匀 m×m 网格”与“Hammersley 低差异点集”的覆盖率估计方差对照。
//
// 每个 trial 从同一个派生随机流取一对 CP 平移 (ux,uy)，两种分布共用
// （公共随机数），随后各自估计同一条边的像素覆盖率；方差在全部 trial 上计算。
func CoverageVarianceTable() []VarianceRow {
	rows := make([]VarianceRow, 0, len(VarianceAngles)*len(VarianceCounts))
	for ai, ang := range VarianceAngles {
		poly := edgePolygon(ang, EdgePhase, 4)
		for ci, n := range VarianceCounts {
			seed := deriveSeed(VarianceSeed, uint64(ai), uint64(ci))
			rng := newSplitMix64(seed)
			var sg, sl welford
			for t := 0; t < VarianceTrials; t++ {
				ux, uy := rng.float64(), rng.float64()
				sg.add(coverageOnce(poly, SampleGrid, n, ux, uy))
				sl.add(coverageOnce(poly, SampleLowDiscrepancy, n, ux, uy))
			}
			rows = append(rows, VarianceRow{
				AngleDeg:     ang,
				Samples:      n,
				Trials:       VarianceTrials,
				Seed:         seed,
				GridVariance: sg.variance(),
				LDVariance:   sl.variance(),
				GridMean:     sg.mean,
				LDMean:       sl.mean,
			})
		}
	}
	return rows
}
