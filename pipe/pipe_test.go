package pipe

import (
	"testing"

	"vecgo/config"
	"vecgo/geom"
)

func TestRunProducesPixels(t *testing.T) {
	scene := [][]geom.Point{{{0, 0}, {10, 0}, {10, 10}, {0, 10}}}
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
	scene := [][]geom.Point{{{0, 0}, {1, 0}, {1, 1}}}
	cfg := config.Default()
	cfg.Gamma = 0
	if _, err := Run(scene, cfg); err == nil {
		t.Fatal("gamma=0 应报错")
	}
}
