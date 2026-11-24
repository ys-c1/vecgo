package interp

import (
	"math"
	"testing"
)

// 三组测试控制点：
//
//	A：单调陡升后接平台（亮部快速拉升，过冲出现在平台入口）
//	B：含局部极值（先升后降，小割线区间内容易回勾）
//	C：非均匀间距单调下降进入暗部
func testSets() map[string]struct{ x, y []float64 } {
	return map[string]struct{ x, y []float64 }{
		"A": {
			x: []float64{0, 1, 2, 3, 4},
			y: []float64{0, 0, 0.1, 1, 1},
		},
		"B": {
			x: []float64{0, 1, 2, 3, 4},
			y: []float64{0, 0.2, 1, 0.9, 0},
		},
		"C": {
			x: []float64{0, 2, 3, 5, 6},
			y: []float64{1, 0.95, 0.4, 0.05, 0},
		},
	}
}

const dense = 400 // 每区间密集采样数

func TestRawSplineOvershoots(t *testing.T) {
	// 前置事实：未加限制器时三组数据都存在过冲，测试才有意义。
	for name, set := range testSets() {
		s, err := New(set.x, set.y, false)
		if err != nil {
			t.Fatal(err)
		}
		if n := s.OvershootCount(dense); n == 0 {
			t.Errorf("集合 %s：原始曲线应存在过冲，实际为 0", name)
		}
	}
}

func TestLimiterKillsOvershoot(t *testing.T) {
	for name, set := range testSets() {
		s, err := New(set.x, set.y, true)
		if err != nil {
			t.Fatal(err)
		}
		if n := s.OvershootCount(dense); n != 0 {
			t.Errorf("集合 %s：限制器后过冲数 = %d，期望 0", name, n)
		}
		if n := s.MonotoneViolations(dense); n != 0 {
			t.Errorf("集合 %s：单调违反区间数 = %d，期望 0", name, n)
		}
	}
}

func TestKnotsInterpolatedExactly(t *testing.T) {
	for name, set := range testSets() {
		for _, limited := range []bool{false, true} {
			s, err := New(set.x, set.y, limited)
			if err != nil {
				t.Fatal(err)
			}
			if e := s.KnotError(); e > 1e-12 {
				t.Errorf("集合 %s limited=%v：控制点偏差 %g，期望 0", name, limited, e)
			}
		}
	}
}

// TestKnotTangents 单独验证控制点处的切线：
//  1. 同号判据：Δ_{k-1}*Δ_k ≤ 0 时切线必须为 0；同号时切线与割线同号；
//  2. 每个区间满足保守圆 α²+β²≤9；
//  3. 每个区间落在精确单调区域内（ExactBetaMax 双向核对）。
func TestKnotTangents(t *testing.T) {
	for name, set := range testSets() {
		s, err := New(set.x, set.y, true)
		if err != nil {
			t.Fatal(err)
		}
		d := Secants(set.x, set.y)
		n := len(set.x)

		// 内部控制点的同号检查。
		for k := 1; k < n-1; k++ {
			prod := d[k-1] * d[k]
			switch {
			case prod <= 0:
				if s.Tangents[k] != 0 {
					t.Errorf("集合 %s 控制点 %d：割线异号/含平台，切线应为 0，实际 %g",
						name, k, s.Tangents[k])
				}
			default:
				if math.Signbit(s.Tangents[k]) != math.Signbit(d[k]) {
					t.Errorf("集合 %s 控制点 %d：切线符号与割线不一致", name, k)
				}
			}
		}

		// 区间级约束。
		for k := 0; k < n-1; k++ {
			delta := d[k]
			if delta == 0 {
				if s.Tangents[k] != 0 || s.Tangents[k+1] != 0 {
					t.Errorf("集合 %s 平台区间 %d 两端切线必须为 0", name, k)
				}
				continue
			}
			alpha := s.Tangents[k] / delta
			beta := s.Tangents[k+1] / delta
			if alpha < 0 || beta < 0 {
				t.Errorf("集合 %s 区间 %d：α=%g β=%g 必须非负", name, k, alpha, beta)
			}
			if alpha*alpha+beta*beta > 9+1e-12 {
				t.Errorf("集合 %s 区间 %d：α²+β²=%g > 9", name, k, alpha*alpha+beta*beta)
			}
			bmax, ok := ExactBetaMax(alpha)
			if !ok || beta > bmax+1e-12 {
				t.Errorf("集合 %s 区间 %d：β=%g 超出精确允许上界 %g (ok=%v)",
					name, k, beta, bmax, ok)
			}
			amax, ok := ExactBetaMax(beta) // 区域关于 α,β 对称
			if !ok || alpha > amax+1e-12 {
				t.Errorf("集合 %s 区间 %d：α=%g 超出精确允许上界 %g (ok=%v)",
					name, k, alpha, amax, ok)
			}
		}
	}
}

