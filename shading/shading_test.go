package shading

import (
	"testing"

	"vecgo/raster"
)

func TestApplyRectMatchesFull(t *testing.T) {
	for _, mode := range []string{"flat", "gradient"} {
		full, _ := raster.New(6, 7)
		for i := range full.Pix {
			full.Pix[i] = 1
		}
		part := full.Clone()
		if err := Apply(full, mode, 1.0, 0.0); err != nil {
			t.Fatal(err)
		}
		if err := ApplyRect(part, mode, 1.0, 0.0, raster.Rect{X0: 1, Y0: 2, X1: 5, Y1: 6}); err != nil {
			t.Fatal(err)
		}
		for y := 2; y < 6; y++ {
			for x := 1; x < 5; x++ {
				if part.Pix[y*6+x] != full.Pix[y*6+x] {
					t.Fatalf("%s 像素(%d,%d): 局部=%d 全量=%d", mode, x, y, part.Pix[y*6+x], full.Pix[y*6+x])
				}
			}
		}
	}
}

func TestApplyRectRejectsBadInput(t *testing.T) {
	im, _ := raster.New(4, 4)
	if err := ApplyRect(im, "flat", 1, 0, raster.Rect{X0: 0, Y0: 0, X1: 5, Y1: 4}); err == nil {
		t.Fatal("越界应报错")
	}
	if err := ApplyRect(im, "nope", 1, 0, raster.Rect{X0: 0, Y0: 0, X1: 4, Y1: 4}); err == nil {
		t.Fatal("未知模式应报错")
	}
}
