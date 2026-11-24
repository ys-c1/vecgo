package post

import (
	"math"

	"vecgo/raster"
	"vecgo/verr"
)

func Gamma(im *raster.Image, gamma float64) error {
	if err := checkImage(im); err != nil {
		return err
	}
	if math.IsNaN(gamma) || math.IsInf(gamma, 0) || gamma < 0.1 || gamma > 10 {
		return verr.Errorf(verr.KindPostGamma, "gamma 超出范围: %v", gamma)
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

func Blur(im *raster.Image, radius int) (*raster.Image, error) {
	if err := checkImage(im); err != nil {
		return nil, err
	}
	if radius < 0 {
		return nil, verr.Errorf(verr.KindPostBlurRadius, "blur 不能为负: %d", radius)
	}
	out, err := raster.New(im.W, im.H)
	if err != nil {
		return nil, verr.Wrap(verr.KindPostBlurImage, err)
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

// checkImage 把 nil 与“像素缓冲和画幅不一致”挡在越界之前。
// 后者原本会在 im.Pix[i] 处 panic，是 post 段看不见的失败。
func checkImage(im *raster.Image) error {
	if im == nil {
		return verr.Errorf(verr.KindPostNilImage, "图像为空")
	}
	if im.W <= 0 || im.H <= 0 || len(im.Pix) != im.W*im.H {
		return verr.Errorf(verr.KindPostBadImage, "像素缓冲与画幅不一致: len=%d %dx%d", len(im.Pix), im.W, im.H)
	}
	return nil
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
