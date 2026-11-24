package alloc

import (
	"math"
	"testing"
)

func approxEq(t *testing.T, name string, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Fatalf("%s = %v, 期望 %v (tol %v)", name, got, want, tol)
	}
}

func TestCoverageFractionAndVariance(t *testing.T) {
	// s=0.25, phi=0.2: p 序列 = .2,.45,.7,.95,.2,...
	rows, err := Coverage(0.25, 0.2, 4)
	if err != nil {
		t.Fatal(err)
	}
	wantP := []float64{0.2, 0.45, 0.7, 0.95}
	for i, r := range rows {
		approxEq(t, "p_i", r.P, wantP[i], 1e-12)
		approxEq(t, "v_i", r.V, wantP[i]*(1-wantP[i]), 1e-12)
	}
}

func TestAllocationsConserveBudgetAndFloor(t *testing.T) {
	rows, _ := Coverage(0.25, 0.2, 8)
	for _, st := range []Strategy{Optimal, Fixed} {
		out, err := Allocate(rows, 80, st)
		if err != nil {
			t.Fatalf("%s: %v", st, err)
		}
		if got := TotalSamples(out); got != 80 {
			t.Fatalf("%s 采样总数 = %d, 期望 80", st, got)
		}
		for _, r := range out {
			if r.V > 0 && r.N < 1 {
				t.Fatalf("%s 正方差行 %d 采样数 %d < 1", st, r.Index, r.N)
			}
		}
	}
}

func TestOptimalBeatsOrMatchesUniform(t *testing.T) {
	cases := []struct {
		s   float64
		phi float64
	}{
		{0.25, 0.2},
		{0.5, 0.2},
		{1.0, 0.2},
	}
	for _, c := range cases {
		rows, _ := Coverage(c.s, c.phi, 8)
		opt, _ := Allocate(rows, 80, Optimal)
		uni, _ := Allocate(rows, 80, Fixed)
		if TotalVariance(opt) > TotalVariance(uni)+1e-15 {
			t.Fatalf("s=%v 最优方差 %v 大于均分 %v", c.s, TotalVariance(opt), TotalVariance(uni))
		}
		// 闭式连续最优 V* 是整数实现的下界。
		star := OptimalVariance(rows, 80)
		if star > TotalVariance(opt)+1e-12 {
			t.Fatalf("s=%v 连续下界 %v 高于整数实现 %v", c.s, star, TotalVariance(opt))
		}
	}
}

func TestClosedFormMatchesLagrangeShare(t *testing.T) {
	rows, _ := Coverage(0.25, 0.2, 8)
	share := ContinuousShare(rows, 80)
	var sumSqrt, tot float64
	for _, r := range rows {
		sumSqrt += math.Sqrt(r.V)
	}
	for i, r := range rows {
		want := 80 * math.Sqrt(r.V) / sumSqrt
		approxEq(t, "n_i*", share[i], want, 1e-12)
		tot += share[i]
	}
	approxEq(t, "Σn_i*", tot, 80, 1e-10)
}

func TestExactRowsCostNoBudget(t *testing.T) {
	// phi=0、s=1 时 p_i=0，全部精确行，最优路径不消耗预算。
	rows, _ := Coverage(1.0, 0.0, 8)
	out, err := Allocate(rows, 8, Optimal)
	if err != nil {
		t.Fatal(err)
	}
	if TotalSamples(out) != 0 {
		t.Fatalf("精确行应不采样, 实际 %d", TotalSamples(out))
	}
	if TotalVariance(out) != 0 {
		t.Fatalf("精确行总方差应为 0, 实际 %v", TotalVariance(out))
	}
}

func TestUniformVarianceFormula(t *testing.T) {
	rows, _ := Coverage(0.5, 0.2, 8)
	uni, _ := Allocate(rows, 80, Fixed)
	approxEq(t, "均分方差", TotalVariance(uni), UniformVariance(rows, 80), 1e-12)
}

func TestMonteCarloTracksAnalyticVariance(t *testing.T) {
	rows, _ := Coverage(0.25, 0.2, 8)
	opt, _ := Allocate(rows, 80, Optimal)
	uni, _ := Allocate(rows, 80, Fixed)
	const trials = 40000
	mseOpt := Simulate(opt, trials, 42)
	mseUni := Simulate(uni, trials, 42)
	// 经验 MSE 与解析方差相对误差应在 ~10% 内（trials 足够大）。
	rel := math.Abs(mseOpt-TotalVariance(opt)) / TotalVariance(opt)
	if rel > 0.1 {
		t.Fatalf("最优路径经验 MSE %v 偏离解析 %v 过远", mseOpt, TotalVariance(opt))
	}
	rel = math.Abs(mseUni-TotalVariance(uni)) / TotalVariance(uni)
	if rel > 0.1 {
		t.Fatalf("均分路径经验 MSE %v 偏离解析 %v 过远", mseUni, TotalVariance(uni))
	}
	// 经验上最优也不应显著差于均分（放宽 15% 容忍随机噪声）。
	if mseOpt > mseUni*1.15 {
		t.Fatalf("经验 MSE: 最优 %v > 均分 %v", mseOpt, mseUni)
	}
}

func TestRejectsBadInputs(t *testing.T) {
	if _, err := Coverage(0.25, 0.2, 0); err == nil {
		t.Fatal("行数 0 应报错")
	}
	rows, _ := Coverage(0.25, 0.2, 8)
	if _, err := Allocate(rows, 3, Fixed); err == nil {
		t.Fatal("预算小于行数时固定均分应报错")
	}
	if _, err := Allocate(rows, 0, Optimal); err == nil {
		t.Fatal("预算 0 应报错")
	}
}
