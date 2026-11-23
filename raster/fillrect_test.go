package raster

import (
	"bytes"
	"testing"

	"vecgo/geom"
)

func square(x, y, s float64) []geom.Point {
	return []geom.Point{{X: x, Y: y}, {X: x + s, Y: y}, {X: x + s, Y: y + s}, {X: x, Y: y + s}}
}

// FillRectInto 的像素必须与整幅 Fill 在同一位置逐字节相同。
func TestFillRectMatchesFull(t *testing.T) {
	polys := [][]geom.Point{square(2, 3, 40), square(50, 20, 30)}
	full, err := New(64, 64)
	if err != nil {
		t.Fatal(err)
	}
	if err := full.Fill(polys, 3); err != nil {
		t.Fatal(err)
	}
	m, err := SceneMapping(polys)
	if err != nil {
		t.Fatal(err)
	}
	part, _ := New(64, 64)
	rects := []Rect{
		{X0: 0, Y0: 0, X1: 64, Y1: 64},
		{X0: 0, Y0: 0, X1: 1, Y1: 1},
		{X0: 63, Y0: 63, X1: 64, Y1: 64},
		{X0: 10, Y0: 12, X1: 21, Y1: 19},
	}
	for _, r := range rects {
		if err := part.FillRectInto(polys, 3, m, r); err != nil {
			t.Fatalf("填充 %+v: %v", r, err)
		}
	}
	if !bytes.Equal(full.Pix, part.Pix) {
		t.Fatal("局部填充与整幅填充不一致")
	}
}

func TestFillRectRejectsOutOfBounds(t *testing.T) {
	im, _ := New(8, 8)
	m, _ := SceneMapping([][]geom.Point{square(0, 0, 10)})
	if err := im.FillRectInto([][]geom.Point{square(0, 0, 10)}, 1, m, Rect{X0: -1, Y0: 0, X1: 4, Y1: 4}); err == nil {
		t.Fatal("越界矩形应报错")
	}
	if err := im.FillRectInto([][]geom.Point{square(0, 0, 10)}, 1, m, Rect{X0: 3, Y0: 3, X1: 3, Y1: 5}); err == nil {
		t.Fatal("退化矩形应报错")
	}
}

func TestPixelBBoxCoversShape(t *testing.T) {
	polys := [][]geom.Point{square(0, 0, 100)}
	m, _ := SceneMapping(polys)
	r := m.PixelBBox(polys[0], 32, 32)
	if r.X0 > 0 || r.Y0 > 0 || r.X1 < 32 || r.Y1 < 32 {
		t.Fatalf("包围盒未覆盖整幅: %+v", r)
	}
	// 完全在画幅外的图形应得到空矩形。
	out := m.PixelBBox([]geom.Point{{X: -50, Y: -50}, {X: -40, Y: -50}, {X: -40, Y: -40}}, 32, 32)
	if !out.Empty() {
		t.Fatalf("画幅外图形应得空矩形: %+v", out)
	}
}

func TestBytesMatchesSavePGM(t *testing.T) {
	im, _ := New(3, 2)
	im.Pix[0], im.Pix[5] = 7, 200
	want := []byte("P5\n3 2\n255\n")
	want = append(want, im.Pix...)
	if got := im.Bytes(); !bytes.Equal(got, want) {
		t.Fatalf("Bytes 与 PGM 头不一致: %q", got[:11])
	}
}
