package shading

import (
	"errors"
	"math"
	"testing"

	"vecgo/param"
	"vecgo/raster"
)

func TestApplyFlat(t *testing.T) {
	im, _ := raster.New(3, 2)
	im.Pix[0] = 255
	im.Pix[5] = 1
	if err := Apply(im, "flat", 0.5, 0); err != nil {
		t.Fatal(err)
	}
	if im.Pix[0] != 127 || im.Pix[5] != 127 {
		t.Fatalf("非零像素应被赋成同一灰度: %d %d", im.Pix[0], im.Pix[5])
	}
	if im.Pix[1] != 0 {
		t.Fatal("零像素应保持 0")
	}
}

func TestApplyGradient(t *testing.T) {
	im, _ := raster.New(1, 3)
	for i := range im.Pix {
		im.Pix[i] = 1
	}
	if err := Apply(im, "gradient", 1, 0); err != nil {
		t.Fatal(err)
	}
	if !(im.Pix[0] == 255 && im.Pix[2] == 0) {
		t.Fatalf("渐变端点错误: %v", im.Pix)
	}
}

func TestRejectsInvalid(t *testing.T) {
	cases := []struct {
		name string
		want string
		run  func() error
	}{
		{"未知模式", "shading.Apply.mode", func() error {
			im, _ := raster.New(2, 2)
			return Apply(im, "bogus", 1, 0)
		}},
		{"top 越界", "shading.Apply.top", func() error {
			im, _ := raster.New(2, 2)
			return Apply(im, "flat", -0.1, 0)
		}},
		{"bottom 越界", "shading.Apply.bottom", func() error {
			im, _ := raster.New(2, 2)
			return Apply(im, "flat", 1, 1.1)
		}},
		{"top NaN（以前静默接受）", "shading.Apply.top", func() error {
			im, _ := raster.New(2, 2)
			return Apply(im, "flat", math.NaN(), 0)
		}},
		{"nil 画布（以前 panic）", "shading.Apply.im", func() error {
			return Apply(nil, "flat", 1, 0)
		}},
	}
	for _, tc := range cases {
		err := tc.run()
		var pe *param.Error
		if !errors.As(err, &pe) || pe.Field != tc.want {
			t.Fatalf("%s: 期望字段 %s，实际 %v", tc.name, tc.want, err)
		}
	}
}
