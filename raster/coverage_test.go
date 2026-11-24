package raster

import (
	"math"
	"testing"

	"vecgo/geom"
	"vecgo/internal/exactcov"
)

// rotSquare 构造边长 side 的轴对齐方块绕原点旋转 deg 度后的多边形。
func rotSquare(side, deg float64) []geom.Point {
	t := deg * math.Pi / 180
	c, s := math.Cos(t), math.Sin(t)
	base := []geom.Point{{X: 0, Y: 0}, {X: side, Y: 0}, {X: side, Y: side}, {X: 0, Y: side}}
	out := make([]geom.Point, len(base))
	for i, p := range base {
		out[i] = geom.Point{X: p.X*c - p.Y*s, Y: p.X*s + p.Y*c}
	}
	return out
}

func TestAreaKnownShapes(t *testing.T) {
	sq := []geom.Point{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}
	if a := geom.Area(sq); math.Abs(a-100) > 1e-12 {
		t.Fatalf("方形面积应为 100, 得 %v", a)
	}
	tri := []geom.Point{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 0, Y: 1}}
	if a := geom.Area(tri); math.Abs(a-0.5) > 1e-12 {
		t.Fatalf("三角形面积应为 0.5, 得 %v", a)
	}
	if a := geom.Area([]geom.Point{{X: 0, Y: 0}, {X: 1, Y: 1}}); a != 0 {
		t.Fatalf("两点退化面积应为 0, 得 %v", a)
	}
}

func TestCoverageExactCases(t *testing.T) {
	// 恰好占当前像素左下一半的三角形（逆时针）。
	half := []geom.Point{{X: 3, Y: 4}, {X: 4, Y: 4}, {X: 3, Y: 5}}
	got, clipped, err := ClipCoveragePx(half, 3, 4)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(got-0.5) > 1e-12 {
		t.Fatalf("半像素覆盖率应为 0.5, 得 %.15f (裁后 %v)", got, clipped)
	}

	// 完整覆盖的像素与完全在外的像素。
	big := []geom.Point{{X: -2, Y: -2}, {X: 8, Y: -2}, {X: 8, Y: 8}, {X: -2, Y: 8}}
	if got, _, _ := ClipCoveragePx(big, 2, 2); math.Abs(got-1) > 1e-12 {
		t.Fatalf("内部像素覆盖率应为 1, 得 %.15f", got)
	}
	if got, _, _ := ClipCoveragePx(big, 9, 9); got != 0 {
		t.Fatalf("外部像素覆盖率应为 0, 得 %.15f", got)
	}

	// 斜边 y=x 对角切过：单位方块右下三角面积 0.5。
	diag := []geom.Point{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 1, Y: 1}}
	if got, _, _ := ClipCoveragePx(diag, 0, 0); math.Abs(got-0.5) > 1e-12 {
		t.Fatalf("对角半切应为 0.5, 得 %.15f", got)
	}
}

// TestSubsampleHelperMatchesFill 保证对比用的 SubsampleRatioPx 与
// Image.Fill 的子点网格、整数灰度完全同一套约定。
func TestSubsampleHelperMatchesFill(t *testing.T) {
	const w, h, ss = 8, 8, 2
	// 包围盒恰好 [0,8]×[0,8]，映射后就是单位像素坐标。
	tri := []geom.Point{{X: 0, Y: 0}, {X: 8, Y: 0}, {X: 0, Y: 8}}
	im, err := New(w, h)
	if err != nil {
		t.Fatal(err)
	}
	if err := im.Fill([][]geom.Point{tri}, ss); err != nil {
		t.Fatal(err)
	}
	for r := 0; r < h; r++ {
		for c := 0; c < w; c++ {
			ratio, hit := SubsampleRatioPx(tri, c, r, ss)
			want := uint8(255 * hit / (ss * ss)) // Fill 的整数运算
			if im.Pix[r*w+c] != want {
				t.Fatalf("(%d,%d) Fill=%d 但辅助公式给 %d", c, r, im.Pix[r*w+c], want)
			}
			if ratio != float64(hit)/4 {
				t.Fatalf("比例计算错误: %v hit=%d", ratio, hit)
			}
		}
	}
}

type angleReport struct {
	deg                   float64
	edgePixels            int
	analyticRMSE, ss2RMSE float64
	analyticMax, ss2Max   float64
	conservation          float64
}

