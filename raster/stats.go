package raster

// FillStats 记录 Fill 的绘制计数。所有计数都从逐像素中间量汇总，不按图形平均；
// 它只做观测，不参与成像。Enabled 为 false 时 Fill 不执行任何统计代码。
type FillStats struct {
	Enabled bool
	// KeepPerPixel 打开后额外保存逐像素重复次数与覆盖数，用于定位热点区域。
	KeepPerPixel bool

	// ActualPosts 是现行成像路径的写入次数：Fill 在首个包含该子采样的
	// 多边形处短路，故每个被覆盖子采样至多写一次。恒等于 CoveredSamples。
	ActualPosts int
	// Posts 是不做短路时的写入次数：每个子采样在每个包含它的多边形上各写一次。
	// 这是可与解析重叠面积直接比较的口径。
	Posts int
	// Tests 是现行路径实际执行的几何判定次数（geom.Inside 调用数）：
	// 每个子采样在首个命中之前逐图形测试，命中即停。
	Tests int
	// Samples 是子采样总数：像素数 × supersample²。
	Samples int
	// CoveredSamples 是被 ≥1 个多边形覆盖的子采样数。
	CoveredSamples int
	// OverlapSamples 是被 ≥2 个多边形同时覆盖的子采样数。
	OverlapSamples int
	// ExtraPosts 是同一子采样在首次写入之后的重复写入次数，恒等于 Posts − CoveredSamples。
	ExtraPosts int
	// Pixels 是至少有一个子采样被覆盖的像素数。
	Pixels int

	// Repeat[k]：重复写入次数为 k 的像素个数（k = 写入次数 − 被覆盖子采样数）。
	Repeat []int

	// RepeatPerPixel / CoveredPerPixel 仅在 KeepPerPixel 时非空，行优先。
	RepeatPerPixel  []int
	CoveredPerPixel []int
	width           int

	// MaxRepeat 是逐像素重复次数的最大值，MaxAt 是首个取到它的像素 (row, col)。
	MaxRepeat int
	MaxAt     [2]int
}

// Reset 清空计数并复用已有切片。
func (s *FillStats) Reset() {
	s.ActualPosts, s.Posts, s.Tests, s.Samples, s.CoveredSamples = 0, 0, 0, 0, 0
	s.OverlapSamples, s.ExtraPosts, s.Pixels = 0, 0, 0
	s.MaxRepeat, s.MaxAt = 0, [2]int{}
	zero(s.Repeat)
	zero(s.RepeatPerPixel)
	zero(s.CoveredPerPixel)
}

func zero(xs []int) {
	for i := range xs {
		xs[i] = 0
	}
}

func (s *FillStats) grow(n int) {
	s.Repeat = growInts(s.Repeat, n)
}

func growInts(xs []int, n int) []int {
	for len(xs) < n {
		xs = append(xs, 0)
	}
	return xs
}

// RepeatAtLeast 返回重复写入次数 ≥ n 的像素数。
func (s *FillStats) RepeatAtLeast(n int) int {
	if n < 0 {
		n = 0
	}
	total := 0
	for k := n; k < len(s.Repeat); k++ {
		total += s.Repeat[k]
	}
	return total
}

// recordPixel 汇总单个像素：covered 为被覆盖子采样数，posts 为不短路写入次数。
func (s *FillStats) recordPixel(row, col, covered, posts int, perPixel bool) {
	if covered > 0 {
		s.Pixels++
	}
	repeat := posts - covered
	s.ExtraPosts += repeat
	s.Repeat = growInts(s.Repeat, repeat+1)
	s.Repeat[repeat]++
	if perPixel {
		idx := row*s.width + col
		s.RepeatPerPixel[idx] = repeat
		s.CoveredPerPixel[idx] = covered
		if repeat > s.MaxRepeat {
			s.MaxRepeat = repeat
			s.MaxAt = [2]int{row, col}
		}
	}
}

// HotRegion 返回重复次数等于峰值的像素外接矩形 (row0, col0, row1, col1) 与像素数。
// 仅在 KeepPerPixel 时有效。
func (s *FillStats) HotRegion() (r0, c0, r1, c1, count int) {
	if !s.KeepPerPixel || s.width <= 0 || len(s.RepeatPerPixel) == 0 || s.MaxRepeat == 0 {
		return 0, 0, 0, 0, 0
	}
	r0, c0, r1, c1 = len(s.RepeatPerPixel)/s.width, s.width, -1, -1
	for i, v := range s.RepeatPerPixel {
		if v != s.MaxRepeat {
			continue
		}
		r, c := i/s.width, i%s.width
		if r < r0 {
			r0 = r
		}
		if r > r1 {
			r1 = r
		}
		if c < c0 {
			c0 = c
		}
		if c > c1 {
			c1 = c
		}
		count++
	}
	if r1 < 0 {
		return 0, 0, 0, 0, 0
	}
	return r0, c0, r1, c1, count
}

// HotMap 把逐像素重复次数压成字符图，每个 cell 覆盖 cell×cell 个像素：
//
//	X = cell 内有像素的重复次数等于全局峰值
//	+ = cell 内有像素存在重复但不是峰值
//	. = cell 内有覆盖但无重复
//	' ' = 完全无覆盖
func (s *FillStats) HotMap(cell int) []string {
	if cell < 1 {
		cell = 1
	}
	if !s.KeepPerPixel || s.width <= 0 || len(s.RepeatPerPixel) == 0 {
		return nil
	}
	h := len(s.RepeatPerPixel) / s.width
	rows := (h + cell - 1) / cell
	cols := (s.width + cell - 1) / cell
	out := make([]string, rows)
	for r := 0; r < rows; r++ {
		line := make([]byte, cols)
		for c := 0; c < cols; c++ {
			b := byte(' ')
			for y := r * cell; y < (r+1)*cell && y < h; y++ {
				for x := c * cell; x < (c+1)*cell && x < s.width; x++ {
					idx := y*s.width + x
					v := s.RepeatPerPixel[idx]
					switch {
					case s.MaxRepeat > 0 && v == s.MaxRepeat:
						b = 'X'
					case v > 0 && b != 'X':
						b = '+'
					case b == ' ' && s.CoveredPerPixel[idx] > 0:
						b = '.'
					}
				}
			}
			line[c] = b
		}
		out[r] = string(line)
	}
	return out
}
