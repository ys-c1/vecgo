package post

import (
	"errors"

	"vecgo/raster"
)

func Gamma(im *raster.Image, gamma float64) error {
	return GammaRect(im, gamma, raster.Rect{X0: 0, Y0: 0, X1: im.W, Y1: im.H})
}

type gammaTable [256]uint8

func buildGammaTable(gamma float64) gammaTable {
	var table gammaTable
	for i := 0; i < 256; i++ {
		table[i] = uint8(255 * pow(float64(i)/255, 1/gamma))
	}
	return table
}

// GammaRect 只对矩形区域应用 gamma。查表与 Gamma 完全相同，
// 因此同一像素在两条路径下结果逐位一致。
func GammaRect(im *raster.Image, gamma float64, r raster.Rect) error {
	if gamma < 0.1 || gamma > 10 {
		return errors.New("gamma 超出范围")
	}
	if !r.InBounds(im.W, im.H) {
		return errors.New("gamma 区域越界")
	}
	table := buildGammaTable(gamma)
	for row := r.Y0; row < r.Y1; row++ {
		base := row * im.W
		for col := r.X0; col < r.X1; col++ {
			im.Pix[base+col] = table[im.Pix[base+col]]
		}
	}
	return nil
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
	rows := make([]raster.Rect, im.H)
	for y := 0; y < im.H; y++ {
		rows[y] = raster.Rect{X0: 0, Y0: y, X1: im.W, Y1: y + 1}
	}
	blurInto(im, out, radius, rows)
	return out, nil
}

// BlurInto 在 dst 上重算矩形区域内的盒式模糊值，其余像素保持不变。
// src 必须已是模糊前的完整图像（即 gamma 之后、模糊之前）。
// 模糊只依赖半径内的邻域，因此只要 rects 已按半径外扩，
// 局部重算的结果与整幅 Blur 逐位相同。
func BlurInto(src, dst *raster.Image, radius int, rects []raster.Rect) error {
	if radius < 0 {
		return errors.New("blur 不能为负")
	}
	if src.W != dst.W || src.H != dst.H {
		return errors.New("模糊源与目标尺寸不一致")
	}
	for _, r := range rects {
		if !r.InBounds(src.W, src.H) {
			return errors.New("模糊区域越界")
		}
	}
	blurInto(src, dst, radius, rects)
	return nil
}

func blurInto(src, dst *raster.Image, radius int, rects []raster.Rect) {
	for _, r := range rects {
		for y := r.Y0; y < r.Y1; y++ {
			for x := r.X0; x < r.X1; x++ {
				sum, count := 0, 0
				for dy := -radius; dy <= radius; dy++ {
					for dx := -radius; dx <= radius; dx++ {
						ny, nx := y+dy, x+dx
						if ny >= 0 && ny < src.H && nx >= 0 && nx < src.W {
							sum += int(src.Pix[ny*src.W+nx])
							count++
						}
					}
				}
				dst.Pix[y*dst.W+x] = uint8(sum / count)
			}
		}
	}
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
