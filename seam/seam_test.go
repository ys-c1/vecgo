package seam

import (
	"math"
	"testing"
)

func testBand() Band {
	// 24×16 画布，覆盖带为第 4..12 行（高 8），横贯全部 24 列。
	return Band{W: 24, H: 16, Y0: 4, Y1: 12}
}

// TestInclusionExclusionIdentity: 对所有缝索引、所有端点规则，
// 差值必须严格等于“重复覆盖数−漏空覆盖数”。
func TestInclusionExclusionIdentity(t *testing.T) {
	b := testBand()
	for k := 1; k < b.W; k++ {
		for _, r := range AllRules() {
			le := ReconcileRule(b, "", float64(k), k, r)
			if le.IdentityResidual != 0 {
				t.Fatalf("k=%d rule=%s 恒等式残差=%d (Δ=%d dup=%d gap=%d)",
					k, r, le.IdentityResidual, le.Delta, le.DupCovered, le.GapCovered)
			}
		}
	}
}

// TestHalfOpenZeroDupAcrossClasses: 三类裁剪位置逐一验证，
// 半开半闭规则下重复像素数与漏空数都必须为 0，和必须严格等于整块。
// 任何一类不为 0，都说明端点没有真正落到半开取舍上。
func TestHalfOpenZeroDupAcrossClasses(t *testing.T) {
	b := testBand()
	seams := map[Class][]float64{
		ClassInteger: {1, 6, 10, 12, 23},
		ClassHalf:    {0.5, 5.5, 10.5, 11.5, 22.5},
		ClassFrac:    {0.25, 3.75, 10.25, 10.75, 15.375, 20.625},
	}
	for class, ss := range seams {
		for _, s := range ss {
			if got := Classify(s); got != class {
				t.Fatalf("s=%v 分类错误: %s != %s", s, got, class)
			}
			k := int(math.Floor(s))
			for _, r := range []Rule{HalfOpenRight, HalfOpenLeft} {
				le := ReconcileRule(b, class, s, k, r)
				if le.DupCovered != 0 || le.GapCovered != 0 || le.Delta != 0 {
					t.Fatalf("[%s] s=%v rule=%s 半开划分失败: dup=%d gap=%d Δ=%d",
						class, s, r, le.DupCovered, le.GapCovered, le.Delta)
				}
				if le.HalfOpenDupNonzero {
					t.Fatalf("[%s] s=%v rule=%s 置位了失败信号", class, s, r)
				}
			}
		}
	}
}

// TestHeadlineNumbers 固定主算例的具体数字，防止口径漂移。
func TestHeadlineNumbers(t *testing.T) {
	b := testBand()
	if b.WholeCount() != 192 {
		t.Fatalf("整块覆盖数应为 192, 实际 %d", b.WholeCount())
	}
	cc := ReconcileRule(b, ClassInteger, 10, 10, ClosedClosed)
	if cc.Left != 88 || cc.Right != 112 || cc.Delta != 8 || cc.DupCovered != 8 {
		t.Fatalf("双闭主算例数字异常: L=%d R=%d Δ=%d dup=%d",
			cc.Left, cc.Right, cc.Delta, cc.DupCovered)
	}
	ho := ReconcileRule(b, ClassInteger, 10, 10, HalfOpenRight)
	if ho.Left != 80 || ho.Right != 112 || ho.Delta != 0 {
		t.Fatalf("半开主算例数字异常: L=%d R=%d Δ=%d", ho.Left, ho.Right, ho.Delta)
	}
	oo := ReconcileRule(b, ClassInteger, 10, 10, OpenOpen)
	if oo.Delta != -8 || oo.GapCovered != 8 {
		t.Fatalf("双开主算例数字异常: Δ=%d gap=%d", oo.Delta, oo.GapCovered)
	}
}

// TestRoundingResidualOnly: 每种取整口径、每类裁剪位置，
// Δ 必须只能由取整项解释（Unexplained=0）。
func TestRoundingResidualOnly(t *testing.T) {
	b := testBand()
	cases := []struct {
		class Class
		s     float64
	}{
		{ClassInteger, 10},
		{ClassHalf, 10.5},
		{ClassFrac, 10.25},
	}
	policies := []SnapPolicy{SnapFloorShared, SnapOutwardHalfOpen, SnapInwardTrunc, SnapNearestSeparate}
	for _, c := range cases {
		for _, p := range policies {
			row := ReconcileSnap(b, c.class, c.s, p)
			if row.Unexplained != 0 {
				t.Fatalf("[%s/%s] 存在取整之外的差值: Δ=%d term=%d unexplained=%d",
					c.class, p, row.Delta, row.RoundingTerm, row.Unexplained)
			}
		}
	}

	// 三个主位置上的关键取整数字也要钉死。
	if r := ReconcileSnap(b, ClassHalf, 10.5, SnapNearestSeparate); r.Delta != 8 {
		t.Fatalf("半像素平局 + 分别四舍五入应 Δ=+8, 实际 %d", r.Delta)
	}
	if r := ReconcileSnap(b, ClassFrac, 10.25, SnapNearestSeparate); r.Delta != 0 {
		t.Fatalf("普通小数 + 分别四舍五入应 Δ=0, 实际 %d", r.Delta)
	}
	if r := ReconcileSnap(b, ClassFrac, 10.25, SnapOutwardHalfOpen); r.Delta != 8 {
		t.Fatalf("普通小数 + 外扩取整应 Δ=+8, 实际 %d", r.Delta)
	}
	if r := ReconcileSnap(b, ClassFrac, 10.25, SnapInwardTrunc); r.Delta != -8 {
		t.Fatalf("普通小数 + 内缩截断应 Δ=-8, 实际 %d", r.Delta)
	}
}

// TestSweepAllFractions 对一整组连续缝坐标做扫描，恒等式与取整解释必须全部成立。
func TestSweepAllFractions(t *testing.T) {
	b := testBand()
	fracs := []float64{0.125, 0.2, 0.25, 1.0 / 3.0, 0.375, 0.49, 0.5, 0.51, 0.625, 2.0 / 3.0, 0.75, 0.875}
	n := 0
	for k := 0; k < b.W; k++ {
		for _, f := range fracs {
			s := float64(k) + f
			if s >= float64(b.W) {
				continue
			}
			class := ClassFrac
			if math.Abs(f-0.5) < 1e-12 {
				class = ClassHalf
			}
			for _, p := range []SnapPolicy{SnapFloorShared, SnapOutwardHalfOpen, SnapInwardTrunc, SnapNearestSeparate} {
				row := ReconcileSnap(b, class, s, p)
				if row.Unexplained != 0 {
					t.Fatalf("s=%v policy=%s unexplained=%d", s, p, row.Unexplained)
				}
				n++
			}
		}
	}
	if n == 0 {
		t.Fatal("扫描用例数为 0")
	}
}
