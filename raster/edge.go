package raster

import (
	"errors"

	"vecgo/coverage"
)

// EdgeMode 选择直边裁剪的像素覆盖率路径。
type EdgeMode int

const (
	// EdgeHard 是保留的硬切路径：只按像素中心做二值判定，输出仅 0/255。
	EdgeHard EdgeMode = iota
	// EdgeAnalytic 使用 coverage 包的闭式覆盖率，边界像素得到连续灰阶。
	EdgeAnalytic
)

// FillVerticalEdge 按像素坐标填充竖直直边 x=c 一侧。
// keepLeft=true 保留 x<=c，否则保留 x>=c。
//
// 像素 col 的区域是 [col,col+1]×[row,row+1]。EdgeHard 走像素中心
// 二值判定；EdgeAnalytic 走闭式覆盖率并四舍五入到 8 位灰度。
func (im *Image) FillVerticalEdge(c float64, keepLeft bool, mode EdgeMode) error {
	if im.W <= 0 || im.H <= 0 {
		return errors.New("空图不能填充")
	}
	for row := 0; row < im.H; row++ {
		for col := 0; col < im.W; col++ {
			var v uint8
			switch mode {
			case EdgeHard:
				center := float64(col) + 0.5
				on := center <= c
				if !keepLeft {
					on = center >= c
				}
				if on {
					v = 255
				}
			case EdgeAnalytic:
				a := coverage.VerticalFraction(c, float64(col), float64(col+1))
				if !keepLeft {
					a = 1 - a
				}
				v = coverage.Gray(a)
			default:
				return errors.New("未知 EdgeMode")
			}
			im.Pix[row*im.W+col] = v
		}
	}
	return nil
}

// FillVerticalEdgeSS 用 n×n 中点超采样填充竖直直边，作为高倍对照路径。
// 十六倍对照取 n=16（每像素 256 个样本）。
func (im *Image) FillVerticalEdgeSS(c float64, keepLeft bool, n int) error {
	if im.W <= 0 || im.H <= 0 {
		return errors.New("空图不能填充")
	}
	if n < 1 {
		return errors.New("supersample 必须为正")
	}
	for row := 0; row < im.H; row++ {
		y0, y1 := float64(row), float64(row+1)
		for col := 0; col < im.W; col++ {
			x0, x1 := float64(col), float64(col+1)
			_, _, a := coverage.SuperSampleVertical(c, x0, y0, x1, y1, n)
			if !keepLeft {
				a = 1 - a
			}
			im.Pix[row*im.W+col] = coverage.Gray(a)
		}
	}
	return nil
}

// DistinctLevels 统计图像中出现的不同灰度值个数。
func (im *Image) DistinctLevels() int {
	var seen [256]bool
	for _, v := range im.Pix {
		seen[v] = true
	}
	n := 0
	for _, ok := range seen {
		if ok {
			n++
		}
	}
	return n
}
