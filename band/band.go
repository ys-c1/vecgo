// Package band 用几种事先约定的口径在渲染产物上“数色带”，并给出对应的闭式预测。
//
// 主口径（与 docs/banding.md 的三行推导一致）是临界对比度接缝数：
// 沿渐变方向（行）取画面中一列，若相邻两行在线性光上的 Weber 相对反差
// |ΔY|/min(Y) 达到临界值 W，就记一条“可见接缝”。单调渐变里接缝把画面
// 分成 N+1 个色带段，因此同时报告接缝数与段数。
package band

import (
	"math"

	"vecgo/colorspace"
	"vecgo/raster"
)

// Column 取出第 x 列自上而下的像素码。x 越界时夹到合法范围。
func Column(im *raster.Image, x int) []uint8 {
	if x < 0 {
		x = 0
	}
	if x >= im.W {
		x = im.W / 2
	}
	col := make([]uint8, im.H)
	for y := 0; y < im.H; y++ {
		col[y] = im.Pix[y*im.W+x]
	}
	return col
}

// CountRuns 数单调（或任意）列里的极大等值段数：每遇到一次相邻值不同就 +1。
func CountRuns(col []uint8) int {
	if len(col) == 0 {
		return 0
	}
	n := 1
	for i := 1; i < len(col); i++ {
		if col[i] != col[i-1] {
			n++
		}
	}
	return n
}

// CountDistinct 数列中出现过多少个不同码值。
func CountDistinct(col []uint8) int {
	seen := [256]bool{}
	for _, v := range col {
		seen[v] = true
	}
	n := 0
	for _, b := range seen {
		if b {
			n++
		}
	}
	return n
}

// Gaps 返回相邻行的码差（前一行减后一行，黑到白渐变里非负）列表与最大码差。
func Gaps(col []uint8) (gaps []int, maxGap int) {
	for i := 1; i < len(col); i++ {
		g := int(col[i-1]) - int(col[i])
		if g < 0 {
			g = -g
		}
		gaps = append(gaps, g)
		if g > maxGap {
			maxGap = g
		}
	}
	return gaps, maxGap
}

// weber 计算相邻两码在线性光上的 Weber 相对反差；底端为 0 时返回 +Inf。
func weber(a, b uint8) float64 {
	ya := colorspace.ByteToLinear(a)
	yb := colorspace.ByteToLinear(b)
	lo := math.Min(ya, yb)
	d := math.Abs(ya - yb)
	if lo <= 1e-15 {
		if d <= 1e-15 {
			return 0
		}
		return math.Inf(1)
	}
	return d / lo
}

// CountWeberCritical 数 Weber 反差 >= w 的相邻接缝数。
func CountWeberCritical(col []uint8, w float64) int {
	n := 0
	for i := 1; i < len(col); i++ {
		if weber(col[i-1], col[i]) >= w {
			n++
		}
	}
	return n
}

// CountDeltaLCritical 数相邻行 CIELAB 明度差 ΔL* >= thr 的接缝数。
func CountDeltaLCritical(col []uint8, thr float64) int {
	n := 0
	prev := colorspace.LinearToLStar(colorspace.ByteToLinear(col[0]))
	for i := 1; i < len(col); i++ {
		l := colorspace.LinearToLStar(colorspace.ByteToLinear(col[i]))
		if math.Abs(l-prev) >= thr {
			n++
		}
		prev = l
	}
	return n
}

// MaxDeltaL 返回列内最大的相邻 ΔL*。
func MaxDeltaL(col []uint8) float64 {
	max := 0.0
	prev := colorspace.LinearToLStar(colorspace.ByteToLinear(col[0]))
	for i := 1; i < len(col); i++ {
		l := colorspace.LinearToLStar(colorspace.ByteToLinear(col[i]))
		if d := math.Abs(l - prev); d > max {
			max = d
		}
		prev = l
	}
	return max
}

// CriticalCodeGap 是三行推导里的临界色阶间距 m_c(k) = w·k/γ（单位：8-bit 码）。
// γ 取幂律 gamma（sRGB 的近似可取 2.2），k 为当前码值（1..255）。
func CriticalCodeGap(k int, w, gamma float64) float64 {
	if k <= 0 {
		return 0
	}
	return w * float64(k) / gamma
}

// LinearSpaceContinuousPrediction 给出线性光均匀插值时临界接缝数的连续闭式估计：
// 行参数步长 ΔY=1/(H-1)，第 j 个底端步的 Weber 反差为 1/j，
// 临界条件 1/j >= w 给出 j <= 1/w，故约 ceil(1/w) 条（含到底黑的端步）。
func LinearSpaceContinuousPrediction(w float64) int {
	return int(math.Ceil(1.0/w + 1e-9))
}
