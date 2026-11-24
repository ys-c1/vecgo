package level

import (
	"math"
	"sort"
)

// DarkCodeVariance 从一段渐变方向上的实际 8-bit 码值里，统计暗部相邻不同色阶的
// 感知间距方差（幂律坐标 P=(k/255)^Alpha）。
//
// 做法：取所有 k<=Cutoff 的码，去重并按码升序排列（即由暗到亮的实际色阶），
// 计算相邻码在 P 坐标上的间距，再取总体方差。返回去重色阶数与间距数。
// 这是“实现并量化成 8-bit 后，暗部相邻色阶实际间距”的客观口径。
func DarkCodeVariance(codes []uint8, alpha float64, cutoff int) (levels, gaps int, variance float64, err error) {
	a, err := CleanAlpha(alpha)
	if err != nil {
		return 0, 0, 0, err
	}
	if cutoff < 0 {
		cutoff = 0
	}
	if cutoff > 255 {
		cutoff = 255
	}
	seen := make(map[int]struct{}, len(codes))
	vals := make([]int, 0, len(codes))
	for _, c := range codes {
		k := int(c)
		if k > cutoff {
			continue
		}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		vals = append(vals, k)
	}
	sort.Ints(vals)
	if len(vals) < 2 {
		return len(vals), 0, 0, nil
	}
	g := make([]float64, 0, len(vals)-1)
	for i := 0; i+1 < len(vals); i++ {
		p0 := powerCode(vals[i], a)
		p1 := powerCode(vals[i+1], a)
		g = append(g, p1-p0)
	}
	return len(vals), len(g), Variance(g), nil
}

// powerCode 返回 8-bit 码 k 在 P=(k/255)^Alpha 坐标下的值。
func powerCode(k int, alpha float64) float64 {
	return math.Pow(float64(k)/255.0, alpha)
}

// ColumnCodes 从连续行优先灰度图数据中抽取第 col 列的 H 个码（自上而下）。
func ColumnCodes(pix []uint8, w, h, col int) []uint8 {
	out := make([]uint8, 0, h)
	if col < 0 || col >= w {
		return out
	}
	for y := 0; y < h; y++ {
		idx := y*w + col
		if idx >= len(pix) {
			break
		}
		out = append(out, pix[idx])
	}
	return out
}
