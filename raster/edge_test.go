package raster

import (
	"math"
	"testing"
)

// 硬切路径只有 0/255 两个灰度阶；解析路径至少多出一个中间阶。
func TestHardVsAnalyticGrayLevels(t *testing.T) {
	const c = 8 + 1.0/3
	hard, err := New(16, 8)
	if err != nil {
		t.Fatal(err)
	}
	if err := hard.FillVerticalEdge(c, true, EdgeHard); err != nil {
		t.Fatal(err)
	}
	if levels := hard.DistinctLevels(); levels != 2 {
		t.Fatalf("硬切应只有 2 个灰度阶, got %d", levels)
	}

	aa, _ := New(16, 8)
	if err := aa.FillVerticalEdge(c, true, EdgeAnalytic); err != nil {
		t.Fatal(err)
	}
	if levels := aa.DistinctLevels(); levels <= 2 {
		t.Fatalf("解析覆盖率路径应有中间灰阶, 阶数=%d", levels)
	}
	if aa.Pix[0*16+8] != 85 { // round(255/3)
		t.Fatalf("边界像素灰度应为 85, got %d", aa.Pix[8])
	}
}

// 裁剪位置为整数（对齐像素网格）时，解析路径必须与硬切路径逐字节相同。
func TestAnalyticEqualsHardAtIntegerPosition(t *testing.T) {
	for _, c := range []float64{-2, 0, 3, 8, 11} {
		for _, keepLeft := range []bool{true, false} {
			h, _ := New(12, 5)
			a, _ := New(12, 5)
			if err := h.FillVerticalEdge(c, keepLeft, EdgeHard); err != nil {
				t.Fatal(err)
			}
			if err := a.FillVerticalEdge(c, keepLeft, EdgeAnalytic); err != nil {
				t.Fatal(err)
			}
			for i := range h.Pix {
				if h.Pix[i] != a.Pix[i] {
					t.Fatalf("c=%v keepLeft=%v 整数位置解析/硬切不一致 @%d: %d vs %d",
						c, keepLeft, i, h.Pix[i], a.Pix[i])
				}
			}
		}
	}
}

// 解析路径与 SS16 图像整体高度一致：只有边界列可能相差至多几个灰度级。
func TestAnalyticImageAgreesWithSS16(t *testing.T) {
	const c = 8 + 1.0/3
	aa, _ := New(16, 8)
	ss, _ := New(16, 8)
	if err := aa.FillVerticalEdge(c, true, EdgeAnalytic); err != nil {
		t.Fatal(err)
	}
	if err := ss.FillVerticalEdgeSS(c, true, 16); err != nil {
		t.Fatal(err)
	}
	diffPixels, maxDiff, sumSq := 0, 0, 0
	for i := range aa.Pix {
		d := int(aa.Pix[i]) - int(ss.Pix[i])
		if d != 0 {
			diffPixels++
		}
		if d < 0 {
			d = -d
		}
		if d > maxDiff {
			maxDiff = d
		}
		sumSq += d * d
	}
	// 直边只影响一列边界像素（8 行），四舍五入后最大差 5 个灰度级。
	if diffPixels > 8 {
		t.Fatalf("差异像素 %d 超出一个边界列", diffPixels)
	}
	if maxDiff > 6 {
		t.Fatalf("最大灰度差 %d 超出预期", maxDiff)
	}
	if rms := math.Sqrt(float64(sumSq) / float64(len(aa.Pix))); rms > 2 {
		t.Fatalf("整图 RMS 灰度差 %.3f 过大", rms)
	}
}

// 扫描全部亚像素位置时，解析路径产生的灰度阶数远多于硬切，
// 也不少于 SS16 能表达的 17 档覆盖率映射。
func TestSweepGrayLevelRichness(t *testing.T) {
	hard, _ := New(1, 101)
	aa, _ := New(1, 101)
	ss, _ := New(1, 101)
	for k := 0; k <= 100; k++ {
		c := float64(k) / 100
		hImg, _ := New(1, 1)
		_ = hImg.FillVerticalEdge(c, true, EdgeHard)
		hard.Pix[k] = hImg.Pix[0]
		aImg, _ := New(1, 1)
		_ = aImg.FillVerticalEdge(c, true, EdgeAnalytic)
		aa.Pix[k] = aImg.Pix[0]
		sImg, _ := New(1, 1)
		_ = sImg.FillVerticalEdgeSS(c, true, 16)
		ss.Pix[k] = sImg.Pix[0]
	}
	if hard.DistinctLevels() != 2 {
		t.Fatalf("硬切扫描应有 2 阶, got %d", hard.DistinctLevels())
	}
	if aa.DistinctLevels() <= ss.DistinctLevels() {
		t.Fatalf("解析灰度阶数 %d 应多于 SS16 的 %d（SS16 仅 17 档覆盖率）",
			aa.DistinctLevels(), ss.DistinctLevels())
	}
	if aa.DistinctLevels() < 100 {
		t.Fatalf("百分之一扫描下解析路径应接近每档一阶, got %d", aa.DistinctLevels())
	}
}
