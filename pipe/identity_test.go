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

func testScene() [][]geom.Point {
	return [][]geom.Point{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}}
}

func runAndHash(t *testing.T, cfg config.Config) (string, int) {
	t.Helper()
	im, err := Run(testScene(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "x.pgm")
	if err := im.SavePGM(p); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), im.CountOn(128)
}

// 这些 sha256 在任何改动之前的 HEAD fad87b8 上录制（与基线 CLI/管线一致）。
const (
	hashFlat64      = "fbda3e5665174433272beab4f25172bc03466e3f8700bcf6007b32c3636f2dc3"
	hashGradient64  = "b31af655f7ee15cb9cc4133a6d4e95ad69b8b9c0fd1e82d9728278189b4224a7"
	hashGradient256 = "005f598269f05555e7b788edc07e6bec4375a9e2a1b1046cf95886c692a742a7"
)

func TestDefaultArtifactsByteIdentical(t *testing.T) {
	// config.Default()：flat 64，LevelDist 缺省为 linear。
	if h, on := runAndHash(t, config.Default()); h != hashFlat64 || on != 4096 {
		t.Fatalf("默认 flat64 产物改变: sha=%s on=%d", h, on)
	}

	g64 := config.Default()
	g64.Shading = "gradient"
	if h, _ := runAndHash(t, g64); h != hashGradient64 {
		t.Fatalf("gradient64 默认线性产物改变: %s", h)
	}

	g256 := config.Default()
	g256.Shading = "gradient"
	g256.Width, g256.Height = 256, 256
	if h, _ := runAndHash(t, g256); h != hashGradient256 {
		t.Fatalf("gradient256 默认线性产物改变: %s", h)
	}

	// 零值 LevelDist（空串）也必须走同一条历史路径。
	empty := g64
	empty.LevelDist = ""
	if h, _ := runAndHash(t, empty); h != hashGradient64 {
		t.Fatalf("LevelDist=\"\" 应与默认线性逐字节一致: %s", h)
	}
}

func TestPerceptualPathChangesOutput(t *testing.T) {
	lin := config.Default()
	lin.Shading = "gradient"
	hLin, _ := runAndHash(t, lin)

	perc := config.Default()
	perc.Shading = "gradient"
	perc.LevelDist = "perceptual"
	hPerc, _ := runAndHash(t, perc)
	if hPerc == hLin {
		t.Fatal("开启 perceptual 后产物应当改变，否则开关未生效")
	}

	lab := config.Default()
	lab.Shading = "gradient"
	lab.LevelDist = "lab"
	if h, _ := runAndHash(t, lab); h == hLin {
		t.Fatal("开启 lab 后产物应当改变")
	}

	bad := config.Default()
	bad.LevelDist = "bogus"
	if _, err := Run(testScene(), bad); err == nil {
		t.Fatal("非法 LevelDist 应被拒绝")
	}
}
