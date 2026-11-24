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

// TestToByteMatchesLegacyExpr 固定转换口径：唯一的 ToByte 必须与改造前各处
// 手写的 uint8(255*t) 完全一致，包括四舍五入与截断的边界。
func TestToByteMatchesLegacyExpr(t *testing.T) {
	if got := ToByte(-0.5); got != 0 {
		t.Fatalf("负值应夹到 0, got %d", got)
	}
	if got := ToByte(1.5); got != 255 {
		t.Fatalf("大于 1 应夹到 255, got %d", got)
	}
	for i := 0; i <= 255; i++ {
		if got, want := ToByte(float64(i)/255), uint8(255*(float64(i)/255)); got != want {
			t.Fatalf("i=%d: ToByte=%d 期望 %d", i, got, want)
		}
	}
	for ss := 1; ss <= 8; ss++ {
		den := ss * ss
		for hit := 0; hit <= den; hit++ {
			if got, want := ToByte(float64(hit)/float64(den)), uint8(255*hit/den); got != want {
				t.Fatalf("ss=%d hit=%d: ToByte=%d 期望 %d", ss, hit, got, want)
			}
		}
	}
}
