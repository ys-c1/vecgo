package shading

import (
	"math"
	"testing"

	"vecgo/raster"
)

func TestApplyNonFinite(t *testing.T) {
	im, _ := raster.New(2, 2)
	for _, tc := range []struct{ top, bottom float64 }{
		{math.NaN(), 0},
		{1, math.NaN()},
		{math.Inf(1), 0},
		{0, math.Inf(-1)},
	} {
		if err := Apply(im, "flat", tc.top, tc.bottom); err == nil {
			t.Fatalf("top=%v bottom=%v 应报错", tc.top, tc.bottom)
		}
	}
}

func TestApplyValid(t *testing.T) {
	im, _ := raster.New(2, 1)
	im.Pix[0] = 1
	if err := Apply(im, "gradient", 1, 0); err != nil {
		t.Fatal(err)
	}
	if im.Pix[0] == 0 {
		t.Fatal("应写入着色值")
	}
}
