package light

import (
	"math"
	"testing"
)

func approx(t *testing.T, name string, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Fatalf("%s = %.12g, 期望 %.12g (容差 %g)", name, got, want, tol)
	}
}

func TestRec709GrayscaleCollapsesToI(t *testing.T) {
	for _, i := range []float64{0, 0.04, 0.5, 1} {
		approx(t, "Y(I,I,I)", Rec709Luminance(i, i, i), i, 1e-15)
	}
}

func TestLightnessAnchorPoints(t *testing.T) {
	approx(t, "L*(0)", Lightness(0), 0, 1e-12)
	approx(t, "L*(1)", Lightness(1), 100, 1e-10)
	approx(t, "L*(ε)=8", Lightness(CIEEpsilon), 8, 1e-9)
	// 线性趾内部必须严格等于 κY。
	approx(t, "L*(0.004)", Lightness(0.004), CIEKappa*0.004, 1e-9)
	// 已知点：Y=0.5 → L*≈76.07。
	approx(t, "L*(0.5)", Lightness(0.5), 76.069261, 1e-5)
}

func TestInverseRoundTrip(t *testing.T) {
	for _, y := range []float64{0.001, 0.004, CIEEpsilon, 0.02, 0.1, 0.5, 1} {
		p := Perceived(y)
		back := InvPerceived(p)
		approx(t, "P^-1(P(y))", back, y, 1e-12)
	}
}

// 三个验收组与报告 cmd/lightreport 保持一致。
func acceptanceGroups() []struct {
	name string
	i1   float64
	i2   float64
} {
	return []struct {
		name string
		i1   float64
		i2   float64
	}{
		{"equal", 0.10, 0.10},
		{"ratio3", 0.18, 0.06},
		{"deep", 0.012, 0.004},
	}
}

func TestConservationSolvedForThreeGroups(t *testing.T) {
	for _, g := range acceptanceGroups() {
		g := g
		t.Run(g.name, func(t *testing.T) {
			r, err := ConservationWeight(g.i1, g.i2, Options{})
			if err != nil {
				t.Fatal(err)
			}
			if !r.Converged {
				t.Fatalf("未收敛: iters=%d residual=%g", r.Iterations, r.Residual)
			}
			// 报告的残差必须真的低于默认容差。
			if r.Residual > DefaultTolResidual {
				t.Fatalf("残差 %g > 容差 %g", r.Residual, DefaultTolResidual)
			}
			// 守恒方程在返回值上成立：P(Out)=P(I1)+P(I2)。
			want := Perceived(g.i1) + Perceived(g.i2)
			approx(t, "P(Out)", r.Brightness, want, 1e-12)
			approx(t, "P(Out) 直接复算", Perceived(r.Out), want, 1e-12)
			// Newton 应当很少迭代（凹函数、解析导数），并与闭式逆解一致。
			if r.Iterations > 10 {
				t.Fatalf("迭代次数过多: %d", r.Iterations)
			}
			if rel := math.Abs(r.Weight-r.ClosedFormWeight) / r.ClosedFormWeight; rel > 1e-12 {
				t.Fatalf("与闭式权重相对差 %g 过大", rel)
			}
			if r.Clipped {
				t.Fatal("验收三组都应在量程内，不应裁切")
			}
		})
	}
}

func TestLinearBaselineIsAverage(t *testing.T) {
	r, err := ConservationWeight(0.10, 0.10, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if r.LinearWeight != 0.5 {
		t.Fatalf("线性归一权重应为 0.5, got %v", r.LinearWeight)
	}
	approx(t, "线性归一输出", r.LinearOut, 0.10, 1e-15)
	approx(t, "线性归一亮度 L*", r.LstarLinear, 37.842430, 1e-5)
	if len(r.WeightsProportional) != 2 ||
		math.Abs(r.WeightsProportional[0]-0.5) > 1e-15 ||
		math.Abs(r.WeightsProportional[1]-0.5) > 1e-15 {
		t.Fatalf("比例归一权重错误: %v", r.WeightsProportional)
	}
}

// 关键不变量：单光源必须逐值恒等、零迭代，路径标记为 identity。
func TestSingleLightPathExactIdentity(t *testing.T) {
	for _, x := range []float64{0, 0.004, 0.1, 0.73} {
		r, err := Compare([]float64{x}, Options{})
		if err != nil {
			t.Fatal(err)
		}
		if r.Weight != 1.0 {
			t.Fatalf("单灯权重必须恰为 1, got %v", r.Weight)
		}
		if r.Out != x { // 刻意用精确 !=，要求 bit 级不变
			t.Fatalf("单灯输出必须原样: got %v want %v", r.Out, x)
		}
		if r.Brightness != Perceived(x) || r.Iterations != 0 || r.Residual != 0 {
			t.Fatalf("单灯记录异常: %+v", r)
		}
		if r.Solver != "single-light-identity" {
			t.Fatalf("应走单灯恒等路径, got %s", r.Solver)
		}
	}
}

func TestEmptyAndAllZeroDeterministic(t *testing.T) {
	r, err := Compare(nil, Options{})
	if err != nil || !r.Converged || r.Out != 0 || r.N != 0 {
		t.Fatalf("空光源结果异常: %+v err=%v", r, err)
	}
	r2, err := Compare([]float64{0, 0}, Options{})
	if err != nil || !r2.Converged || r2.Out != 0 || r2.Weight != 1 {
		t.Fatalf("零强度结果异常: %+v err=%v", r2, err)
	}
}

func TestNegativeAndNonFiniteRejected(t *testing.T) {
	if _, err := Compare([]float64{0.2, -0.1}, Options{}); err == nil {
		t.Fatal("负强度应报错")
	}
	if _, err := Compare([]float64{math.NaN(), 0.1}, Options{}); err == nil {
		t.Fatal("NaN 强度应报错")
	}
	if _, err := Compare([]float64{math.Inf(1), 0.1}, Options{}); err == nil {
		t.Fatal("+Inf 强度应报错")
	}
}

func TestBrightPairClipsToReferenceWhite(t *testing.T) {
	r, err := Compare([]float64{0.9, 0.9}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !r.Clipped {
		t.Fatal("两盏 0.9 的感知和超过 1，应标记裁切")
	}
	if !r.Converged || r.Residual > 1e-12 {
		t.Fatalf("裁切目标也应收敛: %+v", r)
	}
	approx(t, "裁切后 L*", r.Lstar, 100, 1e-9)
	approx(t, "裁切后线性值", r.Out, 1.0, 1e-9)
}

func TestBisectionGuardHandlesArbitraryStart(t *testing.T) {
	// 直接验证带保护求解在很严/很松容差下都给小残差。
	for _, s := range []float64{0.02, 0.24, 1.7} {
		for _, target := range []float64{0.2, 0.99} {
			w, iters, res, ok, _, _ := solveConservation(s, target, Options{}.WithDefaults())
			if !ok || res > 1e-11 {
				t.Fatalf("s=%v target=%v 求解失败 iters=%d res=%g w=%v", s, target, iters, res, w)
			}
		}
	}
}
