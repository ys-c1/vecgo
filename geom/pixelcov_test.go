package geom

import "testing"

// 直接的 4×SSAA 像素覆盖率误差也必须随最差面积比改善而下降。
func TestPixelCoverageErrorImproves(t *testing.T) {
	cases := map[string][]Point{
		"ellipse":  EllipsePolygon(2.0, 0.30, 14),
		"star":     StarPolygon(1.0, 0.45),
		"corridor": CorridorPolygon(1.7, 0.45, 14, 36),
	}
	for name, model := range cases {
		poly := FitToViewport(model, PixelFrame, 4)
		ear, err := EarClip(poly)
		if err != nil {
			t.Fatal(err)
		}
		opt, err := OptimalTriangulation(poly)
		if err != nil {
			t.Fatal(err)
		}
		ce := PixelCoverageError(ear, PixelFrame, PixelSamples)
		co := PixelCoverageError(opt, PixelFrame, PixelSamples)
		if co >= ce {
			t.Fatalf("%s: 像素覆盖率误差未下降: %.4f%% → %.4f%%", name, ce*100, co*100)
		}
	}
}

func TestSampledTriangleAreaSane(t *testing.T) {
	// 轴对齐单位直角三角形，精确面积 0.5。高倍采样下估计值应逼近 0.5
	// （低倍采样会因对角线与样本对齐而有明显量化偏差，故这里用 64×）。
	tr := Triangle{{0, 0}, {1, 0}, {1, 1}}
	got := SampledTriangleArea(tr, PixelFrame, 64)
	if d := got - 0.5; d > 0.02 || d < -0.02 {
		t.Fatalf("直角三角形采样面积异常: %.4f", got)
	}
}
