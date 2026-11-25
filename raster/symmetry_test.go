package raster

import (
	"testing"

	"vecgo/geom"
)

// 三组各含 6 个点的 Grid=4 整数格点布局。
func testLayouts() map[string]SampleSet {
	return map[string]SampleSet{
		// S1：破坏对称的反例，镜像、中心两个等式都不成立。
		"S1-broken": {Grid: 4, Pts: []Sample{
			{I: 0, J: 0}, {I: 0, J: 1}, {I: 0, J: 2},
			{I: 1, J: 1}, {I: 1, J: 3}, {I: 2, J: 0},
		}},
		// S2：关于竖直中线镜像对称，但不中心对称。
		"S2-mirror": {Grid: 4, Pts: []Sample{
			{I: 0, J: 0}, {I: 3, J: 0},
			{I: 0, J: 2}, {I: 3, J: 2},
			{I: 1, J: 1}, {I: 2, J: 1},
		}},
		// S3：关于像素中心中心对称，但不镜像对称。
		"S3-central": {Grid: 4, Pts: []Sample{
			{I: 0, J: 0}, {I: 3, J: 3},
			{I: 0, J: 2}, {I: 3, J: 1},
			{I: 1, J: 0}, {I: 2, J: 3},
		}},
	}
}

func TestSymmetrySetEquations(t *testing.T) {
	wantMirror := map[string]bool{"S1-broken": false, "S2-mirror": true, "S3-central": false}
	wantCentral := map[string]bool{"S1-broken": false, "S2-mirror": false, "S3-central": true}
	for name, ss := range testLayouts() {
		if err := ss.Validate(); err != nil {
			t.Fatalf("%s 布局非法: %v", name, err)
		}
		gotM, mL, mR := ss.MirrorSymmetric()
		if gotM != wantMirror[name] {
			t.Fatalf("%s 镜像等式判定=%v, 期望 %v", name, gotM, wantMirror[name])
		}
		if !gotM && (len(mL) == 0 || len(mR) == 0) {
			t.Fatalf("%s 不满足镜像等式时集合差不应为空", name)
		}
		gotC, cL, cR := ss.CentralSymmetric()
		if gotC != wantCentral[name] {
			t.Fatalf("%s 中心等式判定=%v, 期望 %v", name, gotC, wantCentral[name])
		}
		if !gotC && (len(cL) == 0 || len(cR) == 0) {
			t.Fatalf("%s 不满足中心等式时集合差不应为空", name)
		}
	}
}

func TestValidateRejectsDuplicateAndOutOfRange(t *testing.T) {
	bad := SampleSet{Grid: 4, Pts: []Sample{{I: 0, J: 0}, {I: 0, J: 0}}}
	if err := bad.Validate(); err == nil {
		t.Fatal("重复采样点应报错")
	}
	bad = SampleSet{Grid: 4, Pts: []Sample{{I: 4, J: 0}}}
	if err := bad.Validate(); err == nil {
		t.Fatal("越界采样点应报错")
	}
}

// 非退化三角形：斜边斜率为 17/64 与 47/64，分子为奇数，
// 任何 1/8 奇数倍坐标的采样点都不可能落在边线上。
func testTriangle() []geom.Point {
	return []geom.Point{{X: 0, Y: 0}, {X: 64, Y: 17}, {X: 0, Y: 64}}
}

func normalize(poly []geom.Point) []geom.Point {
	minx, miny := poly[0].X, poly[0].Y
	for _, p := range poly {
		if p.X < minx {
			minx = p.X
		}
		if p.Y < miny {
			miny = p.Y
		}
	}
	out := make([]geom.Point, len(poly))
	for i, p := range poly {
		out[i] = geom.Point{X: p.X - minx, Y: p.Y - miny}
	}
	return out
}

func mirrorShape(poly []geom.Point) []geom.Point {
	minx, maxx := poly[0].X, poly[0].X
	for _, p := range poly {
		if p.X < minx {
			minx = p.X
		}
		if p.X > maxx {
			maxx = p.X
		}
	}
	out := make([]geom.Point, len(poly))
	for i, p := range poly {
		out[i] = geom.Point{X: minx + maxx - p.X, Y: p.Y}
	}
	return normalize(out)
}

func rotateShape(poly []geom.Point) []geom.Point {
	minx, miny := poly[0].X, poly[0].Y
	maxx, maxy := minx, miny
	for _, p := range poly {
		if p.X < minx {
			minx = p.X
		}
		if p.X > maxx {
			maxx = p.X
		}
		if p.Y < miny {
			miny = p.Y
		}
		if p.Y > maxy {
			maxy = p.Y
		}
	}
	out := make([]geom.Point, len(poly))
	for i, p := range poly {
		out[i] = geom.Point{X: minx + maxx - p.X, Y: miny + maxy - p.Y}
	}
	return normalize(out)
}

