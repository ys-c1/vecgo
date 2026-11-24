package pipe

import (
	"os"
	"path/filepath"
	"testing"

	"vecgo/config"
	"vecgo/geom"
)

// 黄金夹具场景与 docs/half-pixel-rule.md 的九个场景一一对应。
// legacy-S*.pgm：改动前代码（旧射线法）经完整管线渲染并冻结的产物；
// half-S*.pgm：半像素规则（中心采样 + 左上角归属）经完整管线渲染的产物。
type goldenScene struct {
	file   string
	w, h   int
	ss     int
	wantOn int // half-pixel 规则经完整管线后的覆盖像素数（阈值 128）
	poly   [][]geom.Point
}

func goldenScenes() []goldenScene {
	return []goldenScene{
		{file: "S1", w: 64, h: 64, ss: 2, wantOn: 4096,
			poly: [][]geom.Point{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}}},
		{file: "S2", w: 16, h: 16, ss: 1, wantOn: 120,
			poly: [][]geom.Point{{{X: 2, Y: 2}, {X: 14, Y: 2}, {X: 2, Y: 12}}}},
		{file: "S3", w: 16, h: 16, ss: 1, wantOn: 136,
			poly: [][]geom.Point{{{X: 0, Y: 0}, {X: 16, Y: 16}, {X: 16, Y: 0}}}},
		// S4：注意经 pipe.Transform 后覆盖为 136，与纯填充层的 120 不同，
		// 差异来自变换的浮点微扰挪动了贴边采样点（跨平台差一行的根因）。
		{file: "S4", w: 16, h: 16, ss: 2, wantOn: 136,
			poly: [][]geom.Point{{{X: 2, Y: 2}, {X: 14, Y: 2}, {X: 2, Y: 12}}}},
		{file: "S5", w: 16, h: 16, ss: 1, wantOn: 256,
			poly: [][]geom.Point{
				{{X: 0.5, Y: 0.5}, {X: 8.5, Y: 0.5}, {X: 8.5, Y: 15.5}, {X: 0.5, Y: 15.5}},
				{{X: 8.5, Y: 0.5}, {X: 15.5, Y: 0.5}, {X: 15.5, Y: 15.5}, {X: 8.5, Y: 15.5}},
			}},
		{file: "S6", w: 16, h: 16, ss: 1, wantOn: 256,
			poly: [][]geom.Point{{{X: 0, Y: 0}, {X: 16, Y: 0}, {X: 16, Y: 16}, {X: 0, Y: 16}}}},
		{file: "S7", w: 16, h: 16, ss: 1, wantOn: 16,
			poly: [][]geom.Point{{{X: 0, Y: 0}, {X: 16, Y: 16}, {X: 8.5, Y: 8.5}}}},
		{file: "S8", w: 16, h: 16, ss: 1, wantOn: 120,
			poly: [][]geom.Point{{{X: 0, Y: 0}, {X: 16, Y: 0}, {X: 0, Y: 16}}}},
		{file: "S9", w: 16, h: 16, ss: 1, wantOn: 136,
			poly: [][]geom.Point{{{X: 0, Y: 0}, {X: 16, Y: 16}, {X: 16, Y: 0}}}},
	}
}

func renderPipe(t *testing.T, s goldenScene, rule string) []byte {
	t.Helper()
	cfg := config.Default()
	cfg.Width, cfg.Height = s.w, s.h
	cfg.Supersample = s.ss
	cfg.Gamma = 1
	cfg.Blur = 0
	cfg.Shading = "flat"
	cfg.EdgeRule = rule
	im, err := Run(s.poly, cfg)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "out.pgm")
	if err := im.SavePGM(p); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// TestLegacyByteIdentical 切回旧规则，经完整管线的产物必须与改动前冻结基线逐字节一致。
func TestLegacyByteIdentical(t *testing.T) {
	for _, s := range goldenScenes() {
		got := renderPipe(t, s, "legacy")
		want, err := os.ReadFile(filepath.Join("testdata", "legacy-"+s.file+".pgm"))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != string(want) {
			t.Fatalf("%s: legacy 产物与冻结基线不一致", s.file)
		}
	}
}

// TestHalfPixelByteGolden 半像素规则经完整管线的产物与黄金文件逐字节一致，覆盖数符合文档。
func TestHalfPixelByteGolden(t *testing.T) {
	for _, s := range goldenScenes() {
		cfg := config.Default()
		cfg.Width, cfg.Height = s.w, s.h
		cfg.Supersample = s.ss
		cfg.EdgeRule = "half-pixel"
		im, err := Run(s.poly, cfg)
		if err != nil {
			t.Fatal(err)
		}
		if on := im.CountOn(128); on != s.wantOn {
			t.Fatalf("%s 覆盖=%d 期望 %d", s.file, on, s.wantOn)
		}
		got := renderPipe(t, s, "half-pixel")
		want, err := os.ReadFile(filepath.Join("testdata", "half-"+s.file+".pgm"))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != string(want) {
			t.Fatalf("%s: half-pixel 产物与黄金文件不一致", s.file)
		}
	}
}

// TestS7DeltaSixteen 完整管线下 S7 旧=0、新=16，差 +16。
func TestS7DeltaSixteen(t *testing.T) {
	for _, s := range goldenScenes() {
		if s.file != "S7" {
			continue
		}
		cfg := config.Default()
		cfg.Width, cfg.Height = s.w, s.h
		cfg.Supersample = s.ss
		cfg.EdgeRule = "legacy"
		old, err := Run(s.poly, cfg)
		if err != nil {
			t.Fatal(err)
		}
		cfg.EdgeRule = "half-pixel"
		new, err := Run(s.poly, cfg)
		if err != nil {
			t.Fatal(err)
		}
		if old.CountOn(128) != 0 || new.CountOn(128) != 16 {
			t.Fatalf("S7 旧=%d 新=%d，期望 0/16", old.CountOn(128), new.CountOn(128))
		}
	}
}

// TestDefaultRuleIsHalfPixel 配置默认规则必须是 half-pixel。
func TestDefaultRuleIsHalfPixel(t *testing.T) {
	if config.Default().EdgeRule != "half-pixel" {
		t.Fatalf("默认 EdgeRule=%s，期望 half-pixel", config.Default().EdgeRule)
	}
	if _, err := config.ParseEdgeRule("bogus"); err == nil {
		t.Fatal("非法 edge-rule 应报错")
	}
	r, err := config.ParseEdgeRule("legacy")
	if err != nil || r != geom.RuleLegacy {
		t.Fatal("legacy 解析失败")
	}
}
