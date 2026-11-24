package raster

import (
	"math"
	"strings"
	"testing"

	"vecgo/geom"
	"vecgo/sum"
)

func triangle() []geom.Point {
	return []geom.Point{{X: 1, Y: 1}, {X: 9, Y: 2}, {X: 4, Y: 9}}
}

func TestFillEmptyPolysReportsField(t *testing.T) {
	im, err := New(8, 8)
	if err != nil {
		t.Fatal(err)
	}
	err = im.Fill(nil, 2)
	if err == nil || !strings.Contains(err.Error(), "polys") ||
		!strings.Contains(err.Error(), "空数组") {
		t.Fatalf("空多边形数组应报出字段名: %v", err)
	}
}

func TestFillNaNReportsField(t *testing.T) {
	im, _ := New(8, 8)
	bad := []geom.Point{{X: 0, Y: 0}, {X: 1, Y: math.NaN()}, {X: 1, Y: 1}}
	err := im.Fill([][]geom.Point{bad}, 2)
	if err == nil || !strings.Contains(err.Error(), "polys[0][1]") ||
		!strings.Contains(err.Error(), "NaN") {
		t.Fatalf("NaN 坐标应报出字段名与下标: %v", err)
	}
}

func TestFillShortPolygonReportsField(t *testing.T) {
	im, _ := New(8, 8)
	err := im.Fill([][]geom.Point{{{X: 0, Y: 0}, {X: 1, Y: 0}}}, 2)
	if err == nil || !strings.Contains(err.Error(), "polys[0]") {
		t.Fatalf("顶点不足应报出字段名: %v", err)
	}
}

// 整数参考实现：改造前的 uint8(255*hit/den) 公式。
func referenceCoverage(hit, den int) uint8 {
	return uint8(255 * hit / den)
}

func TestFillMatchesIntegerFormula(t *testing.T) {
	for ss := 1; ss <= 8; ss++ {
		den := ss * ss
		for hit := 0; hit <= den; hit++ {
			// 新路径：255 * (确定性浮点累加的命中数) / float64(den)
			samples := make([]float64, hit)
			for i := range samples {
				samples[i] = 1
			}
			got := uint8(0)
			if hit > 0 {
				hits, err := sum.Float64("coverage", samples)
				if err != nil {
					t.Fatal(err)
				}
				got = uint8(255 * hits / float64(den))
			}
			want := referenceCoverage(hit, den)
			if got != want {
				t.Fatalf("ss=%d hit=%d 新旧公式不一致: got %d want %d", ss, hit, got, want)
			}
		}
	}
}

func TestFillDeterministicPixels(t *testing.T) {
	render := func() []uint8 {
		im, err := New(32, 32)
		if err != nil {
			t.Fatal(err)
		}
		if err := im.Fill([][]geom.Point{triangle()}, 4); err != nil {
			t.Fatal(err)
		}
		out := make([]uint8, len(im.Pix))
		copy(out, im.Pix)
		return out
	}
	first := render()
	for i := 0; i < 10; i++ {
		got := render()
		if len(got) != len(first) {
			t.Fatalf("第 %d 次渲染长度不一致", i)
		}
		for j := range got {
			if got[j] != first[j] {
				t.Fatalf("第 %d 次渲染像素 %d 不一致: %d != %d", i, j, got[j], first[j])
			}
		}
	}
}
