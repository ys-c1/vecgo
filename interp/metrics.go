package interp

import "math"

// Sample 在 [x0,x1] 上等距取 n 个点（含端点），返回对应的插值序列。
func (s *Spline) Sample(x0, x1 float64, n int) []float64 {
	if n < 2 {
		n = 2
	}
	out := make([]float64, n)
	for i := 0; i < n; i++ {
		z := x0 + (x1-x0)*float64(i)/float64(n-1)
		out[i] = s.Eval(z)
	}
	return out
}

// OvershootCount 统计过冲样本数：样本值超出其所在插值区间
// [min(y_k,y_{k+1}), max(y_k,y_{k+1})] 即记一次（含回勾、上冲、下冲）。
// 端点样本落在控制点上，天然不越界。
func (s *Spline) OvershootCount(samples int) int {
	n := len(s.X)
	count := 0
	for k := 0; k < n-1; k++ {
		lo := math.Min(s.Y[k], s.Y[k+1])
		hi := math.Max(s.Y[k], s.Y[k+1])
		// 区间内取 samples-2 个内点，避免把相邻区间的控制点重复计入。
		for i := 1; i < samples-1; i++ {
			z := s.X[k] + (s.X[k+1]-s.X[k])*float64(i)/float64(samples-1)
			v := s.Eval(z)
			if v < lo || v > hi {
				count++
			}
		}
	}
	return count
}

// ClippedCount 模拟量化到像素域 [0,1]（再映射为 uint8）时的截断：
// 在 bins 个等距样本上统计浮点值低于 0 或高于 1 的个数。过冲一旦超出
// 可表示范围就会被夹到边界，表现为暗部/亮部的平台与回勾。
func (s *Spline) ClippedCount(bins int) (lo, hi int) {
	x0, x1 := s.X[0], s.X[len(s.X)-1]
	for i := 0; i < bins; i++ {
		z := x0 + (x1-x0)*float64(i)/float64(bins-1)
		switch v := s.Eval(z); {
		case v < 0:
			lo++
		case v > 1:
			hi++
		}
	}
	return lo, hi
}

// ReversalCount 统计密集采样序列中的方向反转次数（离散意义上的回勾）。
func ReversalCount(vals []float64) int {
	count := 0
	dir := 0
	for i := 1; i < len(vals); i++ {
		var d int
		switch {
		case vals[i] > vals[i-1]:
			d = 1
		case vals[i] < vals[i-1]:
			d = -1
		default:
			d = 0
		}
		if d != 0 {
			if dir != 0 && d != dir {
				count++
			}
			dir = d
		}
	}
	return count
}

// ShapeDeviation 比较两条曲线在同一组密集样本上的形状偏差，
// 返回最大绝对偏差 maxAbs 和均方根偏差 rms。
func ShapeDeviation(a, b *Spline, samples int) (maxAbs, rms float64) {
	x0, x1 := a.X[0], a.X[len(a.X)-1]
	sum := 0.0
	for i := 0; i < samples; i++ {
		z := x0 + (x1-x0)*float64(i)/float64(samples-1)
		e := math.Abs(a.Eval(z) - b.Eval(z))
		if e > maxAbs {
			maxAbs = e
		}
		sum += e * e
	}
	return maxAbs, math.Sqrt(sum / float64(samples))
}

// KnotError 返回插值曲线在控制点处偏离给定函数值的最大绝对值。
// Hermite 插值在控制点处精确经过，该函数用于显式验证这一不变量。
func (s *Spline) KnotError() float64 {
	e := 0.0
	for k := range s.X {
		if v := math.Abs(s.Eval(s.X[k]) - s.Y[k]); v > e {
			e = v
		}
	}
	return e
}

// MonotoneIntervals 检查每条区间是否严格非降或非升，返回不满足的区间数。
func (s *Spline) MonotoneViolations(samples int) int {
	n := len(s.X)
	bad := 0
	for k := 0; k < n-1; k++ {
		delta := (s.Y[k+1] - s.Y[k]) / (s.X[k+1] - s.X[k])
		vals := make([]float64, samples)
		for i := range vals {
			z := s.X[k] + (s.X[k+1]-s.X[k])*float64(i)/float64(samples-1)
			vals[i] = s.Eval(z)
		}
		for i := 1; i < len(vals); i++ {
			if (delta >= 0 && vals[i] < vals[i-1]) || (delta < 0 && vals[i] > vals[i-1]) {
				bad++
				break
			}
		}
	}
	return bad
}
