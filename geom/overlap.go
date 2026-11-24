package geom

import (
	"errors"
	"math"
	"math/bits"
)

// Area 用鞋带公式计算简单多边形面积，结果与顶点绕向无关。
func Area(poly []Point) float64 {
	if len(poly) < 3 {
		return 0
	}
	s := 0.0
	for i := range poly {
		a := poly[i]
		b := poly[(i+1)%len(poly)]
		s += a.X*b.Y - b.X*a.Y
	}
	return math.Abs(s) / 2
}

func signedArea(poly []Point) float64 {
	s := 0.0
	for i := range poly {
		a := poly[i]
		b := poly[(i+1)%len(poly)]
		s += a.X*b.Y - b.X*a.Y
	}
	return s / 2
}

// ccw 返回逆时针副本；输入本身逆时针时原样返回。
func ccw(poly []Point) []Point {
	if signedArea(poly) >= 0 {
		return poly
	}
	out := make([]Point, len(poly))
	for i := range poly {
		out[i] = poly[len(poly)-1-i]
	}
	return out
}

// isConvex 判断多边形是否为凸（允许共线顶点）。用于界定解析裁剪的适用范围。
func isConvex(poly []Point) bool {
	if len(poly) < 3 {
		return false
	}
	sign := 0
	for i := range poly {
		a := poly[i]
		b := poly[(i+1)%len(poly)]
		c := poly[(i+2)%len(poly)]
		cross := (b.X-a.X)*(c.Y-b.Y) - (b.Y-a.Y)*(c.X-b.X)
		if math.Abs(cross) < 1e-12 {
			continue
		}
		s := 1
		if cross < 0 {
			s = -1
		}
		if sign == 0 {
			sign = s
		} else if sign != s {
			return false
		}
	}
	return sign != 0
}

// IntersectionArea 解析计算两个凸多边形的交集面积（Sutherland–Hodgman 裁剪 + 鞋带公式）。
// 不使用任何采样估计。顶点绕向不限。
func IntersectionArea(a, b []Point) float64 {
	if len(a) < 3 || len(b) < 3 {
		return 0
	}
	out, err := Clip(ccw(a), ccw(b))
	if err != nil || len(out) < 3 {
		return 0
	}
	return Area(out)
}

// Cover 是一次覆盖度解析统计。
type Cover struct {
	Sum   float64 // Σ 各多边形面积
	Union float64 // 并集面积
	// Excess 是 Σ 面积 − 并集面积：被 k 个图形覆盖的区域按 k−1 计权。
	// 它等于“重复绘制的理论面积”。
	Excess float64
	// Multi 是被 ≥2 个图形同时覆盖的面积。
	Multi float64
	// ShapeArea 是每个多边形的面积，顺序同输入。
	ShapeArea []float64
}

// OverlapExcess 返回重复覆盖面积（见 Cover.Excess）。
func OverlapExcess(polys [][]Point) (float64, error) {
	c, err := Measure(polys)
	return c.Excess, err
}

// Measure 用容斥解析计算覆盖度。不采样。
// 仅支持凸多边形；裁剪链中每一步仍是凸的（凸集交仍为凸集）。
func Measure(polys [][]Point) (Cover, error) {
	var cov Cover
	if len(polys) == 0 {
		return cov, errors.New("至少一个多边形")
	}
	if len(polys) > 20 {
		return cov, errors.New("多边形过多，容斥不可行")
	}
	cov.ShapeArea = make([]float64, len(polys))
	for i, p := range polys {
		if len(p) < 3 {
			return cov, errors.New("多边形至少三个顶点")
		}
		if !isConvex(p) {
			return cov, errors.New("解析重叠面积仅支持凸多边形")
		}
		cov.ShapeArea[i] = Area(p)
		cov.Sum += cov.ShapeArea[i]
	}
	for mask := 1; mask < 1<<len(polys); mask++ {
		var inter []Point
		empty := false
		for i := range polys {
			if mask&(1<<i) == 0 {
				continue
			}
			if inter == nil {
				inter = ccw(polys[i])
				continue
			}
			out, err := Clip(inter, ccw(polys[i]))
			if err != nil || len(out) < 3 {
				empty = true
				break
			}
			inter = out
		}
		if empty {
			continue
		}
		a := Area(inter)
		k := bits.OnesCount(uint(mask))
		if k%2 == 1 {
			cov.Union += a
		} else {
			cov.Union -= a
		}
		if k < 2 {
			continue
		}
		// 固定覆盖度 m 的点，对 k≥2 求和 (−1)^k·C(m,k) 得 m−1：Excess 即按重数计权的重复覆盖面积。
		if k%2 == 1 {
			cov.Excess -= a
		} else {
			cov.Excess += a
		}
		// 固定覆盖度 m 的点，对 k≥2 求和 (−1)^k·C(m,k)·(k−1) 恒为 1：Multi 即被 ≥2 个覆盖的面积。
		if k%2 == 1 {
			cov.Multi -= float64(k-1) * a
		} else {
			cov.Multi += float64(k-1) * a
		}
	}
	if cov.Excess < 0 && cov.Excess > -1e-9 {
		cov.Excess = 0
	}
	if cov.Multi < 0 && cov.Multi > -1e-9 {
		cov.Multi = 0
	}
	return cov, nil
}
