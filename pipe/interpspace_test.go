package pipe

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"testing"

	"vecgo/band"
	"vecgo/config"
	"vecgo/geom"
	"vecgo/post"
)

var testScene = [][]geom.Point{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}}

func pgmBytes(t *testing.T, cfg config.Config) []byte {
	t.Helper()
	im, err := Run(testScene, cfg)
	if err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	fmt.Fprintf(&b, "P5\n%d %d\n255\n", im.W, im.H)
	b.Write(im.Pix)
	return b.Bytes()
}

func loadGolden(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestDefaultSpaceByteIdentical 锁定“默认插值空间下产物与现在逐字节相同”。
// 金标准来自改动前 HEAD fad87b8 构建的实际输出，另用 sha256 双保险。
func TestDefaultSpaceByteIdentical(t *testing.T) {
	cases := []struct {
		name   string
		cfg    config.Config
		golden string
		sha    string
	}{
		{"default_flat64", config.Default(), "default_flat64.golden",
			"fbda3e5665174433272beab4f25172bc03466e3f8700bcf6007b32c3636f2dc3"},
		{"gradient64", gradientCfg(64, 64), "gradient64.golden",
			"b31af655f7ee15cb9cc4133a6d4e95ad69b8b9c0fd1e82d9728278189b4224a7"},
		{"gradient256", gradientCfg(256, 256), "gradient256.golden",
			"005f598269f05555e7b788edc07e6bec4375a9e2a1b1046cf95886c692a742a7"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := pgmBytes(t, tc.cfg)
			want := loadGolden(t, tc.golden)
			if !bytes.Equal(got, want) {
				t.Fatalf("与金标准不是逐字节相同: got %d bytes want %d bytes", len(got), len(want))
			}
			if h := fmt.Sprintf("%x", sha256.Sum256(got)); h != tc.sha {
				t.Fatalf("sha256 漂移: got %s want %s", h, tc.sha)
			}
		})
	}
}

// TestEncodedAliasesIdentical 空串/encoded/常见别名必须走到同一条历史路径。
func TestEncodedAliasesIdentical(t *testing.T) {
	base := gradientCfg(64, 64)
	ref := pgmBytes(t, base)
	for _, sp := range []string{"", "encoded", "srgb", "gamma", "default"} {
		cfg := base
		cfg.InterpSpace = sp
		if got := pgmBytes(t, cfg); !bytes.Equal(got, ref) {
			t.Fatalf("space=%q 与默认 encoded 产物不一致", sp)
		}
	}
}

// TestSpaceOptionChangesGradient 新空间必须确实改变渐变（否则选项无意义），且端点保持黑白。
func TestSpaceOptionChangesGradient(t *testing.T) {
	base := gradientCfg(64, 64)
	ref := pgmBytes(t, base)
	for _, sp := range []string{"linear", "perceptual"} {
		cfg := base
		cfg.InterpSpace = sp
		got := pgmBytes(t, cfg)
		if bytes.Equal(got, ref) {
			t.Fatalf("space=%s 应当改变渐变字节", sp)
		}
		if got[len("P5\n64 64\n255\n")] != 255 {
			t.Fatalf("space=%s 顶端应为 255", sp)
		}
	}
}

// TestPredictionMatchesMeasurement 两档画幅 × 三种空间：
// 闭式/量化预测的等值段数与 Weber≥0.10 临界接缝数，必须与真实渲染图中列出来的一致。
func TestPredictionMatchesMeasurement(t *testing.T) {
	for _, size := range []int{64, 256} {
		for _, sp := range []string{"encoded", "linear", "perceptual"} {
			cfg := gradientCfg(size, size)
			cfg.InterpSpace = sp
			im, err := Run(testScene, cfg)
			if err != nil {
				t.Fatal(err)
			}
			col := band.Column(im, size/2)

			var lut *[256]uint8
			if config.NormalizeSpace(sp) == "encoded" {
				tb := post.GammaTable(1)
				lut = &tb
			}
			pred := band.PredictColumn(size, sp, lut)

			if got, want := band.CountRuns(col), band.CountRuns(pred); got != want {
				t.Fatalf("size=%d space=%s 实测段数 %d != 预测 %d", size, sp, got, want)
			}
			if got, want := band.CountWeberCritical(col, 0.10), band.CountWeberCritical(pred, 0.10); got != want {
				t.Fatalf("size=%d space=%s 实测临界接缝 %d != 预测 %d", size, sp, got, want)
			}
		}
	}
}

func gradientCfg(w, h int) config.Config {
	c := config.Default()
	c.Width, c.Height = w, h
	c.Shading = "gradient"
	return c
}
