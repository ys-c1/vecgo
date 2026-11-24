package raster

import (
	"math/rand"
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

func TestFillRandomTrianglePartialAndReproducible(t *testing.T) {
	tri := []geom.Point{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}}
	render := func(seed int64) *Image {
		im, err := New(16, 16)
		if err != nil {
			t.Fatal(err)
		}
		if err := im.FillRandom([][]geom.Point{tri}, 64, rand.New(rand.NewSource(seed))); err != nil {
			t.Fatal(err)
		}
		return im
	}
	a, b := render(123), render(123)
	for i := range a.Pix {
		if a.Pix[i] != b.Pix[i] {
			t.Fatal("固定种子应逐像素复现")
		}
	}
	partial := 0
	for _, v := range a.Pix {
		if v > 20 && v < 235 {
			partial++
		}
	}
	if partial == 0 {
		t.Fatal("三角形边缘应产生中间灰度像素")
	}
	if _, err := New(4, 4); err != nil {
		t.Fatal(err)
	}
	bad, _ := New(4, 4)
	if err := bad.FillRandom([][]geom.Point{tri}, 0, rand.New(rand.NewSource(1))); err == nil {
		t.Fatal("n=0 应报错")
	}
	if err := bad.FillRandom([][]geom.Point{tri}, 8, nil); err == nil {
		t.Fatal("nil rand 源应报错")
	}
}
