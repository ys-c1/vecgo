package coverage

import (
	"math"
	"math/rand"

	"vecgo/geom"
	"vecgo/raster"
)

// TrialSampler 生成“一次覆盖率估计”所用的 n 个样本点。Begin 在每次估计
// 开始时调用一次（聚类采样可借此为本轮选择一个簇中心），随后 Point 被调
// 用 n 次，返回画幅像素坐标系内的点 (x,y)。
type TrialSampler interface {
	Name() string
	Begin(rng *rand.Rand, w, h float64)
	Point(rng *rand.Rand) (x, y float64)
}

// Uniform 是标准 iid 均匀采样：每个点独立均匀分布在整个画幅矩形内，
// 这是二项分布结论严格成立所要求的采样方式。
type Uniform struct {
	w, h float64
}

// Name 返回采样器标识。
func (u *Uniform) Name() string { return "iid-uniform" }

// Begin 记录本轮画幅尺寸；均匀采样不需要轮级状态。
func (u *Uniform) Begin(_ *rand.Rand, w, h float64) { u.w, u.h = w, h }

// Point 返回画幅内一个独立均匀点。
func (u *Uniform) Point(rng *rand.Rand) (float64, float64) {
	return rng.Float64() * u.w, rng.Float64() * u.h
}

// Cluster 是故意构造的“分布不均”采样器，用作阳性对照：每轮先在画幅
// 中央区域选一个簇中心，本轮 n 个点全部从该中心附近的矩形抖动中取出。
// 单轮内点强正相关、轮与轮之间簇中心移动，会使重复估计的方差显著超过
// 二项预测上界。
type Cluster struct {
	// CentralFrac 控制簇中心的活动范围占画幅比例（0.5 表示中央 50%）。
	CentralFrac float64
	// RadiusFrac 是每轮抖动矩形的半边占画幅比例。
	RadiusFrac float64
	w, h       float64
	cx, cy     float64
}

// Name 返回采样器标识。
func (c *Cluster) Name() string { return "cluster-positive-control" }

// Begin 为本轮选择簇中心。
func (c *Cluster) Begin(rng *rand.Rand, w, h float64) {
	c.w, c.h = w, h
	frac := c.CentralFrac
	if frac <= 0 {
		frac = 0.5
	}
	c.cx = (0.5 + (rng.Float64()-0.5)*frac) * w
	c.cy = (0.5 + (rng.Float64()-0.5)*frac) * h
}

// Point 返回簇中心附近的一个点。默认参数下中心位于中央 50% 区域、
// 抖动半边为画幅的 12%，因此所有点都落在画幅内的中部条带。
func (c *Cluster) Point(rng *rand.Rand) (float64, float64) {
	rad := c.RadiusFrac
	if rad <= 0 {
		rad = 0.12
	}
	ox := (2*rng.Float64() - 1) * rad * c.w
	oy := (2*rng.Float64() - 1) * rad * c.h
	x := math.Min(c.w, math.Max(0, c.cx+ox))
	y := math.Min(c.h, math.Max(0, c.cy+oy))
	return x, y
}

// Estimate 用 n 个 iid 均匀样本估计画幅内被任意多边形覆盖的比例，
// 返回 p̂=hits/n 与命中数。
func Estimate(polys [][]geom.Point, w, h, n int, rng *rand.Rand) (hat float64, hits int) {
	return EstimateTrial(polys, w, h, n, rng, &Uniform{})
}

// EstimateTrial 与 Estimate 相同，但允许注入 TrialSampler。
func EstimateTrial(polys [][]geom.Point, w, h, n int, rng *rand.Rand, trial TrialSampler) (float64, int) {
	if n <= 0 {
		panic("coverage: n 必须为正")
	}
	m := raster.NewMapping(polys)
	wf, hf := float64(w), float64(h)
	trial.Begin(rng, wf, hf)
	hits := 0
	for k := 0; k < n; k++ {
		x, y := trial.Point(rng)
		px, py := m.PixelToWorld(w, h, x, y)
		for _, p := range polys {
			if geom.Inside(p, px, py) {
				hits++
				break
			}
		}
	}
	return float64(hits) / float64(n), hits
}
