package kernel

import (
	"math"
	"path/filepath"
	"testing"
)

const eps = 1e-9

func near(t *testing.T, name string, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Fatalf("%s = %.12g, 期望 %.12g (容差 %g)", name, got, want, tol)
	}
}

// ---- 雅可比 ----

func TestHeroJacobianEntries(t *testing.T) {
	// 先转 45° 再缩一半：J = 0.5 R45。
	j := CompositeJacobian(0.5, 0.5, math.Pi/4)
	v := math.Sqrt2 / 4 // 0.5*cos45 = √2/4 ≈ 0.353553
	near(t, "a", j.A, v, eps)
	near(t, "b", j.B, -v, eps)
	near(t, "c", j.C, v, eps)
	near(t, "d", j.D, v, eps)
	near(t, "det", j.Det(), 0.25, eps)
}

func TestCompositeOrderIsScaleAfterRotate(t *testing.T) {
	// 非均匀缩放 diag(0.5,1) 左乘 R45：第二行不被 0.5 缩放。
	j := CompositeJacobian(0.5, 1, math.Pi/4)
	near(t, "c", j.C, math.Sqrt2/2, eps) // 1*sin45
	near(t, "a", j.A, math.Sqrt2/4, eps) // 0.5*cos45
	near(t, "det", j.Det(), 0.5, eps)
}

func TestSingularValuesAndRatio(t *testing.T) {
	j2 := CompositeJacobian(0.5, 0.5, math.Pi/4)
	lo, hi := j2.SingularValues()
	near(t, "G2 sigmaLo", lo, 0.5, eps)
	near(t, "G2 sigmaHi", hi, 0.5, eps)
	near(t, "G2 kappa", j2.AxisRatio(), 1, 1e-9)

	j3 := CompositeJacobian(0.5, 1, math.Pi/4)
	lo, hi = j3.SingularValues()
	near(t, "G3 sigmaLo", lo, 0.5, eps)
	near(t, "G3 sigmaHi", hi, 1.0, eps)
	near(t, "G3 kappa", j3.AxisRatio(), 2, 1e-9)
	near(t, "G3 major angle", j3.MajorAngle(), math.Pi/4, 1e-9)
}

// ---- 面积守恒轴 ----

func TestConservedEllipseAreaOne(t *testing.T) {
	for _, k := range []float64{1, 2, 3.7, 10} {
		e := ConservedEllipse(k, 0.3)
		near(t, "pi*a*b", e.GeometricArea(), 1, 1e-12)
		near(t, "a/b", e.Long/e.Short, k, 1e-12)
	}
}

func TestConservedAxesProductConvention(t *testing.T) {
	a, b := ConservedAxesProduct(4)
	near(t, "ab", a*b, 1, 1e-12)
	near(t, "a/b", a/b, 4, 1e-12)
}

func TestAreaErrorBoundTheorem(t *testing.T) {
	// 性质校验：随机轴比 + 随机扰动，实测残差必须落在解析上界内。
	seed := 1
	rand := func() float64 { // 确定性伪随机 [0,1)
		seed = (seed*1103515245 + 12345) & 0x7fffffff
		return float64(seed) / float64(0x7fffffff)
	}
	for i := 0; i < 200; i++ {
		k := 1 + 9*rand()
		e0 := ConservedEllipse(k, 0)
		epsL := 0.5 * e0.Long * rand()
		epsS := 0.5 * e0.Short * rand()
		dl := (2*rand() - 1) * epsL
		ds := (2*rand() - 1) * epsS
		used := Ellipse{Long: e0.Long + dl, Short: e0.Short + ds}
		got := AreaResidual(used)
		bound := AreaErrorBound(epsL, epsS, e0.Long, e0.Short)
		if got > bound+1e-9 {
			t.Fatalf("第 %d 次抽样面积残差 %.6g 超出上界 %.6g", i, got, bound)
		}
	}
}

func TestOffendingAxis(t *testing.T) {
	e0 := ConservedEllipse(2, 0)
	badL := e0
	badL.Long *= 2
	if got := OffendingAxis(badL, e0.Long, e0.Short, 0.02); got != "long" {
		t.Fatalf("应判定长轴偏差, 得到 %s", got)
	}
	badS := e0
	badS.Short *= 2
	if got := OffendingAxis(badS, e0.Long, e0.Short, 0.02); got != "short" {
		t.Fatalf("应判定短轴偏差, 得到 %s", got)
	}
	if got := OffendingAxis(e0, e0.Long, e0.Short, 0.02); got != "none" {
		t.Fatalf("守恒轴应无偏差, 得到 %s", got)
	}
}

// ---- 解析覆盖率 ----

func TestDiskCapSymmetry(t *testing.T) {
	near(t, "q(-1)", DiskCapFraction(-1), 1, eps)
	near(t, "q(0)", DiskCapFraction(0), 0.5, eps)
	near(t, "q(1)", DiskCapFraction(1), 0, eps)
	for _, h := range []float64{-0.8, -0.2, 0.37, 0.9} {
		near(t, "球冠互补", DiskCapFraction(h)+DiskCapFraction(-h), 1, 1e-12)
	}
	// 外接矩形界必须始终大于真实球冠占比。
	for _, h := range []float64{0.0, 0.2, 0.5, 0.8, 0.95} {
		if CapRectangleBound(h)+1e-12 < DiskCapFraction(h) {
			t.Fatalf("h=%g 时矩形界 %.5f 小于真实球冠 %.5f", h, CapRectangleBound(h), DiskCapFraction(h))
		}
	}
}

