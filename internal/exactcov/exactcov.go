// Package exactcov 提供与 geom.Clip+鞋带公式完全独立的精确覆盖率基准：
// 竖直板条积分。仅适用于凸多边形（验证用的旋转方块满足该条件）。
package exactcov

import (
	"math"
	"sort"

	"vecgo/geom"
)

// ConvexCoverage 计算凸多边形 poly 与轴对齐像素矩形 [x0,x1]×[y0,y1]
// 的交叠面积（矩形为单位像素时即覆盖率）。
//
// 做法：把 [x0,x1] 按多边形顶点的 x 坐标切成竖板；每块板内与多边形
// 相交的下、上边界各是一条直线 y=lo(x)、y=hi(x)；再把它们截断到
// [y0,y1]，按所有折点细分后做梯形积分。
func ConvexCoverage(poly []geom.Point, x0, y0, x1, y1 float64) float64 {
	xs := []float64{x0, x1}
	for _, p := range poly {
		if p.X > x0+eps && p.X < x1-eps {
			xs = append(xs, p.X)
		}
	}
	sort.Float64s(xs)
	xs = unique(xs)

	area := 0.0
	for k := 0; k+1 < len(xs); k++ {
		a, b := xs[k], xs[k+1]
		if b-a < eps {
			continue
		}
		xm := (a + b) / 2

		type edgeHit struct {
			y float64
			i int
		}
		var hits []edgeHit
		n := len(poly)
		for i := 0; i < n; i++ {
			p, q := poly[i], poly[(i+1)%n]
			if math.Abs(q.X-p.X) < 1e-14 {
				continue // 竖直边不会在板中点被穿过
			}
			if (p.X-xm)*(q.X-xm) < 0 {
				t := (xm - p.X) / (q.X - p.X)
				hits = append(hits, edgeHit{y: p.Y + t*(q.Y-p.Y), i: i})
			}
		}
		if len(hits) < 2 {
			continue
		}
		sort.Slice(hits, func(i, j int) bool { return hits[i].y < hits[j].y })
		loEdge, hiEdge := hits[0].i, hits[len(hits)-1].i

		yAt := func(edge int, x float64) float64 {
			p, q := poly[edge], poly[(edge+1)%n]
			t := (x - p.X) / (q.X - p.X)
			return p.Y + t*(q.Y-p.Y)
		}
		loA, loB := yAt(loEdge, a), yAt(loEdge, b)
		hiA, hiB := yAt(hiEdge, a), yAt(hiEdge, b)
		area += integrateClamped(loA, loB, hiA, hiB, a, b, y0, y1)
	}
	return math.Min((x1-x0)*(y1-y0), math.Max(0, area))
}

const eps = 1e-11

// integrateClamped 在 x∈[a,b] 上积分
// clamp(hi(x),y0,y1)-clamp(lo(x),y0,y1)。lo/hi 为线性函数；
// 折点只可能出现在穿过 y0/y1 的位置，加入这些点后每段都是线性的，
// 梯形公式给出精确积分。
func integrateClamped(loA, loB, hiA, hiB, a, b, y0, y1 float64) float64 {
	ts := []float64{0, 1}
	addRoots := func(vA, vB, target float64) {
		if math.Abs(vB-vA) < 1e-14 {
			return
		}
		t := (target - vA) / (vB - vA)
		if t > 1e-12 && t < 1-1e-12 {
			ts = append(ts, t)
		}
	}
	for _, target := range []float64{y0, y1} {
		addRoots(loA, loB, target)
		addRoots(hiA, hiB, target)
	}
	sort.Float64s(ts)
	ts = unique(ts)

	lengthAt := func(t float64) float64 {
		lo := loA + t*(loB-loA)
		hi := hiA + t*(hiB-hiA)
		clamp := func(v float64) float64 { return math.Min(y1, math.Max(y0, v)) }
		return clamp(hi) - clamp(lo) // clamp 单调且 hi>=lo，差值非负
	}
	out := 0.0
	for i := 0; i+1 < len(ts); i++ {
		t0, t1 := ts[i], ts[i+1]
		out += (b - a) * (t1 - t0) * (lengthAt(t0) + lengthAt(t1)) / 2
	}
	return out
}

func unique(xs []float64) []float64 {
	out := xs[:0]
	for _, x := range xs {
		if len(out) == 0 || math.Abs(x-out[len(out)-1]) > eps {
			out = append(out, x)
		}
	}
	return out
}
