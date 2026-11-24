package lab

import (
	"math"
	"testing"
)

func TestThreeTiersAnalyticMatchesExhaustive(t *testing.T) {
	r, err := Run(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Targets) != 3 {
		t.Fatalf("必须恰好三档目标, got %d", len(r.Targets))
	}
	t.Log("\n" + FormatTables(r))

	// 衰减率应接近块均值渐变的理论值 1/2，且拟合优度高。
	if !(r.Model.Q > 0.47 && r.Model.Q < 0.53) {
		t.Fatalf("q=%.4f 不在预期 0.5 附近", r.Model.Q)
	}
	if r.Model.RSquared < 0.99 {
		t.Fatalf("逐层拟合 R²=%.5f 过低", r.Model.RSquared)
	}

	wantN := []int{3, 5, 7}
	for i, tt := range r.Targets {
		if !tt.SolutionsMatch {
			t.Errorf("第 %d 档 ε=%.2f 解析 n=%d 与穷举 n=%d 不一致",
				i+1, tt.Epsilon, tt.AnalyticN, tt.ExhaustiveN)
		}
		if tt.AnalyticN != wantN[i] {
			t.Errorf("第 %d 档层数=%d, 期望 %d", i+1, tt.AnalyticN, wantN[i])
		}
		if math.Abs(tt.RelativeErrorPct) > 10 {
			t.Errorf("第 %d 档实测 %.3f 相对预测 %.3f 偏差 %.1f%% 超过 10%%",
				i+1, tt.MeasuredRMSE, tt.PredictedRMSE, tt.RelativeErrorPct)
		}
		// 解出的层必须真的满足目标（用实测说话）。
		if tt.MeasuredRMSE > tt.Epsilon+1e-9 {
			t.Errorf("第 %d 档 n=%d 实测 %.3f 未达目标 ε=%.3f",
				i+1, tt.AnalyticN, tt.MeasuredRMSE, tt.Epsilon)
		}
		// 前一层必须不满足，佐证“最小”。
		prev := r.Sweep[tt.AnalyticN-2] // N=n-1 -> index n-2
		if prev.Measured <= tt.Epsilon {
			t.Errorf("第 %d 档前一层 n=%d 实测 %.3f 已达标，n=%d 不是最小层",
				i+1, prev.N, prev.Measured, tt.AnalyticN)
		}
	}
}

func TestSweepCoversAllLevels(t *testing.T) {
	r, err := Run(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Sweep) != r.Depth {
		t.Fatalf("穷举点数 %d != 层数 %d", len(r.Sweep), r.Depth)
	}
	if got := r.Sweep[r.Depth-1].Measured; got != 0 {
		t.Fatalf("最精层误差应为 0, got %g", got)
	}
}
