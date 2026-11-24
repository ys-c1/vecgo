package geom

// SignedArea 用鞋带公式返回多边形的有向面积：
// A = 1/2 * sum_i (x_i*y_{i+1} - x_{i+1}*y_i)。
// 逆时针为正，顺时针为负。
func SignedArea(poly []Point) float64 {
	n := len(poly)
	if n < 3 {
		return 0
	}
	sum := 0.0
	for i := 0; i < n; i++ {
		a := poly[i]
		b := poly[(i+1)%n]
		sum += a.X*b.Y - b.X*a.Y
	}
	return sum * 0.5
}

// Area 返回多边形的无向面积。
func Area(poly []Point) float64 {
	a := SignedArea(poly)
	if a < 0 {
		return -a
	}
	return a
}
