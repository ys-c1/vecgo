package raster

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	"vecgo/geom"
)

func TestFillSquare(t *testing.T) {
	im, err := New(8, 8)
	if err != nil {
		t.Fatal(err)
	}
	sq := []geom.Point{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}
	if err := im.Fill([][]geom.Point{sq}, 2); err != nil {
		t.Fatal(err)
	}
	if on := im.CountOn(128); on < 60 || on > 64 {
		t.Fatalf("亮像素异常: %d", on)
	}
}

func TestSaveHeader(t *testing.T) {
	im, _ := New(4, 4)
	dir := t.TempDir()
	path := filepath.Join(dir, "a.pgm")
	if err := im.SavePGM(path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 11+16 {
		t.Fatalf("字节数异常: %d", len(data))
	}
}

func TestNewRejectsBadSize(t *testing.T) {
	if _, err := New(0, 4); err == nil {
		t.Fatal("宽度为 0 应报错")
	}
}

// pentagram is the 5/2 star, which has a winding-2 centre.
func pentagram() []geom.Point {
	verts := make([]geom.Point, 5)
	for k := 0; k < 5; k++ {
		a := (-90 + 72*float64(k)) * 3.14159265358979 / 180
		verts[k] = geom.Point{X: 100 * cos(a), Y: 100 * sin(a)}
	}
	order := []int{0, 2, 4, 1, 3}
	out := make([]geom.Point, 5)
	for k, v := range order {
		out[k] = verts[v]
	}
	return out
}

func cos(a float64) float64 { return math.Cos(a) }
func sin(a float64) float64 { return math.Sin(a) }

// TestFillRuleDefaultsToNonZero checks Fill and FillRule(NonZero) agree.
func TestFillRuleDefaultsToNonZero(t *testing.T) {
	// pentagram via helper above
	poly := pentagram()
	a, _ := New(32, 32)
	b, _ := New(32, 32)
	if err := a.Fill([][]geom.Point{poly}, 2); err != nil {
		t.Fatal(err)
	}
	if err := b.FillRule([][]geom.Point{poly}, 2, geom.NonZero); err != nil {
		t.Fatal(err)
	}
	for i := range a.Pix {
		if a.Pix[i] != b.Pix[i] {
			t.Fatalf("Fill 与 FillRule(NonZero) 在 %d 处不同: %d vs %d", i, a.Pix[i], b.Pix[i])
		}
	}
}

// TestPentagramRulesDiffer checks the two rules really diverge on a
// self-intersecting path, and that non-zero covers strictly more.
func TestPentagramRulesDiffer(t *testing.T) {
	poly := pentagram()
	nz, _ := New(48, 48)
	eo, _ := New(48, 48)
	nz.FillRule([][]geom.Point{poly}, 2, geom.NonZero)
	eo.FillRule([][]geom.Point{poly}, 2, geom.EvenOdd)
	if nz.CountCovered() <= eo.CountCovered() {
		t.Fatalf("五角星上 non-zero 覆盖应严格多于 even-odd: %d vs %d", nz.CountCovered(), eo.CountCovered())
	}
}

// TestSimplePolygonFillsIdentically checks the renderer obeys the theorem on a
// simple polygon, pixel for pixel, under both rules.
func TestSimplePolygonFillsIdentically(t *testing.T) {
	sq := []geom.Point{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}, {X: 4, Y: 3}}
	nz, _ := New(16, 16)
	eo, _ := New(16, 16)
	nz.FillRule([][]geom.Point{sq}, 3, geom.NonZero)
	eo.FillRule([][]geom.Point{sq}, 3, geom.EvenOdd)
	for i := range nz.Pix {
		if nz.Pix[i] != eo.Pix[i] {
			t.Fatalf("简单多边形上两条规则渲染结果不同 at %d: %d vs %d", i, nz.Pix[i], eo.Pix[i])
		}
	}
}

// TestFillRuleRejectsUnknown checks the guard.
func TestFillRuleRejectsUnknown(t *testing.T) {
	im, _ := New(4, 4)
	sq := []geom.Point{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 1, Y: 1}}
	if err := im.FillRule([][]geom.Point{sq}, 1, geom.FillRule(99)); err == nil {
		t.Fatal("未知规则应报错")
	}
}
