package shadow

import (
	"math"
	"testing"

	"vecgo/geom"
	"vecgo/raster"
)

func approx(t *testing.T, name string, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Fatalf("%s = %.6f, 期望 %.6f (容差 %.2g)", name, got, want, tol)
	}
}

func TestPenumbraFormulaAndTwoRatios(t *testing.T) {
	// H=100, h=20 -> r=20/80=1/4；L=12 -> 半宽 1.5，总宽 3 世界单位。
	g := Geometry{Size: 12, Gap: 20, Dist: 100}
	r, l, err := g.TriangleRatios()
	if err != nil {
		t.Fatal(err)
	}
	// 两次相似三角形的比例必须一致：x+/(L/2) = |x-|/(L/2) = h/(H-h)。
	approx(t, "右比例 h/(H-h)", r.Proportion, 0.25, 1e-12)
	approx(t, "左比例 h/(H-h)", l.Proportion, 0.25, 1e-12)
	approx(t, "右半影半宽 x+", r.ReceiverRun, 1.5, 1e-12)
	approx(t, "左半影半宽 |x-|", l.ReceiverRun, 1.5, 1e-12)
	if r.SourceHalf != 6 || r.LightToBlocker != 80 || r.Gap != 20 {
		t.Fatalf("相似三角形直角边记录异常: %+v", r)
	}
	if r.Side != "right" || l.Side != "left" {
		t.Fatalf("左右侧标记错误: %q %q", r.Side, l.Side)
	}

	w, err := g.Penumbra()
	if err != nil {
		t.Fatal(err)
	}
	approx(t, "w = L*h/(H-h)", w, 3.0, 1e-12)
	wp, err := g.PenumbraPixels(4)
	if err != nil {
		t.Fatal(err)
	}
	approx(t, "w_px", wp, 12.0, 1e-12)

	// 端点平移跨度必须恰好等于总半影宽。
	d1, _ := g.EdgeShift(-g.Size / 2)
	d2, _ := g.EdgeShift(g.Size / 2)
	approx(t, "两端点投影跨度", d1-d2, w, 1e-12)
}

func TestPointLimitIsZero(t *testing.T) {
	g := Geometry{Size: 123, Gap: 10, Dist: 40}
	w0, err := PointLimit(g)
	if err != nil {
		t.Fatal(err)
	}
	if w0 != 0 {
		t.Fatalf("L->0 极限应为 0, 得到 %v", w0)
	}
	point := Geometry{Size: 0, Gap: 10, Dist: 40}
	w, _ := point.PenumbraPixels(7)
	if w != 0 {
		t.Fatalf("点光源预测半影应为 0, 得到 %v", w)
	}
	// 比例式中 L/2=0，两个半宽也都为 0。
	r, l, _ := point.TriangleRatios()
	if r.ReceiverRun != 0 || l.ReceiverRun != 0 {
		t.Fatal("点光源两侧半宽必须同时为 0")
	}
}

func TestRejectBadGeometry(t *testing.T) {
	bad := []Geometry{
		{Size: 1, Gap: 0, Dist: 10},
		{Size: 1, Gap: -2, Dist: 10},
		{Size: 1, Gap: 9, Dist: 9},   // H 必须严格大于 h
		{Size: -1, Gap: 2, Dist: 10}, // 负尺寸
	}
	for i, g := range bad {
		if err := g.Validate(); err == nil {
			t.Fatalf("bad[%d] 应校验失败: %+v", i, g)
		}
		if _, err := g.Penumbra(); err == nil {
			t.Fatalf("bad[%d] Penumbra 应报错", i)
		}
	}
	g := Geometry{Size: 1, Gap: 2, Dist: 10}
	if _, err := g.PenumbraPixels(0); err == nil {
		t.Fatal("pxPerUnit=0 应报错")
	}
}

func TestLightPositions(t *testing.T) {
	if p := LightPositions(0, 64); len(p) != 1 || p[0] != 0 {
		t.Fatalf("点光源必须只有中心采样点: %v", p)
	}
	p := LightPositions(12, 4)
	want := []float64{-4.5, -1.5, 1.5, 4.5}
	for i := range want {
		approx(t, "布点", p[i], want[i], 1e-12)
	}
}

