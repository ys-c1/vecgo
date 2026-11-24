package raster

import (
	"math"
	"testing"

	"vecgo/geom"
)

func TestRadicalInverse2(t *testing.T) {
	cases := []struct {
		i    uint64
		want float64
	}{
		{0, 0}, {1, 0.5}, {2, 0.25}, {3, 0.75}, {4, 0.125}, {6, 0.375},
	}
	for _, c := range cases {
		if got := RadicalInverse2(c.i); got != c.want {
			t.Fatalf("phi2(%d)=%v, 期望 %v", c.i, got, c.want)
		}
	}
}

func TestHammersleySampleKnown(t *testing.T) {
	// N=16 时 i=1 -> (1/16, 1/2)
	x, y := HammersleySample(16, 1)
	if x != 1.0/16.0 || y != 0.5 {
		t.Fatalf("Hammersley(16,1)=(%v,%v)", x, y)
	}
	// N=64 时 i=2 -> (2/64, 1/4)
	x, y = HammersleySample(64, 2)
	if x != 2.0/64.0 || y != 0.25 {
		t.Fatalf("Hammersley(64,2)=(%v,%v)", x, y)
	}
}

func TestWrap01(t *testing.T) {
	if got := wrap01(1.25); got != 0.25 {
		t.Fatalf("wrap(1.25)=%v", got)
	}
	if got := wrap01(-0.25); got != 0.75 {
		t.Fatalf("wrap(-.25)=%v", got)
	}
	if got := wrap01(1); got != 0 {
		t.Fatalf("wrap(1)=%v", got)
	}
}

// TestCoverageVarianceTable 锁定题面要求：三种角度 × 两档样本数，
// 低差异方差都必须严格低于均匀网格，且结果可复现。
func TestCoverageVarianceTable(t *testing.T) {
	rows := CoverageVarianceTable()
	if len(rows) != len(VarianceAngles)*len(VarianceCounts) {
		t.Fatalf("对照格数=%d", len(rows))
	}
	seen := map[[2]float64]bool{}
	for _, r := range rows {
		key := [2]float64{r.AngleDeg, float64(r.Samples)}
		if seen[key] {
			t.Fatalf("重复对照格: %v", key)
		}
		seen[key] = true
		if r.Trials != VarianceTrials || r.Seed == 0 {
			t.Fatalf("实验参数异常: %+v", r)
		}
		if !(r.LDVariance > 0) || !(r.GridVariance > 0) {
			t.Fatalf("方差应为正: %+v", r)
		}
		// 固定种子下最坏比值约 0.44，0.8 的裕量足够宽又能体现“明显更低”。
		if !(r.LDVariance < 0.8*r.GridVariance) {
			t.Fatalf("角度 %.3f N=%d 低差异方差未明显更低: grid=%.6e ld=%.6e ratio=%.3f",
				r.AngleDeg, r.Samples, r.GridVariance, r.LDVariance, r.Ratio())
		}
	}
	// 可复现：再算一次必须逐位相同。
	again := CoverageVarianceTable()
	for i := range rows {
		if rows[i] != again[i] {
			t.Fatalf("方差表不可复现: 第 %d 行 %+v vs %+v", i, rows[i], again[i])
		}
	}
}

// TestFillSampledGridMatchesFill 验证新入口的零平移网格路径与历史 Fill
// 在相同 supersample 下逐像素一致（默认产物逐字节相同的代码级保证）。
func TestFillSampledGridMatchesFill(t *testing.T) {
	scenes := [][][]geom.Point{
		{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}},
		{{{X: 1, Y: 2}, {X: 9, Y: 1}, {X: 8, Y: 9}, {X: 2, Y: 8}}},
	}
	for si, scene := range scenes {
		for _, ss := range []int{1, 2, 4} {
			a, err := New(13, 11)
			if err != nil {
				t.Fatal(err)
			}
			if err := a.Fill(scene, ss); err != nil {
				t.Fatal(err)
			}
			b, err := New(13, 11)
			if err != nil {
				t.Fatal(err)
			}
			if err := b.FillSampled(scene, FillOptions{
				Kind: SampleGrid, Samples: ss * ss,
			}); err != nil {
				t.Fatal(err)
			}
			for i := range a.Pix {
				if a.Pix[i] != b.Pix[i] {
					t.Fatalf("scene%d ss=%d 像素 %d 不一致: Fill=%d FillSampled=%d",
						si, ss, i, a.Pix[i], b.Pix[i])
				}
			}
		}
	}
}

