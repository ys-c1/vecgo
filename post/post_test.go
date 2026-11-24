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

func TestBlurZeroCopies(t *testing.T) {
	im, _ := raster.New(2, 2)
	out, err := Blur(im, 0)
	if err != nil {
		t.Fatal(err)
	}
	if out == im {
		t.Fatal("应返回新图")
	}
}

// referenceBlur 是改造前 Blur 的逐字实现：固定分配第二张整帧。
// 用它作为语义参照，核对原地版本在边界窗口下逐像素一致。
func referenceBlur(im *raster.Image, radius int) []uint8 {
	out := make([]uint8, im.W*im.H)
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

func fillPattern(im *raster.Image) {
	for y := 0; y < im.H; y++ {
		for x := 0; x < im.W; x++ {
			im.Pix[y*im.W+x] = uint8((x*37 + y*101 + x*y*3) % 256)
		}
	}
}

func TestBlurIntoMatchesReference(t *testing.T) {
	for _, size := range []struct{ w, h int }{{1, 1}, {1, 7}, {9, 1}, {5, 5}, {17, 13}, {32, 32}} {
		for radius := 0; radius <= 6; radius++ {
			src, _ := raster.New(size.w, size.h)
			fillPattern(src)
			want := referenceBlur(src, radius)

			got, _ := raster.New(size.w, size.h)
			copy(got.Pix, src.Pix)
			if err := BlurInto(got, radius); err != nil {
				t.Fatalf("BlurInto(%dx%d,r=%d): %v", size.w, size.h, radius, err)
			}
			for i := range want {
				if got.Pix[i] != want[i] {
					t.Fatalf("%dx%d r=%d 第 %d 像素不一致: got=%d want=%d", size.w, size.h, radius, i, got.Pix[i], want[i])
				}
			}
		}
	}
}

func TestBlurIntoInPlaceNoKeepBytes(t *testing.T) {
	// 就地版本返回后，im 就是模糊结果；把它与 Blur（返回新图）的结果对齐。
	im1, _ := raster.New(16, 16)
	im2, _ := raster.New(16, 16)
	fillPattern(im1)
	copy(im2.Pix, im1.Pix)
	if err := BlurInto(im1, 2); err != nil {
		t.Fatal(err)
	}
	out, err := Blur(im2, 2)
	if err != nil {
		t.Fatal(err)
	}
	for i := range out.Pix {
		if im1.Pix[i] != out.Pix[i] {
			t.Fatalf("就地与返回新图结果不一致: %d", i)
		}
	}
}

func TestBlurKeepsInput(t *testing.T) {
	im, _ := raster.New(8, 8)
	fillPattern(im)
	before := append([]uint8(nil), im.Pix...)
	if _, err := Blur(im, 2); err != nil {
		t.Fatal(err)
	}
	for i := range before {
		if im.Pix[i] != before[i] {
			t.Fatalf("Blur 不应改动入参: 第 %d 像素", i)
		}
	}
}

func TestBlurZeroStillCopies(t *testing.T) {
	// Blur(…, 0) 契约不变：返回新图且像素相同，并计一次整帧拷贝。
	im, _ := raster.New(4, 4)
	fillPattern(im)
	before := FullFrameCopies.Load()
	out, err := Blur(im, 0)
	if err != nil {
		t.Fatal(err)
	}
	if out == im {
		t.Fatal("应返回新图")
	}
	if FullFrameCopies.Load() != before+1 {
		t.Fatal("Blur 每次整帧拷贝应计一次")
	}
	for i := range im.Pix {
		if out.Pix[i] != im.Pix[i] {
			t.Fatal("radius=0 应原样拷贝")
		}
	}
}

func TestBlurIntoReportsNoFullFrameCopy(t *testing.T) {
	im, _ := raster.New(32, 32)
	fillPattern(im)
	before := FullFrameCopies.Load()
	if err := BlurInto(im, 3); err != nil {
		t.Fatal(err)
	}
	if got := FullFrameCopies.Load(); got != before {
		t.Fatalf("BlurInto 不应整帧拷贝, 计数从 %d 变为 %d", before, got)
	}
}

func TestBlurIntoZeroNoop(t *testing.T) {
	im, _ := raster.New(5, 4)
	fillPattern(im)
	before := append([]uint8(nil), im.Pix...)
	if err := BlurInto(im, 0); err != nil {
		t.Fatal(err)
	}
	for i := range before {
		if im.Pix[i] != before[i] {
			t.Fatalf("radius=0 应原样不动: 第 %d 像素", i)
		}
	}
}

func TestBlurIntoRejectsNegative(t *testing.T) {
	im, _ := raster.New(3, 3)
	if err := BlurInto(im, -1); err == nil {
		t.Fatal("radius<0 应报错")
	}
}
