package raster

import (
	"bytes"
	"testing"

	"vecgo/geom"
)

func sq(x0, y0, x1, y1 float64) []geom.Point {
	return []geom.Point{{X: x0, Y: y0}, {X: x1, Y: y0}, {X: x1, Y: y1}, {X: x0, Y: y1}}
}

// 统计关闭时，Fill 与 FillWithStats(nil) 必须逐字节一致。
func TestFillStatsOffIsByteIdentical(t *testing.T) {
	polys := [][]geom.Point{sq(0, 0, 20, 20), sq(10, 0, 30, 20)}
	a, _ := New(37, 29)
	b, _ := New(37, 29)
	if err := a.Fill(polys, 4); err != nil {
		t.Fatal(err)
	}
	if err := b.FillWithStats(polys, 4, nil); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a.Pix, b.Pix) {
		t.Fatal("无统计路径改变了像素")
	}
}

// 统计开启与关闭必须逐字节一致。
func TestFillStatsOnIsByteIdentical(t *testing.T) {
	polys := [][]geom.Point{sq(0, 0, 20, 20), sq(10, 0, 30, 20), sq(5, 5, 15, 15)}
	off, _ := New(41, 33)
	on, _ := New(41, 33)
	if err := off.Fill(polys, 3); err != nil {
		t.Fatal(err)
	}
	st := &FillStats{Enabled: true, KeepPerPixel: true}
	if err := on.FillWithStats(polys, 3, st); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(off.Pix, on.Pix) {
		t.Fatal("统计改变了像素")
	}
}

// 逐像素口径恒等式：Posts = CoveredSamples + ExtraPosts，且每像素直方图之和等于像素数。
func TestFillStatsIdentity(t *testing.T) {
	polys := [][]geom.Point{sq(0, 0, 20, 20), sq(10, 0, 30, 20)}
	im, _ := New(40, 40)
	st := &FillStats{Enabled: true, KeepPerPixel: true}
	if err := im.FillWithStats(polys, 4, st); err != nil {
		t.Fatal(err)
	}
	if st.Posts != st.CoveredSamples+st.ExtraPosts {
		t.Fatalf("口径不一致: posts=%d covered=%d extra=%d", st.Posts, st.CoveredSamples, st.ExtraPosts)
	}
	if st.ActualPosts != st.CoveredSamples {
		t.Fatalf("短路口径应等于覆盖数: actual=%d covered=%d", st.ActualPosts, st.CoveredSamples)
	}
	sum := 0
	for _, n := range st.Repeat {
		sum += n
	}
	if sum != 40*40 {
		t.Fatalf("逐像素直方图总数 %d != 像素数 %d", sum, 40*40)
	}
	if st.Samples != 40*40*16 {
		t.Fatalf("样本数 %d", st.Samples)
	}
}

// 同一图形重复两次：有覆盖的像素全部重复，峰值重复次数等于每像素样本数。
func TestFillStatsDuplicateShape(t *testing.T) {
	polys := [][]geom.Point{sq(0, 0, 10, 10), sq(0, 0, 10, 10)}
	im, _ := New(8, 8)
	st := &FillStats{Enabled: true, KeepPerPixel: true}
	if err := im.FillWithStats(polys, 2, st); err != nil {
		t.Fatal(err)
	}
	if st.Pixels != 64 {
		t.Fatalf("覆盖像素 %d", st.Pixels)
	}
	if st.RepeatAtLeast(1) != 64 {
		t.Fatalf("应全部重复: %d", st.RepeatAtLeast(1))
	}
	if st.MaxRepeat != 4 {
		t.Fatalf("每像素重复应为 4（ss²），实际 %d", st.MaxRepeat)
	}
	if st.ExtraPosts != st.Pixels*4 {
		t.Fatalf("重复总数 %d", st.ExtraPosts)
	}
}

// 逐像素统计应定位到重叠区域，而不是按图形计。
func TestFillStatsHotRegionPartialOverlap(t *testing.T) {
	// 场景 30×20 → 60×40 像素；重叠区 x∈[10,20] 映射到 col∈[20,40)。
	polys := [][]geom.Point{sq(0, 0, 20, 20), sq(10, 0, 30, 20)}
	im, _ := New(60, 40)
	st := &FillStats{Enabled: true, KeepPerPixel: true}
	if err := im.FillWithStats(polys, 2, st); err != nil {
		t.Fatal(err)
	}
	r0, c0, r1, c1, n := st.HotRegion()
	if r0 != 0 || r1 != 39 || c0 != 20 || c1 != 39 || n != 40*20 {
		t.Fatalf("热点区域错误: rows %d..%d cols %d..%d n=%d", r0, r1, c0, c1, n)
	}
	if lines := st.HotMap(2); len(lines) == 0 {
		t.Fatal("HotMap 应产生输出")
	}
}

// Reset 后计数归零，切片可复用。
func TestFillStatsReset(t *testing.T) {
	st := &FillStats{Enabled: true, KeepPerPixel: true}
	im, _ := New(6, 6)
	if err := im.FillWithStats([][]geom.Point{sq(0, 0, 10, 10)}, 2, st); err != nil {
		t.Fatal(err)
	}
	if st.Posts == 0 {
		t.Fatal("应有计数")
	}
	capBefore := cap(st.RepeatPerPixel)
	st.Reset()
	im2, _ := New(6, 6)
	if err := im2.FillWithStats([][]geom.Point{sq(0, 0, 10, 10)}, 2, st); err != nil {
		t.Fatal(err)
	}
	if st.ExtraPosts != 0 {
		t.Fatalf("单图形不应有重复: %d", st.ExtraPosts)
	}
	if cap(st.RepeatPerPixel) != capBefore {
		t.Logf("切片被重新分配: %d -> %d（非致命）", capBefore, cap(st.RepeatPerPixel))
	}
}
