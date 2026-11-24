package post

import (
	"errors"
	"sync/atomic"

	"vecgo/raster"
)

// FullFrameCopies 统计管线里“把整帧像素写进另一张整帧缓冲”的次数。
// 原实现在 Blur 里新建第二张整帧并整帧读入，因此每次模糊计一次；
// 改为就地模糊后该计数归零。只按调用计数，不进入逐像素热路径。
var FullFrameCopies atomic.Int64

func Gamma(im *raster.Image, gamma float64) error {
	if gamma < 0.1 || gamma > 10 {
		return errors.New("gamma 超出范围")
	}
	var table [256]uint8
	for i := 0; i < 256; i++ {
		table[i] = raster.ToByte(pow(float64(i)/255, 1/gamma))
	}
	for i := range im.Pix {
		im.Pix[i] = table[im.Pix[i]]
	}
	return nil
}

// Blur 返回一张新的模糊图，不改动入参。整帧拷贝本身仍计入 FullFrameCopies，
// 但它和行环共用同一次分配，所以这条“保留入参”的路径也只有一次整帧分配。
func Blur(im *raster.Image, radius int) (*raster.Image, error) {
	if radius < 0 {
		return nil, errors.New("blur 不能为负")
	}
	scratch := 0
	if radius > 0 {
		scratch = (2*radius + 2) * im.W
	}
	out, err := raster.NewWithScratch(im.W, im.H, scratch)
	if err != nil {
		return nil, err
	}
	copy(out.Pix, im.Pix)
	FullFrameCopies.Add(1)
	if err := BlurInto(out, radius); err != nil {
		return nil, err
	}
	return out, nil
}

// BlurInto 就地模糊 im，不再分配第二张整帧。
//
// 逐行推进，用一条 (2*radius+1) 行的环形缓冲保存尚未被写回的原始行：算第 y 行前，
// 先把窗口底端新出现的原始行读进环；写回第 y 行时，第 y+radius 行一定还是原值，
// 因为环宽恰好等于窗口。临时空间向 im 借（raster.Image.Scratch），不另分配，
// 所以一次就地模糊只有“原图”这一张整帧。
func BlurInto(im *raster.Image, radius int) error {
	if radius < 0 {
		return errors.New("blur 不能为负")
	}
	if radius == 0 {
		return nil
	}
	if im.W <= 0 || im.H <= 0 {
		return errors.New("空图不能模糊")
	}
	W, H := im.W, im.H
	slots := 2*radius + 1
	buf := im.Scratch((slots + 1) * W)
	row := func(y int) []uint8 {
		s := y % slots
		return buf[s*W : (s+1)*W]
	}
	line := buf[slots*W:]

	loaded := -1
	preload := radius
	if preload > H-1 {
		preload = H - 1
	}
	for k := 0; k <= preload; k++ {
		copy(row(k), im.Pix[k*W:(k+1)*W])
		loaded = k
	}

	for y := 0; y < H; y++ {
		want := y + radius
		if want > H-1 {
			want = H - 1
		}
		for loaded < want {
			loaded++
			copy(row(loaded), im.Pix[loaded*W:(loaded+1)*W])
		}
		y0 := y - radius
		if y0 < 0 {
			y0 = 0
		}
		y1 := y + radius
		if y1 > H-1 {
			y1 = H - 1
		}
		for x := 0; x < W; x++ {
			x0 := x - radius
			if x0 < 0 {
				x0 = 0
			}
			x1 := x + radius
			if x1 > W-1 {
				x1 = W - 1
			}
			sum, count := 0, 0
			for yy := y0; yy <= y1; yy++ {
				r := row(yy)
				for xx := x0; xx <= x1; xx++ {
					sum += int(r[xx])
				}
				count += x1 - x0 + 1
			}
			line[x] = uint8(sum / count)
		}
		copy(im.Pix[y*W:(y+1)*W], line)
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
