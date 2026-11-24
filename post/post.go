package post

import (
	"errors"

	"vecgo/raster"
)

func Gamma(im *raster.Image, gamma float64) error {
	if gamma < 0.1 || gamma > 10 {
		return errors.New("gamma 超出范围")
	}
	var table [256]uint8
	for i := 0; i < 256; i++ {
		table[i] = uint8(255 * pow(float64(i)/255, 1/gamma))
	}
	pix := im.Lease("post.gamma")
	for i := range pix {
		pix[i] = table[pix[i]]
	}
	return nil
}

// Blur 对共享缓冲做原地盒式模糊。为了与旧实现逐像素一致，它仍然按
// (2r+1)x(2r+1) 窗口对原值求和后整除，只是不再分配第二张整图：
// 已写过的行用 r+1 行的小环形缓冲保留原值，未写过的行直接读共享缓冲。
// 额外内存是 O(W*(r+1))，与整图 O(W*H) 无关；不产生整图拷贝。
func Blur(im *raster.Image, radius int) (*raster.Image, error) {
	if radius < 0 {
		return nil, errors.New("blur 不能为负")
	}
	pix := im.Lease("post.blur")
	if radius == 0 {
		return im, nil
	}
	w, h := im.W, im.H
	ring := make([][]uint8, radius+1)
	for i := range ring {
		ring[i] = make([]uint8, w)
	}
	row := make([]uint8, w)
	for y := 0; y < h; y++ {
		// 当前行此时仍是原值，先存入环，供后面 radius 行使用。
		copy(ring[y%(radius+1)], pix[y*w:(y+1)*w])
		for x := 0; x < w; x++ {
			sum, count := 0, 0
			for dy := -radius; dy <= radius; dy++ {
				ny := y + dy
				if ny < 0 || ny >= h {
					continue
				}
				for dx := -radius; dx <= radius; dx++ {
					nx := x + dx
					if nx < 0 || nx >= w {
						continue
					}
					if ny < y {
						sum += int(ring[ny%(radius+1)][nx])
					} else {
						sum += int(pix[ny*w+nx])
					}
					count++
				}
			}
			row[x] = uint8(sum / count)
		}
		copy(pix[y*w:(y+1)*w], row)
	}
	return im, nil
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
