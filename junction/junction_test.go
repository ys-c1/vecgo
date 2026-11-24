package junction

import (
	"math"
	"testing"
)

func approx(t *testing.T, name string, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Fatalf("%s = %.12g, 想望 %.12g (容差 %.2e)", name, got, want, tol)
	}
}

// 三类标准接点的扇形面积（r=1）与权重必须是已知精确分数。
func TestThreeClassSectorAreasAndWeights(t *testing.T) {
	want := map[string]struct {
		deg [3]float64
		w   [3]float64
		a   [3]float64
	}{
		"eq-y": {
			deg: [3]float64{120, 120, 120},
			w:   [3]float64{1.0 / 3, 1.0 / 3, 1.0 / 3},
			a:   [3]float64{math.Pi / 3, math.Pi / 3, math.Pi / 3},
		},
		"t": {
			deg: [3]float64{90, 90, 180},
			w:   [3]float64{0.25, 0.25, 0.5},
			a:   [3]float64{math.Pi / 4, math.Pi / 4, math.Pi / 2},
		},
		"acute": {
			deg: [3]float64{60, 90, 210},
			w:   [3]float64{1.0 / 6, 0.25, 7.0 / 12},
			a:   [3]float64{math.Pi / 6, math.Pi / 4, 7 * math.Pi / 12},
		},
	}
	for _, j := range Classes() {
		ew := want[j.ID]
		for i := 0; i < 3; i++ {
			approx(t, j.ID+" 角度(deg)", j.Degrees()[i], ew.deg[i], 1e-9)
		}
		ar, err := SectorAreas(j.Angles, 1)
		if err != nil {
			t.Fatal(err)
		}
		w, err := ConservationWeights(j.Angles)
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 3; i++ {
			approx(t, j.ID+" 面积", ar[i], ew.a[i], 1e-12)
			approx(t, j.ID+" 权重", w[i], ew.w[i], 1e-15)
		}
		// 面积入口与角度入口反解出的权重必须一致。
		w2, err := WeightsFromAreas(ar)
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 3; i++ {
			approx(t, j.ID+" 面积反解权重", w2[i], w[i], 1e-15)
		}
	}
}

// 权重之和恒为 1：在每一个接点上单独核对，而不是只看总体平均。
func TestWeightSumOneAtEveryJunction(t *testing.T) {
	all := append(append([]Junction{}, Classes()...), ExtraJunctions()...)
	all = append(all, SpotCheckJunctions()...)
	for _, j := range all {
		j := j
		t.Run(j.ID, func(t *testing.T) {
			res, err := Audit(j, DefaultOptions())
			if err != nil {
				t.Fatalf("接点 %s 核验失败: %v", j.ID, err)
			}
			// 单独核对这一个接点。
			d, err := CheckWeightSum(res.Weights)
			if err != nil {
				t.Fatalf("接点 %s 权重和不守恒: %v (sum=%.15f)", j.ID, err, res.WeightSum)
			}
			if !res.WeightSumHolds {
				t.Fatalf("接点 %s WeightSumHolds=false", j.ID)
			}
			t.Logf("接点 %-12s θ(deg)=%v  w=[%.9f %.9f %.9f]  Σw=%.15f  |Σw-1|=%.2e",
				j.ID, j.Degrees(), res.Weights[0], res.Weights[1], res.Weights[2], res.WeightSum, d)

			// 面积和必须等于 πr²。
			approx(t, "面积和", SumAreas(res.Areas), TotalArea(res.Radius), 1e-12)
			if res.AreaSumError > 1e-12 {
				t.Fatalf("接点 %s 面积和误差过大: %e", j.ID, res.AreaSumError)
			}
		})
	}
}

