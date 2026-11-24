package band

import (
	"vecgo/colorspace"
	"vecgo/config"
)

// PredictColumn 按推导公式独立给出一条自上而下的预测码列（不读渲染产物）。
//
//   - encoded：复现历史量化 uint8(255*t)，若 lut 非 nil 再套一遍后处理 gamma LUT
//     （默认 gamma=1 时这张表含手写 pow 的浮点截断项，必须显式计入才能逐值对齐）；
//   - linear：线性光 Y=1-u 上插值后精确 sRGB 编码、一次截断量化；
//   - perceptual：CIELAB L*=100(1-u) 均匀插值后转回 sRGB、一次量化。
func PredictColumn(h int, space string, lut *[256]uint8) []uint8 {
	out := make([]uint8, h)
	for r := 0; r < h; r++ {
		u := 0.0
		if h > 1 {
			u = float64(r) / float64(h-1)
		}
		var v uint8
		switch config.NormalizeSpace(space) {
		case "encoded":
			t := 1.0 + (0.0-1.0)*u // 与 shading 的历史运算顺序一致
			v = uint8(255 * t)
			if lut != nil {
				v = lut[v]
			}
		case "linear":
			y := 1.0 - u
			v = uint8(255*colorspace.LinearToSRGB(y) + 0.0) // 与 shading 同为截断量化
		case "perceptual":
			l := 100 * (1 - u)
			v = uint8(255 * colorspace.LinearToSRGB(colorspace.LStarToLinear(l)))
		}
		out[r] = v
	}
	return out
}

// PredictWeberCritical 是 PredictColumn 上直接数 Weber 临界接缝的便捷封装。
func PredictWeberCritical(h int, space string, lut *[256]uint8, w float64) int {
	return CountWeberCritical(PredictColumn(h, space, lut), w)
}
