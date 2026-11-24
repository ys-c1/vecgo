package shading

import (
	"strings"
	"testing"

	"vecgo/color"
	"vecgo/raster"
)

// TestGradientInterpolatesLinear 渐变必须在线性光空间插值：
// 中间行的字节应等于线性强度 0.5 的直接量化，而不是任何编码值。
func TestGradientInterpolatesLinear(t *testing.T) {
	im, err := raster.New(3, 5)
	if err != nil {
		t.Fatal(err)
	}
	for i := range im.Pix {
		im.Pix[i] = 1 // 标记为已填充
	}
	if err := Apply(im, "gradient", 1.0, 0.0); err != nil {
		t.Fatal(err)
	}
	want := [5]uint8{255, color.LinearToByte(0.75), color.LinearToByte(0.5),
		color.LinearToByte(0.25), 0}
	for row := 0; row < 5; row++ {
		if got := im.Pix[row*3]; got != want[row] {
			t.Fatalf("row %d: 线性插值错误 got=%d want=%d", row, got, want[row])
		}
	}
}

func TestFlatWritesLinearByte(t *testing.T) {
	im, _ := raster.New(2, 2)
	for i := range im.Pix {
		im.Pix[i] = 7
	}
	if err := Apply(im, "flat", 0.5, 0.5); err != nil {
		t.Fatal(err)
	}
	want := color.LinearToByte(0.5) // 127，与 color.LinearToByte 同一口径
	for i, v := range im.Pix {
		if v != want {
			t.Fatalf("pixel %d: got=%d want=%d", i, v, want)
		}
	}
}

func TestZeroStaysZero(t *testing.T) {
	im, _ := raster.New(2, 1)
	im.Pix[0] = 1
	// im.Pix[1] 保持 0：未填充像素不能被着色
	if err := Apply(im, "flat", 1, 1); err != nil {
		t.Fatal(err)
	}
	if im.Pix[0] != 255 || im.Pix[1] != 0 {
		t.Fatalf("未填充像素被写入: %v", im.Pix)
	}
}

func TestErrorsCarryPackageAndField(t *testing.T) {
	im, _ := raster.New(1, 1)
	if err := Apply(im, "bogus", 0, 0); err == nil ||
		!strings.Contains(err.Error(), "shading:") || !strings.Contains(err.Error(), "mode") {
		t.Fatalf("mode 错误缺少包名/字段名: %v", err)
	}
	if err := Apply(im, "flat", 1.2, 0); err == nil ||
		!strings.Contains(err.Error(), "shading:") || !strings.Contains(err.Error(), "top") {
		t.Fatalf("top 错误缺少包名/字段名: %v", err)
	}
	if err := Apply(im, "flat", 0, -0.1); err == nil ||
		!strings.Contains(err.Error(), "shading:") || !strings.Contains(err.Error(), "bottom") {
		t.Fatalf("bottom 错误缺少包名/字段名: %v", err)
	}
}
