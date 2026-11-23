package mat

import (
	"math"
	"testing"
)

const eps = 1e-12

// TestRecomposeAllTier1：四部分乘回必须与原矩阵逐元素一致。
func TestRecomposeAllTier1(t *testing.T) {
	for _, s := range Tier1Scenarios() {
		if s.ExpectErr {
			continue
		}
		r := RunScenario(s)
		if r.Err != nil {
			t.Fatalf("%s: 未预期的分解错误: %v", s.Name, r.Err)
		}
		if r.MaxErr > eps {
			t.Fatalf("%s: 乘回逐元素最大误差 %g 超过容差 %g", s.Name, r.MaxErr, eps)
		}
		if r.OrderErr != nil {
			t.Fatalf("%s: 顺序核对失败: %v", s.Name, r.OrderErr)
		}
	}
}

// TestPureCases 逐分量核对各纯变换的分解参数。
func TestPureCases(t *testing.T) {
	// 纯平移
	p, err := Decompose(Translate(3, -2))
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(p.TX-3) > eps || math.Abs(p.TY+2) > eps {
		t.Fatalf("平移分量错误: %+v", p)
	}
	if math.Abs(p.SX-1) > eps || math.Abs(p.SY-1) > eps ||
		math.Abs(p.SH) > eps || math.Abs(p.Angle) > eps {
		t.Fatalf("纯平移的其余分量必须为单位: %+v", p)
	}

	// 纯旋转
	theta := math.Pi / 3
	p, err = Decompose(Rotate(theta))
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(p.Angle-theta) > eps {
		t.Fatalf("旋转角错误: got %g want %g", p.Angle, theta)
	}
	if math.Abs(p.SX-1) > eps || math.Abs(p.SY-1) > eps || math.Abs(p.SH) > eps {
		t.Fatalf("纯旋转的缩放/错切必须为单位: %+v", p)
	}

	// 非均匀缩放
	p, err = Decompose(Scale(2, 0.5))
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(p.SX-2) > eps || math.Abs(p.SY-0.5) > eps {
		t.Fatalf("缩放分量错误: %+v", p)
	}
	if math.Abs(p.Angle) > eps || math.Abs(p.SH) > eps {
		t.Fatalf("纯缩放的旋转/错切必须为单位: %+v", p)
	}

	// 含错切
	p, err = Decompose(Shear(0.7))
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(p.SH-0.7) > eps {
		t.Fatalf("错切分量错误: got %g want 0.7", p.SH)
	}
}

