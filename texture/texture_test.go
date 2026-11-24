package texture

import (
	"math"
	"testing"
)

func TestDecayWeightShape(t *testing.T) {
	if got := DecayWeight(0, CheckLambda); got != 1 {
		t.Fatalf("w(0) 必须为 1, got %v", got)
	}
	prev := 1.0
	for d := 0.5; d <= 40; d += 0.5 {
		w := DecayWeight(d, CheckLambda)
		if w <= 0 || w >= prev {
			t.Fatalf("权重必须为正且严格递减: d=%v w=%v prev=%v", d, w, prev)
		}
		prev = w
	}
	if w := DecayWeight(1e6, CheckLambda); w > 1e-30 {
		t.Fatalf("极远处权重应趋近 0, got %v", w)
	}
	// 系数换算：β_k = kλ，两档系数分别为 0.18 / 0.36。
	if math.Abs(CheckBeta(2)-0.18) > 1e-12 {
		t.Fatalf("β2=%v", CheckBeta(2))
	}
	if math.Abs(CheckBeta(4)-0.36) > 1e-12 {
		t.Fatalf("β4=%v", CheckBeta(4))
	}
}

func TestOvershootDistance(t *testing.T) {
	cases := []struct {
		x, y, mx, my float64
		wantOOB      bool
		wantD        float64
	}{
		{5, 5, 7, 7, false, 0},
		{-2, 3, 7, 7, true, 2},
		{9, 7, 7, 7, true, 2},
		{-3, -4, 7, 7, true, 5}, // hypot(3,4)
	}
	for _, c := range cases {
		_, _, d := Overshoot(c.x, c.y, c.mx, c.my)
		if math.Abs(d-c.wantD) > 1e-12 {
			t.Fatalf("Overshoot(%v,%v) d=%v want %v", c.x, c.y, d, c.wantD)
		}
		if got := OutOfBounds(c.x, c.y, c.mx, c.my); got != c.wantOOB {
			t.Fatalf("OOB(%v,%v)=%v want %v", c.x, c.y, got, c.wantOOB)
		}
	}
}

// TestInsidePixelsUnchanged 保证带权外插在界内与普通双线性采样逐像素一致，
// 新机制只影响真正越界的位置。
func TestInsidePixelsUnchanged(t *testing.T) {
	tex := BuildReferenceTex(32)
	s := NewSampler(tex)
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			tx := float64(x)
			ty := float64(y)
			if OutOfBounds(tx, ty, 31, 31) {
				continue
			}
			a := s.Clamp(tx, ty)
			b := s.Weighted(tx, ty, CheckLambda, CheckFallback(), 1)
			if math.Abs(a.R-b.R) > 1e-12 || math.Abs(a.G-b.G) > 1e-12 || math.Abs(a.B-b.B) > 1e-12 {
				t.Fatalf("界内 (%d,%d) 带权采样与双线性不一致: %+v vs %+v", x, y, b, a)
			}
		}
	}
}

// TestTwoTierOOBCounts 锁定两档缩小下的越界像素数（几何固定，数量必须精确）。
func TestTwoTierOOBCounts(t *testing.T) {
	r2 := RunTier(ReferenceField, RefSize, 2, CheckLambda)
	r4 := RunTier(ReferenceField, RefSize, 4, CheckLambda)
	if r2.OOBCount != 5120 || r2.InsideCount != 4096 {
		t.Fatalf("x2 越界数异常: oob=%d inside=%d", r2.OOBCount, r2.InsideCount)
	}
	if r4.OOBCount != 8192 || r4.InsideCount != 1024 {
		t.Fatalf("x4 越界数异常: oob=%d inside=%d", r4.OOBCount, r4.InsideCount)
	}
}

// TestWeightedExtrapBeatsClampAtTwoTiers 是核心验收：
// 两档缩小倍数下，越界区域的“平均”色差都必须相对截断版显著下降。
func TestWeightedExtrapBeatsClampAtTwoTiers(t *testing.T) {
	for _, k := range []int{2, 4} {
		r := RunTier(ReferenceField, RefSize, k, CheckLambda)
		if r.OOBCount == 0 {
			t.Fatalf("k=%d 没有统计到越界像素", k)
		}
		if !(r.ExtrapMean < r.ClampMean*0.10) {
			t.Fatalf("k=%d 平均色差下降不足90%%: clamp=%.3f extrap=%.3f", k, r.ClampMean, r.ExtrapMean)
		}
		if r.ExtrapMean > 8 {
			t.Fatalf("k=%d 外插平均色差绝对值得过大: %.3f", k, r.ExtrapMean)
		}
		t.Logf("k=%d oob=%d meanRGB clamp=%.3f extrap=%.3f improvement=%.2f%% (max %.1f->%.1f)",
			k, r.OOBCount, r.ClampMean, r.ExtrapMean, r.ImprovementPct(), r.ClampMax, r.ExtrapMax)
	}
}

// TestFirstOrderLinearExtrapolation 证明 order=1 是真正的一阶线性外插：
// 对线性渐变贴图，边界梯度为常数，在 λ=0（权重恒 1）时必须精确推出界外值。
func TestFirstOrderLinearExtrapolation(t *testing.T) {
	const n = 8
	tex, _ := New(n, n)
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			tex.Pix[y*n+x] = RGB{R: float64(x) / (n - 1), G: float64(y) / (n - 1)}
		}
	}
	s := NewSampler(tex)
	got := s.Weighted(-2, 3.5, 0, RGB{}, 1) // 左边界外 2 纹素
	wantR := -2.0 / (n - 1)
	if math.Abs(got.R-wantR) > 1e-9 {
		t.Fatalf("一阶外插 R=%v want %v", got.R, wantR)
	}
	// 3.5 行坐标双线性给出 G=0.5；越界只发生在 x 轴，G 不应被外插改变。
	if math.Abs(got.G-0.5) > 1e-9 {
		t.Fatalf("一阶外插 G=%v want 0.5", got.G)
	}
}

// TestRobustMismatchedField 防止结论依赖“真值恰好等于指数权重形状”。
// 对平方指数的高斯场，同一个 λ 仍应让平均色差大幅下降。
func TestRobustMismatchedField(t *testing.T) {
	for _, k := range []int{2, 4} {
		r := RunTier(GaussianMismatchField, RefSize, k, CheckLambda)
		if !(r.ExtrapMean < r.ClampMean*0.20) {
			t.Fatalf("异配场 k=%d 下降不足80%%: clamp=%.3f extrap=%.3f", k, r.ClampMean, r.ExtrapMean)
		}
		t.Logf("mismatch k=%d clamp=%.3f extrap=%.3f improvement=%.1f%%",
			k, r.ClampMean, r.ExtrapMean, r.ImprovementPct())
	}
}
