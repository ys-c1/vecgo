package interp

import (
	"math"
	"testing"
)

// 三组场函数：
//
//	fSin  光滑，M2 ≤ |fxx|+2|fxy|+|fyy| ≤ 4π²
//	fQuad 二次，Hessian 恒定，M2 = 2+2·1+4 = 8
//	fKink 非光滑，x=1/3 处一阶导跳跃，二阶导为 2δ（无界）
var (
	fSin  Field = func(x, y float64) float64 { return math.Sin(math.Pi*x) * math.Sin(math.Pi*y) }
	fQuad Field = func(x, y float64) float64 { return x*x + x*y + 2*y*y }
	fKink Field = func(x, y float64) float64 { return math.Abs(x - 1.0/3.0) }
)

const (
	m2Sin  = 4 * math.Pi * math.Pi
	m2Quad = 8.0
)

// samples 取 12n：可被 3 整除，采样格点恰好经过折线 x=1/3，
// 非光滑场的最大误差（在折线上）不会被漏采。
func samples(n int) int { return 12 * n }

// 误差界 |f − I_h f| ≤ (M2/4)·h² 在多种网格尺寸下成立。
func TestErrorBoundHolds(t *testing.T) {
	cases := []struct {
		name string
		f    Field
		m2   float64
	}{
		{"sin(πx)sin(πy)", fSin, m2Sin},
		{"x²+xy+2y²", fQuad, m2Quad},
	}
	for _, c := range cases {
		for _, n := range []int{8, 16, 32, 64, 128} {
			m := NewMesh(n)
			h := m.H()
			e := m.MaxError(c.f, samples(n))
			b := Bound(c.m2, h)
			if e > b*(1+1e-9) {
				t.Errorf("%s n=%d: 实测误差 %.6e 超出界 (M2/4)h² = %.6e", c.name, n, e, b)
			}
			t.Logf("%s n=%3d h=%.6f 实测=%.6e 界=%.6e 余量=%.1fx",
				c.name, n, h, e, b, b/e)
		}
	}
}

// 按目标误差反算边长与网格，改完后实测误差必须落在界内。
func TestTargetError(t *testing.T) {
	const eps = 0.02
	cases := []struct {
		name string
		f    Field
		m2   float64
	}{
		{"sin(πx)sin(πy)", fSin, m2Sin},
		{"x²+xy+2y²", fQuad, m2Quad},
	}
	for _, c := range cases {
		h := HForError(c.m2, eps)
		m := MeshForError(c.m2, eps)
		e := m.MaxError(c.f, samples(m.N))
		b := Bound(c.m2, m.H())
		if b > eps {
			t.Errorf("%s: 界 %.6e 超过目标 %.6e（反算有误）", c.name, b, eps)
		}
		if e > eps {
			t.Errorf("%s: 实测误差 %.6e 未落在目标 %.6e 内", c.name, e, eps)
		}
		t.Logf("%s 目标eps=%.3f → h≤%.6f 取n=%d(h=%.6f) 界=%.6e 实测=%.6e ✓在界内",
			c.name, eps, h, m.N, m.H(), b, e)
	}
}

// 单独验证：加密一档（n→2n），光滑场误差降到 1/4，非光滑场只降到 1/2。
func TestRefinementQuartersError(t *testing.T) {
	const n1, n2 = 64, 128
	e1 := NewMesh(n1).MaxError(fSin, samples(n1))
	e2 := NewMesh(n2).MaxError(fSin, samples(n2))
	r := e1 / e2
	if r < 3.5 || r > 4.5 {
		t.Errorf("fSin 加密一档误差比 %.3f，期望约 4", r)
	}
	t.Logf("fSin  E(%d)=%.6e E(%d)=%.6e 比=%.3f 阶=%.3f", n1, e1, n2, e2, r, math.Log2(r))

	q1 := NewMesh(n1).MaxError(fQuad, samples(n1))
	q2 := NewMesh(n2).MaxError(fQuad, samples(n2))
	rq := q1 / q2
	if rq < 3.5 || rq > 4.5 {
		t.Errorf("fQuad 加密一档误差比 %.3f，期望约 4", rq)
	}
	t.Logf("fQuad E(%d)=%.6e E(%d)=%.6e 比=%.3f 阶=%.3f", n1, q1, n2, q2, rq, math.Log2(rq))

	k1 := NewMesh(n1).MaxError(fKink, samples(n1))
	k2 := NewMesh(n2).MaxError(fKink, samples(n2))
	rk := k1 / k2
	if rk < 1.8 || rk > 2.2 {
		t.Errorf("fKink 加密一档误差比 %.3f，期望约 2（一阶）", rk)
	}
	if p := math.Log2(rk); p > 1.5 {
		t.Errorf("fKink 观测阶 %.3f，非光滑场不应达到二阶", p)
	}
	t.Logf("fKink E(%d)=%.6e E(%d)=%.6e 比=%.3f 阶=%.3f（非光滑，只有一阶）",
		n1, k1, n2, k2, rk, math.Log2(rk))
}

// 汇总表：三组场函数的边长、实测误差、预测界、加密比与观测阶。
// 用 go test -run TestConvergenceReport -v 查看。
func TestConvergenceReport(t *testing.T) {
	ns := []int{16, 32, 64, 128}
	cases := []struct {
		name string
		f    Field
		m2   float64 // 0 表示不存在（非光滑）
	}{
		{"sin(πx)sin(πy)", fSin, m2Sin},
		{"x²+xy+2y²", fQuad, m2Quad},
		{"|x−1/3|（非光滑）", fKink, 0},
	}
	for _, c := range cases {
		t.Logf("场函数 %s", c.name)
		prev := 0.0
		for _, n := range ns {
			m := NewMesh(n)
			h := m.H()
			e := m.MaxError(c.f, samples(n))
			ratio, order := math.NaN(), math.NaN()
			if prev > 0 {
				ratio = prev / e
				order = math.Log2(ratio)
			}
			if c.m2 > 0 {
				t.Logf("  n=%3d h=%.6f 实测E=%.6e 预测界(M2/4)h²=%.6e 加密比=%.3f 观测阶=%.3f",
					n, h, e, Bound(c.m2, h), ratio, order)
			} else {
				t.Logf("  n=%3d h=%.6f 实测E=%.6e 预测界=不适用(M2不存在) 加密比=%.3f 观测阶=%.3f",
					n, h, e, ratio, order)
			}
			prev = e
		}
	}
}
