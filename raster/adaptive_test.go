package raster

import (
	"math"
	"strings"
	"testing"

	"vecgo/geom"
)

var testView = View{MinX: 0, MinY: 0, MaxX: 16, MaxY: 16}

func TestAdaptiveEmptyImage(t *testing.T) {
	im := &Image{}
	sq := []geom.Point{{X: 2, Y: 2}, {X: 14, Y: 2}, {X: 14, Y: 14}, {X: 2, Y: 14}}
	if _, err := im.FillAdaptive([][]geom.Point{sq}, testView, 0.1, 16); err == nil {
		t.Fatal("空图应报错")
	}
}

func TestAdaptiveEmptyScene(t *testing.T) {
	im, _ := New(8, 8)
	if _, err := im.FillAdaptive(nil, testView, 0.1, 16); err == nil {
		t.Fatal("空场景应报错")
	}
}

func TestAdaptiveZeroBudget(t *testing.T) {
	im, _ := New(8, 8)
	sq := []geom.Point{{X: 2, Y: 2}, {X: 14, Y: 2}, {X: 14, Y: 14}, {X: 2, Y: 14}}
	_, err := im.FillAdaptive([][]geom.Point{sq}, testView, 0.1, 0)
	if err == nil || !strings.Contains(err.Error(), "(0,0)") {
		t.Fatalf("样本上限为 0 应报错并给出像素坐标: %v", err)
	}
}

func TestAdaptiveZeroGradient(t *testing.T) {
	im, _ := New(8, 8)
	full := []geom.Point{{X: -4, Y: -4}, {X: 20, Y: -4}, {X: 20, Y: 20}, {X: -4, Y: 20}}
	_, err := im.FillAdaptive([][]geom.Point{full}, testView, 0.1, 16)
	if err == nil || !strings.Contains(err.Error(), "梯度处处为零") {
		t.Fatalf("整幅同色应报梯度错误并给出坐标: %v", err)
	}
}

func TestAdaptiveIllegalVertex(t *testing.T) {
	im, _ := New(8, 8)
	bad := []geom.Point{{X: math.NaN(), Y: 0}, {X: 1, Y: 0}, {X: 1, Y: 1}}
	_, err := im.FillAdaptive([][]geom.Point{bad}, testView, 0.1, 16)
	if err == nil || !strings.Contains(err.Error(), "起点非法") {
		t.Fatalf("非法起点应报错并给出坐标: %v", err)
	}
}

func TestAdaptiveBadView(t *testing.T) {
	im, _ := New(8, 8)
	sq := []geom.Point{{X: 2, Y: 2}, {X: 14, Y: 2}, {X: 14, Y: 14}, {X: 2, Y: 14}}
	if _, err := im.FillAdaptive([][]geom.Point{sq}, View{MinX: 0, MinY: 0, MaxX: 0, MaxY: 1}, 0.1, 16); err == nil {
		t.Fatal("退化视口应报错")
	}
}

func TestAdaptiveVsUniform(t *testing.T) {
	im, _ := New(32, 32)
	ref, _ := New(32, 32)
	view := View{MinX: 0, MinY: 0, MaxX: 32, MaxY: 32}
	diamond := []geom.Point{{X: 16, Y: 2}, {X: 30, Y: 16}, {X: 16, Y: 30}, {X: 2, Y: 16}}
	scene := [][]geom.Point{diamond}
	st, err := im.FillAdaptive(scene, view, 0.1, 64)
	if err != nil {
		t.Fatal(err)
	}
	if err := ref.FillUniform(scene, view, 8); err != nil {
		t.Fatal(err)
	}
	maxDiff := 0
	for i := range im.Pix {
		d := int(im.Pix[i]) - int(ref.Pix[i])
		if d < 0 {
			d = -d
		}
		if d > maxDiff {
			maxDiff = d
		}
	}
	if maxDiff > 26 {
		t.Fatalf("与统一高采样差异超阈值: %d", maxDiff)
	}
	uniform := 32 * 32 * 64
	if st.Total() >= uniform/2 {
		t.Fatalf("样本数未降到统一采样的一半以下: %d >= %d", st.Total(), uniform/2)
	}
	if st.Total() != st.ProbeSamples+st.PixelSamples {
		t.Fatalf("样本账不平: %+v", st)
	}
}
