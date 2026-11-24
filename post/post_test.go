package post

import (
	"errors"
	"math"
	"testing"

	"vecgo/param"
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

// 与 config 共用同一上限：超出 [0,16] 的半径以前会被静默接受。
func TestBlurRejectsRadiusOverMax(t *testing.T) {
	im, _ := raster.New(8, 8)
	_, err := Blur(im, param.BlurMax+1)
	assertField(t, err, "post.Blur.radius")
}

func TestRejectsNaN(t *testing.T) {
	im, _ := raster.New(2, 2)
	assertField(t, Gamma(im, math.NaN()), "post.Gamma.gamma")
}

func TestRejectsNilImage(t *testing.T) {
	assertField(t, Gamma(nil, 1), "post.Gamma.im")
	_, err := Blur(nil, 0)
	assertField(t, err, "post.Blur.im")
}

func assertField(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("应报错并指明 %s", want)
	}
	var pe *param.Error
	if !errors.As(err, &pe) {
		t.Fatalf("错误类型应为 *param.Error，实际 %T: %v", err, err)
	}
	if pe.Field != want {
		t.Fatalf("字段应为 %s，实际 %s", want, pe.Field)
	}
}
