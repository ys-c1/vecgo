package bounds

import (
	"errors"
	"math"
	"strings"
	"testing"
)

func TestClassifyThreeKinds(t *testing.T) {
	f := Frame{X0: 0, Y0: 0, X1: 10, Y1: 10}
	cases := []struct {
		name string
		x, y float64
		want Verdict
	}{
		{"内部", 5, 5, Inside},
		{"负坐标-x", -0.001, 5, Negative},
		{"负坐标-两维", -1, -1, Negative},
		{"超画幅-x", 10.001, 5, Beyond},
		{"超画幅-两维", 11, 11, Beyond},
		{"恰好等于左/上边界", 0, 0, OnEdge},
		{"恰好等于右/下边界", 10, 10, OnEdge},
		{"恰好等于单边", 10, 5, OnEdge},
		{"NaN", math.NaN(), 5, NonFinite},
		{"+Inf", math.Inf(1), 5, NonFinite},
		{"-Inf", 5, math.Inf(-1), NonFinite},
	}
	for _, tc := range cases {
		if got := Classify(tc.x, tc.y, f); got != tc.want {
			t.Errorf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
}

func TestEdgeAccepted(t *testing.T) {
	f := PixelFrame(8, 8)
	for _, p := range []Point{{0, 0}, {8, 8}, {0, 8}, {8, 0}, {4, 8}, {8, 4}} {
		if !f.Contains(p.X, p.Y) {
			t.Errorf("贴边点 %v 应接受", p)
		}
	}
}

func TestGroupsReportsPackageAndIndex(t *testing.T) {
	f := PixelFrame(16, 16)
	groups := [][]Point{
		{{X: 1, Y: 1}, {X: 2, Y: 2}, {X: 3, Y: 3}},
		{{X: 4, Y: 4}, {X: 17, Y: 5}, {X: 6, Y: 6}},
	}
	err := Groups("raster", "FillAt", "polys", f, groups)
	if err == nil {
		t.Fatal("应拒绝超画幅点")
	}
	msg := err.Error()
	for _, want := range []string{"raster.FillAt", "polys[1][1]", "(17, 5)", "超画幅", "x=17 > 16"} {
		if !strings.Contains(msg, want) {
			t.Errorf("错误信息缺少 %q: %s", want, msg)
		}
	}
	var be *Error
	if !errors.As(err, &be) {
		t.Fatal("应返回 *bounds.Error")
	}
	if be.Pkg != "raster" || be.Fn != "FillAt" || be.Verdict != Beyond {
		t.Errorf("字段异常: %+v", be)
	}
}

func TestPointsNegativeReportsPoint(t *testing.T) {
	f := PixelFrame(16, 16)
	err := Points("geom", "Bound", "poly", f, []Point{{X: 1, Y: 1}, {X: -2, Y: 3}, {X: 4, Y: 4}})
	if err == nil {
		t.Fatal("应拒绝负坐标")
	}
	msg := err.Error()
	for _, want := range []string{"geom.Bound", "poly[1]", "(-2, 3)", "负坐标", "x=-2 < 0"} {
		if !strings.Contains(msg, want) {
			t.Errorf("错误信息缺少 %q: %s", want, msg)
		}
	}
}

func TestOnEdgeAcceptedAsWholePolygon(t *testing.T) {
	f := PixelFrame(16, 16)
	poly := []Point{{0, 0}, {16, 0}, {16, 16}, {0, 16}}
	if err := Points("geom", "Bound", "poly", f, poly); err != nil {
		t.Fatalf("整块贴边应接受: %v", err)
	}
}

func TestNonFiniteRejected(t *testing.T) {
	err := FinitePoints("geom", "Flatten", "p", []Point{{0, 0}, {math.NaN(), 1}})
	if err == nil || !strings.Contains(err.Error(), "p[1]") {
		t.Fatalf("NaN 应报出下标: %v", err)
	}
}

func TestExtentRejectsZeroSpan(t *testing.T) {
	if err := Extent("raster", "FillAt", "polys[0]", 0, 5); err == nil {
		t.Fatal("零宽应拒绝")
	}
	if err := Extent("raster", "FillAt", "polys[0]", 5, 0); err == nil {
		t.Fatal("零高应拒绝")
	}
	if err := Extent("raster", "FillAt", "polys[0]", 0.5, 0.5); err != nil {
		t.Fatalf("正宽高应接受: %v", err)
	}
}

func TestFrameRejectsInverted(t *testing.T) {
	if err := CheckFrame("raster", "FillAt", Frame{X0: 10, X1: 0, Y0: 0, Y1: 10}); err == nil {
		t.Fatal("倒置域应拒绝")
	}
	if err := CheckFrame("raster", "FillAt", Frame{X0: 0, X1: 0, Y0: 0, Y1: 10}); err == nil {
		t.Fatal("零宽域应拒绝")
	}
}

func TestContainsPixelHalfOpen(t *testing.T) {
	if !ContainsPixel(0, 0, 4, 4) || !ContainsPixel(3, 3, 4, 4) {
		t.Fatal("界内像素应接受")
	}
	for _, p := range [][2]int{{-1, 0}, {4, 0}, {0, -1}, {0, 4}} {
		if ContainsPixel(p[0], p[1], 4, 4) {
			t.Errorf("界外像素 %v 应拒绝", p)
		}
	}
}

func TestCheckRectEdges(t *testing.T) {
	if err := CheckRect("post", "GammaRect", "rect", 8, 8, Full(8, 8)); err != nil {
		t.Fatalf("整幅区域应接受: %v", err)
	}
	err := CheckRect("post", "GammaRect", "rect", 8, 8, Rect{X0: -1, Y0: 0, X1: 4, Y1: 4})
	if err == nil || !strings.Contains(err.Error(), "负坐标") || !strings.Contains(err.Error(), "rect=") {
		t.Fatalf("负区域应报出: %v", err)
	}
	err = CheckRect("post", "GammaRect", "rect", 8, 8, Rect{X0: 0, Y0: 0, X1: 9, Y1: 4})
	if err == nil || !strings.Contains(err.Error(), "超画幅") {
		t.Fatalf("超画幅区域应报出: %v", err)
	}
	err = CheckRect("post", "GammaRect", "rect", 8, 8, Rect{X0: 4, Y0: 0, X1: 4, Y1: 4})
	if err == nil || !strings.Contains(err.Error(), "区域为空") {
		t.Fatalf("空区域应报出: %v", err)
	}
}
