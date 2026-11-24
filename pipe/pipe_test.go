package pipe

import (
	"errors"
	"math"
	"testing"

	"vecgo/config"
	"vecgo/geom"
	"vecgo/param"
)

func TestRunProducesPixels(t *testing.T) {
	scene := [][]geom.Point{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}}
	cfg := config.Default()
	cfg.Width, cfg.Height = 16, 16
	im, err := Run(scene, cfg)
	if err != nil {
		t.Fatal(err)
	}
	on, err := im.CountOn(1)
	if err != nil {
		t.Fatal(err)
	}
	if on == 0 {
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

func TestRunReportsConfigField(t *testing.T) {
	scene := [][]geom.Point{{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 1, Y: 1}}}
	cfg := config.Default()
	cfg.Scale = math.NaN()
	_, err := Run(scene, cfg)
	var pe *param.Error
	if !errors.As(err, &pe) || pe.Field != "config.Scale" {
		t.Fatalf("应指明 config.Scale，实际 %v", err)
	}
}

func TestRunRejectsBadScene(t *testing.T) {
	cfg := config.Default()
	cfg.Width, cfg.Height = 16, 16
	cases := []struct {
		name  string
		scene [][]geom.Point
		want  string
	}{
		{"空场景", nil, "pipe.Run.scene"},
		{"两个顶点", [][]geom.Point{{{X: 0, Y: 0}, {X: 1, Y: 1}}}, "pipe.Run.scene[0]"},
		{"坐标 NaN", [][]geom.Point{{{X: math.NaN()}, {X: 1, Y: 1}, {X: 1, Y: 0}}}, "pipe.Run.scene[0][0]"},
	}
	for _, tc := range cases {
		_, err := Run(tc.scene, cfg)
		var pe *param.Error
		if !errors.As(err, &pe) {
			t.Fatalf("%s: 期望 *param.Error，实际 %v", tc.name, err)
		}
		if pe.Field[0:len(tc.want)] != tc.want {
			t.Fatalf("%s: 期望字段前缀 %s，实际 %s", tc.name, tc.want, pe.Field)
		}
	}
}

func TestTransformRejectsNaN(t *testing.T) {
	pts := []geom.Point{{X: 0, Y: 0}, {X: 1, Y: 1}}
	if _, err := Transform(pts, 1, math.NaN()); err == nil {
		t.Fatal("rotate=NaN 应报错")
	}
	if _, err := Transform(pts, 0, 0); err == nil {
		t.Fatal("scale=0 应报错")
	}
}
