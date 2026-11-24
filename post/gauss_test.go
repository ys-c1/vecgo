package post

import (
	"math"
	"testing"

	"vecgo/raster"
)

// 黄金值来自独立 Python 预演算（kernel 卷积 + erfc 尾部界），
// Go 实现必须复算到 1e-4 以内。
func TestInvertReproducesGoldenTable(t *testing.T) {
	cases := []struct {
		sigma       float64
		widths      [3]int
		wIdeal      float64
		variance    float64
		boundGolden float64
		l1Golden    float64
		tailGolden  float64
	}{
		{2.0, [3]int{5, 3, 3}, 4.1231, 3.3333, 17.091, 0.065059, 6.8e-6},
		{5.0, [3]int{11, 9, 9}, 10.0499, 23.3333, 10.958, 0.041013, 6.7e-8},
	}
	for _, c := range cases {
		plan, err := Invert(c.sigma, 0, 8)
		if err != nil {
			t.Fatal(err)
		}
		if plan.N != 3 {
			t.Fatalf("sigma=%v 应固定三级，得到 %d", c.sigma, plan.N)
		}
		for i, w := range c.widths {
			if plan.Widths[i] != w {
				t.Fatalf("sigma=%v 第 %d 级箱宽 = %d，期望 %d，全表 %v",
					c.sigma, i, plan.Widths[i], w, plan.Widths)
			}
		}
		if math.Abs(plan.WIdeal-c.wIdeal) > 5e-4 {
			t.Fatalf("sigma=%v wIdeal=%v，期望 %v", c.sigma, plan.WIdeal, c.wIdeal)
		}
		if math.Abs(plan.Variance-c.variance) > 5e-4 {
			t.Fatalf("sigma=%v variance=%v，期望 %v", c.sigma, plan.Variance, c.variance)
		}
		if math.Abs(plan.L1Dist-c.l1Golden) > 2e-5 {
			t.Fatalf("sigma=%v L1=%v，期望 %v", c.sigma, plan.L1Dist, c.l1Golden)
		}
		if math.Abs(plan.Tail-c.tailGolden) > c.tailGolden {
			t.Fatalf("sigma=%v tail=%v，期望量级 %v", c.sigma, plan.Tail, c.tailGolden)
		}
		if math.Abs(plan.Bound255-c.boundGolden) > 0.02 {
			t.Fatalf("sigma=%v Bound255=%v，期望 %v", c.sigma, plan.Bound255, c.boundGolden)
		}
	}
}

// 5% 预算（12.75/255）：σ=5 三级即够，σ=2 三级不够、需六级。
func TestInvertBudgetSaysWhenThreeStagesInsufficient(t *testing.T) {
	budget := 0.05 * 255
	plan2, _ := Invert(2.0, budget, 8)
	if plan2.NStar != 6 {
		t.Fatalf("sigma=2 在 5%% 预算下 NStar=%d，期望 6（三级不够）", plan2.NStar)
	}
	if plan2.Bound255 <= budget {
		t.Fatal("sigma=2 三级上界应超过 5% 预算")
	}
	plan5, _ := Invert(5.0, budget, 8)
	if plan5.NStar != 3 {
		t.Fatalf("sigma=5 在 5%% 预算下 NStar=%d，期望 3", plan5.NStar)
	}
	if plan5.Bound255 > budget {
		t.Fatal("sigma=5 三级上界应满足 5% 预算")
	}
}

// 反解所需级数随预算单调：预算放宽，NStar 不应变大。
func TestInvertMonotoneBudget(t *testing.T) {
	prev := 99
	// sigma=2 在 n<=8 内最好上界为 3.44%（n=6），故最低预算取 3.5%
	for _, eps := range []float64{0.035, 0.05, 0.075, 0.10} {
		plan, _ := Invert(2.0, eps*255, 8)
		if plan.NStar < 0 {
			t.Fatalf("eps=%v maxN 内无解", eps)
		}
		if plan.NStar > prev {
			t.Fatalf("eps=%v NStar=%d 比更紧预算的 %d 还大", eps, plan.NStar, prev)
		}
		prev = plan.NStar
	}
}

// 冲激图像：输出差即核差，逐像素上界必须处处成立。
func TestPointwiseBoundImpulse(t *testing.T) {
	for _, tc := range []struct {
		sigma  float64
		widths [3]int
	}{
		{2.0, [3]int{5, 3, 3}},
		{5.0, [3]int{11, 9, 9}},
	} {
		im, _ := raster.New(48, 48)
		im.Pix[24*48+24] = 255
		plan, _ := Invert(tc.sigma, 0, 8)
		var stages [3][]float64
		out, err := GaussBlur3(im, tc.widths, &stages)
		if err != nil {
			t.Fatal(err)
		}
		ref := GaussReference(im, tc.sigma, plan.TruncK)
		maxD, mean := MaxAbsDiff(out, ref)
		if float64(maxD) > plan.Bound255 {
			t.Fatalf("sigma=%v 实测最大差 %d 超过上界 %.3f", tc.sigma, maxD, plan.Bound255)
		}
		if maxD == 0 {
			t.Fatalf("sigma=%v 冲激响应理应存在非零差", tc.sigma)
		}
		t.Logf("sigma=%v 上界=%.3f 实测max=%d mean=%.4f", tc.sigma, plan.Bound255, maxD, mean)
		// 每级输出各存一份
		for s, st := range stages {
			if len(st) != len(im.Pix) {
				t.Fatalf("第 %d 级未存输出", s)
			}
		}
		// 第三级浮点输出取整即最终图
		for i, v := range stages[2] {
			if roundUint8(v) != out.Pix[i] {
				t.Fatal("第三级输出与最终图不一致")
			}
		}
	}
}

