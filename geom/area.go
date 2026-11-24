package geom

import "math"

// Area 用鞋带公式求多边形面积 A = 1/2*|Σ(x_i*y_{i+1} - x_{i+1}*y_i)|。
// 顶点少于三个的退化裁切结果面积记为 0。
func Area(poly []Point) float64 {
	if len(poly) < 3 {
		return 0
	}
	twice := 0.0
	n := len(poly)
	for i := 0; i < n; i++ {
		j := (i + 1) % n
		twice += poly[i].X*poly[j].Y - poly[j].X*poly[i].Y
	}
	return math.Abs(twice) * 0.5
}
