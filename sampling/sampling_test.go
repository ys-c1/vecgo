package sampling

import (
	"math"
	"math/rand"
	"testing"

	"vecgo/geom"
)

const eps = 1e-12

func TestHalfPlaneArea(t *testing.T) {
	cases := []struct {
		name string
		hp   HalfPlane
		want float64
	}{
		{"竖直边 x<=0.3", HalfPlane{Ux: 1, Uy: 0, C: 0.3}, 0.3},
		{"水平边 y<=0.3", HalfPlane{Ux: 0, Uy: 1, C: 0.3}, 0.3},
		{"对角线 x+y<=1", HalfPlane{Ux: 1 / math.Sqrt2, Uy: 1 / math.Sqrt2, C: 1 / math.Sqrt2}, 0.5},
		{"外侧半平面", HalfPlane{Ux: 1, Uy: 0, C: -0.1}, 0},
		{"包含全像素", HalfPlane{Ux: 1, Uy: 0, C: 2}, 1},
	}
	for _, tc := range cases {
		if got := tc.hp.Area(); math.Abs(got-tc.want) > eps {
			t.Errorf("%s: 面积 = %v, 期望 %v", tc.name, got, tc.want)
		}
	}
}

func TestStripArea(t *testing.T) {
	// 水平条带 0.2 <= y <= 0.6，面积 0.4
	s := Strip{Ux: 0, Uy: 1, Lo: 0.2, Hi: 0.6}
	if got := s.Area(); math.Abs(got-0.4) > eps {
		t.Errorf("条带面积 = %v, 期望 0.4", got)
	}
	// 条带与半平面的包含关系一致
	p := geom.Point{X: 0.5, Y: 0.3}
	if !s.Contains(p) {
		t.Error("条带内部点被误判为外部")
	}
	if s.Contains(geom.Point{X: 0.5, Y: 0.8}) {
		t.Error("条带外部点被误判为内部")
	}
}

func TestRadicalInverse(t *testing.T) {
	// 基数 2：1→1/2, 2→1/4, 3→3/4, 4→1/8
	want := map[int]float64{1: 0.5, 2: 0.25, 3: 0.75, 4: 0.125}
	for i, w := range want {
		if got := radicalInverse(2, i); math.Abs(got-w) > eps {
			t.Errorf("radicalInverse(2,%d) = %v, 期望 %v", i, got, w)
		}
	}
}

func TestGridBase(t *testing.T) {
	pts := GridBase(16)
	if len(pts) != 16 {
		t.Fatalf("网格点数 = %d, 期望 16", len(pts))
	}
	// 点必须落在 [0,1) 内，且第一个点为中点 (0.125,0.125)
	for _, p := range pts {
		if p.X <= 0 || p.X >= 1 || p.Y <= 0 || p.Y >= 1 {
			t.Fatalf("网格点越界: %v", p)
		}
	}
	if d := math.Abs(pts[0].X-0.125) + math.Abs(pts[0].Y-0.125); d > eps {
		t.Errorf("首点 = %v, 期望 (0.125,0.125)", pts[0])
	}
}

func TestGridRejectsNonSquare(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("n=15 不是完全平方数，应当 panic")
		}
	}()
	GridBase(15)
}

func TestCranleyPattersonUnbiasedMarginals(t *testing.T) {
	// 平移后单点边缘均匀：追踪单个锚点在 10×10 格中的落点频次。
	// 注意不能聚合同一平移下的多个点——共享平移使各点计数强相关。
	rng := rand.New(rand.NewSource(55))
	const bins, reps = 10, 50000
	counts := make([][]int, bins)
	for i := range counts {
		counts[i] = make([]int, bins)
	}
	base := GridBase(4)
	for k := 0; k < reps; k++ {
		pts := cranleyPatterson(base, rng)
		p := pts[0]
		counts[int(p.X*bins)][int(p.Y*bins)]++
	}
	want := float64(reps) / float64(bins*bins)
	for i := 0; i < bins; i++ {
		for j := 0; j < bins; j++ {
			if math.Abs(float64(counts[i][j])-want)/want > 0.15 {
				t.Fatalf("格子(%d,%d) 频次 %d，期望 %.0f", i, j, counts[i][j], want)
			}
		}
	}
}

// bruteForceL2 用细网格数值积分 D₂²，作为 Warnock 闭式公式的独立校验。
func bruteForceL2(pts []geom.Point, res int) float64 {
	ptsBox := func(tx, ty float64) float64 {
		c := 0
		for _, p := range pts {
			if p.X < tx && p.Y < ty {
				c++
			}
		}
		return float64(c) / float64(len(pts))
	}
	sum := 0.0
	for j := 0; j < res; j++ {
		for i := 0; i < res; i++ {
			tx, ty := (float64(i)+0.5)/float64(res), (float64(j)+0.5)/float64(res)
			err := ptsBox(tx, ty) - tx*ty
			sum += err * err
		}
	}
	return sum / float64(res*res)
}

func TestStarL2Warnock(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	for _, n := range []int{4, 16, 64} {
		pts := IID{}.Sample(n, rng)
		got := StarL2Squared(pts)
		want := bruteForceL2(pts, 300)
		if math.Abs(got-want) > 2e-3 {
			t.Errorf("n=%d: Warnock D₂²=%v, 数值积分 %v", n, got, want)
		}
		if got < 0 {
			t.Errorf("n=%d: D₂² 不应为负: %v", n, got)
		}
	}
}

func TestBenchmarkIIDReachesCRB(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	shapes := make([]Shape, 4000)
	for i := range shapes {
		shapes[i] = RandomShape(rng)
	}
	res := BenchmarkCoverage(IID{}, 64, shapes, rand.New(rand.NewSource(99)))
	// 无偏性：平均有符号误差应接近 0。
	if math.Abs(res.BiasMean) > 1e-3 {
		t.Errorf("IID 平均偏差 = %v，应接近 0", res.BiasMean)
	}
	// IID 是有效估计量，实测方差/下界应在 1 附近（蒙特卡洛涨落 ±~sqrt(2/trials)）。
	if r := res.Ratio(); math.Abs(r-1) > 0.08 {
		t.Errorf("IID 方差/下界 = %.4f，应接近 1", r)
	}
}

func TestAllSamplersUnbiased(t *testing.T) {
	rng := rand.New(rand.NewSource(11))
	shapes := make([]Shape, 8000)
	for i := range shapes {
		shapes[i] = RandomShape(rng)
	}
	for _, s := range []Sampler{IID{}, Grid{}, Halton{}} {
		res := BenchmarkCoverage(s, 64, shapes, rand.New(rand.NewSource(123)))
		if math.Abs(res.BiasMean) > 1e-3 {
			t.Errorf("%s 平均偏差 = %v，应接近 0", s.Name(), res.BiasMean)
		}
	}
}
