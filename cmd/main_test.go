package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"vecgo/config"
	"vecgo/geom"
	"vecgo/pipe"
)

// 统计关闭时，默认场景的产物必须与引入统计之前的基线逐字节一致。
// 基线哈希取自改动前的 cmd 输出（64×64，内容为 0/255 两值，跨平台确定）。
const baselineSHA256 = "fbda3e5665174433272beab4f25172bc03466e3f8700bcf6007b32c3636f2dc3"

func TestDefaultOutputPinned(t *testing.T) {
	scene := [][]geom.Point{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}}
	im, err := pipe.Run(scene, config.Default())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "out.pgm")
	if err := im.SavePGM(path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	if got := hex.EncodeToString(sum[:]); got != baselineSHA256 {
		t.Fatalf("产物与基线不一致: %s != %s", got, baselineSHA256)
	}
}