func testStrip(x0, x1 float64) [][]geom.Point {
	return [][]geom.Point{{
		{X: x0, Y: -4000}, {X: x1, Y: -4000},
		{X: x1, Y: 5000}, {X: x0, Y: 5000},
	}}
}

func TestUnifiedPointPathIgnoresSamplesAndIsHard(t *testing.T) {
	g := Geometry{Size: 0, Gap: 20, Dist: 100}
	opts := Options{Width: 120, Height: 40, PxPerUnit: 4, SamplesX: 64, Lit: 238, Dark: 16}
	im1, err := Render(testStrip(40, 80), g, opts)
	if err != nil {
		t.Fatal(err)
	}
	opts.SamplesX = 1
	im2, err := Render(testStrip(40, 80), g, opts)
	if err != nil {
		t.Fatal(err)
	}
	for i := range im1.Pix {
		if im1.Pix[i] != im2.Pix[i] {
			t.Fatal("Size=0 时采样数必须被忽略，两次渲染应一致")
		}
	}
	// 像素对齐的硬边：画面里只允许出现亮/暗两个灰度。
	for _, v := range im1.Pix {
		if v != 238 && v != 16 {
			t.Fatalf("点光源应为纯硬边，出现中间灰度 %d", v)
		}
	}
}

func TestSyntheticLinearRampMeasurement(t *testing.T) {
	const w, hgt = 120, 4
	im, _ := raster.New(w, hgt)
	const c, pw, dark, lit = 60.0, 20.0, 20.0, 230.0
	for x := 0; x < w; x++ {
		xc := float64(x) + 0.5
		t := (xc - (c - pw/2)) / pw
		if t < 0 {
			t = 0
		} else if t > 1 {
			t = 1
		}
		v := uint8(math.Round(dark + (lit-dark)*t))
		for y := 0; y < hgt; y++ {
			im.Pix[y*w+x] = v
		}
	}
	p, err := ColumnProfile(im, 0, hgt)
	if err != nil {
		t.Fatal(err)
	}
	m, err := MeasureEdge(p, c, pw)
	if err != nil {
		t.Fatal(err)
	}
	approx(t, "合成斜坡 x50", m.XHalfGray, c, 0.05)
	approx(t, "合成斜坡宽度", m.WidthHalfGray, pw, 0.15)
	approx(t, "10-90 宽度", m.Width1090, pw, 0.2)
}

func TestRenderedWidthMatchesSimilarTriangles(t *testing.T) {
	cases := []struct {
		size    float64
		samples int
		wantPx  float64
		tolPx   float64
	}{
		{0, 1, 0, 0}, // 点光源：几何宽 0；实测允许 1px 基底，单独断言
		{12, 192, 12, 0.25},
		{36, 384, 36, 0.35},
	}
	const W, H = 360, 90
	for _, tc := range cases {
		g := Geometry{Size: tc.size, Gap: 20, Dist: 100}
		pred, _ := g.PenumbraPixels(4)
		if pred != tc.wantPx {
			t.Fatalf("预测宽度异常: %v != %v", pred, tc.wantPx)
		}
		im, err := Render(testStrip(120, 240), g, Options{
			Width: W, Height: H, PxPerUnit: 4, SamplesX: tc.samples, SamplesY: 1,
			Lit: 238, Dark: 16,
		})
		if err != nil {
			t.Fatal(err)
		}
		p, err := ColumnProfile(im, H/4, 3*H/4)
		if err != nil {
			t.Fatal(err)
		}
		search := math.Max(pred, 2) + 4
		ml, err := MeasureEdge(p, 120, search)
		if err != nil {
			t.Fatal(err)
		}
		mr, err := MeasureEdge(p, 240, search)
		if err != nil {
			t.Fatal(err)
		}
		got := (ml.WidthHalfGray + mr.WidthHalfGray) / 2
		if tc.size == 0 {
			if math.Abs(got-1.0) > 0.01 {
				t.Fatalf("点光源实测应为 1px 离散基底, got %.3f", got)
			}
			continue
		}
		if math.Abs(got-pred) > tc.tolPx {
			t.Fatalf("L=%v 实测 %.3f vs 预测 %.2f 超差 %.2f", tc.size, got, pred, tc.tolPx)
		}
		// 半灰位置必须正好落在中心投影边上。
		approx(t, "左 x50", ml.XHalfGray, 120, 0.05)
		approx(t, "右 x50", mr.XHalfGray, 240, 0.05)
	}
}
