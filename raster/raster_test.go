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

func TestFillHollowSquare(t *testing.T) {
	im, err := New(40, 40)
	if err != nil {
		t.Fatal(err)
	}
	outer := []geom.Point{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}
	// CW 洞环，与外环 CCW 相反；洞边对齐像素网格（3.13..6.5，边长 3.37=13.5 像素不严格对齐，
	// 这里用 2.5..7.5 严格对齐：2.5/0.25=10, 7.5/0.25=30）
	hole := []geom.Point{{X: 2.5, Y: 2.5}, {X: 2.5, Y: 7.5}, {X: 7.5, Y: 7.5}, {X: 7.5, Y: 2.5}}
	if err := im.Fill([][]geom.Point{outer, hole}, 4); err != nil {
		t.Fatal(err)
	}
	// 解析面积：100 - 25 = 75；每像素 0.0625，严格对齐时覆盖面积应恰好 75
	area := 0.0
	for _, v := range im.Pix {
		area += float64(v) / 255.0 * 0.0625
	}
	if math.Abs(area-75.0) > 1e-9 {
		t.Fatalf("带洞覆盖面积应等于解析面积 75, 实际 %.6f", area)
	}
	// 逐点核对：洞内 20x20=400 个像素中心必须全部留空
	emptied := 0
	for row := 10; row < 30; row++ {
		for col := 10; col < 30; col++ {
			if im.Pix[row*40+col] == 0 {
				emptied++
			}
		}
	}
	if emptied != 400 {
		t.Fatalf("洞内应有 400 个留空像素, 实际 %d", emptied)
	}
}

func TestFillRejectsSameWinding(t *testing.T) {
	im, err := New(8, 8)
	if err != nil {
		t.Fatal(err)
	}
	outer := []geom.Point{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}
	holeCCW := []geom.Point{{X: 3, Y: 3}, {X: 7, Y: 3}, {X: 7, Y: 7}, {X: 3, Y: 7}}
	if err := im.Fill([][]geom.Point{outer, holeCCW}, 1); err == nil {
		t.Fatal("洞与外环绕向相同应报错")
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
