package raster

import (
	"math"
	"reflect"
	"testing"

	"vecgo/geom"
)

func goodSquares() [][]geom.Point {
	return [][]geom.Point{
		{{X: 0, Y: 0}, {X: 4, Y: 0}, {X: 4, Y: 4}, {X: 0, Y: 4}},
		{{X: 6, Y: 6}, {X: 9, Y: 6}, {X: 9, Y: 9}, {X: 6, Y: 9}},
		{{X: 1, Y: 7}, {X: 3, Y: 7}, {X: 3, Y: 9}, {X: 1, Y: 9}},
	}
}

// 正常场景下，隔离开启与关闭必须逐像素相同。
func TestIsolatedPixelIdenticalOnValidScene(t *testing.T) {
	polys := goodSquares()

	imA, _ := New(32, 32)
	if err := imA.Fill(polys, 2); err != nil {
		t.Fatal(err)
	}
	imB, _ := New(32, 32)
	skipped, err := imB.FillIsolated(polys, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(skipped) != 0 {
		t.Fatalf("正常场景不应有跳过: %v", skipped)
	}
	if !reflect.DeepEqual(imA.Pix, imB.Pix) {
		t.Fatal("隔离开启后正常场景像素与传统路径不一致")
	}
}

// 坏图形（Inf 坐标、顶点不足）只跳过自身，其余照常绘制。
func TestIsolatedSkipsBadPolygons(t *testing.T) {
	polys := goodSquares()
	// #1 注入 Inf 顶点，#3 顶点不足，#4 含 NaN。
	polys[1][1].X = math.Inf(1)
	polys = append(polys,
		[]geom.Point{{X: 0, Y: 0}, {X: 1, Y: 1}},
		[]geom.Point{{X: 0, Y: 0}, {X: math.NaN(), Y: 1}, {X: 2, Y: 2}},
	)

	im, _ := New(32, 32)
	skipped, err := im.FillIsolated(polys, 2)
	if err != nil {
		t.Fatal(err)
	}
	gotIDs := skipped.IDs()
	wantIDs := []int{1, 3, 4}
	if !reflect.DeepEqual(gotIDs, wantIDs) {
		t.Fatalf("跳过编号应为升序 %v，实际 %v", wantIDs, gotIDs)
	}
	// 好图形 #0、#2 仍然点亮像素。
	if im.CountOn(1) == 0 {
		t.Fatal("剔除坏图形后其余图形应照常出图，不能黑帧")
	}

	// 与仅含好图形 #0、#2 的传统路径逐像素对照：跳过坏图形后的画面
	// 必须和好图形子集完全一致，证明好图形未被误杀、渲染结果无偏差。
	goodOnly := [][]geom.Point{goodSquares()[0], goodSquares()[2]}
	want, _ := New(32, 32)
	if err := want.Fill(goodOnly, 2); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(im.Pix, want.Pix) {
		t.Fatal("隔离路径输出与好图形子集的传统路径输出不一致")
	}
}

// 同一组输入在非隔离路径下：Inf 坐标静默产出全黑帧。
func TestNonIsolatedInfProducesBlackFrame(t *testing.T) {
	polys := goodSquares()
	polys[1][1].X = math.Inf(1)

	im, _ := New(32, 32)
	if err := im.Fill(polys, 2); err != nil {
		t.Fatalf("Inf 坐标在传统路径下不报错（静默黑帧），实际: %v", err)
	}
	if im.CountOn(1) != 0 {
		t.Fatalf("传统路径应为全黑帧，实际亮像素 %d", im.CountOn(1))
	}
}

// 同一组输入在非隔离路径下：顶点不足直接报错，整帧无产出。
func TestNonIsolatedShortPolygonFails(t *testing.T) {
	polys := append(goodSquares(), []geom.Point{{X: 0, Y: 0}, {X: 1, Y: 1}})
	im, _ := New(32, 32)
	if err := im.Fill(polys, 2); err == nil {
		t.Fatal("顶点不足在传统路径下应整体报错")
	}
}

// 全部图形都坏：隔离路径返回全黑图但不报错，清单包含全部编号。
func TestIsolatedAllBadStillReturnsImage(t *testing.T) {
	polys := [][]geom.Point{
		{{X: 0, Y: 0}, {X: 1, Y: 1}},
		{{X: 0, Y: 0}, {X: math.NaN(), Y: 1}, {X: 2, Y: 2}},
	}
	im, _ := New(8, 8)
	skipped, err := im.FillIsolated(polys, 2)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(skipped.IDs(), []int{0, 1}) {
		t.Fatalf("跳过清单错误: %v", skipped.IDs())
	}
	if len(im.Pix) != 64 || im.CountOn(1) != 0 {
		t.Fatal("应返回尺寸正确的全黑图，而不是报错")
	}
}

// 空场景：隔离路径不 panic，返回空清单与全黑图。
func TestIsolatedEmptyScene(t *testing.T) {
	im, _ := New(8, 8)
	skipped, err := im.FillIsolated(nil, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(skipped) != 0 || im.CountOn(1) != 0 {
		t.Fatal("空场景应返回空清单与全黑图")
	}
}
