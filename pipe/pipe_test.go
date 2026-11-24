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
	res, err := Run(scene, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if res.Weighted.CountOn(1) == 0 {
		t.Fatal("应有像素被点亮")
	}
	if res.Binary == nil {
		t.Fatal("coverage 模式应生成二值图")
	}
	if res.RelErr >= 1e-3 {
		t.Fatalf("加权面积相对误差过大: %.3e", res.RelErr)
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

func TestRunSupersampleMode(t *testing.T) {
	scene := [][]geom.Point{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}}
	cfg := config.Default()
	cfg.Width, cfg.Height = 16, 16
	cfg.FillMode = "supersample"
	res, err := Run(scene, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if res.Cov != nil || res.Binary != nil {
		t.Fatal("采样模式不应生成覆盖率与二值图")
	}
	if res.SupersampleArea <= 0 {
		t.Fatal("采样面积应大于零")
	}
}

func TestRunCoverageAreaMatchesGreen(t *testing.T) {
	tri := []geom.Point{{X: 0, Y: 0}, {X: 7, Y: 0}, {X: 2.5, Y: 5}}
	cfg := config.Default()
	cfg.Width, cfg.Height = 64, 64
	cfg.Scale = 3
	cfg.Rotate = 0.7
	res, err := Run([][]geom.Point{tri}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if res.RelErr >= 1e-9 {
		t.Fatalf("旋转缩放后加权面积相对误差过大: %.3e", res.RelErr)
	}
	if res.BinDiff > res.BinBound+1e-9 {
		t.Fatalf("二值差异 %v 超过上界 %v", res.BinDiff, res.BinBound)
	}
}