// ---- 三组对照的端到端验收 ----

func TestDefaultReportAcceptance(t *testing.T) {
	rep := DefaultReport()
	if len(rep.Groups) != 3 {
		t.Fatalf("应有三组对照, 实际 %d", len(rep.Groups))
	}
	byID := map[string]*GroupReport{}
	for _, g := range rep.Groups {
		byID[g.ID] = g
		if !g.BoundHolds {
			t.Fatalf("%s 面积实测 %.4g 未落在上界 %.4g 内", g.ID, g.AreaResidual, g.AreaErrorBound)
		}
		if !g.RatioBoundHolds {
			t.Fatalf("%s 轴比误差 %.4g 未落在上界 %.4g 内", g.ID, g.RatioError, g.RatioErrorBound)
		}
		if !g.GrayBoundHolds {
			t.Fatalf("%s 灰度实测 %.1f 未落在上界 %.1f 内", g.ID, g.Edge.MaxSideDev, g.GrayErrorBound)
		}
		if g.Image == nil || g.Image.W != 64 {
			t.Fatalf("%s 缺少渲染图", g.ID)
		}
	}

	g1, g2, g3 := byID["G1"], byID["G2"], byID["G3"]

	// 守恒组成立且核面积严格等于一。
	for _, g := range []*GroupReport{g1, g2} {
		if !g.ConservesArea {
			t.Fatalf("%s 应满足面积守恒", g.ID)
		}
		near(t, g.ID+" 核面积", g.UsedAxes.GeometricArea, 1, 1e-9)
		if g.Edge.MaxSideDev > 2 {
			t.Fatalf("%s 边缘两侧灰度差 %.0f 超过两档", g.ID, g.Edge.MaxSideDev)
		}
		if g.Edge.FogPixels != 0 {
			t.Fatalf("%s 灰雾应消失, 仍有 %d px", g.ID, g.Edge.FogPixels)
		}
	}

	// 题目主场景的雅可比关键值落在 G2。
	near(t, "G2 det", g2.Det, 0.25, eps)

	// 负对照 G3：守恒不成立、偏差在长轴、灰雾与灰度差都被实测到。
	if g3.ConservesArea {
		t.Fatal("G3 应不满足面积守恒")
	}
	if g3.OffendingAxis != "long" {
		t.Fatalf("G3 偏差应落在长轴, 实际 %s", g3.OffendingAxis)
	}
	if g3.Edge.MaxSideDev <= 2 {
		t.Fatalf("G3 应有明显边缘灰度差, 实际 %.0f", g3.Edge.MaxSideDev)
	}
	if g3.Edge.FogPixels <= 0 {
		t.Fatal("G3 应统计到灰雾像素")
	}

	// 像素数回归钉死：三组灰雾面积各一个确定数。
	wantFog := map[string]int{"G1": 0, "G2": 0, "G3": 256}
	for id, n := range wantFog {
		if got := byID[id].Edge.FogPixels; got != n {
			t.Fatalf("%s 灰雾像素 = %d, 期望 %d", id, got, n)
		}
	}
}

func TestHeroBeforeFixHasFogThenFixed(t *testing.T) {
	rep := DefaultReport()
	before := rep.Hero.Before.Edge
	if before.FogPixels == 0 || before.MaxSideDev <= 2 {
		t.Fatalf("修复前应复现灰雾, fog=%d dev=%.0f", before.FogPixels, before.MaxSideDev)
	}
	if before.FogPixels != 128 {
		t.Fatalf("修复前灰雾像素 = %d, 期望 128", before.FogPixels)
	}
	if rep.Hero.InvDet != 4 {
		t.Fatalf("逆面积因子应为 4, 实际 %.1f", rep.Hero.InvDet)
	}
	// 修复后即 G2。
	if g2 := rep.Groups[1]; g2.Edge.FogPixels != 0 || g2.Edge.MaxSideDev > 2 {
		t.Fatal("修复后灰雾应消失且灰度差≤2")
	}
}

func TestRenderAndSavePGM(t *testing.T) {
	rep := DefaultReport()
	path := filepath.Join(t.TempDir(), "g2.pgm")
	if err := rep.Groups[1].Image.SavePGM(path); err != nil {
		t.Fatal(err)
	}
}

func TestComboScanRawVsConserved(t *testing.T) {
	rows := ComboScan()
	if len(rows) != 4 {
		t.Fatalf("应有 4 个变换组合扫描行, 实际 %d", len(rows))
	}
	for _, r := range rows {
		near(t, r.Name+" 守恒面积", r.Conserved.GeometricArea, 1, 1e-12)
		if r.ConsEdge.FogPixels != 0 || r.ConsEdge.MaxSideDev > 2 {
			t.Fatalf("%s 守恒核应无灰雾且灰度差≤2, fog=%d dev=%.0f",
				r.Name, r.ConsEdge.FogPixels, r.ConsEdge.MaxSideDev)
		}
	}
	// 题目复合变换：det=.25，原始足迹面积因子=4，且应复现灰雾。
	hero := rows[2]
	near(t, "hero det", hero.Det, 0.25, eps)
	near(t, "hero raw area factor", hero.RawAreaProd, 4, eps)
	if hero.RawEdge.FogPixels == 0 {
		t.Fatal("未归一化的复合足迹核应产生灰雾")
	}
	// 非均匀组应被拉成长短轴不等（κ=2）。
	if rows[3].Kappa < 1.99 {
		t.Fatalf("非均匀组 κ 应≈2, 实际 %.3f", rows[3].Kappa)
	}
}
