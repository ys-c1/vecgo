package pipe

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"vecgo/config"
	"vecgo/dedup"
	"vecgo/geom"
)

func sq(x, y, s float64) []geom.Point {
	return []geom.Point{
		{X: x, Y: y},
		{X: x + s, Y: y},
		{X: x + s, Y: y + s},
		{X: x, Y: y + s},
	}
}

func tri(x, y, s float64) []geom.Point {
	return []geom.Point{{X: x, Y: y}, {X: x + s, Y: y}, {X: x, Y: y + s}}
}

// sceneCases are the required shapes: all-same, partly-same, all-different,
// a single figure and degenerate figures.
func sceneCases() []struct {
	name  string
	scene [][]geom.Point
} {
	collinear := []geom.Point{{X: 0, Y: 0}, {X: 5, Y: 0}, {X: 10, Y: 0}}
	spike := []geom.Point{{X: 10, Y: 50}, {X: 10, Y: 50}, {X: 14, Y: 50}, {X: 30, Y: 52}}
	return []struct {
		name  string
		scene [][]geom.Point
	}{
		{"全同", [][]geom.Point{sq(0, 0, 10), sq(0, 0, 10), sq(0, 0, 10), sq(0, 0, 10)}},
		{"部分同", [][]geom.Point{sq(0, 0, 10), sq(20, 0, 5), sq(0, 0, 10), sq(20, 0, 5), tri(40, 40, 6)}},
		{"部分同（环旋转与反向）", [][]geom.Point{sq(0, 0, 10), {sq(0, 0, 10)[2], sq(0, 0, 10)[3], sq(0, 0, 10)[0], sq(0, 0, 10)[1]}, {sq(0, 0, 10)[0], sq(0, 0, 10)[3], sq(0, 0, 10)[2], sq(0, 0, 10)[1]}, sq(30, 30, 4)}},
		{"全不同", [][]geom.Point{sq(0, 0, 10), sq(20, 0, 10), sq(40, 0, 10), sq(60, 0, 10)}},
		{"单图形", [][]geom.Point{tri(3, 3, 7)}},
		{"退化图形", [][]geom.Point{collinear, spike, collinear, spike, sq(0, 0, 10)}},
	}
}

func pgmBytes(t *testing.T, im interface {
	SavePGM(string) error
}) []byte {
	t.Helper()
	path := filepath.Join(t.TempDir(), "im.pgm")
	if err := im.SavePGM(path); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestRunMergesAndIsByteIdentical(t *testing.T) {
	for _, c := range sceneCases() {
		t.Run(c.name, func(t *testing.T) {
			cfg := config.Default()
			cfg.Width, cfg.Height = 24, 24
			cfg.Supersample = 2

			merged, stats, err := RunWithStats(c.scene, cfg)
			if err != nil {
				t.Fatal(err)
			}
			baseline, err := RenderUndeduplicated(c.scene, cfg)
			if err != nil {
				t.Fatal(err)
			}
			// "画面逐字节不变" is checked on the serialised PGM, not just on
			// the pixel slice.
			if a, b := pgmBytes(t, merged), pgmBytes(t, baseline); !bytes.Equal(a, b) {
				t.Fatalf("%s: 合并后画面不是逐字节不变", c.name)
			}
			if stats.Drawn != stats.Unique {
				t.Fatalf("%s: 绘制次数 %d != 去重后图形数 %d", c.name, stats.Drawn, stats.Unique)
			}
			if stats.Submitted-stats.Unique != stats.Duplicates {
				t.Fatalf("%s: 去重计数不自洽: %+v", c.name, stats)
			}
			if stats.FingerprintBits > stats.RequiredBits {
				t.Fatalf("%s: 实际指纹 %d 超过反解值 %d", c.name, stats.FingerprintBits, stats.RequiredBits)
			}
		})
	}
}

func TestRunSameSignatureNoDedupRegression(t *testing.T) {
	// Run keeps its original signature; a plain call still renders.
	cfg := config.Default()
	cfg.Width, cfg.Height = 8, 8
	im, err := Run([][]geom.Point{sq(0, 0, 10)}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if im.CountOn(1) == 0 {
		t.Fatal("应有像素被点亮")
	}
}

func TestRunReportsEmptyFigure(t *testing.T) {
	cfg := config.Default()
	cfg.Width, cfg.Height = 8, 8
	scene := [][]geom.Point{sq(0, 0, 10), {}, sq(1, 1, 2)}
	_, _, err := RunWithStats(scene, cfg)
	var fe *dedup.FigureError
	if !errors.As(err, &fe) || fe.Kind != dedup.ErrEmptyFigure || fe.Indices[0] != 1 {
		t.Fatalf("应报出空图形号 1: %v", err)
	}
}

func TestRunReportsTooFewVertices(t *testing.T) {
	cfg := config.Default()
	cfg.Width, cfg.Height = 8, 8
	scene := [][]geom.Point{sq(0, 0, 10), {{X: 0, Y: 0}, {X: 1, Y: 1}}}
	_, _, err := RunWithStats(scene, cfg)
	var fe *dedup.FigureError
	if !errors.As(err, &fe) || fe.Kind != dedup.ErrTooFewVertices || fe.Indices[0] != 1 {
		t.Fatalf("应报出顶点不足图形号 1: %v", err)
	}
}

func TestRunReportsFingerprintCollision(t *testing.T) {
	cfg := config.Default()
	cfg.Width, cfg.Height = 8, 8
	// Two distinct figures can only collide when the fingerprint is too short;
	// zero bits make that deterministic.
	scene := [][]geom.Point{sq(0, 0, 10), tri(20, 20, 5)}
	_, err := RenderDeduped(scene, cfg, 0)
	var fe *dedup.FigureError
	if !errors.As(err, &fe) || fe.Kind != dedup.ErrFingerprintCollision {
		t.Fatalf("应报出指纹冲突: %v", err)
	}
	if len(fe.Indices) != 2 || fe.Indices[0] != 0 || fe.Indices[1] != 1 {
		t.Fatalf("应报出两个图形号: %+v", fe)
	}
}
