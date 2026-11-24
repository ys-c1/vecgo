package post

import (
	"errors"

	"vecgo/bounds"
	"vecgo/raster"
)

// Gamma 对整幅画布做伽马校正。gamma 的判定与改前一致（只查范围）。
func Gamma(im *raster.Image, gamma float64) error {
	if err := checkGamma(gamma); err != nil {
		return err
	}
	return gammaRect(im, gamma, bounds.Full(im.W, im.H))
}

// GammaRect 只对像素区域 r 应用伽马。区域必须非空且完全落在画幅内，
// 四条边恰好贴画幅算界内；越界返回 *bounds.Error，报出越界那个角及坐标。
func GammaRect(im *raster.Image, gamma float64, r bounds.Rect) error {
	if err := bounds.CheckRect("post", "GammaRect", "rect", im.W, im.H, r); err != nil {
		return err
	}
	if err := checkGamma(gamma); err != nil {
		return err
	}
	return gammaRect(im, gamma, r)
}

func checkGamma(gamma float64) error {
	if gamma < 0.1 || gamma > 10 {
		return errors.New("gamma 超出范围")
	}
	return nil
}

func gammaRect(im *raster.Image, gamma float64, r bounds.Rect) error {
	var table [256]uint8
	for i := 0; i < 256; i++ {
		table[i] = uint8(255 * pow(float64(i)/255, 1/gamma))
	}
	for row := r.Y0; row < r.Y1; row++ {
		base := row * im.W
		for col := r.X0; col < r.X1; col++ {
			im.Pix[base+col] = table[im.Pix[base+col]]
		}
	}
	return nil
}

// Blur 做盒式模糊并返回新画布。radius 不能为负。
//
// 卷积邻域的“界外跳过”不再散落在循环里：每个样本都经 bounds.ContainsPixel
// 判定，与旧实现的语义一致（贴边的像素按画幅内的邻居求均值），判定只有一处。
func Blur(im *raster.Image, radius int) (*raster.Image, error) {
	if radius < 0 {
		return nil, errors.New("blur 不能为负")
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
					if bounds.ContainsPixel(nx, ny, im.W, im.H) {
						sum += int(im.Pix[ny*im.W+nx])
						count++
					}
				}
			}
			// 核中心 (x,y) 本身总在画幅内（循环边界来自画幅），
			// 因此 count 至少为 1，不会除零。
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
