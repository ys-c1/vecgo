package raster

import (
	"testing"

	"vecgo/geom"
)

func TestFillViewRejectsBadViewport(t *testing.T) {
	im, _ := New(4, 4)
	sq := []geom.Point{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 1, Y: 1}, {X: 0, Y: 1}}
	if err := im.FillView([][]geom.Point{sq}, 2, View{PPU: 0}); err == nil {
		t.Fatal("PPU=0 应报错")
	}
	if err := im.FillView([][]geom.Point{sq}, 0, View{PPU: 4}); err == nil {
		t.Fatal("supersample=0 应报错")
	}
}

// TestFillViewMatchesFillMapping 证明 FillView 与原 Fill 用的是同一个采样/
// 量化核：当 View 精确取 Fill 的包围盒映射（PPU=W/span）时，逐像素相同。
func TestFillViewMatchesFillMapping(t *testing.T) {
	const w, h, ss = 8, 8, 2
	sq := []geom.Point{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}

	a, err := New(w, h)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Fill([][]geom.Point{sq}, ss); err != nil {
		t.Fatal(err)
	}

	b, _ := New(w, h)
	// Fill: world = min + sample*span/W = sample*10/8  <=>  PPU=W/span=0.8
	v := View{MinX: 0, MinY: 0, PPU: float64(w) / 10.0}
	if err := b.FillView([][]geom.Point{sq}, ss, v); err != nil {
		t.Fatal(err)
	}
	for i := range a.Pix {
		if a.Pix[i] != b.Pix[i] {
			t.Fatalf("位置 %d Fill=%d FillView=%d 不一致", i, a.Pix[i], b.Pix[i])
		}
	}
}

// TestFillBBoxPathScaleInvariant 原始 Fill 按多边形自身包围盒归一化，
// 均匀缩放 s 会在映射中被约掉：同一个方形在五个缩放档下应逐像素相同。
func TestFillBBoxPathScaleInvariant(t *testing.T) {
	base := []geom.Point{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}
	scales := []float64{1, 0.875, 0.75, 0.625, 0.5}
	var want []uint8
	for _, s := range scales {
		poly := make([]geom.Point, len(base))
		for i, p := range base {
			poly[i] = geom.Point{X: p.X * s, Y: p.Y * s}
		}
		im, err := New(32, 32)
		if err != nil {
			t.Fatal(err)
		}
		if err := im.Fill([][]geom.Point{poly}, 2); err != nil {
			t.Fatal(err)
		}
		if want == nil {
			want = append([]uint8{}, im.Pix...)
			continue
		}
		for i := range want {
			if im.Pix[i] != want[i] {
				t.Fatalf("bbox 路径 scale=%v 像素 %d =%d 与 s=1 的 %d 不同", s, i, im.Pix[i], want[i])
			}
		}
	}
}

// TestFillViewNestedScaleMonotone 凸方形关于顶点 (0,0) 缩放，尺度升序时
// sK 两两嵌套，固定 View 下每个像素灰度必须非降。
func TestFillViewNestedScaleMonotone(t *testing.T) {
	base := []geom.Point{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}
	scales := []float64{0.5, 0.625, 0.75, 0.875, 1}
	v := View{MinX: -3, MinY: -3, PPU: 4}
	var prev []uint8
	for _, s := range scales {
		poly := make([]geom.Point, len(base))
		for i, p := range base {
			poly[i] = geom.Point{X: p.X * s, Y: p.Y * s}
		}
		im, err := New(64, 64)
		if err != nil {
			t.Fatal(err)
		}
		if err := im.FillView([][]geom.Point{poly}, 2, v); err != nil {
			t.Fatal(err)
		}
		if prev != nil {
			for i := range im.Pix {
				if im.Pix[i] < prev[i] {
					t.Fatalf("scale=%v 像素 %d 灰度 %d < 上一档 %d，违反单调", s, i, im.Pix[i], prev[i])
				}
			}
		}
		prev = im.Pix
	}
}
