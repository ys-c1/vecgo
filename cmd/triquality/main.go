package main

// 三角化质量实验：对比耳切（EarClip，基线）与最坏情况最优（OptimalTriangulation）。
//
// 输出：
//   - 三组多边形的最小角、最差三角形面积比、覆盖率误差（表格）
//   - 改进前后最差三角形的对照 SVG（artifacts/tri-*.svg）

import (
	"fmt"
	"math"
	"os"
	"path/filepath"

	"vecgo/geom"
)

type scene struct {
	name string
	poly []geom.Point
}

type row struct {
	name         string
	polyMinAngle float64 // 多边形最小内角，度
	bound        float64 // θ* 上界，度
	earMinAngle  float64 // 度
	optMinAngle  float64 // 度
	earQuality   float64
	optQuality   float64
	earCoverage  float64 // 子像素量化面积误差（主指标，与图中标签一致）
	optCoverage  float64
	earPixCov    float64 // 4×SSAA 逐像素覆盖率误差（对照指标）
	optPixCov    float64
	earWorstCov  float64
	optWorstCov  float64
	earWorstTri  geom.Triangle
	optWorstTri  geom.Triangle
	qualityCeil  float64
	earTris      int
	optTris      int
}

const levels = geom.SubpixelLevels

func main() {
	// 三组多边形先在模型空间构造，再等比放进 64 像素视口：
	// 覆盖率误差按像素子像素量化度量，必须在设备空间里算才有物理意义。
	scenes := []scene{
		{"ellipse 14 边", geom.FitToViewport(geom.EllipsePolygon(2.0, 0.30, 14), 64, 4)},
		{"星形 (R=1, r=0.45)", geom.FitToViewport(geom.StarPolygon(1.0, 0.45), 64, 4)},
		{"S 形走廊", geom.FitToViewport(geom.CorridorPolygon(1.7, 0.45, 14, 36), 64, 4)},
	}

	rows := make([]row, 0, len(scenes))
	for _, sc := range scenes {
		r, err := analyze(sc)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", sc.name, err)
			os.Exit(1)
		}
		rows = append(rows, r)
	}

	outDir := "artifacts"
	if len(os.Args) > 1 {
		outDir = os.Args[1]
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "创建输出目录失败:", err)
		os.Exit(2)
	}

	fmt.Printf("%-20s %8s %8s %14s %14s %12s %12s %12s %12s\n",
		"多边形", "α(°)", "θ*上界", "最小角 ear→opt", "面积比 ear→opt",
		"覆盖率ear", "覆盖率opt", "最差ear", "最差opt")
	for _, r := range rows {
		fmt.Printf("%-20s %8.2f %8.2f %6.2f→%6.2f %6.3f→%6.3f %11.3f%% %11.3f%% %11.2f%% %11.2f%%\n",
			r.name, r.polyMinAngle, r.bound,
			r.earMinAngle, r.optMinAngle,
			r.earQuality, r.optQuality,
			r.earCoverage*100, r.optCoverage*100,
			r.earWorstCov*100, r.optWorstCov*100)
	}

	fmt.Println()
	fmt.Printf("%-20s %12s %12s %12s\n", "多边形", "q(θ*上界)", "ear 达界比", "opt 达界比")
	for _, r := range rows {
		fmt.Printf("%-20s %12.3f %11.1f%% %11.1f%%\n",
			r.name, r.qualityCeil,
			angleRatio(r.earMinAngle, r.bound)*100,
			angleRatio(r.optMinAngle, r.bound)*100)
	}

	fmt.Println()
	fmt.Printf("%-20s %26s %26s\n", "多边形", "像素覆盖率误差 ear→opt", "下降倍数")
	for _, r := range rows {
		fmt.Printf("%-20s %12.3f%% → %10.3f%% %18.2fx\n",
			r.name, r.earPixCov*100, r.optPixCov*100, r.earPixCov/r.optPixCov)
	}

	for i, r := range rows {
		base := filepath.Join(outDir, fmt.Sprintf("tri-%d", i+1))
		if err := saveComparison(base, scenes[i].poly, r); err != nil {
			fmt.Fprintf(os.Stderr, "导出对比图失败 %s: %v\n", base, err)
			os.Exit(3)
		}
		fmt.Printf("已导出 %s-ear.svg / %s-opt.svg / %s-ear.png / %s-opt.png\n", base, base, base, base)
		fmt.Printf("最差三角形特写 %s-ear-closeup.png / %s-opt-closeup.png\n", base, base)
	}
}

