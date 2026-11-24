package raster

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"vecgo/color"
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
	if _, err := New(0, 4); err == nil || !strings.Contains(err.Error(), "raster:") {
		t.Fatalf("宽度为 0 应报带包名的错误")
	}
}

// TestLinearPGMUsesSharedColorTable SaveLinearPGM 的编码字节必须与
// color.EncodeLUT 逐值一致——PGM 路径与 post 路径共用同一份转换函数。
func TestLinearPGMUsesSharedColorTable(t *testing.T) {
	for _, g := range []float64{0.5, 1, 2.2} {
		im, _ := New(256, 1)
		for i := 0; i < 256; i++ {
			im.Pix[i] = uint8(i)
		}
		dir := t.TempDir()
		path := filepath.Join(dir, "lut.pgm")
		if err := im.SaveLinearPGM(path, g); err != nil {
			t.Fatal(err)
		}
		loaded, err := LoadLinearPGM(path, g)
		if err != nil {
			t.Fatal(err)
		}
		table, err := color.EncodeLUT(g)
		if err != nil {
			t.Fatal(err)
		}
		// 读回的是线性字节；重新编码后应落在原编码值 ±1 以内。
		for i := 0; i < 256; i++ {
			d := int(table[loaded.Pix[i]]) - int(table[uint8(i)])
			if d < 0 {
				d = -d
			}
			if g == 1 && d != 0 {
				t.Fatalf("g=1 应恒等，i=%d 偏差 %d", i, d)
			}
			if d > 1 {
				t.Fatalf("g=%v i=%d PGM 往返偏差 %d 超出一个量化级", g, i, d)
			}
		}
	}
}

// TestSaveLinearPGMGammaOneRaw  gamma=1 时 SaveLinearPGM 写出的字节
// 必须与线性缓冲完全相同（恒等路径）。
func TestSaveLinearPGMGammaOneRaw(t *testing.T) {
	im, _ := New(4, 1)
	im.Pix = []uint8{0, 1, 127, 255}
	dir := t.TempDir()
	path := filepath.Join(dir, "id.pgm")
	if err := im.SaveLinearPGM(path, 1); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := data[len(data)-4:]
	for i := range im.Pix {
		if got[i] != im.Pix[i] {
			t.Fatalf("gamma=1 PGM 写出不是恒等: %v", got)
		}
	}
}

func TestLoadLinearPGMRejectsBadInput(t *testing.T) {
	dir := t.TempDir()

	notPGM := filepath.Join(dir, "x.pgm")
	if err := os.WriteFile(notPGM, []byte("P2\n2 2\n255\nxxxx"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadLinearPGM(notPGM, 1); err == nil ||
		!strings.Contains(err.Error(), "raster:") {
		t.Fatalf("非 P5 文件应报带包名的错误: %v", err)
	}

	badMax := filepath.Join(dir, "y.pgm")
	if err := os.WriteFile(badMax, []byte("P5\n2 2\n128\nxxxx"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadLinearPGM(badMax, 1); err == nil ||
		!strings.Contains(err.Error(), "raster:") || !strings.Contains(err.Error(), "maxval") {
		t.Fatalf("maxval 错误应带包名与字段名: %v", err)
	}

	truncated := filepath.Join(dir, "z.pgm")
	if err := os.WriteFile(truncated, []byte("P5\n4 4\n255\nxx"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadLinearPGM(truncated, 1); err == nil ||
		!strings.Contains(err.Error(), "raster:") {
		t.Fatalf("截断文件应报带包名的错误: %v", err)
	}

	if _, err := LoadLinearPGM(filepath.Join(dir, "missing.pgm"), 1); err == nil {
		t.Fatal("不存在的文件应报错")
	}
}
