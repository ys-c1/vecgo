package raster

import (
	"testing"

	"vecgo/geom"
)

func TestFillContainCentersSquareInWideFrame(t *testing.T) {
	im, err := New(32, 16)
	if err != nil {
		t.Fatal(err)
	}
	sq := []geom.Point{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}
	if err := im.FillContain([][]geom.Point{sq}, 1); err != nil {
		t.Fatal(err)
	}

	// scale=1.6，offsetX=(32-16)/2=8；像素中心映射后，亮列应为 8..23。
	y := 8
	for x := 0; x < 8; x++ {
		if im.Pix[y*im.W+x] != 0 {
			t.Fatalf("左侧留白列 %d 被填充: %d", x, im.Pix[y*im.W+x])
		}
	}
	for x := 8; x <= 23; x++ {
		if im.Pix[y*im.W+x] == 0 {
			t.Fatalf("正方形内部列 %d 未填充", x)
		}
	}
	for x := 24; x < 32; x++ {
		if im.Pix[y*im.W+x] != 0 {
			t.Fatalf("右侧留白列 %d 被填充: %d", x, im.Pix[y*im.W+x])
		}
	}
}

func TestFillStretchRemainsAvailable(t *testing.T) {
	im, err := New(32, 16)
	if err != nil {
		t.Fatal(err)
	}
	sq := []geom.Point{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}
	if err := im.Fill([][]geom.Point{sq}, 1); err != nil {
		t.Fatal(err)
	}
	// 旧映射 sx=3.2、sy=1.6，会把正方形横向拉满；中线上不应有整段侧留白。
	if im.Pix[8*im.W+0] == 0 || im.Pix[8*im.W+31] == 0 {
		t.Fatal("旧的拉伸 Fill 行为应保留")
	}
}
