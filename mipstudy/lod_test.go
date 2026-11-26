package mipstudy

import (
	"math"
	"testing"
)

const eps = 1e-12

// 三档缩放比：短轴固定 b=2 纹素，长轴 a=2、4、8，轴比 r=1、2、4。
type tier struct {
	name      string
	a, b      float64
	wantR     float64
	wantL0    float64 // 未修正面积公式层号
	wantLstar float64 // 标准（最长边）层号
	wantErr   float64 // e(r) = -1/2*log2(r)
}

var tiers = []tier{
	{name: "2:1x 缩(等方)", a: 2, b: 2, wantR: 1, wantL0: 1, wantLstar: 1, wantErr: 0},
	{name: "4:1x 缩", a: 4, b: 2, wantR: 2, wantL0: 1.5, wantLstar: 2, wantErr: -0.5},
	{name: "8:1x 缩", a: 8, b: 2, wantR: 4, wantL0: 2, wantLstar: 3, wantErr: -1},
}

// 误差只由轴比决定：e(r) = L0 - L* = -1/2*log2(r)，且随 r 单调增大（绝对值）。
func TestErrorIsAxisRatioOnly(t *testing.T) {
	for _, tc := range tiers {
		r := AxisRatio(tc.a, tc.b)
		if math.Abs(r-tc.wantR) > eps {
			t.Errorf("%s: 轴比 = %v, 期望 %v", tc.name, r, tc.wantR)
		}
		l0 := LevelArea(tc.a, tc.b)
		lstar := LevelMaxAxis(tc.a, tc.b)
		if math.Abs(l0-tc.wantL0) > eps {
			t.Errorf("%s: L0 = %v, 期望 %v", tc.name, l0, tc.wantL0)
		}
		if math.Abs(lstar-tc.wantLstar) > eps {
			t.Errorf("%s: L* = %v, 期望 %v", tc.name, lstar, tc.wantLstar)
		}
		got := l0 - lstar
		if math.Abs(got-tc.wantErr) > eps {
			t.Errorf("%s: 误差 = %v, 期望 %v", tc.name, got, tc.wantErr)
		}
		if math.Abs(got-ErrorArea(r)) > eps {
			t.Errorf("%s: 实测误差 %v 与 ErrorArea(r)=%v 不符", tc.name, got, ErrorArea(r))
		}
	}
	// 同一轴比、不同绝对尺度，误差必须相同：误差是 r 的一元函数。
	e1 := LevelArea(16, 4) - LevelMaxAxis(16, 4) // r=4
	e2 := LevelArea(100, 25) - LevelMaxAxis(100, 25)
	if math.Abs(e1-e2) > eps || math.Abs(e1-(-1)) > eps {
		t.Errorf("同轴比误差不一致: %v vs %v", e1, e2)
	}
	// |e| 随 r 单调不减。
	prev := 0.0
	for r := 1.0; r <= 16; r *= 2 {
		e := -ErrorArea(r)
		if e < prev-eps {
			t.Fatalf("误差未随轴比增大: r=%v e=%v", r, e)
		}
		prev = e
	}
}

// 按轴比修正后与标准公式恒等：L0 + C(r) = L*，连续残差恒为零，误差有界（数值零界内）。
func TestCorrectionBounded(t *testing.T) {
	for r := 1.0; r <= 1024; r *= 1.25 {
		a, b := r, 1.0
		residual := LevelCorrected(a, b) - LevelMaxAxis(a, b)
		if math.Abs(residual) > eps {
			t.Errorf("r=%v 修正残差 = %v, 期望 0", r, residual)
		}
	}
	// 离散选层（floor）相对连续 L* 的偏差落在 [-1, 0]，与轴比无关地有界。
	maxLevel := 8
	for r := 1.0; r <= 128; r *= 1.1 {
		lstar := LevelMaxAxis(r, 1)
		k := FloorLevel(LevelCorrected(r, 1), maxLevel)
		d := float64(k) - lstar
		if d < -1-eps || d > eps {
			t.Errorf("r=%v 离散层偏差 %v 越出 [-1,0]", r, d)
		}
	}
}

// 轴比趋一时修正项退化为零，且修正前后层号相同（三档中的等方档）。
func TestCorrectionVanishesAtUnity(t *testing.T) {
	if math.Abs(Correction(1)) > eps {
		t.Fatalf("C(1) = %v, 期望 0", Correction(1))
	}
	// r -> 1 极限：1±δ 时修正项趋于 0。
	for _, delta := range []float64{0.1, 0.01, 0.001} {
		if math.Abs(Correction(1+delta)) >= math.Abs(Correction(1+10*delta)) {
			t.Fatalf("修正项未随 r->1 趋零: δ=%v", delta)
		}
	}
	tc := tiers[0]
	if math.Abs(LevelArea(tc.a, tc.b)-LevelCorrected(tc.a, tc.b)) > eps {
		t.Fatal("等方足迹下修正前后层号应相同")
	}
}

// 两档（非等方）层号差与轴比对得上：
// L*(r2)-L*(r1) = log2(r2/r1)，同样的差也等于轴比对数差。
func TestLevelDifferencesMatchAxisRatio(t *testing.T) {
	r1, r2, r3 := 1.0, 2.0, 4.0
	l1, l2, l3 := LevelMaxAxis(2, 2), LevelMaxAxis(4, 2), LevelMaxAxis(8, 2)
	if math.Abs((l2-l1)-math.Log2(r2/r1)) > eps {
		t.Errorf("档1->档2 层号差 %v 与轴比对数 %v 不符", l2-l1, math.Log2(r2/r1))
	}
	if math.Abs((l3-l2)-math.Log2(r3/r2)) > eps {
		t.Errorf("档2->档3 层号差 %v 与轴比对数 %v 不符", l3-l2, math.Log2(r3/r2))
	}
	if math.Abs((l3-l1)-math.Log2(r3/r1)) > eps {
		t.Errorf("档1->档3 层号差 %v 与轴比对数 %v 不符", l3-l1, math.Log2(r3/r1))
	}
	// 未修正公式的层号差被轴比“吃掉”一半：档1->档3 只有 1 级而不是 2 级。
	d0 := LevelArea(8, 2) - LevelArea(2, 2)
	if math.Abs(d0-1) > eps {
		t.Errorf("未修正档1->档3 层号差 = %v, 期望 1", d0)
	}
}
