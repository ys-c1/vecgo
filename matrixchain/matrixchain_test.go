package matrixchain

import (
	"math"
	"strings"
	"testing"
)

func TestThreeChainsOptimalErrorIsSmallerAndBounded(t *testing.T) {
	chains := DefaultChains()
	if len(chains) != 3 {
		t.Fatalf("需要三组矩阵链，实际 %d", len(chains))
	}

	for _, ch := range chains {
		r := BuildReport(ch)
		if !strings.Contains(r.Order, "A1·(((A2·A3)·A4)·A5)") {
			t.Fatalf("%s 的最优顺序不符合预期: %s", ch.Name, r.Order)
		}
		if !(r.OptDev < r.LeftDev) {
			t.Fatalf("%s 最优顺序偏差 %.17g 未小于左结合偏差 %.17g", ch.Name, r.OptDev, r.LeftDev)
		}
		if r.OptDev > r.OptBound {
			t.Fatalf("%s 最优偏差 %.3e 超过上界 %.3e", ch.Name, r.OptDev, r.OptBound)
		}
		if r.LeftDev > r.LeftBound {
			t.Fatalf("%s 左结合偏差 %.3e 超过上界 %.3e", ch.Name, r.LeftDev, r.LeftBound)
		}
		if r.OptDev > r.OptCertBound || r.LeftDev > r.LeftCertBound {
			t.Fatalf("%s 偏差超过递归 Wilkinson 认证上界", ch.Name)
		}
		if r.OptCertBound > r.OptBound*1.000001 || r.LeftCertBound > r.LeftBound*1.000001 {
			t.Fatalf("%s 递归认证上界与加性条件数上界不一致", ch.Name)
		}
		if r.Elements.MaxDiff == 0 {
			t.Fatalf("%s 两种顺序完全相同，无法比较结合顺序误差", ch.Name)
		}
	}
}

func TestElementwiseMaxDifference(t *testing.T) {
	r := BuildReport(DefaultChains()[2])
	const want = 2.8421709430404007e-14
	if math.Abs(r.Elements.MaxDiff-want) > 1e-30 {
		t.Fatalf("最大逐元素顺序差 = %.17g, want %.17g", r.Elements.MaxDiff, want)
	}
}
