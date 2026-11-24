package coverage

import (
	"math"
	"strconv"
	"testing"

	"vecgo/geom"
)

func almost(t *testing.T, name string, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Fatalf("%s = %.12g, 期望 %.12g (容差 %g)", name, got, want, tol)
	}
}

// 覆盖率函数在裁剪位置取整数时必须退化为 0 或 1。
func TestIntegerClipPositionIsBinary(t *testing.T) {
	for k := -20; k <= 20; k++ {
		c := float64(k)
		for i := -5; i <= 5; i++ {
			a := UnitVertical(c, i)
			if a != 0 && a != 1 {
				t.Fatalf("竖直: c=%d i=%d 覆盖率=%v 不是 0/1", k, i, a)
			}
			b := UnitHorizontal(c, i)
			if b != 0 && b != 1 {
				t.Fatalf("水平: c=%d j=%d 覆盖率=%v 不是 0/1", k, i, b)
			}
			// 与硬切语义一致：边落在像素左界 -> 0，落在右界 -> 1。
			if got := UnitVertical(c, k); got != 0 {
				t.Fatalf("c=k 应对齐像素左界且覆盖为 0, got %v", got)
			}
			if got := UnitVertical(c, k-1); got != 1 {
				t.Fatalf("c=k 应对齐上一像素右界且覆盖为 1, got %v", got)
			}
		}
	}
}

// 三档裁剪位置（1/3、1/2、2/3）的闭式值与 SS16 中点网格精确计数。
func TestThreeTiersClosedFormVsSS16(t *testing.T) {
	tiers := []struct {
		name       string
		f          float64
		wantAlpha  float64
		wantHits   int
		wantBeta   float64
		wantSigned float64 // alpha-beta
	}{
		{"浅切 1/3", 1.0 / 3, 1.0 / 3, 80, 5.0 / 16, 1.0 / 48},
		{"半切 1/2", 1.0 / 2, 1.0 / 2, 128, 1.0 / 2, 0},
		{"深切 2/3", 2.0 / 3, 2.0 / 3, 176, 11.0 / 16, -1.0 / 48},
	}
	for _, tc := range tiers {
		t.Run(tc.name, func(t *testing.T) {
			a := UnitVertical(tc.f, 0)
			almost(t, "闭式覆盖率", a, tc.wantAlpha, 1e-15)

			hits, total, beta := SuperSampleVertical(tc.f, 0, 0, 1, 1, 16)
			if total != 256 {
				t.Fatalf("SS16 总样本数应为 256, got %d", total)
			}
			if hits != tc.wantHits {
				t.Fatalf("SS16 命中数 = %d, 期望 %d", hits, tc.wantHits)
			}
			almost(t, "SS16 覆盖率", beta, tc.wantBeta, 1e-15)
			almost(t, "alpha-beta", a-beta, tc.wantSigned, 1e-15)
		})
	}
}

// SS16 中点网格对轴对齐直边的确定性误差上界是 1/(2n)=1/32。
// 全位置扫描验证该界。
func TestSS16DeterministicBound(t *testing.T) {
	const n = 16
	const bound = 1.0 / (2 * n)
	for m := 0; m <= 1000; m++ {
		c := float64(m) / 1000
		_, _, beta := SuperSampleVertical(c, 0, 0, 1, 1, n)
		alpha := VerticalFraction(c, 0, 1)
		if math.Abs(beta-alpha) > bound+1e-12 {
			t.Fatalf("c=%.3f 误差 %.6g 超过中点网格界 %.6g", c, math.Abs(beta-alpha), bound)
		}
	}
}

// 面积守恒：覆盖率密度（截面长度）对裁剪位置的积分等于像素面积。
// 这里用独立的细中点黎曼和数值积分验证多组非单位像素。
func TestCoverageDensityIntegratesToPixelArea(t *testing.T) {
	cases := []struct{ x0, y0, x1, y1 float64 }{
		{0, 0, 1, 1},
		{-2, 3, 1, 7},           // w=3 h=4 area=12
		{0.5, -1.25, 2.5, 0.75}, // w=2 h=2 area=4
		{1, 1, 4, 3},            // w=3 h=2 area=6
	}
	const step = 1e-4
	for _, r := range cases {
		area := (r.x1 - r.x0) * (r.y1 - r.y0)
		lo, hi := r.x0-2, r.x1+2
		sum := 0.0
		for c := lo + step/2; c < hi; c += step {
			sum += VerticalDensity(c, r.x0, r.y0, r.x1, r.y1) * step
		}
		if math.Abs(sum-area) > math.Max(area*1e-3, 2*step*(r.y1-r.y0)) {
			t.Fatalf("数值积分 %.6f != 像素面积 %.6f", sum, area)
		}
		// 闭式累积覆盖率在两端的总增量同样等于面积（FTC）。
		aLo := VerticalArea(lo, r.x0, r.y0, r.x1, r.y1)
		aHi := VerticalArea(hi, r.x0, r.y0, r.x1, r.y1)
		almost(t, "A(+inf)-A(-inf)", aHi-aLo, area, 1e-12)
		almost(t, "饱和覆盖面积", aHi, area, 1e-12)
	}
}

