package shading

import (
	"math"
	"testing"

	"vecgo/raster"
	"vecgo/verr"
)

func TestApplyKinds(t *testing.T) {
	im, _ := raster.New(4, 3)
	im.Pix[0] = 9
	cases := []struct {
		name string
		err  error
		want verr.Kind
	}{
		{"nil", Apply(nil, "flat", 1, 0), verr.KindShadingNilImage},
		{"mode", Apply(im, "bogus", 1, 0), verr.KindShadingMode},
		{"top", Apply(im, "flat", 1.5, 0), verr.KindShadingStrength},
		{"bottom", Apply(im, "gradient", 1, 2), verr.KindShadingStrength},
		{"nan", Apply(im, "flat", math.NaN(), 0), verr.KindShadingStrength},
	}
	for _, tc := range cases {
		k, ok := verr.Of(tc.err)
		if !ok {
			t.Fatalf("%s 未归一: %v", tc.name, tc.err)
		}
		if k != tc.want {
			t.Fatalf("%s 期望 %s 实际 %s", tc.name, tc.want.Hex(), k.Hex())
		}
		if k.Stage() != verr.StageShading {
			t.Fatalf("%s 段不对: %v", tc.name, k.Stage())
		}
	}
}

func TestApplyFlatAndGradient(t *testing.T) {
	im, _ := raster.New(3, 3)
	im.Pix[0] = 1
	im.Pix[3] = 1
	im.Pix[6] = 1
	if err := Apply(im, "flat", 1, 0); err != nil {
		t.Fatal(err)
	}
	if im.Pix[0] != 255 || im.Pix[3] != 255 || im.Pix[6] != 255 {
		t.Fatalf("flat 应全为 255: %v", []uint8{im.Pix[0], im.Pix[3], im.Pix[6]})
	}
	if im.Pix[1] != 0 {
		t.Fatal("零像素不应被着色")
	}

	im2, _ := raster.New(3, 3)
	for i := range im2.Pix {
		im2.Pix[i] = 1
	}
	if err := Apply(im2, "gradient", 1, 0); err != nil {
		t.Fatal(err)
	}
	if im2.Pix[0] != 255 || im2.Pix[8] != 0 {
		t.Fatalf("gradient 应从亮到暗: %d -> %d", im2.Pix[0], im2.Pix[8])
	}
}
