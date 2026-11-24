package post

import (
	"strings"
	"testing"

	"vecgo/bounds"
	"vecgo/raster"
)

// GammaRect：四条边贴画幅算界内。
func TestGammaRectAcceptsFullFrame(t *testing.T) {
	im, _ := raster.New(4, 4)
	for i := range im.Pix {
		im.Pix[i] = 100
	}
	if err := GammaRect(im, 2, bounds.Full(4, 4)); err != nil {
		t.Fatalf("整幅区域应接受: %v", err)
	}
}

func TestGammaRectRejectsNegativeCorner(t *testing.T) {
	im, _ := raster.New(16, 16)
	err := GammaRect(im, 2, bounds.Rect{X0: -4, Y0: 4, X1: 4, Y1: 12})
	if err == nil {
		t.Fatal("负区域应拒绝")
	}
	for _, want := range []string{"post.GammaRect", "rect=", "负坐标", "(-4, 4)"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("缺少 %q: %v", want, err)
		}
	}
}

func TestGammaRectRejectsBeyondCorner(t *testing.T) {
	im, _ := raster.New(16, 16)
	err := GammaRect(im, 2, bounds.Rect{X0: 8, Y0: 8, X1: 20, Y1: 12})
	if err == nil {
		t.Fatal("超画幅区域应拒绝")
	}
	for _, want := range []string{"post.GammaRect", "超画幅", "(20, 8)"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("缺少 %q: %v", want, err)
		}
	}
}

func TestGammaRectRejectsEmpty(t *testing.T) {
	im, _ := raster.New(16, 16)
	err := GammaRect(im, 2, bounds.Rect{X0: 4, Y0: 0, X1: 4, Y1: 4})
	if err == nil || !strings.Contains(err.Error(), "区域为空") {
		t.Fatalf("空区域应拒绝: %v", err)
	}
}

// Blur 的邻域判定：贴边像素按画幅内邻居求均值，和旧实现一致。
func TestBlurEdgeMatchesReference(t *testing.T) {
	im, _ := raster.New(3, 1)
	im.Pix[0] = 255
	out, err := Blur(im, 1)
	if err != nil {
		t.Fatal(err)
	}
	// (0,0): 邻居 (0,0),(1,0) -> 127; (1,0): 全部 3 个 -> 85; (2,0): (1,0),(2,0) -> 0
	want := []uint8{127, 85, 0}
	for i := range want {
		if out.Pix[i] != want[i] {
			t.Fatalf("第 %d 像素 %d != %d", i, out.Pix[i], want[i])
		}
	}
}

func TestBlurZeroRadiusOnEdge(t *testing.T) {
	im, _ := raster.New(2, 2)
	im.Pix[0] = 200
	out, err := Blur(im, 0)
	if err != nil {
		t.Fatal(err)
	}
	if out.Pix[0] != 200 {
		t.Fatalf("radius=0 应原样拷贝: %d", out.Pix[0])
	}
}
