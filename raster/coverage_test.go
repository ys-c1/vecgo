package raster

import (
	"math"
	"testing"

	"vecgo/geom"
)

func TestRasterizeAreaMatchesGreen(t *testing.T) {
	cases := []struct {
		name string
		poly []geom.Point
	}{
		{"triangle", []geom.Point{{X: 0, Y: 0}, {X: 7, Y: 0}, {X: 2.5, Y: 5}}},
		{"pentagon", regularTest(5, 5, -math.Pi/2)},
		{"star", starTest(10, 6, 2.4)},
	}
	for _, c := range cases {
		exact := geom.Area(c.poly)
		for _, n := range []int{7, 33, 96} {
			cov, err := Rasterize([][]geom.Point{c.poly}, n, n)
			if err != nil {
				t.Fatalf("%s n=%d: %v", c.name, n, err)
			}
			got := cov.WeightedArea()
			if rel := math.Abs(got-exact) / exact; rel >= 1e-9 {
				t.Fatalf("%s n=%d 加权面积 %.15f 与精确 %.15f 相对误差 %.3e",
					c.name, n, got, exact, rel)
			}
		}
	}
}

func TestRasterizeCoverageRange(t *testing.T) {
	sq := []geom.Point{{X: 0, Y: 0}, {X: 4, Y: 0}, {X: 4, Y: 4}, {X: 0, Y: 4}}
	cov, err := Rasterize([][]geom.Point{sq}, 4, 4)
	if err != nil {
		t.Fatal(err)
	}
	for i, v := range cov.Cov {
		if v < 0 || v > 1 {
			t.Fatalf("像素 %d 覆盖率越界: %v", i, v)
		}
	}
	// 正方形占满画幅，全部像素应为满覆盖。
	for i, v := range cov.Cov {
		if math.Abs(v-1) > 1e-12 {
			t.Fatalf("像素 %d 应满覆盖，实为 %v", i, v)
		}
	}
}

func TestRasterizeUnionNotDoubleCount(t *testing.T) {
	a := []geom.Point{{X: 0, Y: 0}, {X: 2, Y: 0}, {X: 2, Y: 2}, {X: 0, Y: 2}}
	b := []geom.Point{{X: 1, Y: 0}, {X: 3, Y: 0}, {X: 3, Y: 2}, {X: 1, Y: 2}}
	cov, err := Rasterize([][]geom.Point{a, b}, 9, 9)
	if err != nil {
		t.Fatal(err)
	}
	// 两个 2×2 正方形重叠 1×2，并集面积 6。
	if got := cov.WeightedArea(); math.Abs(got-6) > 1e-9 {
		t.Fatalf("并集面积应为 6，实为 %v", got)
	}
}

func TestBinaryStatsBound(t *testing.T) {
	tri := []geom.Point{{X: 0, Y: 0}, {X: 7, Y: 0}, {X: 2.5, Y: 5}}
	cov, err := Rasterize([][]geom.Point{tri}, 40, 40)
	if err != nil {
		t.Fatal(err)
	}
	binArea, l1, bound, diff, partial := cov.BinaryStats(0.5)
	if partial == 0 {
		t.Fatal("斜边必然产生部分覆盖像素")
	}
	if diff > l1+1e-9 || l1 > bound+1e-9 {
		t.Fatalf("差异应满足 diff(%v) ≤ l1(%v) ≤ bound(%v)", diff, l1, bound)
	}
	if want := 0.5 * cov.PixelArea() * float64(partial); math.Abs(bound-want) > 1e-12 {
		t.Fatalf("阈值为 0.5 时上界应为 %.12f，实为 %.12f", want, bound)
	}
	if math.Abs(binArea-cov.WeightedArea()) > bound+1e-9 {
		t.Fatalf("二值面积偏离加权面积超过上界")
	}
}

func TestImageQuantizesCoverage(t *testing.T) {
	cov := &Coverage{W: 3, H: 1, PixelW: 1, PixelH: 1, Cov: []float64{0, 0.5, 1}}
	im := cov.Image()
	if im.Pix[0] != 0 || im.Pix[1] != 128 || im.Pix[2] != 255 {
		t.Fatalf("量化结果错误: %v", im.Pix)
	}
	bin := cov.BinaryImage(0.5)
	if bin.Pix[0] != 0 || bin.Pix[1] != 255 || bin.Pix[2] != 255 {
		t.Fatalf("二值结果错误: %v", bin.Pix)
	}
}

func TestRasterizeRejectsBadInput(t *testing.T) {
	if _, err := Rasterize(nil, 4, 4); err == nil {
		t.Fatal("空场景应报错")
	}
	if _, err := Rasterize([][]geom.Point{{{X: 0, Y: 0}}}, 4, 4); err == nil {
		t.Fatal("顶点不足应报错")
	}
	if _, err := Rasterize([][]geom.Point{{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 1, Y: 1}}}, 0, 4); err == nil {
		t.Fatal("画幅非正应报错")
	}
}

func regularTest(k int, r, start float64) []geom.Point {
	out := make([]geom.Point, k)
	for i := range out {
		a := start + 2*math.Pi*float64(i)/float64(k)
		out[i] = geom.Point{X: r * math.Cos(a), Y: r * math.Sin(a)}
	}
	return out
}

func starTest(points int, rOuter, rInner float64) []geom.Point {
	out := make([]geom.Point, points)
	for i := range out {
		r := rOuter
		if i%2 == 1 {
			r = rInner
		}
		a := -math.Pi/2 + math.Pi*float64(i)/float64(points/2)
		out[i] = geom.Point{X: r * math.Cos(a), Y: r * math.Sin(a)}
	}
	return out
}