// 含方块与渐变的自然图：逐像素上界同样必须成立。
func TestPointwiseBoundStructuredImage(t *testing.T) {
	const n = 96
	im, _ := raster.New(n, n)
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			v := uint8((x * 2) % 256)
			if x >= 20 && x < 70 && y >= 30 && y < 60 {
				v = 240
			}
			if (x/8)%2 == 0 && y < 12 {
				v = 255 - v
			}
			im.Pix[y*n+x] = v
		}
	}
	for _, tc := range []struct {
		sigma  float64
		widths [3]int
	}{
		{2.0, [3]int{5, 3, 3}},
		{5.0, [3]int{11, 9, 9}},
	} {
		plan, _ := Invert(tc.sigma, 0, 8)
		out, err := GaussBlur3(im, tc.widths, nil)
		if err != nil {
			t.Fatal(err)
		}
		ref := GaussReference(im, tc.sigma, plan.TruncK)
		maxD, mean := MaxAbsDiff(out, ref)
		if float64(maxD) > plan.Bound255 {
			t.Fatalf("sigma=%v 实测最大差 %d 超过上界 %.3f", tc.sigma, maxD, plan.Bound255)
		}
		t.Logf("结构化图 sigma=%v 上界=%.3f 实测max=%d mean=%.4f",
			tc.sigma, plan.Bound255, maxD, mean)
	}
}

// 分离与不分离的三级串联应给出相同结果（浮点求和次序不同，允许 1 个量化级）。
func TestSeparableMatchesNonSeparable(t *testing.T) {
	im, _ := raster.New(40, 32)
	for i := range im.Pix {
		im.Pix[i] = uint8((i * 37) % 256)
	}
	widths := [3]int{11, 9, 9}
	a, _ := GaussBlur3(im, widths, nil)
	b, _ := GaussBlur2DNonSeparable(im, widths)
	maxD, _ := MaxAbsDiff(a, b)
	if maxD > 1 {
		t.Fatalf("分离与不分离结果差 %d 个量化级，期望 <=1", maxD)
	}
}

// sigma=2 三级超预算时，按反解 NStar=6 实际跑六级，误差必须落入预算。
func TestSixStagesMeetBudgetWhenThreeFail(t *testing.T) {
	budget := 0.05 * 255
	plan, _ := Invert(2.0, budget, 8)
	if plan.NStar != 6 {
		t.Fatalf("NStar=%d，期望 6", plan.NStar)
	}
	n6 := plan.NStar
	ws6, wi6, lo6, up6, _ := kovesiWidths(2.0, n6)
	p6 := &GaussPlan{Sigma: 2.0}
	fillPlan(p6, n6, ws6, wi6, lo6, up6)
	if p6.Bound255 > budget {
		t.Fatalf("六级上界 %.3f 仍超预算 %.2f", p6.Bound255, budget)
	}

	const n = 96
	im, _ := raster.New(n, n)
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			v := uint8((x * 2) % 256)
			if x >= 20 && x < 70 && y >= 30 && y < 60 {
				v = 240
			}
			im.Pix[y*n+x] = v
		}
	}
	cur := pixToFloat(im.Pix)
	for _, w := range ws6 {
		cur = boxPassH(cur, n, n, w)
		cur = boxPassV(cur, n, n, w)
	}
	out, _ := raster.New(n, n)
	for i, v := range cur {
		out.Pix[i] = roundUint8(v)
	}
	ref := GaussReference(im, 2.0, p6.TruncK)
	maxD, _ := MaxAbsDiff(out, ref)
	if float64(maxD) > budget {
		t.Fatalf("六级实测最大差 %d 超过预算 %.2f", maxD, budget)
	}
	if float64(maxD) > p6.Bound255 {
		t.Fatalf("六级实测最大差 %d 超过其上界 %.3f", maxD, p6.Bound255)
	}
	t.Logf("六级宽度=%v 上界=%.3f 实测max=%d 预算=%.2f", ws6, p6.Bound255, maxD, budget)
}

func TestRejectsEvenWidth(t *testing.T) {
	im, _ := raster.New(4, 4)
	if _, err := GaussBlur3(im, [3]int{4, 3, 3}, nil); err == nil {
		t.Fatal("偶数箱宽应报错")
	}
	if _, err := GaussBlur2DNonSeparable(im, [3]int{4, 3, 3}); err == nil {
		t.Fatal("偶数箱宽应报错")
	}
}

func BenchmarkSeparable3StageSigma2(b *testing.B) {
	im, _ := raster.New(128, 128)
	for i := range im.Pix {
		im.Pix[i] = uint8((i * 37) % 256)
	}
	widths := [3]int{5, 3, 3}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		GaussBlur3(im, widths, nil)
	}
}

func BenchmarkNonSeparable3StageSigma2(b *testing.B) {
	im, _ := raster.New(128, 128)
	for i := range im.Pix {
		im.Pix[i] = uint8((i * 37) % 256)
	}
	widths := [3]int{5, 3, 3}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		GaussBlur2DNonSeparable(im, widths)
	}
}

func BenchmarkSeparable3Stage(b *testing.B) {
	im, _ := raster.New(128, 128)
	for i := range im.Pix {
		im.Pix[i] = uint8((i * 37) % 256)
	}
	widths := [3]int{11, 9, 9}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		GaussBlur3(im, widths, nil)
	}
}

func BenchmarkNonSeparable3Stage(b *testing.B) {
	im, _ := raster.New(128, 128)
	for i := range im.Pix {
		im.Pix[i] = uint8((i * 37) % 256)
	}
	widths := [3]int{11, 9, 9}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		GaussBlur2DNonSeparable(im, widths)
	}
}
