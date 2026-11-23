package pipe

import (
	"testing"

	"vecgo/config"
	"vecgo/geom"
)

func TestRunProducesPixels(t *testing.T) {
	scene := [][]geom.Point{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}}
	cfg := config.Default()
	cfg.Width, cfg.Height = 16, 16
	im, err := Run(scene, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if im.CountOn(1) == 0 {
		t.Fatal("应有像素被点亮")
	}
}

func TestRunRejectsBadConfig(t *testing.T) {
	scene := [][]geom.Point{{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 1, Y: 1}}}
	cfg := config.Default()
	cfg.Gamma = 0
	if _, err := Run(scene, cfg); err == nil {
		t.Fatal("gamma=0 应报错")
	}
}

// 平坦色与渐变两条既有路径的默认输出不得改变：
// 关闭抖动时，无论抖动字段取什么值，输出都必须逐像素一致。
func TestRunDefaultPathsUnchanged(t *testing.T) {
	scene := [][]geom.Point{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}}
	for _, mode := range []string{"flat", "gradient"} {
		base := config.Default()
		base.Shading = mode
		im1, err := Run(scene, base)
		if err != nil {
			t.Fatal(err)
		}
		mut := base
		mut.Dither = false
		mut.DitherKernel = "bogus"
		mut.DitherStrength = -1
		mut.DitherDepth = 0
		im2, err := Run(scene, mut)
		if err != nil {
			t.Fatalf("%s 路径关闭抖动时不应报错: %v", mode, err)
		}
		for i := range im1.Pix {
			if im1.Pix[i] != im2.Pix[i] {
				t.Fatalf("%s 路径像素 %d 发生变化: %d -> %d", mode, i, im1.Pix[i], im2.Pix[i])
			}
		}
	}
}

// 开启抖动后输出只含合法量化级。
func TestRunWithDither(t *testing.T) {
	scene := [][]geom.Point{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}}
	cfg := config.Default()
	cfg.Shading = "gradient"
	cfg.Dither = true
	cfg.DitherDepth = 4
	im, err := Run(scene, cfg)
	if err != nil {
		t.Fatal(err)
	}
	for i, v := range im.Pix {
		if v%17 != 0 {
			t.Fatalf("像素 %d 不是 4 位深合法级别: %d", i, v)
		}
	}
}
