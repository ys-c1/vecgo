package post

import (
	"testing"

	"vecgo/raster"
)

func TestGammaBrightens(t *testing.T) {
	im, _ := raster.New(2, 1)
	im.Pix[0] = 128
	if err := Gamma(im, 2); err != nil {
		t.Fatal(err)
	}
	if im.Pix[0] <= 128 {
		t.Fatal("gamma>1 应提亮中间调")
	}
}

func TestBlurSmooths(t *testing.T) {
	im, _ := raster.New(3, 3)
	im.Pix[4] = 255
	out, err := Blur(im, 1)
	if err != nil {
		t.Fatal(err)
	}
	if out.Pix[4] >= 255 {
		t.Fatal("峰值应被拉低")
	}
}

func TestBlurZeroIsInPlace(t *testing.T) {
	im, _ := raster.New(2, 2)
	im.Pix[0], im.Pix[1], im.Pix[2], im.Pix[3] = 1, 2, 3, 4
	out, err := Blur(im, 0)
	if err != nil {
		t.Fatal(err)
	}
	if out != im {
		t.Fatal("blur=0 应在共享缓冲上原地返回同一张图")
	}
	if im.Pix[0] != 1 || im.Pix[1] != 2 || im.Pix[2] != 3 || im.Pix[3] != 4 {
		t.Fatal("blur=0 不应改动像素")
	}
}

// TestBlurMatchesReference 用旧的“分配第二张图”实现作为参考，逐像素
// 核对原地版本；这是改动前后输出一致性的直接证据。
func TestBlurMatchesReference(t *testing.T) {
	for _, size := range [][2]int{{1, 1}, {3, 3}, {7, 5}, {16, 9}} {
		for _, radius := range []int{0, 1, 2, 3, 5} {
			src, _ := raster.New(size[0], size[1])
			for i := range src.Pix {
				src.Pix[i] = uint8((i*37 + 11) % 256)
			}
			want := referenceBlur(src, radius)
			got, err := Blur(src, radius)
			if err != nil {
				t.Fatalf("size=%v radius=%d: %v", size, radius, err)
			}
			for i := range want {
				if got.Pix[i] != want[i] {
					t.Fatalf("size=%v radius=%d 像素 %d: got %d want %d",
						size, radius, i, got.Pix[i], want[i])
				}
			}
		}
	}
}

func referenceBlur(im *raster.Image, radius int) []uint8 {
	out := make([]uint8, len(im.Pix))
	copy(out, im.Pix)
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
			out[y*im.W+x] = uint8(sum / count)
		}
	}
	return out
}