// 反例数据：统一除以 3 在非对称接点必然留下可量化缺口。
func TestEqualThirdsLeavesGapCounterexample(t *testing.T) {
	byID := map[string]Junction{}
	for _, j := range Classes() {
		byID[j.ID] = j
	}

	// T 接 90/90/180：大扇区真实权重 1/2，却只分到 1/3。
	g, err := EqualThirdsGap(byID["t"], 1, 255)
	if err != nil {
		t.Fatal(err)
	}
	approx(t, "T 大扇区面积缺口", g.MaxDeficitArea, math.Pi/6, 1e-12) // π/2 - π/3 = π/6
	approx(t, "T 大扇区欠亮阶数", g.WorstDarkLevels, 85.0, 1e-9)      // 255*(2/3-1) = -85
	approx(t, "T 小扇区过亮阶数", g.WorstBrightLevels, 85.0, 1e-9)

	// 锐角偏置 60/90/210：最大缺口 π/4，欠亮约 109.29 阶；60° 小扇区被抬到 +255。
	ga, err := EqualThirdsGap(byID["acute"], 1, 255)
	if err != nil {
		t.Fatal(err)
	}
	approx(t, "acute 最大面积缺口", ga.MaxDeficitArea, math.Pi/4, 1e-12) // 7π/12 - π/3 = π/4
	approx(t, "acute 最大欠亮", ga.WorstDarkLevels, 255*(1-1.0/3/(7.0/12)), 1e-9)
	if ga.WorstDarkLevels <= 109 || ga.WorstDarkLevels >= 110 {
		t.Fatalf("acute 欠亮应约 109.286，实际 %.3f", ga.WorstDarkLevels)
	}
	approx(t, "acute 最小扇区过亮", ga.BrightnessDelta[0], 255.0, 1e-9) // 1/3 / (1/6) - 1 = 1

	// 字面“每个扇区各自除以 3”：总覆盖只剩 1/3，缺口恒为总面积的 2/3。
	for _, j := range Classes() {
		g, err := EqualThirdsGap(j, 1, 200)
		if err != nil {
			t.Fatal(err)
		}
		approx(t, j.ID+" 字面缺口比例", g.LiteralGapFraction, 2.0/3.0, 1e-15)
		approx(t, j.ID+" 字面缺口面积", g.LiteralGapArea, 2*math.Pi/3, 1e-12)
		approx(t, j.ID+" 字面欠亮阶数", g.LiteralDarkLevels, 200*2.0/3.0, 1e-9)
	}

	// 对照组：等角 Y 上 1/3 恰好等于守恒权重，错配为 0——
	// 这解释了“为什么除以 3 看起来有时能用”，也使反例不是伪造的。
	ge, err := EqualThirdsGap(byID["eq-y"], 1, 255)
	if err != nil {
		t.Fatal(err)
	}
	if ge.MaxDeficitArea != 0 || ge.WorstDarkLevels != 0 || ge.WorstBrightLevels != 0 {
		t.Fatalf("等角接点上除以 3 应完全无错配: %+v", ge)
	}
}

// 修正后环上每个像素亮度必须与周围连续（8-bit 下 0 偏差），
// 而朴素相加明显过亮、除以三明显欠亮。
func TestBrightnessContinuity(t *testing.T) {
	for _, j := range Classes() {
		au, err := AuditRing(j, 360, 200)
		if err != nil {
			t.Fatal(err)
		}
		if au.CorrectedMaxDelta != 0 {
			t.Fatalf("%s 修正后仍有 %d 阶亮度偏差", j.ID, au.CorrectedMaxDelta)
		}
		if au.LedgerMaxFloatDelta > 1e-12 {
			t.Fatalf("%s 台账浮点偏差过大: %e", j.ID, au.LedgerMaxFloatDelta)
		}
		if au.NaiveMaxOvershoot != 55 { // min(255,600)=255 vs 200
			t.Fatalf("%s 朴素相加过冲应为 55 阶，实际 %d", j.ID, au.NaiveMaxOvershoot)
		}
		if au.ThirdsLiteralMaxUndershoot != 133 { // 200 - round(66.67)=133
			t.Fatalf("%s 除以三欠亮应为 133 阶，实际 %d", j.ID, au.ThirdsLiteralMaxUndershoot)
		}
		// 每个环像素都要满足修正灰度==周围灰度（逐像素，不是只看汇总）。
		for _, p := range au.Pixels {
			if p.CorrectedGray != p.SurroundGray {
				t.Fatalf("%s 环像素 %d 不连续: %d vs %d", j.ID, p.Index, p.CorrectedGray, p.SurroundGray)
			}
		}
	}
}

// 独立的 geom.Clip 面积路径：每个接点中心像素上三扇区面积之和必须为 1。
func TestPixelClippingAreaConservation(t *testing.T) {
	for _, j := range append(Classes(), ExtraJunctions()...) {
		a, err := PixelSectorAreas(j, 0, 0, 64)
		if err != nil {
			t.Fatal(err)
		}
		s := a[0] + a[1] + a[2]
		if math.Abs(s-1) > 1e-9 {
			t.Fatalf("%s 中心像素面积不守恒: sum=%.12f", j.ID, s)
		}
		// 周围若干像素同样守恒（平面被三扇区铺满）。
		for _, p := range [][2]float64{{2, 3}, {-4, 1}, {5, -5}, {0, 8}} {
			_, d, err := ConservationAtPixel(j, p[0], p[1])
			if err != nil {
				t.Fatal(err)
			}
			if d > 1e-9 {
				t.Fatalf("%s 像素 %v 守恒残差 %e", j.ID, p, d)
			}
		}
	}
}