// TestErrorPositions 核对三类报错都带具体元素位置。
func TestErrorPositions(t *testing.T) {
	cases := []struct {
		name   string
		m      Mat3
		rowCol [2]int
		msg    string
	}{
		{"含NaN", New(1, math.NaN(), 0, 0, 1, 0, 0, 0, 1), [2]int{0, 1}, "NaN"},
		{"缩放为零sy", Scale(3, 0), [2]int{1, 1}, "缩放为零"},
		{"缩放为零sx", Scale(0, 3), [2]int{0, 0}, "缩放为零"},
		{"行列式为零", New(1, 2, 0.5, 2, 4, -1, 0, 0, 1), [2]int{0, 0}, "行列式"},
		{"非仿射底行", New(1, 0, 0, 0, 1, 0, 0.5, 0, 1), [2]int{2, 0}, "非仿射"},
	}
	for _, tc := range cases {
		_, err := Decompose(tc.m)
		if err == nil {
			t.Fatalf("%s: 应报错", tc.name)
		}
		pe, ok := AsPosError(err)
		if !ok {
			t.Fatalf("%s: 错误未携带元素位置: %v", tc.name, err)
		}
		if pe.Row != tc.rowCol[0] || pe.Col != tc.rowCol[1] {
			t.Fatalf("%s: 位置错误 got (%d,%d) want (%d,%d)",
				tc.name, pe.Row, pe.Col, tc.rowCol[0], tc.rowCol[1])
		}
		if !contains(pe.Msg, tc.msg) {
			t.Fatalf("%s: 错误信息 %q 不含 %q", tc.name, pe.Msg, tc.msg)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// TestOrderInference 反解顺序必须与记录的构造顺序一致。
func TestOrderInference(t *testing.T) {
	b := NewBuilder().
		Translate(-1.5, 2.25).
		Rotate(-0.6).
		Scale(2, 3).
		Shear(0.3)
	p, err := Decompose(b.Matrix())
	if err != nil {
		t.Fatal(err)
	}
	want := []OpKind{OpTranslate, OpRotate, OpScale, OpShear}
	got := InferredOrder(p)
	if len(got) != len(want) {
		t.Fatalf("反解顺序长度错误: got %v want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("反解顺序错误: got %v want %v", got, want)
		}
	}
	if err := CheckOrder(b.Ops(), p); err != nil {
		t.Fatalf("顺序核对失败: %v", err)
	}
}

// TestOrderMismatchDetected 故意篡改记录顺序，核对必须失败。
func TestOrderMismatchDetected(t *testing.T) {
	m := NewBuilder().Rotate(0.5).Scale(2, 2).Matrix()
	p, err := Decompose(m)
	if err != nil {
		t.Fatal(err)
	}
	fake := []Op{{Kind: OpShear, Param: 0.5}, {Kind: OpScale, Param: 2, Param2: 2}}
	if err := CheckOrder(fake, p); err == nil {
		t.Fatal("错误的记录顺序应当核对失败")
	}
	// 单位原语不应干扰核对。
	good := []Op{
		{Kind: OpTranslate, Param: 0, Param2: 0}, // 单位平移
		{Kind: OpRotate, Param: 0.5},
		{Kind: OpScale, Param: 2, Param2: 2},
	}
	if err := CheckOrder(good, p); err != nil {
		t.Fatalf("含单位原语的正确顺序核对失败: %v", err)
	}
}

// TestStress 第二档：大量随机非退化矩阵，分解误差与顺序全量核对。
func TestStress(t *testing.T) {
	const n = 10000
	res := RunStress(n, 20261124)
	if res.RecomposeFailures != 0 {
		t.Fatalf("存在 %d 个意外分解失败", res.RecomposeFailures)
	}
	if res.OrderMismatches != 0 {
		t.Fatalf("存在 %d 个顺序不匹配", res.OrderMismatches)
	}
	if res.MaxRecomposeErr > 1e-10 {
		t.Fatalf("压力用例最大乘回误差 %g 超阈值", res.MaxRecomposeErr)
	}
	t.Logf("第二档 n=%d 最大分解误差=%.3e 总耗时=%s 平均=%s",
		n, res.MaxRecomposeErr, res.TotalElapsed, res.AvgElapsed)
}

// TestReflection 负缩放（反射）也应可分解并乘回一致。
// 反射存在固有歧义：R·(-sx) 与 (R+π)·sx 表示同一矩阵，
// 因此反射情形只核对乘回一致性、反射标记与分量数，不强求参数逐项还原。
func TestReflection(t *testing.T) {
	m := NewBuilder().Translate(1, 1).Rotate(0.3).Scale(-2, 1.5).Shear(0.2).Matrix()
	p, err := Decompose(m)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Reflected {
		t.Fatal("det<0 应标记 Reflected")
	}
	if d := MaxAbsDiff(m, p.Recompose()); d > eps {
		t.Fatalf("反射矩阵乘回误差 %g", d)
	}
	want := []OpKind{OpTranslate, OpRotate, OpScale, OpShear}
	if got := InferredOrder(p); len(got) != len(want) {
		t.Fatalf("反射矩阵反解分量数错误: got %v want %v", got, want)
	}
}

// TestMulAndApply 矩阵乘法与点变换的基本性质。
func TestMulAndApply(t *testing.T) {
	m := Translate(10, 20).Mul(Rotate(math.Pi / 2))
	x, y := m.Apply(1, 0)
	// 先旋转 (1,0)->(0,1)，再平移 ->(10,21)
	if math.Abs(x-10) > eps || math.Abs(y-21) > eps {
		t.Fatalf("复合变换点计算错误: (%g,%g)", x, y)
	}
}

func TestTier1Table(t *testing.T) {
	for _, r := range RunTier1() {
		t.Logf("%-16s err=%v maxErr=%v elapsed=%s orderErr=%v",
			r.Name, r.Err, r.MaxErr, r.Elapsed, r.OrderErr)
	}
}
