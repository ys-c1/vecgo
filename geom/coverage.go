package geom

import (
	"errors"
	"math"
)

// SubpixelLevels 是光栅器顶点坐标的子像素精度：1/SubpixelLevels 像素。
// 32 位定点光栅器常用 1/256；这里取 1/64 作为保守的默认值。
const SubpixelLevels = 64

// quantPhases 是量化格子的四个相位偏移（以子像素为单位），
// 用它们取最坏情况，避免单相位恰好对齐造成误差被低估。
var quantPhases = [4]float64{0, 0.25, 0.5, 0.75}

// QuantErr 返回三角形顶点被量化到 1/levels 像素网格后，
// 面积相对误差的最大值（对 x、y 相位各取 4 个）。
//
// 若三个顶点各有不超过 δ 的位移、三角形周长为 P、面积为 A，则
//
//	|ΔA| ≤ δ·P/2
//
// 因此相对误差 |ΔA|/A ≤ δ·P/(2A)。细长三角形 P/A 大，
// 在固定子像素精度下覆盖率（面积）的数值误差也大——这正是
// “耳切产生的细长三角形让覆盖率算不准”的量化根源。
func (t Triangle) QuantErr(levels float64) float64 {
	a := t.Area()
	if a <= 0 {
		return 0
	}
	worst := 0.0
	for _, ox := range quantPhases {
		for _, oy := range quantPhases {
			q := Triangle{
				snapPoint(t[0], levels, ox, oy),
				snapPoint(t[1], levels, ox, oy),
				snapPoint(t[2], levels, ox, oy),
			}
			if e := math.Abs(q.Area() - a); e > worst {
				worst = e
			}
		}
	}
	return worst / a
}

func snapPoint(p Point, levels, ox, oy float64) Point {
	return Point{
		X: (math.Floor(p.X*levels-ox+0.5) + ox) / levels,
		Y: (math.Floor(p.Y*levels-oy+0.5) + oy) / levels,
	}
}

// CoverageError 返回整网格的覆盖率误差：各三角形量化面积误差按面积加权后
// 除以网格总面积。它把“覆盖率算不准”变成可比较的单一数值。
func CoverageError(tris []Triangle, levels float64) (float64, error) {
	if len(tris) == 0 {
		return 0, errors.New("空网格")
	}
	if levels <= 0 {
		return 0, errors.New("levels 必须为正")
	}
	total, acc := 0.0, 0.0
	for _, t := range tris {
		a := t.Area()
		if a <= 0 {
			return 0, errors.New("退化三角形")
		}
		acc += t.QuantErr(levels) * a
		total += a
	}
	return acc / total, nil
}

// WorstCoverageError 返回单个三角形上最大的覆盖率相对误差。
func WorstCoverageError(tris []Triangle, levels float64) float64 {
	worst := 0.0
	for _, t := range tris {
		if e := t.QuantErr(levels); e > worst {
			worst = e
		}
	}
	return worst
}

// WorstTriangle 返回面积比最小（最细长）的三角形。
func WorstTriangle(tris []Triangle) Triangle {
	best := math.Inf(1)
	var out Triangle
	for _, t := range tris {
		if q := t.Quality(); q < best {
			best, out = q, t
		}
	}
	return out
}
