package raster

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"testing"

	"vecgo/geom"
	"vecgo/param"
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
	on, err := im.CountOn(128)
	if err != nil {
		t.Fatal(err)
	}
	if on < 60 || on > 64 {
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

// 新旧口径统一后，raster 也拒绝超出画幅上限的画布，而不是默默分配。
func TestNewRejectsOversize(t *testing.T) {
	if _, err := New(param.SideMax+1, 4); err == nil {
		t.Fatal("超大画幅应报错")
	}
}

func TestFillRejectsInvalid(t *testing.T) {
	sq := []geom.Point{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}
	cases := []struct {
		name string
		want string
		run  func() error
	}{
		{"空多边形列表（以前 panic）", "raster.Fill.polys", func() error {
			im, _ := New(4, 4)
			return im.Fill(nil, 1)
		}},
		{"supersample 过大", "raster.Fill.supersample", func() error {
			im, _ := New(4, 4)
			return im.Fill([][]geom.Point{sq}, param.SupersampleMax+1)
		}},
		{"退化多边形：宽为零", "raster.Fill.polys[0]", func() error {
			im, _ := New(4, 4)
			return im.Fill([][]geom.Point{{{X: 1, Y: 0}, {X: 1, Y: 1}, {X: 1, Y: 2}}}, 1)
		}},
		{"退化多边形：高为零", "raster.Fill.polys[0]", func() error {
			im, _ := New(4, 4)
			return im.Fill([][]geom.Point{{{X: 0, Y: 1}, {X: 1, Y: 1}, {X: 2, Y: 1}}}, 1)
		}},
		{"多边形含 NaN", "raster.Fill.polys[0][0]", func() error {
			im, _ := New(4, 4)
			return im.Fill([][]geom.Point{{{X: math.NaN(), Y: 0}, {X: 1, Y: 0}, {X: 1, Y: 1}}}, 1)
		}},
	}
	for _, tc := range cases {
		err := tc.run()
		var pe *param.Error
		if !errors.As(err, &pe) {
			t.Fatalf("%s: 期望 *param.Error，实际 %T %v", tc.name, err, err)
		}
		if pe.Field != tc.want {
			t.Fatalf("%s: 期望字段 %s，实际 %s", tc.name, tc.want, pe.Field)
		}
	}
}

func TestSaveRejectsEmptyPath(t *testing.T) {
	im, _ := New(2, 2)
	err := im.SavePGM("")
	var pe *param.Error
	if !errors.As(err, &pe) || pe.Field != "raster.SavePGM.path" {
		t.Fatalf("空路径应返回字段错误，实际 %v", err)
	}
}

func TestSaveDoesNotCreateOnInvalid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "never.pgm")
	im, _ := New(2, 2)
	im.Pix = im.Pix[:1] // Pix 长度不符
	if err := im.SavePGM(path); err == nil {
		t.Fatal("Pix 长度不符应报错")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("校验失败时不应创建文件")
	}
}
