package sampling

import (
	"math"
	"math/rand"

	"vecgo/geom"
)

// StarL2Squared 返回二维点集的星号 L2 偏差平方 D₂(P,*)²：
//
//	D₂² = ∫_[0,1]² ( |P∩[0,t)|/n - vol([0,t)) )² dt
//
// 用 Warnock 闭式公式在 O(n²) 内精确求值（d=2 特例）：
//
//	D₂² = 1/9
//	  - (2/n) Σ_i Π_k (1-x_{ik}²)/2
//	  + (1/n²) Σ_{i,j} Π_k (1 - max(x_{ik},x_{jk}))
//
// 这正是 Cranley–Patterson 随机平移下，盒子族积分均方误差的逐点集版本。
func StarL2Squared(pts []geom.Point) float64 {
	n := float64(len(pts))
	term2 := 0.0
	for _, p := range pts {
		term2 += ((1 - p.X*p.X) / 2) * ((1 - p.Y*p.Y) / 2)
	}
	term1 := 0.0
	for _, a := range pts {
		for _, b := range pts {
			term1 += (1 - math.Max(a.X, b.X)) * (1 - math.Max(a.Y, b.Y))
		}
	}
	return 1.0/9.0 - 2.0/n*term2 + 1.0/(n*n)*term1
}

// MeanShiftedL2 对确定点集做 K 次独立 Cranley–Patterson 随机平移
// （各坐标 mod 1），返回平移后 D₂² 的均值。平移保证估计量无偏，
// 而 D₂² 的量级直接给出盒子族上的均方误差。
func MeanShiftedL2(base []geom.Point, shifts int, rng *rand.Rand) float64 {
	shifted := make([]geom.Point, len(base))
	var sum float64
	for k := 0; k < shifts; k++ {
		ux, uy := rng.Float64(), rng.Float64()
		for i, p := range base {
			shifted[i] = geom.Point{
				X: math.Mod(p.X+ux, 1),
				Y: math.Mod(p.Y+uy, 1),
			}
		}
		sum += StarL2Squared(shifted)
	}
	return sum / float64(shifts)
}
