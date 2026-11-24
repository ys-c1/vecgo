package post

import (
	"errors"

	"vecgo/color"
	"vecgo/raster"
)

// Gamma 把线性光缓冲按 gamma 编码为编码字节。
// 转换表由 color.EncodeLUT 统一生成，本包不再持有任何幂函数实现。
func Gamma(im *raster.Image, gamma float64) error {
	if gamma < 0.1 || gamma > 10 {
		return errors.New("post: gamma 超出范围 [0.1,10]")
	}
	table, err := color.EncodeLUT(gamma)
	if err != nil {
		return err
	}
	for i := range im.Pix {
		im.Pix[i] = table[im.Pix[i]]
	}
	return nil
}

func Blur(im *raster.Image, radius int) (*raster.Image, error) {
	if radius < 0 {
		return nil, errors.New("post: blur radius 不能为负")
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