func angleRatio(a, b float64) float64 {
	if b <= 0 {
		return 0
	}
	return a / b
}

func analyze(sc scene) (row, error) {
	r := row{name: sc.name}

	alpha, _, err := geom.MinInteriorAngle(sc.poly)
	if err != nil {
		return r, err
	}
	bound, err := geom.MaxMinAngleBound(sc.poly)
	if err != nil {
		return r, err
	}
	r.polyMinAngle = deg(alpha)
	r.bound = deg(bound)
	r.qualityCeil = geom.QualityCeiling(bound)

	ear, err := geom.EarClip(sc.poly)
	if err != nil {
		return r, fmt.Errorf("耳切失败: %w", err)
	}
	opt, err := geom.OptimalTriangulation(sc.poly)
	if err != nil {
		return r, fmt.Errorf("最优三角化失败: %w", err)
	}
	r.earTris, r.optTris = len(ear), len(opt)

	r.earMinAngle = deg(geom.MeshMinAngle(ear))
	r.optMinAngle = deg(geom.MeshMinAngle(opt))
	r.earQuality = geom.MeshMinQuality(ear)
	r.optQuality = geom.MeshMinQuality(opt)

	r.earCoverage, err = geom.CoverageError(ear, levels)
	if err != nil {
		return r, err
	}
	r.optCoverage, err = geom.CoverageError(opt, levels)
	if err != nil {
		return r, err
	}
	r.earPixCov = geom.PixelCoverageError(ear, geom.PixelFrame, geom.PixelSamples)
	r.optPixCov = geom.PixelCoverageError(opt, geom.PixelFrame, geom.PixelSamples)
	r.earWorstCov = geom.WorstCoverageError(ear, levels)
	r.optWorstCov = geom.WorstCoverageError(opt, levels)
	r.earWorstTri = geom.WorstTriangle(ear)
	r.optWorstTri = geom.WorstTriangle(opt)

	if err := checkInvariants(sc.poly, ear, opt, r); err != nil {
		return r, err
	}
	return r, nil
}

func checkInvariants(poly []geom.Point, ear, opt []geom.Triangle, r row) error {
	pa := math.Abs(geom.SignedArea(poly))
	for _, m := range []struct {
		name string
		tris []geom.Triangle
	}{{"ear", ear}, {"opt", opt}} {
		if d := math.Abs(geom.MeshArea(m.tris) - pa); d > 1e-9*math.Max(1, pa) {
			return fmt.Errorf("%s 面积不守恒: %.3e", m.name, d)
		}
	}
	if r.optMinAngle+1e-9 < r.earMinAngle {
		return fmt.Errorf("改进后最小角反而变小: %.4f < %.4f", r.optMinAngle, r.earMinAngle)
	}
	if r.optQuality+1e-12 < r.earQuality {
		return fmt.Errorf("改进后最差面积比反而变小: %.6f < %.6f", r.optQuality, r.earQuality)
	}
	if r.optMinAngle > r.bound+1e-7 {
		return fmt.Errorf("最小角 %.4f 超过上界 %.4f", r.optMinAngle, r.bound)
	}
	if r.optCoverage > r.earCoverage+1e-12 || r.optWorstCov > r.earWorstCov+1e-12 {
		return fmt.Errorf("覆盖率误差未随面积比改善而下降: mean %.6f→%.6f worst %.6f→%.6f",
			r.earCoverage, r.optCoverage, r.earWorstCov, r.optWorstCov)
	}
	if r.optPixCov >= r.earPixCov {
		return fmt.Errorf("像素覆盖率误差未下降: %.6f→%.6f", r.earPixCov, r.optPixCov)
	}
	return nil
}

func deg(rad float64) float64 { return rad * 180 / math.Pi }
