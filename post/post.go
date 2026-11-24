package post

import (
	"errors"

	"vecgo/raster"
)

func Gamma(im *raster.Image, gamma float64) error {
	if gamma < 0.1 || gamma > 10 {
		return errors.New("gamma 超出范围")
	}
	table := GammaTable(gamma)
	for i := range im.Pix {
		im.Pix[i] = table[im.Pix[i]]
	}
	return nil
}

// GammaTable 返回 Gamma 使用的 256 项查找表。抽出它是为了让“预测端”
// 能逐值复现后处理阶段（含 gamma=1 时手写 pow 的浮点截断行为）。
func GammaTable(gamma float64) [256]uint8 {
	var table [256]uint8
	for i := 0; i < 256; i++ {
		table[i] = uint8(255 * pow(float64(i)/255, 1/gamma))
	}
	return table
}

func Blur(im *raster.Image, radius int) (*raster.Image, error) {
	if radius < 0 {
		return nil, errors.New("blur 不能为负")
	}
	if radius == 0 {
		out, _ := raster.New(im.W, im.H)
		copy(out.Pix, im.Pix)
		return out, nil
	}
	out, _ := raster.New(im.W, im.H)
	for y := 0; y < im.H; y++ {
		for x := 0; x < im.W; x++ {
			sum, count := 0, 0
			for dy := -radius; dy <= radius; dy++ {
				for dx := -radius; dx <= radius; dx++ {
					ny, nx := y+dy, x+dx
					if ny >= 0 && ny < im.H && nx >= 0 && nx < im.W {
						sum += int(im.Pix[ny*im.W+nx])
						count++
					}
				}
			}
			out.Pix[y*im.W+x] = uint8(sum / count)
		}
	}
	return out, nil
}

func pow(a, b float64) float64 {
	if a <= 0 {
		return 0
	}
	r := 1.0
	e := b
	for e > 0.001 {
		if e > 1 {
			r *= a
			e -= 1
		} else {
			r *= 1 + (a-1)*e
			e = 0
		}
	}
	return r
}
