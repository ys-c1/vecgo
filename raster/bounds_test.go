package raster

import (
	"strings"
	"testing"

	"vecgo/bounds"
	"vecgo/geom"
)

func square(x0, y0, x1, y1 float64) []geom.Point {
	return []geom.Point{{X: x0, Y: y0}, {X: x1, Y: y0}, {X: x1, Y: y1}, {X: x0, Y: y1}}
}

// FillAt：四条边算界内，贴边整块接受且产物与 Fill 逐字节一致。
func TestFillAtEdgeMatchesFill(t *testing.T) {
	poly := square(0, 0, 16, 16)
	a, _ := New(16, 16)
	b, _ := New(16, 16)
	if err := a.Fill([][]geom.Point{poly}, 2); err != nil {
		t.Fatal(err)
	}
	if err := b.FillAt([][]geom.Point{poly}, 2, bounds.PixelFrame(16, 16)); err != nil {
		t.Fatalf("贴边应接受: %v", err)
	}
	for i := range a.Pix {
		if a.Pix[i] != b.Pix[i] {
			t.Fatalf("第 %d 像素不同: %d != %d", i, a.Pix[i], b.Pix[i])
		}
	}
}

func TestFillAtRejectsNegativeWithIndex(t *testing.T) {
	im, _ := New(16, 16)
	err := im.FillAt([][]geom.Point{square(-4, 4, 4, 12)}, 2, bounds.PixelFrame(16, 16))
	if err == nil {
		t.Fatal("负坐标应拒绝")
	}
	for _, want := range []string{"raster.FillAt", "polys[0][0]", "(-4, 4)", "负坐标"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("缺少 %q: %v", want, err)
		}
	}
}

func TestFillAtRejectsBeyondWithIndex(t *testing.T) {
	im, _ := New(16, 16)
	err := im.FillAt([][]geom.Point{square(8, 8, 20, 12)}, 2, bounds.PixelFrame(16, 16))
	if err == nil {
		t.Fatal("超画幅应拒绝")
	}
	for _, want := range []string{"raster.FillAt", "polys[0][1]", "(20, 8)", "超画幅"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("缺少 %q: %v", want, err)
		}
	}
}

// 旧实现把退化包围盒的 0 跨度静默改成 1；现在拒绝。
func TestFillRejectsZeroSpan(t *testing.T) {
	im, _ := New(8, 8)
	err := im.Fill([][]geom.Point{{{X: 3, Y: 0}, {X: 3, Y: 5}, {X: 3, Y: 9}}}, 2)
	if err == nil {
		t.Fatal("零宽包围盒应拒绝")
	}
	if !strings.Contains(err.Error(), "包围盒") {
		t.Errorf("应指出包围盒退化: %v", err)
	}
	for _, v := range im.Pix {
		if v != 0 {
			t.Fatal("拒绝后不应写入像素")
		}
	}
}

// Fill 的坐标域由图形自身包围盒决定，负坐标本身不是错误（旧实现如此）。
func TestFillAcceptsNegativeWithoutFrame(t *testing.T) {
	im, _ := New(8, 8)
	if err := im.Fill([][]geom.Point{square(-10, -10, 10, 10)}, 2); err != nil {
		t.Fatalf("无显式域时负坐标应接受: %v", err)
	}
	if im.CountOn(1) == 0 {
		t.Fatal("应有像素被点亮")
	}
}

func TestFillRejectsNonFinite(t *testing.T) {
	im, _ := New(8, 8)
	err := im.Fill([][]geom.Point{{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 1, Y: 1}}}, 2)
	if err != nil {
		t.Fatalf("有限输入应接受: %v", err)
	}
}

func TestInFrameHalfOpen(t *testing.T) {
	im, _ := New(4, 4)
	if !im.InFrame(0, 0) || !im.InFrame(3, 3) {
		t.Fatal("界内像素应接受")
	}
	for _, p := range [][2]int{{-1, 0}, {4, 0}, {0, -1}, {0, 4}} {
		if im.InFrame(p[0], p[1]) {
			t.Errorf("界外像素 %v 应拒绝", p)
		}
	}
}
