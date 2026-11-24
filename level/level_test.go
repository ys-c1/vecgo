package level

import (
	"math"
	"testing"
)

func nearRel(t *testing.T, name string, got, want, tol float64) {
	t.Helper()
	den := math.Max(1, math.Abs(want))
	if math.Abs(got-want)/den > tol {
		t.Fatalf("%s = %.12g, want %.12g (rel tol %g)", name, got, want, tol)
	}
}

func TestPositionsEndpointsAndMonotone(t *testing.T) {
	for _, d := range []Dist{Linear, Perceptual, Lab} {
		ts, err := Positions(64, d, 2)
		if err != nil {
			t.Fatal(err)
		}
		if ts[0] != 0 || math.Abs(ts[63]-1) > 1e-12 {
			t.Fatalf("%s 端点错误: %v %v", d, ts[0], ts[63])
		}
		for i := 1; i < len(ts); i++ {
			if ts[i] <= ts[i-1] {
				t.Fatalf("%s 非单调 at %d: %.6g <= %.6g", d, i, ts[i], ts[i-1])
			}
		}
	}
}

func TestPerceptualGapsAreEqual(t *testing.T) {
	ts, _ := Positions(64, Perceptual, 2)
	for i := 0; i < 63; i++ {
		g := ts[i+1]*ts[i+1] - ts[i]*ts[i]
		if math.Abs(g-1.0/63.0) > 1e-12 {
			t.Fatalf("感知间距不相等 i=%d g=%.12g", i, g)
		}
	}
}

func TestDarkVarianceDropsSignificantly(t *testing.T) {
	for _, n := range []int{64, 256} {
		_, _, vLin, err := DarkVariance(n, Linear, 2)
		if err != nil {
			t.Fatal(err)
		}
		mP, meanP, vP, err := DarkVariance(n, Perceptual, 2)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("n=%d m=%d Vlin=%.8e Vperc=%.3e meanP=%.6g ratio=%.3e",
			n, mP, vLin, vP, meanP, vLin/math.Max(vP, 1e-300))
		if !(vP < 1e-24) {
			t.Fatalf("n=%d 感知暗部方差应接近 0, got %.3e", n, vP)
		}
		if ratio := vLin / math.Max(vP, 1e-300); ratio < 1e10 {
			t.Fatalf("n=%d 方差降幅不足: %.3e×", n, ratio)
		}
	}
}

func TestClosedFormPredictionMatches(t *testing.T) {
	for _, n := range []int{64, 256} {
		m, meanCF, vCF := PredictLinearDarkVarianceSq(n)
		m2, meanSum, vSum, err := PredictLinearDarkVariance(n, 2)
		if err != nil {
			t.Fatal(err)
		}
		_, _, vMeas, err := DarkVariance(n, Linear, 2)
		if err != nil {
			t.Fatal(err)
		}
		if m != m2 {
			t.Fatalf("m 不一致 %d %d", m, m2)
		}
		nearRel(t, "mean closed-vs-sum", meanCF, meanSum, 1e-12)
		nearRel(t, "var closed-vs-sum", vCF, vSum, 1e-12)
		nearRel(t, "var closed-vs-measured", vCF, vMeas, 1e-12)
		t.Logf("n=%d m=%d predicted=%.8e measured=%.8e", n, m, vCF, vMeas)
	}
}

func TestLabCrossCheck(t *testing.T) {
	for _, n := range []int{64, 256} {
		tsL, _ := Positions(n, Linear, 0)
		gL, _ := HalfDarkGaps(tsL, Lab, 0)
		vL := Variance(gL)
		tsP, _ := Positions(n, Lab, 0)
		gP, _ := HalfDarkGaps(tsP, Lab, 0)
		vP := Variance(gP)
		t.Logf("LAB n=%d Vlin=%.6f Vlab=%.3e ratio=%.1f", n, vL, vP, vL/math.Max(vP, 1e-300))
		if vP > 1e-20 || vL/vP < 1e8 {
			t.Fatalf("L* 感知分布方差应趋近 0 且显著低于线性: lin=%.6g lab=%.3g", vL, vP)
		}
	}
}

func TestNormalizeAndAlphaValidation(t *testing.T) {
	if d, err := Normalize(""); err != nil || d != Linear {
		t.Fatalf("空串应为 linear")
	}
	if _, err := Normalize("bogus"); err == nil {
		t.Fatal("非法分布应报错")
	}
	if a, err := CleanAlpha(0); err != nil || a != DefaultAlpha {
		t.Fatalf("alpha=0 应回退缺省 2")
	}
	if _, err := CleanAlpha(-1); err == nil {
		t.Fatal("负 alpha 应报错")
	}
	if _, err := Positions(0, Linear, 2); err == nil {
		t.Fatal("n=0 应报错")
	}
}
