package raster

import (
	"bytes"
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

func TestFillClearsBufferResidue(t *testing.T) {
	sq := []geom.Point{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}

	dirty, _ := New(8, 8)
	for i := range dirty.Pix {
		dirty.Pix[i] = 255 // 模拟跨次渲染复用同一 Image 留下的旧像素
	}
	if err := dirty.Fill([][]geom.Point{sq}, 2); err != nil {
		t.Fatal(err)
	}

	fresh, _ := New(8, 8)
	if err := fresh.Fill([][]geom.Point{sq}, 2); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(dirty.Pix, fresh.Pix) {
		t.Fatal("复用带残留的 Image 后输出与全新 Image 不一致")
	}
}

func TestFillOrderIndependent(t *testing.T) {
	sqA := []geom.Point{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}
	sqB := []geom.Point{{X: 5, Y: 5}, {X: 15, Y: 5}, {X: 15, Y: 15}, {X: 5, Y: 15}}

	im1, _ := New(16, 16)
	if err := im1.Fill([][]geom.Point{sqA, sqB}, 2); err != nil {
		t.Fatal(err)
	}
	im2, _ := New(16, 16)
	if err := im2.Fill([][]geom.Point{sqB, sqA}, 2); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(im1.Pix, im2.Pix) {
		t.Fatal("同一组多边形调换输入顺序后像素不一致")
	}
}
