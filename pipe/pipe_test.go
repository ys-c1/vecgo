package pipe

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
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

func TestRunParallelMatchesSerial(t *testing.T) {
	scene := [][]geom.Point{
		{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}},
		{{X: 2, Y: 8}, {X: 8, Y: 8}, {X: 5, Y: 2}},
	}
	serialCfg := config.Default()
	serialCfg.Width, serialCfg.Height = 128, 128
	serial, err := Run(scene, serialCfg)
	if err != nil {
		t.Fatal(err)
	}

	parCfg := serialCfg
	parCfg.Blocks, parCfg.Workers = 8, 4
	parallel, err := Run(scene, parCfg)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(serial.Pix, parallel.Pix) {
		t.Fatal("端到端串行与并行管线产出的 PGM 像素必须逐字节相同")
	}
}

func TestRunParallelRejectsBadBlocks(t *testing.T) {
	scene := [][]geom.Point{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}}
	cfg := config.Default()
	cfg.Width, cfg.Height = 64, 10
	cfg.Blocks, cfg.Workers = 3, 2 // 3 不能整除高度 10
	if _, err := Run(scene, cfg); err == nil || !strings.Contains(err.Error(), "块号 2") {
		t.Fatalf("不能整除时必须返回带块号的错误，不允许静默降级: %v", err)
	}
}

func TestPGMByteIdenticalAcrossPaths(t *testing.T) {
	scene := [][]geom.Point{
		{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}},
		{{X: 2, Y: 8}, {X: 8, Y: 8}, {X: 5, Y: 2}},
	}
	dir := t.TempDir()
	serialCfg := config.Default()
	serialCfg.Width, serialCfg.Height = 96, 96
	serialCfg.Output = filepath.Join(dir, "serial.pgm")
	serial, err := Run(scene, serialCfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := serial.SavePGM(serialCfg.Output); err != nil {
		t.Fatal(err)
	}

	parCfg := serialCfg
	parCfg.Blocks, parCfg.Workers = 6, 3
	parCfg.Output = filepath.Join(dir, "parallel.pgm")
	parallel, err := Run(scene, parCfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := parallel.SavePGM(parCfg.Output); err != nil {
		t.Fatal(err)
	}

	a, err := os.ReadFile(serialCfg.Output)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(parCfg.Output)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a, b) {
		t.Fatalf("两条路径的 PGM 必须逐字节相同: %d 字节 vs %d 字节", len(a), len(b))
	}
}
