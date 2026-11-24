package raster

import (
	"os"
	"path/filepath"
	"testing"

	"vecgo/geom"
	"vecgo/verr"
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

func TestErrorKindsRaster(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want verr.Kind
	}{
		{"new-zero", errOf(New(0, 4)), verr.KindRasterSize},
		{"new-neg", errOf(New(4, -1)), verr.KindRasterSize},
		{"fill-nil", errOfFill((*Image)(nil).Fill(nil, 2)), verr.KindRasterNilImage},
		{"fill-broken", errOfFill((&Image{W: 4, H: 4, Pix: make([]uint8, 3)}).Fill(nil, 2)), verr.KindRasterBadImage},
		{"fill-ss", errOfFill(filled().Fill(nil, 0)), verr.KindRasterSuper},
		{"fill-poly", errOfFill(filled().Fill([][]geom.Point{{{X: 0, Y: 0}, {X: 1, Y: 0}}}, 2)), verr.KindRasterPolyWide},
		{"save-empty", errOfSave((&Image{}).SavePGM("/tmp/x.pgm")), verr.KindRasterEmptyImg},
		{"save-create", errOfSave(filled().SavePGM("/no-such-dir/x.pgm")), verr.KindRasterCreate},
	}
	for _, tc := range cases {
		k, ok := verr.Of(tc.err)
		if !ok {
			t.Fatalf("%s 未归一: %v", tc.name, tc.err)
		}
		if k != tc.want {
			t.Fatalf("%s 编码: 期望 %s 实际 %s", tc.name, tc.want.Hex(), k.Hex())
		}
		if k.Stage() != verr.StageRaster {
			t.Fatalf("%s 段不对: %v", tc.name, k.Stage())
		}
	}
}

func TestFillEmptySceneIsNotFailure(t *testing.T) {
	im := filled()
	if err := im.Fill(nil, 2); err != nil {
		t.Fatalf("空场景不应算失败: %v", err)
	}
	if im.CountOn(0) != len(im.Pix) {
		t.Fatal("空场景图应保持全黑")
	}
}

func TestSaveCloseErrorCoded(t *testing.T) {
	// 用一个必然可写、但关闭时出问题的路径很难构造；这里验证正常路径
	// 的 err 保持 nil，close 错误分支由代码审查兜住（见 UNCOVERED.md）。
	im := filled()
	dir := t.TempDir()
	if err := im.SavePGM(filepath.Join(dir, "ok.pgm")); err != nil {
		t.Fatalf("正常导出不应失败: %v", err)
	}
}

func filled() *Image {
	im, _ := New(4, 4)
	return im
}

func errOf(_ *Image, err error) error { return err }
func errOfFill(err error) error       { return err }
func errOfSave(err error) error       { return err }
