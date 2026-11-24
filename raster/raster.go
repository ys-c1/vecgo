package raster

import (
	"errors"
	"fmt"
	"os"

	"vecgo/geom"
)

type Image struct {
	W, H int
	Pix  []uint8

	// scratch 是与 Pix 共用同一底层数组的一段尾部空闲区，供就地算法
	// （如模糊的行环）临时使用；没有预留时为 nil。
	scratch []uint8
}

// Scratch 返回一段长度不少于 n 的临时字节缓冲，供就地算法使用，用完即弃。
// 若建图时预留了 scratch（见 NewWithScratch）就直接借用，否则临时分配一块。
func (im *Image) Scratch(n int) []uint8 {
	if n <= 0 {
		return nil
	}
	if len(im.scratch) >= n {
		return im.scratch[:n]
	}
	return make([]uint8, n)
}

// ToByte 把 [0,1] 的强度统一换算成 8 位像素值，先夹取再截断。
// 覆盖度、着色与 gamma 三条路径都只调用这一份换算，避免各自写一套
// uint8(255*…) 而在边界上产生分歧。
func ToByte(t float64) uint8 {
	if t <= 0 {
		return 0
	}
	if t >= 1 {
		return 255
	}
	return uint8(255 * t)
}

func New(w, h int) (*Image, error) {
	return NewWithScratch(w, h, 0)
}

// NewWithScratch 建一张 w×h 的图，并额外预留 scratch 字节的临时区。
// 像素与临时区共用同一次分配：整幅处理始终只有一次整帧分配，临时空间只是
// 这张图尾部多出的一小段，而不是另起一块。
func NewWithScratch(w, h, scratch int) (*Image, error) {
	if w <= 0 || h <= 0 {
		return nil, errors.New("画幅必须为正")
	}
	if scratch < 0 {
		scratch = 0
	}
	buf := make([]uint8, w*h+scratch)
	return &Image{W: w, H: h, Pix: buf[:w*h], scratch: buf[w*h:]}, nil
}

func (im *Image) Fill(polys [][]geom.Point, supersample int) error {
	if supersample < 1 {
		return errors.New("supersample 必须为正")
	}
	for _, p := range polys {
		if len(p) < 3 {
			return errors.New("多边形至少三个顶点")
		}
	}
	minx, miny := polys[0][0].X, polys[0][0].Y
	maxx, maxy := minx, miny
	for _, p := range polys {
		for _, q := range p {
			if q.X < minx {
				minx = q.X
			}
			if q.X > maxx {
				maxx = q.X
			}
			if q.Y < miny {
				miny = q.Y
			}
			if q.Y > maxy {
				maxy = q.Y
			}
		}
	}
	spanx, spany := maxx-minx, maxy-miny
	if spanx == 0 {
		spanx = 1
	}
	if spany == 0 {
		spany = 1
	}
	den := supersample * supersample
	for row := 0; row < im.H; row++ {
		for col := 0; col < im.W; col++ {
			hit := 0
			for sj := 0; sj < supersample; sj++ {
				for si := 0; si < supersample; si++ {
					px := minx + (float64(col)+(float64(si)+0.5)/float64(supersample))*spanx/float64(im.W)
					py := miny + (float64(row)+(float64(sj)+0.5)/float64(supersample))*spany/float64(im.H)
					for _, p := range polys {
						if geom.Inside(p, px, py) {
							hit++
							break
						}
					}
				}
			}
			im.Pix[row*im.W+col] = ToByte(float64(hit) / float64(den))
		}
	}
	return nil
}

func (im *Image) SavePGM(path string) error {
	if im.W == 0 || im.H == 0 {
		return errors.New("空图不能导出")
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := fmt.Fprintf(f, "P5\n%d %d\n255\n", im.W, im.H); err != nil {
		return err
	}
	_, err = f.Write(im.Pix)
	return err
}

func (im *Image) CountOn(threshold uint8) int {
	n := 0
	for _, v := range im.Pix {
		if v >= threshold {
			n++
		}
	}
	return n
}
