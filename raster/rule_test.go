package raster

import (
	"testing"

	"vecgo/geom"
)

// 纯填充层语义测试（不经过 pipe.Transform，坐标即最终采样坐标）。
// 经完整管线的字节级黄金对比见 pipe 包。
type fillCase struct {
	name       string
	w, h, ss   int
	poly       [][]geom.Point
	legacyOn   int
	halfOn     int
}

func fillCases() []fillCase {
	return []fillCase{
		{name: "S1_square_ss2", w: 64, h: 64, ss: 2, legacyOn: 4096, halfOn: 4096,
			poly: [][]geom.Point{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}}},
		{name: "S2_triangle", w: 16, h: 16, ss: 1, legacyOn: 120, halfOn: 120,
			poly: [][]geom.Point{{{X: 2, Y: 2}, {X: 14, Y: 2}, {X: 2, Y: 12}}}},
		{name: "S3_other_tri", w: 16, h: 16, ss: 1, legacyOn: 136, halfOn: 136,
			poly: [][]geom.Point{{{X: 0, Y: 0}, {X: 16, Y: 16}, {X: 16, Y: 0}}}},
		{name: "S4_triangle_ss2", w: 16, h: 16, ss: 2, legacyOn: 120, halfOn: 120,
			poly: [][]geom.Point{{{X: 2, Y: 2}, {X: 14, Y: 2}, {X: 2, Y: 12}}}},
		{name: "S5_tiles", w: 16, h: 16, ss: 1, legacyOn: 256, halfOn: 256,
			poly: [][]geom.Point{
				{{X: 0.5, Y: 0.5}, {X: 8.5, Y: 0.5}, {X: 8.5, Y: 15.5}, {X: 0.5, Y: 15.5}},
				{{X: 8.5, Y: 0.5}, {X: 15.5, Y: 0.5}, {X: 15.5, Y: 15.5}, {X: 8.5, Y: 15.5}},
			}},
		{name: "S6_full", w: 16, h: 16, ss: 1, legacyOn: 256, halfOn: 256,
			poly: [][]geom.Point{{{X: 0, Y: 0}, {X: 16, Y: 0}, {X: 16, Y: 16}, {X: 0, Y: 16}}}},
		// 退化三角形：两规则唯一可复现差异，旧 0、新 16，差 +16。
		{name: "S7_degenerate", w: 16, h: 16, ss: 1, legacyOn: 0, halfOn: 16,
			poly: [][]geom.Point{{{X: 0, Y: 0}, {X: 16, Y: 16}, {X: 8.5, Y: 8.5}}}},
		{name: "S8_control", w: 16, h: 16, ss: 1, legacyOn: 120, halfOn: 120,
			poly: [][]geom.Point{{{X: 0, Y: 0}, {X: 16, Y: 0}, {X: 0, Y: 16}}}},
		{name: "S9_control", w: 16, h: 16, ss: 1, legacyOn: 136, halfOn: 136,
			poly: [][]geom.Point{{{X: 0, Y: 0}, {X: 16, Y: 16}, {X: 16, Y: 0}}}},
	}
}

func mustFill(t *testing.T, c fillCase, rule geom.EdgeRule) *Image {
	t.Helper()
	im, err := New(c.w, c.h)
	if err != nil {
		t.Fatal(err)
	}
	if err := im.FillRule(c.poly, c.ss, rule); err != nil {
		t.Fatal(err)
	}
	return im
}

// TestFillRuleCounts 两规则在填充层的具体覆盖像素数（阈值 128）。
func TestFillRuleCounts(t *testing.T) {
	for _, c := range fillCases() {
		if got := mustFill(t, c, geom.RuleLegacy).CountOn(128); got != c.legacyOn {
			t.Fatalf("%s legacy 覆盖=%d 期望 %d", c.name, got, c.legacyOn)
		}
		if got := mustFill(t, c, geom.RuleHalfPixel).CountOn(128); got != c.halfOn {
			t.Fatalf("%s half-pixel 覆盖=%d 期望 %d", c.name, got, c.halfOn)
		}
	}
}

// TestS7Delta 文档承诺的具体差值：退化三角形 旧=0、新=16，差 +16，逐像素位于主对角线。
func TestS7Delta(t *testing.T) {
	c := fillCases()[6]
	imOld := mustFill(t, c, geom.RuleLegacy)
	imNew := mustFill(t, c, geom.RuleHalfPixel)
	if d := imNew.CountOn(128) - imOld.CountOn(128); d != 16 {
		t.Fatalf("S7 差值=%d 期望 +16", d)
	}
	changed := 0
	for i := range imNew.Pix {
		if imNew.Pix[i] != imOld.Pix[i] {
			changed++
		}
	}
	if changed != 16 {
		t.Fatalf("S7 变化像素=%d 期望 16", changed)
	}
}

// TestNonDegenerateUnchanged 所有非退化场景两规则必须逐像素一致。
func TestNonDegenerateUnchanged(t *testing.T) {
	for _, c := range fillCases() {
		if c.name == "S7_degenerate" {
			continue
		}
		a := mustFill(t, c, geom.RuleLegacy).Pix
		b := mustFill(t, c, geom.RuleHalfPixel).Pix
		for i := range a {
			if a[i] != b[i] {
				t.Fatalf("%s 非退化场景在像素 %d 出现差异 %d->%d", c.name, i, a[i], b[i])
			}
		}
	}
}

// TestFillDefaultsToHalfPixel 无参 Fill 默认走半像素规则。
func TestFillDefaultsToHalfPixel(t *testing.T) {
	c := fillCases()[6]
	im, _ := New(c.w, c.h)
	if err := im.Fill(c.poly, c.ss); err != nil {
		t.Fatal(err)
	}
	if im.CountOn(128) != 16 {
		t.Fatalf("Fill 默认得 %d，期望 half-pixel 的 16", im.CountOn(128))
	}
}

// TestFillRejectsBadRuleInput 保留原有的入参校验。
func TestFillRejectsBadRuleInput(t *testing.T) {
	im, _ := New(4, 4)
	if err := im.FillRule([][]geom.Point{{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 1, Y: 1}}}, 0, geom.RuleHalfPixel); err == nil {
		t.Fatal("supersample=0 应报错")
	}
}
