package post

import (
	"strings"
	"testing"

	"vecgo/color"
	"vecgo/raster"
)

// legacyPow 是统一前 post 包私有的手写幂函数，保留在测试里用于量化
// 统一前后的残差来源：它在小数指数上只是一阶线性近似。
func legacyPow(a, b float64) float64 {
	if a <= 0 {
		return 0
	}
	r := 1.0
	e := b
	for e > 0.001 {
		if e > 1 {
			r *= a
			e -= 1
		} else {
			r *= 1 + (a-1)*e
			e = 0
		}
	}
	return r
}

func legacyLUT(gamma float64) [256]uint8 {
	var table [256]uint8
	for i := 0; i < 256; i++ {
		table[i] = uint8(255 * legacyPow(float64(i)/255, 1/gamma))
	}
	return table
}

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

func TestGammaOneIsIdentity(t *testing.T) {
	// 统一后 gamma=1 的表必须是严格恒等表。
	table, err := color.EncodeLUT(1)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 256; i++ {
		if table[i] != uint8(i) {
			t.Fatalf("gamma=1 不是恒等表: i=%d -> %d", i, table[i])
		}
	}
	im, _ := raster.New(2, 1)
	im.Pix[0], im.Pix[1] = 37, 214
	keep := [2]uint8{im.Pix[0], im.Pix[1]}
	if err := Gamma(im, 1); err != nil {
		t.Fatal(err)
	}
	if im.Pix[0] != keep[0] || im.Pix[1] != keep[1] {
		t.Fatalf("gamma=1 改动了像素: %v", im.Pix)
	}
}

func TestGammaUsesSharedColorTable(t *testing.T) {
	// post 不再持有私有转换：输出必须与 color.EncodeLUT 逐值相同。
	for _, g := range []float64{0.5, 1, 2.2} {
		want, err := color.EncodeLUT(g)
		if err != nil {
			t.Fatal(err)
		}
		im, _ := raster.New(256, 1)
		for i := 0; i < 256; i++ {
			im.Pix[i] = uint8(i)
		}
		if err := Gamma(im, g); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 256; i++ {
			if im.Pix[i] != want[i] {
				t.Fatalf("g=%v i=%d: post=%d color=%d", g, i, im.Pix[i], want[i])
			}
		}
	}
}

// TestResidualAgainstLegacyPow 用两档 gamma 量化统一前后的 LUT 残差，
// 说明差异集中在哪里。
func TestResidualAgainstLegacyPow(t *testing.T) {
	cases := []struct {
		gamma          float64
		wantMaxAbs     int
		wantDarkMaxAbs int // 暗部（线性 i<64）的最大绝对差
	}{
		{gamma: 0.5, wantMaxAbs: 0, wantDarkMaxAbs: 0},
		{gamma: 2.2, wantMaxAbs: 119, wantDarkMaxAbs: 119},
	}
	for _, tc := range cases {
		newTable, err := color.EncodeLUT(tc.gamma)
		if err != nil {
			t.Fatal(err)
		}
		old := legacyLUT(tc.gamma)
		maxAbs, darkMaxAbs, changed := 0, 0, 0
		for i := 0; i < 256; i++ {
			d := int(newTable[i]) - int(old[i])
			if d < 0 {
				d = -d
			}
			if d > 0 {
				changed++
			}
			if d > maxAbs {
				maxAbs = d
			}
			if i < 64 && d > darkMaxAbs {
				darkMaxAbs = d
			}
		}
		if maxAbs != tc.wantMaxAbs || darkMaxAbs != tc.wantDarkMaxAbs {
			t.Fatalf("g=%v 残差不符: maxAbs=%d darkMaxAbs=%d changed=%d",
				tc.gamma, maxAbs, darkMaxAbs, changed)
		}
		t.Logf("gamma=%v: %d/256 个表项变化, 全局最大绝对差=%d, 暗部最大绝对差=%d",
			tc.gamma, changed, maxAbs, darkMaxAbs)
	}
}

func TestGammaErrorHasPackageAndField(t *testing.T) {
	im, _ := raster.New(1, 1)
	err := Gamma(im, 0.05)
	if err == nil || !strings.Contains(err.Error(), "post:") ||
		!strings.Contains(err.Error(), "gamma") {
		t.Fatalf("错误缺少包名/字段名: %v", err)
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

func TestBlurErrorHasPackageAndField(t *testing.T) {
	if _, err := Blur(&raster.Image{W: 1, H: 1, Pix: []uint8{0}}, -1); err == nil ||
		!strings.Contains(err.Error(), "post:") || !strings.Contains(err.Error(), "blur") {
		t.Fatalf("错误缺少包名/字段名: %v", err)
	}
}
