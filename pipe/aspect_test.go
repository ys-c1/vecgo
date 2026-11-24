package pipe

import (
	"testing"

	"vecgo/config"
	"vecgo/geom"
)

func TestRunKeepsSquareProportionsAcrossAspectFrames(t *testing.T) {
	scene := [][]geom.Point{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}}
	cfg := config.Default()
	cfg.Width, cfg.Height = 32, 16
	cfg.Supersample = 1

	im, err := Run(scene, cfg)
	if err != nil {
		t.Fatal(err)
	}
	y := 8
	if im.Pix[y*im.W+0] != 0 {
		t.Fatal("宽画幅左侧应有 contain 留白")
	}
	if im.Pix[y*im.W+8] == 0 {
		t.Fatal("居中正方形左缘内部应被填充")
	}
	if im.Pix[y*im.W+23] == 0 {
		t.Fatal("居中正方形右缘内部应被填充")
	}
	if im.Pix[y*im.W+31] != 0 {
		t.Fatal("宽画幅右侧应有 contain 留白")
	}
}
