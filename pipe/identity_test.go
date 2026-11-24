package pipe

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"vecgo/config"
	"vecgo/geom"
)

// TestDefaultOutputByteIdentical 锁定硬约束：默认采样配置（Samples=0，
// 即 2×2 确定性超采样）下，cmd 场景经 pipe 导出的 PGM 必须与改动前捕获
// 的黄金字节逐字节相同（黄金文件 sha256 前缀 fbda3e56…）。
func TestDefaultOutputByteIdentical(t *testing.T) {
	scene := [][]geom.Point{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}}
	cfg := config.Default()
	if cfg.Samples != 0 {
		t.Fatalf("默认 Samples 必须为 0 以保留老路径，实际 %d", cfg.Samples)
	}
	im, err := Run(scene, cfg)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "out.pgm")
	if err := im.SavePGM(path); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", "default_square.golden"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("默认产物不再逐字节相同：got %d bytes, want %d bytes", len(got), len(want))
	}
}
