package pipe

import (
	"bytes"
	"math"
	"testing"

	"vecgo/config"
	"vecgo/geom"
)

func sq(x0, y0, x1, y1 float64) []geom.Point {
	return []geom.Point{{X: x0, Y: y0}, {X: x1, Y: y0}, {X: x1, Y: y1}, {X: x0, Y: y1}}
}

// 统计关闭时，Run 与 RunWithStats 的图像逐字节一致；Run 不碰统计路径。
func TestRunUnchangedByStats(t *testing.T) {
	cfg := config.Default()
	cfg.Width, cfg.Height = 64, 48
	cfg.Supersample = 3
	scene := [][]geom.Point{sq(0, 0, 20, 20), sq(10, 0, 30, 20)}

	off, err := Run(scene, cfg)
	if err != nil {
		t.Fatal(err)
	}
	st := &Stats{Enabled: true, KeepPerPixel: true}
	on, err := RunWithStats(scene, cfg, st)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(off.Pix, on.Pix) {
		t.Fatal("统计改变了成像")
	}
	if st.Fill.Posts == 0 || st.Fill.ExtraPosts == 0 {
		t.Fatal("统计未采集")
	}
}

// 理论值必须严格等于解析面积占比 × 每像素样本数，而不是采样估计。
func TestTheoryFromAnalyticArea(t *testing.T) {
	cfg := config.Default()
	cfg.Width, cfg.Height = 120, 80
	cfg.Supersample = 4
	// 画幅 30×20=600；两个 20×20，交集 200，重叠面积 200。
	st := &Stats{Enabled: true}
	if _, err := RunWithStats([][]geom.Point{sq(0, 0, 20, 20), sq(10, 0, 30, 20)}, cfg, st); err != nil {
		t.Fatal(err)
	}
	if st.CanvasArea != 600 || st.PixelArea != 600.0/(120*80) {
		t.Fatalf("画幅口径错误: canvas=%v pixel=%v", st.CanvasArea, st.PixelArea)
	}
	if math.Abs(st.OverlapArea-200) > 1e-9 {
		t.Fatalf("重叠面积 %v", st.OverlapArea)
	}
	if math.Abs(st.TheoryRepeatPerPixel()-200.0/600*16) > 1e-9 {
		t.Fatalf("理论重复次数 %v", st.TheoryRepeatPerPixel())
	}
	if math.Abs(st.TheoryPostsPerPixel()-800.0/600*16) > 1e-9 {
		t.Fatalf("理论写入次数 %v", st.TheoryPostsPerPixel())
	}
}

// 三类重叠的理论/实测必须吻合；旋转后仍应吻合（采样只带来小量误差）。
func TestTheoryMatchesMeasurement(t *testing.T) {
	cases := []struct {
		name  string
		scene [][]geom.Point
		w, h  int
		want  float64 // 重叠面积
	}{
		{"duplicate", [][]geom.Point{sq(0, 0, 20, 20), sq(0, 0, 20, 20)}, 80, 80, 400},
		{"partial", [][]geom.Point{sq(0, 0, 20, 20), sq(10, 0, 30, 20)}, 120, 80, 200},
		{"contained", [][]geom.Point{sq(0, 0, 20, 20), sq(5, 5, 15, 15)}, 80, 80, 100},
	}
	for _, tc := range cases {
		for _, rot := range []float64{0, 0.37} {
			cfg := config.Default()
			cfg.Width, cfg.Height, cfg.Supersample = tc.w, tc.h, 4
			cfg.Rotate = rot
			st := &Stats{Enabled: true}
			if _, err := RunWithStats(tc.scene, cfg, st); err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			if rot == 0 && math.Abs(st.OverlapArea-tc.want) > 1e-9 {
				t.Fatalf("%s: 解析重叠面积 %v want %v", tc.name, st.OverlapArea, tc.want)
			}
			tp, mp := st.TheoryRepeatPerPixel(), st.MeasuredNaiveRepeatPerPixel()
			if d := math.Abs(RelDiff(mp, tp)); d > 0.01 {
				t.Fatalf("%s rot=%.2f: 理论 %.4f 实测 %.4f 相对差 %.2f%%", tc.name, rot, tp, mp, 100*d)
			}
			// 覆盖占比同样应吻合：它是面积占比的独立校验。
			if d := math.Abs(RelDiff(st.MeasuredCoveredFraction(), st.UnionFraction)); d > 0.01 {
				t.Fatalf("%s rot=%.2f: 覆盖占比 理论 %.4f 实测 %.4f", tc.name, rot, st.UnionFraction, st.MeasuredCoveredFraction())
			}
			// 被 ≥2 个图形覆盖的样本占比也应吻合。
			if d := math.Abs(RelDiff(st.MeasuredOverlapFraction(), st.MultiFraction())); d > 0.01 {
				t.Fatalf("%s rot=%.2f: 多重覆盖占比 理论 %.4f 实测 %.4f", tc.name, rot, st.MultiFraction(), st.MeasuredOverlapFraction())
			}
		}
	}
}

// 现行路径首命中短路：实际写入次数恒等于覆盖样本数，重复为 0。
func TestActualPathShortCircuits(t *testing.T) {
	cfg := config.Default()
	cfg.Width, cfg.Height, cfg.Supersample = 40, 40, 2
	st := &Stats{Enabled: true}
	if _, err := RunWithStats([][]geom.Point{sq(0, 0, 20, 20), sq(5, 5, 15, 15)}, cfg, st); err != nil {
		t.Fatal(err)
	}
	if st.MeasuredActualRepeatPerPixel() != 0 {
		t.Fatalf("短路后重复应为 0，实际 %v", st.MeasuredActualRepeatPerPixel())
	}
	if st.Fill.ActualPosts != st.Fill.CoveredSamples {
		t.Fatalf("actual=%d covered=%d", st.Fill.ActualPosts, st.Fill.CoveredSamples)
	}
}

// 统计只允许对凸多边形给出理论值：凹多边形必须报错而不是给出错误数字。
func TestRunWithStatsRejectsNonConvex(t *testing.T) {
	cfg := config.Default()
	non := []geom.Point{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 5, Y: 3}, {X: 10, Y: 10}, {X: 0, Y: 10}}
	if _, err := RunWithStats([][]geom.Point{non}, cfg, &Stats{Enabled: true}); err == nil {
		t.Fatal("应报错")
	}
	// 无统计路径照常渲染。
	if _, err := Run([][]geom.Point{non}, cfg); err != nil {
		t.Fatalf("普通渲染不应受统计限制: %v", err)
	}
}
