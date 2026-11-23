package sweep

import (
	"math"

	"vecgo/geom"
)

// lcg 是固定参数的线性同余生成器：同 seed 在任何机器、任何进程
// 产生完全相同的坐标序列，保证两档基准与哈希可复现。
type lcg struct{ state uint64 }

func (r *lcg) next() uint64 {
	// Numerical Recipes 常量。
	r.state = r.state*6364136223846793005 + 1442695040888963407
	return r.state
}

func (r *lcg) float64() float64 {
	// 取高 53 位映射到 [0,1)，不含 NaN/Inf。
	return float64(r.next()>>11) / float64(uint64(1)<<53)
}

// MakeFixture 生成 n 条确定性随机线段：起点与长度均由 seed 决定，
// 坐标限定在有限范围内。用于两档基准与稳定性测试。
func MakeFixture(n int, seed uint64) []Segment {
	r := &lcg{state: seed}
	out := make([]Segment, n)
	for i := range out {
		x0 := r.float64()*1000 - 500
		y0 := r.float64()*1000 - 500
		// 长度限制在 [2,120]，控制交点密度，避免极端退化。
		l := 2 + r.float64()*118
		ang := r.float64() * 2 * math.Pi
		x1 := x0 + l*math.Cos(ang)
		y1 := y0 + l*math.Sin(ang)
		out[i] = Segment{
			P0: geom.Point{X: x0, Y: y0},
			P1: geom.Point{X: x1, Y: y1},
		}
	}
	return out
}

// MakeExtremeFixture 生成极端退化档：一半是同一条水平线上首尾相接的
// 共线段（大量共端点/共线事件挤在同一坐标），另一半是几乎平行、
// 斜率差极小的斜线（强迫 treap 在近乎相同的 y 上反复比较）。
// 输入合法：无零长段、无整段重复、无 NaN。
func MakeExtremeFixture(n int, seed uint64) []Segment {
	r := &lcg{state: seed}
	out := make([]Segment, 0, n)
	half := n / 2
	// 共线首尾相接段：段 i 占 [i, i+1]，相邻段共享端点。
	for i := 0; i < half; i++ {
		out = append(out, Segment{
			P0: geom.Point{X: float64(i), Y: 0},
			P1: geom.Point{X: float64(i + 1), Y: 0},
		})
	}
	// 近乎平行的斜线：基准斜率 1，每条加上 1e-9*i 的扰动。
	for i := half; i < n; i++ {
		x0 := r.float64() * float64(half)
		m := 1 + float64(i-half)*1e-9
		out = append(out, Segment{
			P0: geom.Point{X: x0, Y: -500 + m*x0},
			P1: geom.Point{X: x0 + 10, Y: -500 + m*(x0+10)},
		})
	}
	return out
}
