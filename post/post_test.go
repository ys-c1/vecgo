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

func TestGammaRectMatchesFull(t *testing.T) {
	full, _ := raster.New(6, 5)
	for i := range full.Pix {
		full.Pix[i] = uint8(i * 7)
	}
	part := full.Clone()
	r := raster.Rect{X0: 1, Y0: 2, X1: 4, Y1: 4}
	if err := Gamma(full, 2.2); err != nil {
		t.Fatal(err)
	}
	if err := GammaRect(part, 2.2, r); err != nil {
		t.Fatal(err)
	}
	for y := r.Y0; y < r.Y1; y++ {
		for x := r.X0; x < r.X1; x++ {
			i := y*6 + x
			if full.Pix[i] != part.Pix[i] {
				t.Fatalf("像素(%d,%d): 全量=%d 区域=%d", x, y, full.Pix[i], part.Pix[i])
			}
		}
	}
	if err := GammaRect(part, 2, raster.Rect{X0: 0, Y0: 0, X1: 7, Y1: 5}); err == nil {
		t.Fatal("越界区域应报错")
	}
}

func TestBlurIntoMatchesFull(t *testing.T) {
	src, _ := raster.New(9, 9)
	for i := range src.Pix {
		src.Pix[i] = uint8((i * 31) % 256)
	}
	full, err := Blur(src, 2)
	if err != nil {
		t.Fatal(err)
	}
	part, _ := raster.New(9, 9)
	r := raster.Rect{X0: 2, Y0: 3, X1: 6, Y1: 7}
	if err := BlurInto(src, part, 2, []raster.Rect{r}); err != nil {
		t.Fatal(err)
	}
	for y := r.Y0; y < r.Y1; y++ {
		for x := r.X0; x < r.X1; x++ {
			if part.Pix[y*9+x] != full.Pix[y*9+x] {
				t.Fatalf("像素(%d,%d): 局部=%d 全量=%d", x, y, part.Pix[y*9+x], full.Pix[y*9+x])
			}
		}
	}
	// 区域外必须保持原样。
	if part.Pix[0] != 0 {
		t.Fatalf("区域外像素被改动: %d", part.Pix[0])
	}
	if err := BlurInto(src, part, 1, []raster.Rect{{X0: 0, Y0: 0, X1: 10, Y1: 9}}); err == nil {
		t.Fatal("越界区域应报错")
	}
}