func flipH(im *Image) *Image {
	out, _ := New(im.W, im.H)
	for row := 0; row < im.H; row++ {
		for col := 0; col < im.W; col++ {
			out.Pix[row*im.W+(im.W-1-col)] = im.Pix[row*im.W+col]
		}
	}
	return out
}

func flip180(im *Image) *Image {
	out, _ := New(im.W, im.H)
	for row := 0; row < im.H; row++ {
		for col := 0; col < im.W; col++ {
			out.Pix[(im.H-1-row)*im.W+(im.W-1-col)] = im.Pix[row*im.W+col]
		}
	}
	return out
}

func diffPixels(a, b *Image) int {
	n := 0
	for i := range a.Pix {
		if a.Pix[i] != b.Pix[i] {
			n++
		}
	}
	return n
}

// TestSymmetricLayoutsGiveZeroDeviation 是核心性质：
// 集合等式成立 => 镜像（或旋转）前后逐像素相同，差异像素数恒为零。
func TestSymmetricLayoutsGiveZeroDeviation(t *testing.T) {
	tri := normalize(testTriangle())
	mir := mirrorShape(testTriangle())
	rot := rotateShape(testTriangle())
	for _, tc := range []struct {
		name        string
		useMirror   bool
		wantZeroFor string
	}{
		{name: "水平镜像", useMirror: true, wantZeroFor: "S2-mirror"},
		{name: "180°旋转", useMirror: false, wantZeroFor: "S3-central"},
	} {
		for name, ss := range testLayouts() {
			orig, _ := New(64, 64)
			if err := orig.FillSamples([][]geom.Point{tri}, ss); err != nil {
				t.Fatal(err)
			}
			var other *Image
			if tc.useMirror {
				tmp, _ := New(64, 64)
				if err := tmp.FillSamples([][]geom.Point{mir}, ss); err != nil {
					t.Fatal(err)
				}
				other = flipH(tmp)
			} else {
				tmp, _ := New(64, 64)
				if err := tmp.FillSamples([][]geom.Point{rot}, ss); err != nil {
					t.Fatal(err)
				}
				other = flip180(tmp)
			}
			got := diffPixels(orig, other)
			want := -1 // 默认只要求非负
			if name == tc.wantZeroFor {
				want = 0
			}
			if want == 0 && got != 0 {
				t.Fatalf("%s 下 %s 满足集合等式但差异像素=%d，期望 0", tc.name, name, got)
			}
			if want != 0 && got == 0 {
				t.Fatalf("%s 下 %s 不满足集合等式却零差异，实验设计失效", tc.name, name)
			}
		}
	}
}

// TestBrokenLayoutHasNonzeroDeviation 固定反例的差异像素数，防止后续改动把信号抹掉。
func TestBrokenLayoutHasNonzeroDeviation(t *testing.T) {
	ss := testLayouts()["S1-broken"]
	tri := normalize(testTriangle())
	mir := mirrorShape(testTriangle())
	orig, _ := New(64, 64)
	tmp, _ := New(64, 64)
	if err := orig.FillSamples([][]geom.Point{tri}, ss); err != nil {
		t.Fatal(err)
	}
	if err := tmp.FillSamples([][]geom.Point{mir}, ss); err != nil {
		t.Fatal(err)
	}
	if got := diffPixels(orig, flipH(tmp)); got != 104 {
		t.Fatalf("S1 镜像差异像素=%d，期望 104", got)
	}
}

// TestFillBackwardCompatible 确认 Fill 重构后与显式网格布局结果一致。
func TestFillBackwardCompatible(t *testing.T) {
	sq := []geom.Point{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}
	a, _ := New(8, 8)
	b, _ := New(8, 8)
	if err := a.Fill([][]geom.Point{sq}, 2); err != nil {
		t.Fatal(err)
	}
	grid := SampleSet{Grid: 2, Pts: []Sample{{0, 0}, {1, 0}, {0, 1}, {1, 1}}}
	if err := b.FillSamples([][]geom.Point{sq}, grid); err != nil {
		t.Fatal(err)
	}
	for i := range a.Pix {
		if a.Pix[i] != b.Pix[i] {
			t.Fatalf("像素 %d: Fill=%d FillSamples=%d", i, a.Pix[i], b.Pix[i])
		}
	}
}
