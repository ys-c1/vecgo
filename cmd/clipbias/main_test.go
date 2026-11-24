package main

import "testing"

var testPoly = rect{x0: 0, y0: 0, x1: 10, y1: 10}

func TestFCLibraryEquivalence(t *testing.T) {
	if n, d := fcMatchesLibrary(testPoly); n != 0 || d != 0 {
		t.Fatalf("FC 渲染与 raster.Fill 不一致: 差异像素=%d 最大差=%d", n, d)
	}
}

func TestSameGridIdentity(t *testing.T) {
	cases := map[string]rect{
		"contain":        {x0: -1, y0: -1, x1: 11, y1: 11},
		"intersect":      {x0: -1, y0: -1, x1: 9.99, y1: 9.99},
		"intersect_dark": {x0: -1, y0: -1, x1: 9.90, y1: 11},
		"outside":        {x0: 20, y0: 20, x1: 30, y1: 30},
	}
	for name, win := range cases {
		if n, d := sameGridIdentity(testPoly, win); n != 0 || d != 0 {
			t.Fatalf("%s: 同网格下两种顺序不恒等: 差异像素=%d 最大差=%d", name, n, d)
		}
	}
}

func TestContainCaseNoBias(t *testing.T) {
	win := rect{x0: -1, y0: -1, x1: 11, y1: 11}
	rep := runCase("contain", testPoly, win, t.TempDir())
	if rep.predCount != 0 || len(rep.cells) != 0 || rep.maxGray != 0 {
		t.Fatalf("包含类应无偏差: pred=%d measured=%d max=%d", rep.predCount, len(rep.cells), rep.maxGray)
	}
}

func TestOutsideCaseNoBias(t *testing.T) {
	win := rect{x0: 20, y0: 20, x1: 30, y1: 30}
	rep := runCase("outside", testPoly, win, t.TempDir())
	if rep.predCount != 0 || len(rep.cells) != 0 || rep.maxGray != 0 {
		t.Fatalf("完全在外类应无偏差: pred=%d measured=%d max=%d", rep.predCount, len(rep.cells), rep.maxGray)
	}
}

func TestIntersectCasePredictionMatches(t *testing.T) {
	win := rect{x0: -1, y0: -1, x1: 9.99, y1: 9.99}
	rep := runCase("intersect", testPoly, win, t.TempDir())
	if rep.predCount == 0 || rep.predMax == 0 {
		t.Fatal("相交类预测不应为 0")
	}
	// 同量级：实测偏差像素数落在预测的 [0.5,2] 倍
	ratio := float64(len(rep.cells)) / float64(rep.predCount)
	if ratio < 0.5 || ratio > 2 {
		t.Fatalf("偏差像素数与预测不同量级: measured=%d predicted=%d ratio=%.3f", len(rep.cells), rep.predCount, ratio)
	}
	// 小位移案例（δ≈0.02h < h/N=0.125h）：角点两轴各至多翻转 1 个采样，
	// 上界 255(2N-1)/N²，外加 1 个量化量子的截断容差
	cornerBound := 255*(2*ssN-1)/(ssN*ssN) + 255/(ssN*ssN) + 1
	if rep.maxGray > cornerBound {
		t.Fatalf("小位移最大灰度差 %d 超过角点上界 %d", rep.maxGray, cornerBound)
	}
	// 硬上界：任何情况下逐像素差不超过 255
	if rep.maxGray > 255 {
		t.Fatalf("最大灰度差超过 255: %d", rep.maxGray)
	}
	if len(rep.mismatch) != 0 {
		t.Fatalf("存在预测超限像素: %v", rep.mismatch[:min(5, len(rep.mismatch))])
	}
	// 所有差异必须是暗部（FC 未饱和）像素，亮部不应出现偏差
	if bright := len(rep.cells) - len(rep.darkCells); bright != 0 {
		t.Fatalf("亮部(FC=255)出现 %d 个偏差像素，与边缘局部性矛盾", bright)
	}
}

func TestIntersectDarkCase(t *testing.T) {
	win := rect{x0: -1, y0: -1, x1: 9.90, y1: 11}
	rep := runCase("intersect_dark", testPoly, win, t.TempDir())
	if rep.predCount == 0 || len(rep.cells) != rep.predCount || len(rep.mismatch) != 0 {
		t.Fatalf("辅助暗部案例预测/实测不一致: pred=%d measured=%d mismatch=%d",
			rep.predCount, len(rep.cells), len(rep.mismatch))
	}
	// 该几何平移 δ≈0.32h > h/N，属一般位移区间，单边可翻转多个采样；
	// 边缘列 FC 灰度应明确落在暗部（<128），验证间距方差确实统计在暗区上
	if rep.minDarkFC >= 128 {
		t.Fatalf("暗部案例最低 FC 灰度=%d 未落入暗部(<128)", rep.minDarkFC)
	}
	if len(rep.darkCells) < 2 {
		t.Fatal("暗部差异像素不足，无法统计间距方差")
	}
	// 单列边线：行主序间距应恒为 imgW，方差为 0
	if rep.gapVar != 0 {
		t.Fatalf("规则边线间距方差应为 0，实际 %.4f", rep.gapVar)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestShiftRegime(t *testing.T) {
	small := runCase("intersect", testPoly, rect{x0: -1, y0: -1, x1: 9.99, y1: 9.99}, t.TempDir())
	if small.shiftMaxX >= 1.0/ssN || small.shiftMaxY >= 1.0/ssN {
		t.Fatalf("小位移案例平移 %.4fh 应 < h/N=%.4fh", small.shiftMaxX, 1.0/ssN)
	}
	large := runCase("intersect_dark", testPoly, rect{x0: -1, y0: -1, x1: 9.90, y1: 11}, t.TempDir())
	if large.shiftMaxX <= 1.0/ssN {
		t.Fatalf("一般位移案例平移 %.4fh 应 > h/N=%.4fh", large.shiftMaxX, 1.0/ssN)
	}
}

func TestBoundaryKindClassifier(t *testing.T) {
	poly := rect{x0: 0, y0: 0, x1: 10, y1: 10}
	region := rect{x0: 0, y0: 0, x1: 9.9, y1: 10} // 裁剪框边在 x=9.9，多边形边在 x=0/10
	// 靠近 x=9.9 的列（col≈63）应判为裁剪框边界像素
	col := 63
	if got := boundaryKind(region, poly, 30, col, poly); got != "裁剪框边界像素" {
		t.Fatalf("col=%d 应判为裁剪框边界，实际 %s", col, got)
	}
	// 靠近 x=0 的列（col=0）应判为多边形边界像素
	if got := boundaryKind(region, poly, 30, 0, poly); got != "多边形边界像素" {
		t.Fatalf("col=0 应判为多边形边界，实际 %s", got)
	}
}
