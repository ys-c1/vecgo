package pipe

import (
	"bytes"
	"math"
	"os"
	"path/filepath"
	"testing"

	"vecgo/config"
	"vecgo/geom"
)

// square 与 golden 基准图使用的场景一致。
func square() [][]geom.Point {
	return [][]geom.Point{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}}
}

// TestGoldenByteIdentical 锁住“诊断关闭时输出逐字节不变”。
// testdata/*.golden 由改动前的管线生成；校验逻辑不得改变任何合法
// 配置的像素输出。
func TestGoldenByteIdentical(t *testing.T) {
	cases := []struct {
		file string
		mut  func(*config.Config)
	}{
		{"default.golden", func(c *config.Config) {}},
		{"tiny-scale-low-gamma.golden", func(c *config.Config) {
			c.Width, c.Height = 16, 16
			c.Scale = 0.001
			c.Gamma = 0.1
		}},
		{"wide-rot-gradient-blur.golden", func(c *config.Config) {
			c.Width, c.Height = 32, 24
			c.Scale = 100
			c.Rotate = 1.5
			c.Supersample = 8
			c.Shading = "gradient"
			c.Gamma = 10
			c.Blur = 16
		}},
		{"rot-neg-ss1.golden", func(c *config.Config) {
			c.Width, c.Height = 8, 8
			c.Scale = 0.5
			c.Rotate = -3.14159
			c.Supersample = 1
			c.Gamma = 2.5
			c.Blur = 3
		}},
		{"flat-blur0-ss1.golden", func(c *config.Config) {
			c.Width, c.Height = 12, 20
			c.Supersample = 1
			c.Blur = 0
		}},
	}
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			cfg := config.Default()
			tc.mut(&cfg)
			im, err := Run(square(), cfg)
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			got := filepath.Join(dir, "out.pgm")
			if err := im.SavePGM(got); err != nil {
				t.Fatal(err)
			}
			gotPix, err := os.ReadFile(got)
			if err != nil {
				t.Fatal(err)
			}
			wantPix, err := os.ReadFile(filepath.Join("testdata", tc.file))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(gotPix, wantPix) {
				t.Fatalf("输出与改动前不一致: got %d 字节, want %d 字节", len(gotPix), len(wantPix))
			}
		})
	}
}

// TestRunRejectsNonFinite 确认 NaN/Inf 不再进入渲染。
func TestRunRejectsNonFinite(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*config.Config)
	}{
		{"scale=NaN", func(c *config.Config) { c.Scale = math.NaN() }},
		{"rotate=NaN", func(c *config.Config) { c.Rotate = math.NaN() }},
		{"rotate=Inf", func(c *config.Config) { c.Rotate = math.Inf(1) }},
		{"gamma=NaN", func(c *config.Config) { c.Gamma = math.NaN() }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Default()
			cfg.Width, cfg.Height = 16, 16
			tc.mut(&cfg)
			im, err := Run(square(), cfg)
			if err == nil {
				t.Fatalf("应拒绝并返回 nil 图，实际返回 %dx%d", im.W, im.H)
			}
			if im != nil {
				t.Fatal("拒绝时不应返回图像")
			}
		})
	}
}
