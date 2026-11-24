package ramp

import (
	"math"
	"testing"

	"vecgo/raster"
)

func testCurve(t *testing.T) *Curve {
	t.Helper()
	c, err := New([]Stop{
		{0.00, 0.970}, {0.15, 0.850}, {0.30, 0.625}, {0.45, 0.385},
		{0.58, 0.177}, {0.73, 0.161}, {0.88, 0.143}, {1.00, 0.133},
	})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func almost(a, b, eps float64) bool { return math.Abs(a-b) <= eps }

func TestNewValidation(t *testing.T) {
	if _, err := New([]Stop{{0, 1}}); err == nil {
		t.Fatal("少于两个点应报错")
	}
	if _, err := New([]Stop{{X: .5, Y: .5}, {X: .4, Y: 1}}); err == nil {
		t.Fatal("X 非递增应报错")
	}
	if _, err := New([]Stop{{X: 0, Y: 1.2}, {X: 1, Y: 0}}); err == nil {
		t.Fatal("Y 越界应报错")
	}
}

func TestHermiteEnds(t *testing.T) {
	got := Hermite(.2, .8, -1, -.5, .4, 0)
	if !almost(got, .2, 1e-15) {
		t.Fatalf("u=0 必须等于 y0, got %v", got)
	}
	got = Hermite(.2, .8, -1, -.5, .4, 1)
	if !almost(got, .8, 1e-15) {
		t.Fatalf("u=1 必须等于 y1, got %v", got)
	}
}

// 当两端切线都等于弦斜率时，Hermite 必须退化为直线，目标函数为 0。
func TestQuadraticLinearSegmentZero(t *testing.T) {
	// hL=hR=1, dL=dR=-1, a=b=-1 -> tau=-1 且 J(tau)=0。
	A, B, C, tau := Quadratic(1, 1, -1, -1, -1, -1)
	if A <= 0 {
		t.Fatal("目标函数必须严格凸 A>0")
	}
	if !almost(tau, -1, 1e-12) || !almost(B/A, tau, 1e-12) {
		t.Fatalf("驻点错误 tau=%v B/A=%v", tau, B/A)
	}
	if j := A*tau*tau - 2*B*tau + C; !almost(j, 0, 1e-12) {
		t.Fatalf("直线情形 J 应为 0, got %v", j)
	}
	if !almost(tau, StationaryTangent(1, 1, -1, -1, -1, -1), 1e-12) {
		t.Fatal("两种驻点公式不一致")
	}
}

// 驻点公式必须与目标函数数值积分的有限差分梯度一致。
func TestStationaryMatchesFiniteDifference(t *testing.T) {
	cases := []struct{ hL, hR, dL, dR, a, b float64 }{
		{.18, .18, -.5, -1, -.5, -1.21875},
		{.18, .16, -1, -1.4375, -.75, -1.59375},
		{.13, .15, -1.6, -.1067, -1.6, -.1133},
		{.3, .12, .8, 2.1, .4, 1.1},
	}
	for _, q := range cases {
		A, B, _, tau := Quadratic(q.hL, q.hR, q.dL, q.dR, q.a, q.b)
		tau2 := StationaryTangent(q.hL, q.hR, q.dL, q.dR, q.a, q.b)
		if !almost(tau, tau2, 1e-12) {
			t.Fatalf("公式不一致: %v vs %v", tau, tau2)
		}
		// J'(tau) 应为 0；用中心差分核对解析导数 -2B+2At。
		eps := 1e-6
		j := func(x float64) float64 {
			aa, bb, cc, _ := Quadratic(q.hL, q.hR, q.dL, q.dR, q.a, q.b)
			return aa*x*x - 2*bb*x + cc
		}
		fd := (j(tau+eps) - j(tau-eps)) / (2 * eps)
		if math.Abs(fd) > 1e-6 {
			t.Fatalf("驻点处有限差分梯度=%v 非零 (tau=%v A=%v B=%v)", fd, tau, A, B)
		}
	}
}

func TestFCIntervalSign(t *testing.T) {
	// 下降弦：区间必须是 [-R,0]。
	lo, hi := FCInterval(-1, -1, -1, -1)
	if hi != 0 || lo >= 0 || !almost(lo, -math.Sqrt(8), 1e-12) {
		t.Fatalf("下降区间错误 [%v,%v]", lo, hi)
	}
	// 上升弦：[0,R]。
	lo, hi = FCInterval(1, 1, 1, 1)
	if lo != 0 || hi <= 0 || !almost(hi, math.Sqrt(8), 1e-12) {
		t.Fatalf("上升区间错误 [%v,%v]", lo, hi)
	}
	// 相邻弦斜率反号（数据转折）：只能取 0。
	if lo, hi := FCInterval(-.5, -.4, .2, .1); lo != 0 || hi != 0 {
		t.Fatalf("反号弦应得到退化区间, got [%v,%v]", lo, hi)
	}
	// α=β=2.4, d=1 -> R=sqrt(9-5.76)=1.8（Pythagorean 3-4-5 缩放）。
	lo, hi = FCInterval(-1, -2.4, -1, -2.4)
	if !almost(lo, -1.8, 1e-12) || hi != 0 {
		t.Fatalf("幅值端点区间错误 [%v,%v]", lo, hi)
	}
}

func TestThreeCases(t *testing.T) {
	// 情形一：左（幅值）端点。
	s := SolveOne(1, 1, -1, -1, -2.4, -2.4)
	if s.Case != "lower" || !almost(s.That, -1.8, 1e-12) {
		t.Fatalf("左端点评判错误: case=%s that=%v", s.Case, s.That)
	}
	// 约束最优必须等于 J(lo)；无约束驻点在区间外，其 J 更小但不可行。
	if jLo := s.A*s.Lo*s.Lo - 2*s.B*s.Lo + s.C; !almost(s.JHat, jLo, 1e-12) {
		t.Fatal("JHat 必须等于 J(lo)")
	}

	// 情形二：右（符号）端点。
	s = SolveOne(1, 1, -1, -1, 1, 1)
	if s.Case != "upper" || !almost(s.Tau, .5, 1e-12) || s.That != 0 {
		t.Fatalf("右端点评判错误: case=%s tau=%v that=%v", s.Case, s.Tau, s.That)
	}

	// 情形三：内部驻点。
	s = SolveOne(1, 1, -1, -1, -1, -1)
	if s.Case != "interior" || !almost(s.That, -1, 1e-12) {
		t.Fatalf("内部驻点评判错误: case=%s that=%v", s.Case, s.That)
	}
	// 凸二次上，内部驻点必须严格优于两个端点。
	jLo := s.A*s.Lo*s.Lo - 2*s.B*s.Lo + s.C
	jHi := s.A*s.Hi*s.Hi - 2*s.B*s.Hi + s.C
	if !(s.JHat < jLo && s.JHat < jHi) {
		t.Fatal("内部驻点应严格优于两端")
	}
}

func jAt(s OneSolution, x float64) float64 { return s.A*x*x - 2*s.B*x + s.C }

// 事故版本必须确实复现过冲，否则修复前后对比没有意义。
func TestNaiveReproducesOvershoot(t *testing.T) {
	c := testCurve(t)
	im, err := raster.New(256, 256)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.FillImage(im); err != nil {
		t.Fatal(err)
	}
	px, rows, _, _ := c.OvershootPixels(im)
	if px == 0 || rows == 0 {
		t.Fatal("朴素切线应当产生过冲像素")
	}
	bad, excess := c.OvershootDense(400)
	if bad == 0 || excess == 0 {
		t.Fatal("连续密采也应见过冲")
	}
}

// 修复后：任意尺寸图像过冲像素为 0、连续密采无越界、FC 约束成立、亮部锚点零偏离。
func TestOptimizedZeroOvershoot(t *testing.T) {
	for _, size := range []int{64, 256} {
		c := testCurve(t)
		if _, mc := c.Optimize(2000, 1e-14); mc > 1e-10 {
			t.Fatalf("size=%d 扫描未收敛 mc=%v", size, mc)
		}
		if !c.FCFeasible(1e-7) {
			t.Fatalf("size=%d 修复后不满足 FC 约束", size)
		}
		im, err := raster.New(size, size)
		if err != nil {
			t.Fatal(err)
		}
		if err := c.FillImage(im); err != nil {
			t.Fatal(err)
		}
		if px, rows, _, detail := c.OvershootPixels(im); px != 0 || rows != 0 {
			n := 3
			if len(detail) < n {
				n = len(detail)
			}
			t.Fatalf("size=%d 仍有过冲 px=%d rows=%d detail=%+v", size, px, rows, detail[:n])
		}
		if bad, ex := c.OvershootDense(1200); bad != 0 || ex != 0 {
			t.Fatalf("size=%d 连续过冲 bad=%d excess=%v", size, bad, ex)
		}
		if d := c.KnotDeviation([]int{0, 1, 2, 3}); d > 1e-12 {
			t.Fatalf("亮部控制点偏离 %v 过大", d)
		}
	}
}

func TestByteMonotone(t *testing.T) {
	// 量化必须单调，否则“端点灰阶区间”判定会有假阳性。
	prev := Byte(0)
	for i := 1; i <= 1000; i++ {
		b := Byte(float64(i) / 1000)
		if b < prev {
			t.Fatal("Byte 非单调")
		}
		prev = b
	}
}