// evaluateAngle 在 24×24 画幅、给定旋转角下，以独立精确基准评估
// 解析面积裁切与默认 2×2 子点法，统计边缘像素上的覆盖率 RMSE。
func evaluateAngle(t *testing.T, deg float64) angleReport {
	t.Helper()
	const w, h = 24, 24
	poly := rotSquare(10, deg)
	m, err := BoundsMap([][]geom.Point{poly}, w, h)
	if err != nil {
		t.Fatal(err)
	}
	px := m.MapPoly(poly)

	var rep angleReport
	rep.deg = deg
	var anSE, ssSE, sumAn float64
	for r := 0; r < h; r++ {
		for c := 0; c < w; c++ {
			an, _, err := ClipCoveragePx(px, c, r)
			if err != nil {
				t.Fatal(err)
			}
			truth := exactcov.ConvexCoverage(px, float64(c), float64(r), float64(c+1), float64(r+1))
			ss2, _ := SubsampleRatioPx(px, c, r, 2)
			sumAn += an
			// 边缘像素：解析覆盖率严格落在 (0,1)。
			if an > 1e-9 && an < 1-1e-9 {
				rep.edgePixels++
				e := an - truth
				anSE += e * e
				rep.analyticMax = math.Max(rep.analyticMax, math.Abs(e))
				e2 := ss2 - truth
				ssSE += e2 * e2
				rep.ss2Max = math.Max(rep.ss2Max, math.Abs(e2))
			}
		}
	}
	n := float64(rep.edgePixels)
	rep.analyticRMSE = math.Sqrt(anSE / n)
	rep.ss2RMSE = math.Sqrt(ssSE / n)
	rep.conservation = math.Abs(sumAn - geom.Area(px))
	return rep
}

// TestAnalyticBeatsSubsampleAtTwoAngles 是任务的核心验收：
// 两档角度（15° 缓斜、45° 对角）各验一次，解析法对精确基准的误差
// 必须小于默认 2×2 子点法误差的一半；面积总和必须守恒。
func TestAnalyticBeatsSubsampleAtTwoAngles(t *testing.T) {
	for _, deg := range []float64{15, 45} {
		rep := evaluateAngle(t, deg)
		t.Logf("angle=%.0f edge=%d analyticRMSE=%.3e(max %.3e) ss2RMSE=%.5f(max %.4f) ratio=%.2e conservation=%.2e",
			rep.deg, rep.edgePixels, rep.analyticRMSE, rep.analyticMax,
			rep.ss2RMSE, rep.ss2Max, rep.analyticRMSE/rep.ss2RMSE, rep.conservation)

		if rep.edgePixels < 20 {
			t.Fatalf("边缘像素样本过少: %d", rep.edgePixels)
		}
		// 实现与独立精确基准应在浮点精度内一致。
		if rep.analyticRMSE > 1e-9 {
			t.Fatalf("%.0f° 解析覆盖率偏离精确基准: RMSE=%v", deg, rep.analyticRMSE)
		}
		// 硬性要求：解析误差 < 子点法误差的一半（实际余量约 10^12 倍）。
		if !(rep.analyticRMSE < 0.5*rep.ss2RMSE) {
			t.Fatalf("%.0f° 解析误差 %v 未小于子点误差 %v 的一半", deg, rep.analyticRMSE, rep.ss2RMSE)
		}
		// 所有像素交叠面积之和必须等于多边形面积（裁切面积可核）。
		if rep.conservation > 1e-8 {
			t.Fatalf("%.0f° 面积不守恒，差额=%v", deg, rep.conservation)
		}
	}
}

func TestFillCoverageEndToEnd(t *testing.T) {
	im, err := New(16, 16)
	if err != nil {
		t.Fatal(err)
	}
	// 45° 旋转方块在整数格点上呈菱形：每条斜边边缘像素覆盖率都约为 0.5，
	// 按 Fill 的截断量化 uint8(255*c) 一律为 127（不是四舍五入的 128）。
	if err := im.FillCoverage(rotSquare(10, 45)); err != nil {
		t.Fatal(err)
	}
	var mid, full, zero int
	for _, v := range im.Pix {
		switch {
		case v == 127:
			mid++
		case v == 255:
			full++
		case v == 0:
			zero++
		default:
			// 该场景解析上只有 0/0.5/1 三档覆盖，对应灰度 0/127/255。
			t.Fatalf("出现非预期灰度 %d", v)
		}
	}
	if mid < 20 || full < 50 || zero < 50 {
		t.Fatalf("0/127/255 分布异常: zero=%d mid=%d full=%d", zero, mid, full)
	}
	if err := im.FillCoverage([]geom.Point{{X: 0, Y: 0}, {X: 1, Y: 0}}); err == nil {
		t.Fatal("少于三个顶点应报错")
	}
}
