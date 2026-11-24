package composite

import (
	"math"
	"testing"
)

// scenario 是两份常用层栈：等 α=0.3 的五层与十层，颜色微有起伏以避开
// 恰好整除的相位。用户场景是“十层半透明叠合”，十层栈就是它。
func scenario(n int) []Layer {
	ls := make([]Layer, n)
	for i := range ls {
		ls[i] = Layer{Color: 0.40 + 0.02*float64(i%3), Alpha: 0.30}
	}
	return ls
}

func alphasOf(ls []Layer) []float64 {
	a := make([]float64, len(ls))
	for i := range ls {
		a[i] = ls[i].Alpha
	}
	return a
}

// 累积式：e_N = Σ w_i·δ_i。实跑逐层叠合得到的偏差必须与闭式求和一致。
func TestAccumulationIdentity(t *testing.T) {
	for _, n := range []int{5, 10} {
		ls := scenario(n)
		sch, err := Plan(alphasOf(ls), Budget)
		if err != nil {
			t.Fatal(err)
		}
		res, err := OverFixed(ls, sch)
		if err != nil {
			t.Fatal(err)
		}
		w := Weights(alphasOf(ls))
		closed := 0.0
		for i := range w {
			closed += w[i] * res.Deltas[i]
		}
		if math.Abs(closed-res.Deviation) > 1e-9 {
			t.Fatalf("n=%d 累积式不符: 闭式 %g vs 实测 %g", n, closed, res.Deviation)
		}
	}
}

// 反解分配：每层取满时 Σ w_i·ε_i* 恰为一档，且保证界不超过一档。
func TestAllocTightAndUnderBudget(t *testing.T) {
	for _, n := range []int{5, 10} {
		ls := scenario(n)
		a := alphasOf(ls)
		eps, err := Allowed(a, Budget)
		if err != nil {
			t.Fatal(err)
		}
		w := Weights(a)
		total := 0.0
		for i := range w {
			total += w[i] * eps[i]
		}
		if math.Abs(total-Budget) > 1e-9 {
			t.Fatalf("n=%d 反解未贴满预算: %.12f", n, total)
		}
		sch, err := Plan(a, Budget)
		if err != nil {
			t.Fatal(err)
		}
		b, err := Bound(a, sch)
		if err != nil {
			t.Fatal(err)
		}
		if b > Budget*(1+1e-12) {
			t.Fatalf("n=%d 兑现后保证界超一档: %g", n, b)
		}
	}
}

// 兑现的格式误差不超过分给它的额度，否则分配表自相矛盾。
func TestRealizeHonorsAllocation(t *testing.T) {
	ls := scenario(10)
	eps, err := Allowed(alphasOf(ls), Budget)
	if err != nil {
		t.Fatal(err)
	}
	for i, e := range eps {
		s, err := Realize(e)
		if err != nil {
			t.Fatalf("第 %d 层: %v", i+1, err)
		}
		if s.Err() > e*(1+1e-12) {
			t.Fatalf("第 %d 层格式 %v 误差 %g > 额度 %g", i+1, s, s.Err(), e)
		}
	}
}

// 十层叠加后实际偏差小于一档——这是用户要求的可观测结论。
func TestTenLayerDeviationBelowOneLevel(t *testing.T) {
	ls := scenario(10)
	sch := make([]Scheme, len(ls))
	for i := range sch {
		sch[i] = Scheme{Mode: Truncate, Guard: 0} // 朴素 8 位截断
	}
	trunc, err := OverFixed(ls, sch)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(trunc.Deviation) <= 1.0 {
		t.Fatalf("前提不成立：朴素截断本应超一档，实际 %g", trunc.Deviation)
	}
	plan, err := Plan(alphasOf(ls), Budget)
	if err != nil {
		t.Fatal(err)
	}
	got, err := OverFixed(ls, plan)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(got.Deviation) >= 1.0 {
		t.Fatalf("反解后偏差仍不在一档内: %g", got.Deviation)
	}
}