func TestAngleValidation(t *testing.T) {
	bad := [][3]float64{
		{0, math.Pi, math.Pi},    // 第 0 个为 0
		{1, 1, 1},                // 和不为 2π
		{-1, 2, Tau - 1},         // 负角
		{math.NaN(), 1, Tau - 1}, // NaN
	}
	for i, b := range bad {
		if err := ValidateAngles(Angles(b)); err == nil {
			t.Fatalf("坏角度用例 %d 应被拒绝: %v", i, b)
		}
		if _, err := ConservationWeights(Angles(b)); err == nil {
			t.Fatalf("坏角度用例 %d 权重应报错", i)
		}
	}
	if _, err := SectorAreas(MustAnglesDeg([3]float64{120, 120, 120}), 0); err == nil {
		t.Fatal("r=0 应报错")
	}
}

func TestSectorAnglesFromRays(t *testing.T) {
	// 三条射线 0°,120°,240°（任意顺序给入）应还原为三个 120°。
	a, err := SectorAngles([3]float64{0, 4 * math.Pi / 3, 2 * math.Pi / 3})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		approx(t, "等角还原", a[i], 2*math.Pi/3, 1e-12)
	}
	// 重合射线必须被拒绝并指出下标。
	if _, err := SectorAngles([3]float64{0, 0, math.Pi}); err == nil {
		t.Fatal("重合射线应报错")
	}
}

// 端到端像素事实：渲染进 raster.Image 后，面积权重修正的接点环必须与周围
// 背景逐像素同灰度；朴素相加必须偏亮，字面除以三必须偏暗。
func TestRenderedRingPixelContinuity(t *testing.T) {
	const n, ink = 121, 200.0
	for _, j := range Classes() {
		corr, err := RenderRingTile(j, n, ModeWeighted, ink)
		if err != nil {
			t.Fatal(err)
		}
		npx, maxD, mean := RingContinuity(corr, j, ink, n)
		if npx == 0 {
			t.Fatalf("%s 环带没有采到像素", j.ID)
		}
		if maxD != 0 || mean != 0 {
			t.Fatalf("%s 修正后接点环不连续: maxΔ=%d meanΔ=%.3f", j.ID, maxD, mean)
		}

		bug, _ := RenderRingTile(j, n, ModeNaiveAdd, ink)
		_, bugMax, bugMean := RingContinuity(bug, j, ink, n)
		if bugMean <= 50 || bugMax != 55 { // 255 vs 200
			t.Fatalf("%s 朴素相加应明显亮一圈: max=%d mean=%+.1f", j.ID, bugMax, bugMean)
		}

		thirds, _ := RenderRingTile(j, n, ModeThirdsLiteral, ink)
		_, _, thirdMean := RingContinuity(thirds, j, ink, n)
		if thirdMean >= -130 { // 67 vs 200 => -133
			t.Fatalf("%s 除以三应留下暗缺口: mean=%+.1f", j.ID, thirdMean)
		}
	}

	// 非对称接点上，统一 1/3 权重必须在宽扇区产生可测的欠亮像素。
	tJ := Classes()[1]
	uni, _ := RenderRingTile(tJ, n, ModeThirdsUniform, ink)
	sawDark := false
	for _, v := range uni.Pix {
		if int(v) < 200-60 {
			sawDark = true
		}
	}
	if !sawDark {
		t.Fatal("T 接统一 1/3 权重应出现宽扇区暗像素（缺口反例）")
	}
}

// 合成算子：等索赔时面积权重结果必须等于索赔本身（不放大、不除以 3）。
func TestBlendClaimsPartitionOfUnity(t *testing.T) {
	for _, j := range append(Classes(), ExtraJunctions()...) {
		w, err := ConservationWeights(j.Angles)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range []float64{0.25, 0.5, 1.0} {
			got, err := BlendClaims([3]float64{c, c, c}, w)
			if err != nil {
				t.Fatal(err)
			}
			if math.Abs(got-c) > 1e-15 {
				t.Fatalf("%s BlendClaims(%.2f) = %.15f, 应等于索赔本身", j.ID, c, got)
			}
		}
		// 朴素相加在全索赔时为 1，但三份 0.5 索赔会被加到 1.5→截断 1，
		// 而守恒合成仍是 0.5——这就是亮圈的数值来源。
		if got := NaiveAdd([3]float64{0.5, 0.5, 0.5}); got != 1 {
			t.Fatalf("朴素相加应截断为 1，实际 %.2f", got)
		}
	}
	// 权重和不为 1 必须拒绝。
	if _, err := BlendClaims([3]float64{1, 1, 1}, Weights{0.5, 0.5, 0.5}); err == nil {
		t.Fatal("权重和 1.5 应被 CheckWeightSum 拒绝")
	}
	if _, err := BlendClaims([3]float64{2, 1, 1}, Weights{1.0 / 3, 1.0 / 3, 1.0 / 3}); err == nil {
		t.Fatal("覆盖率越界应报错")
	}
}
