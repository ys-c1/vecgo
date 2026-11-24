package coverage

import (
	"math"
	"math/rand"
	"testing"

	"vecgo/geom"
)

func TestVarianceAndInvert(t *testing.T) {
	// p=0.5 时 p(1-p)/n = 1/(4n)。
	if got := Variance(0.5, 2500); math.Abs(got-1e-4) > 1e-15 {
		t.Fatalf("Variance(0.5,2500)=%.17g", got)
	}
	if got := WorstVariance(2500); math.Abs(got-1e-4) > 1e-15 {
		t.Fatalf("WorstVariance(2500)=%.17g", got)
	}
	n, err := RequiredSamples(1e-4)
	if err != nil || n != 2500 {
		t.Fatalf("RequiredSamples(1e-4)=%d,%v", n, err)
	}
	// 反解值必须真的让上界 <= 目标，而 n-1 不行（边界情形）。
	if WorstVariance(n) > 1e-4 {
		t.Fatal("反解 n 未满足方差目标")
	}
	if WorstVariance(n-1) <= 1e-4 {
		t.Fatal("反解 n 不是最小整数")
	}
	nSE, err := RequiredSamplesSE(0.01)
	if err != nil || nSE != 2500 {
		t.Fatalf("RequiredSamplesSE(0.01)=%d,%v", nSE, err)
	}
	if _, err := RequiredSamples(0); err == nil {
		t.Fatal("vmax=0 应报错")
	}
}

func TestNormalQuantile(t *testing.T) {
	for prob, want := range map[float64]float64{0.025: -1.959964, 0.5: 0, 0.975: 1.959964} {
		got := NormalQuantile(prob)
		if math.Abs(got-want) > 2e-5 {
			t.Fatalf("NormalQuantile(%v)=%.6f want %.6f", prob, got, want)
		}
	}
}

func TestChiSquareQuantileKnownDF49(t *testing.T) {
	// 教科书表：df=49 时 q0.025≈31.555, q0.975≈70.222。
	lo := ChiSquareQuantile(0.025, 49)
	hi := ChiSquareQuantile(0.975, 49)
	if math.Abs(lo-31.55) > 0.1 {
		t.Fatalf("chi2 q0.025 df49=%.3f", lo)
	}
	if math.Abs(hi-70.22) > 0.15 {
		t.Fatalf("chi2 q0.975 df49=%.3f", hi)
	}
}

func TestEstimateTriangleHalfCoverage(t *testing.T) {
	tri := [][]geom.Point{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}}}
	rng := rand.New(rand.NewSource(42))
	hat, hits := Estimate(tri, 64, 64, 120000, rng)
	if hits < 0 || hits > 120000 {
		t.Fatal("hits 越界")
	}
	if math.Abs(hat-0.5) > 0.012 { // ~7 个标准误，固定种子下稳定
		t.Fatalf("三角形覆盖率估计 %.4f 偏离 0.5 过多", hat)
	}
}

func TestUniformBinsLookUniform(t *testing.T) {
	// 固定种子的大样本均匀采样，χ² 不应越过 df=99 的 0.999 临界（约 148）。
	b := NewBinAccumulator(10, 10)
	rng := rand.New(rand.NewSource(7))
	const n = 200000
	for i := 0; i < n; i++ {
		b.Add(rng.Float64()*64, rng.Float64()*64, 64, 64)
	}
	rep := b.Report(3)
	if rep.N != n {
		t.Fatalf("计数 N=%d", rep.N)
	}
	if rep.Chi2 > 149 {
		t.Fatalf("均匀采样 χ²=%.1f 异常偏大", rep.Chi2)
	}
	if rep.PValue < 1e-3 {
		t.Fatalf("均匀采样 p=%.3g 异常偏小", rep.PValue)
	}
}

func TestClusterIsDetectedAsUneven(t *testing.T) {
	// 同样本量下，聚类采样的空间 χ² 必须远超均匀临界，且最热格相对期望
	// 明显偏高，保证“指出不均在哪”的诊断链路有效。
	b := NewBinAccumulator(10, 10)
	c := &Cluster{CentralFrac: 0.5, RadiusFrac: 0.12}
	rng := rand.New(rand.NewSource(9))
	const reps, n = 400, 2500
	for r := 0; r < reps; r++ {
		c.Begin(rng, 64, 64)
		for k := 0; k < n; k++ {
			x, y := c.Point(rng)
			b.Add(x, y, 64, 64)
		}
	}
	rep := b.Report(3)
	if rep.Chi2 < 200 {
		t.Fatalf("聚类采样 χ²=%.1f 未被识别为不均", rep.Chi2)
	}
	if len(rep.Hot) == 0 || rep.Hot[0].RelDev < 0.5 {
		t.Fatalf("最热格相对偏差过小: %+v", rep.Hot)
	}
}