func TestSameSignCriterion(t *testing.T) {
	cases := []struct {
		a, b float64
		want bool
	}{
		{1, 2, true}, {-1, -2, true},
		{1, -1, false}, {-1, 1, false},
		{0, 1, false}, {1, 0, false}, {0, 0, false},
	}
	for _, c := range cases {
		if got := SameSign(c.a, c.b); got != c.want {
			t.Errorf("SameSign(%g,%g)=%v，期望 %v", c.a, c.b, got, c.want)
		}
	}
}

// TestExactBetaMaxNumeric 用密集数值扫描反推可行 β 的边界，
// 与解析解 ExactBetaMax 核对。
func TestExactBetaMaxNumeric(t *testing.T) {
	const grid = 4001
	for _, alpha := range []float64{0, 0.5, 1, 1.49, 1.5, 2, 2.5, 3, 3.5, 3.99, 4, 4.01} {
		var numericMax float64 = -1
		for i := 0; i < grid; i++ {
			beta := 6 * float64(i) / (grid - 1) // 扫描 β∈[0,6]
			if segmentMonotone(alpha, beta) {
				numericMax = beta
			}
		}
		got, ok := ExactBetaMax(alpha)
		if alpha > 4 {
			if ok {
				t.Errorf("α=%g 应无可行 β", alpha)
			}
			continue
		}
		if !ok {
			t.Errorf("α=%g 应存在可行 β", alpha)
			continue
		}
		if math.Abs(got-numericMax) > 6.0/(grid-1)*2 {
			t.Errorf("α=%g：解析 βmax=%g 与数值边界 %g 不符", alpha, got, numericMax)
		}
	}
}

// segmentMonotone 数值判定单位区间 Hermite 段（y0=0,y1=1，
// 切线 α、β）是否单调非降且不越界。
func segmentMonotone(alpha, beta float64) bool {
	prev := 0.0
	for i := 0; i <= 2000; i++ {
		t := float64(i) / 2000
		v := h10(t)*alpha + h01(t) + h11(t)*beta
		if v < -1e-10 || v > 1+1e-10 || v < prev-1e-10 {
			return false
		}
		prev = v
	}
	return true
}

func TestLinearDataLimiterNoop(t *testing.T) {
	x := []float64{0, 1, 2, 3}
	y := []float64{0, 2, 4, 6}
	raw, _ := New(x, y, false)
	lim, _ := New(x, y, true)
	for k := range raw.Tangents {
		if math.Abs(raw.Tangents[k]-lim.Tangents[k]) > 1e-12 {
			t.Fatalf("线性数据上限制器不应改动切线，k=%d", k)
		}
	}
	if mx, rms := ShapeDeviation(raw, lim, 500); mx > 1e-12 || rms > 1e-12 {
		t.Fatalf("线性数据上两曲线应完全一致，mx=%g rms=%g", mx, rms)
	}
}

func TestPlatformInterval(t *testing.T) {
	x := []float64{0, 1, 2, 3}
	y := []float64{0.5, 0.5, 1, 1}
	s, err := New(x, y, true)
	if err != nil {
		t.Fatal(err)
	}
	if n := s.OvershootCount(dense); n != 0 {
		t.Fatalf("平台数据过冲数 = %d，期望 0", n)
	}
}

// TestClippedPixels 验证像素域截断：原始曲线存在被 [0,1] 夹住的样本，
// 限制器后截断数必须为 0（过冲像素数为零）。
func TestClippedPixels(t *testing.T) {
	for name, set := range testSets() {
		raw, _ := New(set.x, set.y, false)
		lim, _ := New(set.x, set.y, true)
		rlo, rhi := raw.ClippedCount(256)
		llo, lhi := lim.ClippedCount(256)
		if rlo+rhi == 0 {
			t.Errorf("集合 %s：原始曲线应有截断像素，实际为 0", name)
		}
		if llo+lhi != 0 {
			t.Errorf("集合 %s：限制后截断像素 = %d，期望 0", name, llo+lhi)
		}
	}
}

func TestRejectsBadInput(t *testing.T) {
	if _, err := New([]float64{0, 1}, []float64{0}, false); err == nil {
		t.Fatal("长度不一致应报错")
	}
	if _, err := New([]float64{1, 0}, []float64{0, 1}, false); err == nil {
		t.Fatal("x 非递增应报错")
	}
}
