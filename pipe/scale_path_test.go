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

var parityScene = [][]geom.Point{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}}

// baselinePGMSHA 是本任务改动前 `go run ./cmd /tmp/base.pgm`（默认
// 64×64、SS=2、flat、gamma=1、blur=0）产物的 sha256。原 bbox 缩放路径
// 必须逐字节保持该结果。
const baselinePGMSHA = "fbda3e5665174433272beab4f25172bc03466e3f8700bcf6007b32c3636f2dc3"

func TestDefaultAndBBoxPathIdentical(t *testing.T) {
	def := config.Default()
	im1, err := Run(parityScene, def)
	if err != nil {
		t.Fatal(err)
	}

	bbox := config.Default()
	bbox.Frame = "bbox"
	im2, err := Run(parityScene, bbox)
	if err != nil {
		t.Fatal(err)
	}
	if len(im1.Pix) != len(im2.Pix) {
		t.Fatal("两路径长度不同")
	}
	for i := range im1.Pix {
		if im1.Pix[i] != im2.Pix[i] {
			t.Fatalf("默认路径与 Frame=bbox 在像素 %d 不一致: %d vs %d", i, im1.Pix[i], im2.Pix[i])
		}
	}
}

func TestDefaultPGMByteHashUnchanged(t *testing.T) {
	cfg := config.Default()
	im, err := Run(parityScene, cfg)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "out.pgm")
	if err := im.SavePGM(p); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	if got := hex.EncodeToString(sum[:]); got != baselinePGMSHA {
		t.Fatalf("默认产物哈希变化:\n got %s\nwant %s", got, baselinePGMSHA)
	}
}

func TestWorldSwitchRunsAndValidates(t *testing.T) {
	bad := config.Default()
	bad.Frame = "world"
	bad.PPU = 0
	if _, err := Run(parityScene, bad); err == nil {
		t.Fatal("world 路径 PPU=0 应被拒绝")
	}

	cfg := config.Default()
	cfg.Frame = "world"
	cfg.PPU = 4
	cfg.MinX, cfg.MinY = -3, -3
	im, err := Run(parityScene, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if im.CountOn(1) == 0 {
		t.Fatal("world 路径应渲染出像素")
	}
}
