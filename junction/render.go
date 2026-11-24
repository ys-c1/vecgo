package junction

import (
	"fmt"
	"math"

	"vecgo/raster"
)

// RingMode 是接点环的四种亮度台账。
type RingMode int

const (
	// ModeWeighted 用面积守恒权重组合三份接点索赔：灰度 = ink·Σw = ink，环消失。
	ModeWeighted RingMode = iota
	// ModeNaiveAdd 三份索赔直接相加并截断：亮一圈。
	ModeNaiveAdd
	// ModeThirdsLiteral 字面把每个扇区的覆盖除以 3：暗一圈（2/3 缺口）。
	ModeThirdsLiteral
	// ModeThirdsUniform 权重一律 1/3 作用于总预算再折算回扇区：非对称错配。
	ModeThirdsUniform
)

// SectorAt 返回方向角 phi（弧度，[0,2π)）落在第几个扇区。
func SectorAt(a Angles, phi float64) int {
	if phi < a[0] {
		return 0
	}
	if phi < a[0]+a[1] {
		return 1
	}
	return 2
}

// RingGray 给出某个扇区环像素在指定台账下的 8-bit 灰度。
func RingGray(mode RingMode, sector int, w Weights, ink float64) uint8 {
	switch mode {
	case ModeNaiveAdd:
		return uint8(math.Min(255, math.Round(3*ink)))
	case ModeThirdsLiteral:
		return uint8(math.Round(clamp(ink/3, 0, 255)))
	case ModeThirdsUniform:
		v := math.Round(clamp(ink*(1.0/3.0)/w[sector], 0, 255))
		return uint8(v)
	case ModeWeighted:
		fallthrough
	default:
		// Σw 恒为 1；这里显式乘出来，而不是直接返回 ink。
		return uint8(math.Round(clamp(ink*SumWeights(w), 0, 255)))
	}
}

// RenderRingTile 把接点周围一圈像素的台账模型渲染进真实的 vecgo/raster.Image：
// n×n 灰度图，中心为接点，半径约 n*0.22 的环带是“周围一圈像素”，其余为背景 ink。
// 该函数让“修正后接点像素亮度与周围连续”成为可以直接读 Pix 断言的像素事实。
func RenderRingTile(j Junction, n int, mode RingMode, ink float64) (*raster.Image, error) {
	if n < 7 {
		return nil, fmt.Errorf("tile 尺寸至少 7，实际 %d", n)
	}
	if ink < 0 || ink > 255 {
		return nil, fmt.Errorf("灰度必须在 0..255，实际 %g", ink)
	}
	w, err := ConservationWeights(j.Angles)
	if err != nil {
		return nil, err
	}
	im, err := raster.New(n, n)
	if err != nil {
		return nil, err
	}
	bg := uint8(math.Round(ink))
	center := float64(n) / 2
	ringR := float64(n) * 0.22
	ringW := math.Max(1.2, float64(n)*0.014)
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			g := bg
			dx := float64(x) + 0.5 - center
			dy := -(float64(y) + 0.5 - center) // 翻 y，角度按数学逆时针
			if d := math.Hypot(dx, dy); math.Abs(d-ringR) <= ringW {
				phi := math.Atan2(dy, dx)
				if phi < 0 {
					phi += Tau
				}
				sec := SectorAt(j.Angles, phi)
				g = RingGray(mode, sec, w, ink)
			}
			im.Pix[y*n+x] = g
		}
	}
	return im, nil
}

// RingContinuity 量测一张环 tile：返回环带像素数、环带最大|灰度−背景|、
// 以及环带平均灰度与背景灰度之差。corrected 模式下两个偏差都应为 0。
func RingContinuity(im *raster.Image, j Junction, ink float64, n int) (ringPixels, maxDelta int, meanSigned float64) {
	bg := int(math.Round(ink))
	center := float64(n) / 2
	ringR := float64(n) * 0.22
	ringW := math.Max(1.2, float64(n)*0.014)
	sum := 0
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			dx := float64(x) + 0.5 - center
			dy := -(float64(y) + 0.5 - center)
			if d := math.Hypot(dx, dy); math.Abs(d-ringR) <= ringW {
				g := int(im.Pix[y*n+x])
				ringPixels++
				sum += g - bg
				if d := absInt(g - bg); d > maxDelta {
					maxDelta = d
				}
			}
		}
	}
	if ringPixels > 0 {
		meanSigned = float64(sum) / float64(ringPixels)
	}
	return ringPixels, maxDelta, meanSigned
}