// 等分误差方案在同样层数下更差：它的保证界严格大于分配方案。
func TestEqualSplitWorse(t *testing.T) {
	for _, n := range []int{5, 10} {
		ls := scenario(n)
		alloc, err := BuildAlloc(ls, Budget)
		if err != nil {
			t.Fatal(err)
		}
		eq, err := BuildEqual(ls, Scheme{Mode: Truncate, Guard: 0})
		if err != nil {
			t.Fatal(err)
		}
		if !(eq.Bound > alloc.Bound) {
			t.Fatalf("n=%d 等分界 %.4f 未大于分配界 %.4f", n, eq.Bound, alloc.Bound)
		}
		if eq.Bound-alloc.Bound <= 0 {
			t.Fatalf("n=%d 差值应为正数", n)
		}
	}
}

// 最后一行的总误差要与界核对：表里的 Bound 必须不大于一档，且等于
// 各层 w_i·ε_i 的合计。
func TestTableTotalMatchesBound(t *testing.T) {
	ls := scenario(10)
	tb, err := BuildAlloc(ls, Budget)
	if err != nil {
		t.Fatal(err)
	}
	sum := 0.0
	for _, r := range tb.Rows {
		sum += r.WErr
	}
	if math.Abs(sum-tb.Bound) > 1e-12 {
		t.Fatalf("合计行 %g 与逐层 w·ε 之和 %g 不符", tb.Bound, sum)
	}
	if tb.Bound > Budget*(1+1e-12) {
		t.Fatalf("合计行超一档: %g", tb.Bound)
	}
	// 合计偏差也应等于逐层 w·δ 之和。
	dsum := 0.0
	for _, r := range tb.Rows {
		dsum += r.WDelta
	}
	if math.Abs(dsum-tb.Deviation) > 1e-9 {
		t.Fatalf("合计偏差 %g 与逐层之和 %g 不符", tb.Deviation, dsum)
	}
}

// 边界：最顶层权重必为 1，误差直接落到终值。
func TestTopWeightIsOne(t *testing.T) {
	for _, n := range []int{1, 5, 10} {
		w := Weights(make([]float64, n))
		if len(w) == 0 {
			w = Weights([]float64{0})
		}
		if w[len(w)-1] != 1 {
			t.Fatalf("n=%d 顶层权重应为 1，实际 %g", n, w[len(w)-1])
		}
	}
}

// 全遮挡的层（w=0）不参与终值：上界为 +∞，格式取最粗。
func TestFullyOccludedLayer(t *testing.T) {
	a := []float64{0.5, 0.5, 1.0}
	eps, err := Allowed(a, Budget)
	if err != nil {
		t.Fatal(err)
	}
	if !math.IsInf(eps[1], 1) { // 第 2 层被顶层 α=1 完全遮住
		t.Fatalf("第 2 层应为 +∞：%g", eps[1])
	}
	sch, err := Plan(a, Budget)
	if err != nil {
		t.Fatal(err)
	}
	if sch[1] != (Scheme{Mode: Truncate, Guard: 0}) {
		t.Fatalf("全遮挡层应用最粗格式: %v", sch[1])
	}
}

func TestRejectsBadInput(t *testing.T) {
	if _, err := Allowed(nil, Budget); err != ErrNoLayers {
		t.Fatalf("空层应报 ErrNoLayers: %v", err)
	}
	if _, err := Allowed([]float64{1.5}, Budget); err == nil {
		t.Fatal("α 越界应报错")
	}
	if _, err := Allowed([]float64{0.5}, 0); err == nil {
		t.Fatal("零预算应报错")
	}
	if _, err := Realize(1e-12); err == nil {
		t.Fatal("过紧预算应报错")
	}
	if _, err := BuildEqual(scenario(5), Scheme{Mode: Nearest, Guard: -1}); err == nil {
		t.Fatal("负保护位应报错")
	}
	if _, err := OverFixed(scenario(3), []Scheme{{Mode: Truncate, Guard: 0}}); err == nil {
		t.Fatal("层数与方案数不符应报错")
	}
	ls := scenario(2)
	if _, err := OverFixed(ls, []Scheme{{Mode: Truncate, Guard: 0}, {Mode: Mode(9), Guard: 0}}); err == nil {
		t.Fatal("非法舍入方式应报错")
	}
}
