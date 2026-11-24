package post

import (
	"vecgo/param"
	"vecgo/raster"
)

// Gamma 对画布做伽马校正。gamma 须是 [0.1, 10] 内的有限数，画布须合法，
// 否则返回 *param.Error，字段如 post.Gamma.gamma。
func Gamma(im *raster.Image, gamma float64) error {
	if err := validate("post.Gamma", im); err != nil {
		return err
	}
	if err := param.FloatIn("post.Gamma.gamma", gamma, param.GammaMin, param.GammaMax); err != nil {
		return err
	}
	var table [256]uint8
	for i := 0; i < 256; i++ {
		table[i] = uint8(255 * pow(float64(i)/255, 1/gamma))
	}
	for i := range im.Pix {
		im.Pix[i] = table[im.Pix[i]]
	}
	return nil
}

// Blur 做盒式模糊并返回新画布。radius 须在 [0, 16]，画布须合法，否则返回
// *param.Error，字段如 post.Blur.radius。
func Blur(im *raster.Image, radius int) (*raster.Image, error) {
	if err := validate("post.Blur", im); err != nil {
		return nil, err
	}
	if err := param.IntIn("post.Blur.radius", radius, param.BlurMin, param.BlurMax); err != nil {
		return nil, err
	}
	out, err := raster.New(im.W, im.H)
	if err != nil {
		return nil, err
	}
	if radius == 0 {
		copy(out.Pix, im.Pix)
		return out, nil
	}
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

func validate(entry string, im *raster.Image) error {
	return raster.ValidateImage(entry, im)
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
