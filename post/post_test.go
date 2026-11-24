package post

import (
	"math"
	"testing"

	"vecgo/raster"
	"vecgo/verr"
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

func TestPostKinds(t *testing.T) {
	im, _ := raster.New(2, 2)
	cases := []struct {
		name string
		err  error
		want verr.Kind
	}{
		{"gamma-nil", Gamma(nil, 2), verr.KindPostNilImage},
		{"gamma-range", Gamma(im, 0), verr.KindPostGamma},
		{"gamma-nan", Gamma(im, math.NaN()), verr.KindPostGamma},
		{"gamma-inf", Gamma(im, math.Inf(1)), verr.KindPostGamma},
		{"blur-nil", errOf(Blur(nil, 1)), verr.KindPostNilImage},
		{"blur-neg", errOf(Blur(im, -1)), verr.KindPostBlurRadius},
	}
	for _, tc := range cases {
		k, ok := verr.Of(tc.err)
		if !ok {
			t.Fatalf("%s 未归一: %v", tc.name, tc.err)
		}
		if k != tc.want {
			t.Fatalf("%s 期望 %s 实际 %s", tc.name, tc.want.Hex(), k.Hex())
		}
		if k.Stage() != verr.StagePost {
			t.Fatalf("%s 段不对: %v", tc.name, k.Stage())
		}
	}
}

func errOf(_ *raster.Image, err error) error { return err }
