package pipe

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"vecgo/config"
	"vecgo/geom"
)

// baselineDefaultSHA256 是引入解析覆盖率之前（HEAD fad87b8），
// config.Default() + 默认方块场景导出 PGM 的 sha256。
// 默认子点路径一行未改，此哈希必须保持不变。
const baselineDefaultSHA256 = "fbda3e5665174433272beab4f25172bc03466e3f8700bcf6007b32c3636f2dc3"

func TestDefaultArtifactByteIdentical(t *testing.T) {
	scene := [][]geom.Point{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}}
	im, err := Run(scene, config.Default())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "default.pgm")
	if err := im.SavePGM(path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	if got := hex.EncodeToString(sum[:]); got != baselineDefaultSHA256 {
		t.Fatalf("默认子点产物不再逐字节相同: got %s, want %s", got, baselineDefaultSHA256)
	}
}