func TestFillSampledRejectsNonSquareGrid(t *testing.T) {
	im, _ := New(4, 4)
	err := im.FillSampled([][]geom.Point{{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 0, Y: 1}}},
		FillOptions{Kind: SampleGrid, Samples: 10})
	if err == nil {
		t.Fatal("非平方网格样本数应报错")
	}
}

// rotatedSquare 生成绕原点旋转 deg 度的 10×10 方形。
func rotatedSquare(deg float64) [][]geom.Point {
	a := deg * math.Pi / 180
	c, s := math.Cos(a), math.Sin(a)
	base := []geom.Point{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}
	out := make([]geom.Point, len(base))
	for i, p := range base {
		out[i] = geom.Point{X: p.X*c - p.Y*s, Y: p.X*s + p.Y*c}
	}
	return [][]geom.Point{out}
}

// TestRenderedEdgeFlickerLower 是端到端佐证：固定视口下对同一角度重复出图，
// 统计每个边缘像素跨图的灰度方差再取平均，LD 应低于均匀网格。
func TestRenderedEdgeFlickerLower(t *testing.T) {
	const trials = 120
	const w, h = 20, 20
	view := &ViewBox{X: -9, Y: -9, W: 24, H: 24}
	for ai, ang := range VarianceAngles {
		scene := rotatedSquare(ang)
		for ci, n := range VarianceCounts {
			seed := deriveSeed(VarianceSeed^0x51ed, uint64(ai), uint64(ci))
			rng := newSplitMix64(seed)
			gAcc := make([]welford, w*h)
			lAcc := make([]welford, w*h)
			for tt := 0; tt < trials; tt++ {
				ux, uy := rng.float64(), rng.float64()
				gm, err := New(w, h)
				if err != nil {
					t.Fatal(err)
				}
				if err := gm.FillSampled(scene, FillOptions{
					Kind: SampleGrid, Samples: n, ShiftX: ux, ShiftY: uy, View: view,
				}); err != nil {
					t.Fatal(err)
				}
				lm, err := New(w, h)
				if err != nil {
					t.Fatal(err)
				}
				if err := lm.FillSampled(scene, FillOptions{
					Kind: SampleLowDiscrepancy, Samples: n, ShiftX: ux, ShiftY: uy, View: view,
				}); err != nil {
					t.Fatal(err)
				}
				for i := range gm.Pix {
					gAcc[i].add(float64(gm.Pix[i]))
					lAcc[i].add(float64(lm.Pix[i]))
				}
			}
			var gSum, lSum float64
			edge := 0
			for i := range gAcc {
				// 以网格均值挑出真正的边缘像素，排除恒定内/外部像素。
				if gAcc[i].mean > 2 && gAcc[i].mean < 253 {
					gSum += gAcc[i].variance()
					lSum += lAcc[i].variance()
					edge++
				}
			}
			if edge == 0 {
				t.Fatalf("角度 %.3f N=%d 未找到边缘像素", ang, n)
			}
			if !(lSum < gSum) {
				t.Fatalf("角度 %.3f N=%d 端到端边缘方差未降低: grid=%.3e ld=%.3e",
					ang, n, gSum/float64(edge), lSum/float64(edge))
			}
			t.Logf("ang=%6.3f N=%2d edgepx=%3d gridPixVar=%.3e ldPixVar=%.3e ratio=%.3f",
				ang, n, edge, gSum/float64(edge), lSum/float64(edge), lSum/gSum)
		}
	}
}