// 穿过区间上累积覆盖率自身的积分是 A*w/2（单位像素为 1/2），
// 明确它与“密度积分等于面积”的区别。
func TestCumulativeRampIntegral(t *testing.T) {
	const step = 1e-5
	var sum float64
	for c := step / 2; c < 1; c += step {
		sum += UnitVertical(c, 0) * step
	}
	almost(t, "int_0^1 clamp(c,0,1) dc", sum, 0.5, 1e-4)
}

// 互补对称：alpha(f)+alpha(1-f)=1。
func TestComplementSymmetry(t *testing.T) {
	for m := 0; m <= 100; m++ {
		f := float64(m) / 100
		if a, b := UnitVertical(f, 0), UnitVertical(1-f, 0); math.Abs(a+b-1) > 1e-12 {
			t.Fatalf("f=%v 互补对称失败: %v+%v", f, a, b)
		}
	}
}

// 任意方向直边：单位方块被 x+y<=d 切割时的已知二次闭式面积。
func TestHalfPlaneDiagonalExactAreas(t *testing.T) {
	sq := []geom.Point{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 1, Y: 1}, {X: 0, Y: 1}}
	cases := []struct {
		d    float64
		want float64
	}{
		{0, 0},
		{0.5, 0.125}, // 直角三角形 d^2/2
		{1, 0.5},
		{1.5, 0.875}, // 1-(2-d)^2/2
		{2, 1},
	}
	for _, tc := range cases {
		got := HalfPlaneArea(sq, 1, 1, tc.d)
		almost(t, "x+y<="+strconv.FormatFloat(tc.d, 'f', -1, 64), got, tc.want, 1e-12)
	}
}

// 斜直边的精确面积覆盖率与 SS16 一致。斜边误差大于轴对齐的 1/(2n)
// 界（SS16 实测最大约 0.023），并应随 n 增大而收敛。
func TestHalfPlaneDiagonalVsSupersample(t *testing.T) {
	sq := []geom.Point{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 1, Y: 1}, {X: 0, Y: 1}}
	for _, d := range []float64{0.37, 0.8, 1.13, 1.62} {
		exact := HalfPlaneArea(sq, 1, 1, d)
		_, _, ss16 := SuperSampleHalfPlane(0, 0, 1, 1, 1, 1, d, 16)
		if math.Abs(exact-ss16) > 0.03 {
			t.Fatalf("d=%v exact=%.5f ss16=%.5f 偏差过大", d, exact, ss16)
		}
		_, _, ss64 := SuperSampleHalfPlane(0, 0, 1, 1, 1, 1, d, 64)
		if math.Abs(exact-ss64) > 0.005 {
			t.Fatalf("d=%v exact=%.5f ss64=%.5f 偏差过大", d, exact, ss64)
		}
		if math.Abs(exact-ss64) >= math.Abs(exact-ss16) {
			t.Fatalf("d=%v SS64 未比 SS16 更接近精确值", d)
		}
	}
}

// 三档逆方差权重为精确的 9:8:9 / 13 体系，加权误差为已知闭式值。
func TestWeightedErrorsExact(t *testing.T) {
	alpha := []float64{1.0 / 3, 1.0 / 2, 2.0 / 3}
	beta := []float64{5.0 / 16, 1.0 / 2, 11.0 / 16}
	w := BinomialWeights(alpha)
	wantW := []float64{9.0 / 26, 8.0 / 26, 9.0 / 26}
	for i := range w {
		almost(t, "权重", w[i], wantW[i], 1e-15)
	}
	almost(t, "加权 MAE", WeightedMAE(alpha, beta, w), 3.0/208, 1e-15)
	almost(t, "加权 RMSE", WeightedRMSE(alpha, beta, w), 1.0/math.Sqrt(3328), 1e-15)
}
