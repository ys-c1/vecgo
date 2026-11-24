package raster

import (
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

// TestCopyFrameCounts 证明“整图拷贝”计数是活的：真正发生一次拷贝时计数
// 会加一，而不是永远为 0 的摆设。
func TestCopyFrameCounts(t *testing.T) {
	ResetStats()
	src, _ := New(4, 4)
	dst, _ := New(4, 4)
	src.Pix[0] = 7
	if err := copyFrame(dst, src); err != nil {
		t.Fatal(err)
	}
	if got := Snapshot().FullCopies; got != 1 {
		t.Fatalf("整图拷贝计数应为 1，实际 %d", got)
	}
	if dst.Pix[0] != 7 {
		t.Fatal("拷贝后像素应一致")
	}
	if err := copyFrame(dst, nil); err == nil {
		t.Fatal("空图拷贝应报错")
	}
}

// TestLeaseDoesNotCopy 证明 Lease 返回的就是原缓冲，不是副本。
func TestLeaseDoesNotCopy(t *testing.T) {
	im, _ := New(4, 4)
	pix := im.Lease("test")
	pix[3] = 9
	if im.Pix[3] != 9 {
		t.Fatal("Lease 返回的应是同一块缓冲")
	}
}

func TestFillRejectsEmptyScene(t *testing.T) {
	im, _ := New(4, 4)
	if err := im.Fill(nil, 1); err == nil {
		t.Fatal("空场景应报错而不是 panic")
	}
}
