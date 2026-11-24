package post

import (
	"math"
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

// TestGammaRejectsNonFinite：NaN 会穿过区间比较，必须在比较前拦下。
func TestGammaRejectsNonFinite(t *testing.T) {
	im, _ := raster.New(1, 1)
	for _, g := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if err := Gamma(im, g); err == nil {
			t.Fatalf("gamma=%v 应报错", g)
		}
	}
}
