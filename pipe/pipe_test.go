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

func TestRunHonoursEvenOdd(t *testing.T) {
	// A triangle traced twice the same way: winding 2 everywhere, so even-odd
	// discards the whole shape while non-zero paints it.
	tri := []geom.Point{{X: 0, Y: -90}, {X: 78, Y: 45}, {X: -78, Y: 45}}
	poly := append(append([]geom.Point{}, tri...), tri...)
	cfg := config.Default()
	cfg.Width, cfg.Height = 48, 48
	cfg.FillRule = "non-zero"
	nz, err := Run([][]geom.Point{poly}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	cfg.FillRule = "even-odd"
	eo, err := Run([][]geom.Point{poly}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if eo.CountCovered() != 0 {
		t.Fatalf("even-odd 应完全不覆盖，得到 %d", eo.CountCovered())
	}
	if nz.CountCovered() == 0 {
		t.Fatal("non-zero 应有覆盖")
	}
}
