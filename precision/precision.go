package precision

import (
	"errors"
	"math"

	"vecgo/geom"
)

// Chain 是一条“平移→旋转→缩放”的变换链。
// Rotate 以弧度计；Scale 为缩放倍数；Shift 为平移量。
type Chain struct {
	Name   string
	Scale  float64
	Rotate float64
	Shift  geom.Point
}

// Weights 返回三步舍入误差传到最终坐标时的放大系数。
// 平移和旋转发生在缩放之前，误差都会被随后的 scale 放大；缩放步本身只放大一次（系数 1）。
func (c Chain) Weights() [3]float64 {
	return [3]float64{c.Scale, c.Scale, 1}
}

// StepNoise 是第 i 步按 digits[i] 位小数舍入后，两坐标合成误差的欧氏上界：
// 每个坐标的舍入不超过 0.5*10^-d，两坐标合成不超过 10^-d/sqrt(2)。
func StepNoise(d int) float64 {
	return math.Pow10(-d) / math.Sqrt2
}

// Bound 返回给定小数位数分配下，最终欧氏偏差的保守上界。
func Bound(weights [3]float64, digits [3]int) float64 {
	total := 0.0
	for i, w := range weights {
		total += w * StepNoise(digits[i])
	}
	return total
}

// Allocate 在总小数位数最少的前提下，求满足 Bound<=eps 的分配。
// 同总位数下取理论上界最小的一组，结果确定。
func Allocate(weights [3]float64, eps float64) ([3]int, error) {
	if eps <= 0 || math.IsNaN(eps) || math.IsInf(eps, 0) {
		return [3]int{}, errors.New("误差上界必须为正的有限值")
	}
	limit := 0
	for _, w := range weights {
		if w <= 0 {
			return [3]int{}, errors.New("放大系数必须为正")
		}
		d := int(math.Ceil(math.Log10(3*w/(math.Sqrt2*eps)))) + 2
		if d > limit {
			limit = d
		}
	}
	if limit < 0 {
		limit = 0
	}
	ok := func(b float64) bool { return b <= eps*(1+1e-12) }

	best := [3]int{-1, -1, -1}
	bestB := 0.0
	for total := 0; total <= 3*limit; total++ {
		for d0 := 0; d0 <= total; d0++ {
			for d1 := 0; d1 <= total-d0; d1++ {
				dg := [3]int{d0, d1, total - d0 - d1}
				b := Bound(weights, dg)
				if !ok(b) {
					continue
				}
				if best[0] < 0 || b < bestB-1e-15 {
					best, bestB = dg, b
				}
			}
		}
		if best[0] >= 0 {
			return best, nil
		}
	}
	return best, errors.New("在搜索范围内无可行分配")
}

// AllocateGreedy 与 Allocate 等价，但按边际收益贪心加位：
// 每加一位使该项界减小 0.9*w*10^-d/sqrt(2)，单调递减，
// 故每次把下一位给当前边际收益最大的步骤即为最少总位数。
func AllocateGreedy(weights [3]float64, eps float64) ([3]int, error) {
	if eps <= 0 || math.IsNaN(eps) || math.IsInf(eps, 0) {
		return [3]int{}, errors.New("误差上界必须为正的有限值")
	}
	for _, w := range weights {
		if w <= 0 {
			return [3]int{}, errors.New("放大系数必须为正")
		}
	}
	dg := [3]int{}
	for guard := 0; guard < 10000; guard++ {
		if Bound(weights, dg) <= eps*(1+1e-12) {
			return dg, nil
		}
		best, bestGain := 0, -1.0
		for i, w := range weights {
			gain := w * math.Pow10(-dg[i]) / math.Sqrt2 * 0.9
			if gain > bestGain {
				best, bestGain = i, gain
			}
		}
		dg[best]++
	}
	return dg, errors.New("贪心未收敛")
}

// EqualDigits 返回等精度方案所需的小数位数（三步相同）。
func EqualDigits(weights [3]float64, eps float64) (int, error) {
	if eps <= 0 || math.IsNaN(eps) || math.IsInf(eps, 0) {
		return 0, errors.New("误差上界必须为正的有限值")
	}
	sum := 0.0
	for _, w := range weights {
		if w <= 0 {
			return 0, errors.New("放大系数必须为正")
		}
		sum += w
	}
	ok := func(d int) bool { return Bound(weights, [3]int{d, d, d}) <= eps*(1+1e-12) }
	d := int(math.Ceil(math.Log10(sum / (math.Sqrt2 * eps))))
	if d < 0 {
		d = 0
	}
	for !ok(d) {
		d++
	}
	for d > 0 && ok(d-1) {
		d--
	}
	return d, nil
}

// Apply 按给定小数位数执行整条链，每步结果坐标四舍五入到定点。
func (c Chain) Apply(p geom.Point, digits [3]int) geom.Point {
	cs, sn := math.Cos(c.Rotate), math.Sin(c.Rotate)
	p = geom.Point{
		X: round(p.X+c.Shift.X, digits[0]),
		Y: round(p.Y+c.Shift.Y, digits[0]),
	}
	p = geom.Point{
		X: round(p.X*cs-p.Y*sn, digits[1]),
		Y: round(p.X*sn+p.Y*cs, digits[1]),
	}
	return geom.Point{
		X: round(c.Scale*p.X, digits[2]),
		Y: round(c.Scale*p.Y, digits[2]),
	}
}

// ApplyExact 用双精度原样执行同一条链，作为实测的参考值。
func (c Chain) ApplyExact(p geom.Point) geom.Point {
	cs, sn := math.Cos(c.Rotate), math.Sin(c.Rotate)
	p = geom.Point{X: p.X + c.Shift.X, Y: p.Y + c.Shift.Y}
	p = geom.Point{X: p.X*cs - p.Y*sn, Y: p.X*sn + p.Y*cs}
	return geom.Point{X: c.Scale * p.X, Y: c.Scale * p.Y}
}

// MaxDeviation 在样本点上返回量化链相对精确链的最大欧氏偏差与最大单坐标偏差。
func (c Chain) MaxDeviation(points []geom.Point, digits [3]int) (euclid, coord float64) {
	for _, p := range points {
		exact := c.ApplyExact(p)
		got := c.Apply(p, digits)
		dx, dy := math.Abs(got.X-exact.X), math.Abs(got.Y-exact.Y)
		if d := math.Hypot(dx, dy); d > euclid {
			euclid = d
		}
		if dx > coord {
			coord = dx
		}
		if dy > coord {
			coord = dy
		}
	}
	return euclid, coord
}

func round(x float64, d int) float64 {
	if d <= 0 {
		return math.Round(x)
	}
	s := math.Pow10(d)
	return math.Round(x*s) / s
}

// Grid 生成 [0,size]x[0,size] 上 n×n 的样本点。
func Grid(n int, size float64) []geom.Point {
	out := make([]geom.Point, 0, n*n)
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			out = append(out, geom.Point{
				X: size * float64(i) / float64(n-1),
				Y: size * float64(j) / float64(n-1),
			})
		}
	}
	return out
}
