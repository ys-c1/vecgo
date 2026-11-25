// Package sampling 提供像素覆盖率估计所需的采样点集与测量工具。
//
// 三种采样方式：
//   - IID：独立均匀随机采样，命中数服从二项分布，是 Cramér–Rao 下界的参照；
//   - Grid：规则中点网格（布局与 raster.Fill 中 (si+0.5)/m 一致），
//     每次采样做 Cranley–Patterson 随机平移以保证严格无偏；
//   - Halton：(2,3) Halton 低差异序列，同样配合 Cranley–Patterson 随机平移。
package sampling

import (
	"math"
	"math/rand"

	"vecgo/geom"
)

// Sampler 产生单位正方形 [0,1)^2 内的 n 个采样点。
// 所有实现的单次点集期望都等于均匀测度，即 E[P̂] = 覆盖率真值，
// 区别只在点间相关性（方差与偏差结构）。
type Sampler interface {
	Name() string
	Sample(n int, rng *rand.Rand) []geom.Point
}

// IID 独立均匀随机采样。
type IID struct{}

func (IID) Name() string { return "iid-随机" }

func (IID) Sample(n int, rng *rand.Rand) []geom.Point {
	pts := make([]geom.Point, n)
	for i := range pts {
		pts[i] = geom.Point{X: rng.Float64(), Y: rng.Float64()}
	}
	return pts
}

// GridBase 返回确定的规则中点网格，n 必须为完全平方数。
// 布局与 raster.Fill 的 supersample 网格逐点相同。
func GridBase(n int) []geom.Point {
	m := int(math.Round(math.Sqrt(float64(n))))
	if m*m != n {
		panic("sampling: 网格采样要求 n 为完全平方数")
	}
	pts := make([]geom.Point, 0, n)
	for j := 0; j < m; j++ {
		for i := 0; i < m; i++ {
			pts = append(pts, geom.Point{
				X: (float64(i) + 0.5) / float64(m),
				Y: (float64(j) + 0.5) / float64(m),
			})
		}
	}
	return pts
}

// Grid 规则中点网格 + Cranley–Patterson 随机平移（坐标 mod 1）。
type Grid struct{}

func (Grid) Name() string { return "网格" }

func (Grid) Sample(n int, rng *rand.Rand) []geom.Point {
	return cranleyPatterson(GridBase(n), rng)
}

// HaltonBase 返回确定的 (2,3) Halton 点集，取第 1..n 项（跳过原点项）。
func HaltonBase(n int) []geom.Point {
	pts := make([]geom.Point, n)
	for i := 0; i < n; i++ {
		pts[i] = geom.Point{X: radicalInverse(2, i+1), Y: radicalInverse(3, i+1)}
	}
	return pts
}

// Halton (2,3) Halton 低差异序列 + Cranley–Patterson 随机平移。
type Halton struct{}

func (Halton) Name() string { return "Halton-低差异" }

func (Halton) Sample(n int, rng *rand.Rand) []geom.Point {
	return cranleyPatterson(HaltonBase(n), rng)
}

// cranleyPatterson 对点集施加一次独立均匀随机平移（环面 mod 1）。
// 平移后每个点的边缘分布仍是 [0,1)² 上的均匀分布，因此对任意
// 可积指示函数，估计量逐形状严格无偏；点间相关性结构保持不变。
func cranleyPatterson(base []geom.Point, rng *rand.Rand) []geom.Point {
	ux, uy := rng.Float64(), rng.Float64()
	pts := make([]geom.Point, len(base))
	for i, p := range base {
		pts[i] = geom.Point{
			X: math.Mod(p.X+ux, 1),
			Y: math.Mod(p.Y+uy, 1),
		}
	}
	return pts
}

// radicalInverse 返回整数 i 在基数 base 下的根式逆序（Van der Corput 序列分量）。
func radicalInverse(base, i int) float64 {
	f, inv, v := 1.0, 1.0/float64(base), 0.0
	for i > 0 {
		f *= inv
		v += f * float64(i%base)
		i /= base
	}
	return v
}
