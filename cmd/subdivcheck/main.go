// Command subdivcheck 复现“中点细分两次后面积总和变化”的完整分析：
// 生成三组三角形的面积差/上界/细分次数表、细分前后覆盖图、
// 覆盖像素核对以及含证明的中文报告，全部写入 artifacts/。
//
// 用法：go run ./cmd/subdivcheck [输出目录]
package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"vecgo/raster"
	"vecgo/subdiv"
)

const (
	width  = 160
	height = 160
	ss     = 4
	res    = 1.0 / 256.0 // 坐标分辨率 ρ，单位 px（24.8 定点的小数精度）
	budget = 0.01        // 可接受面积误差预算：初始面积的 1%
	maxN   = 8
)

type groupDef struct {
	ID   string
	Name string
	T    subdiv.Triangle
}

var groups = []groupDef{
	{"g1", "第一组", subdiv.Triangle{{X: 53.928, Y: 43.3}, {X: 13.988, Y: 109.108}, {X: 113.543, Y: 142.499}}},
	{"g2", "第二组", subdiv.Triangle{{X: 41.344, Y: 10.936}, {X: 37.952, Y: 149.017}, {X: 137.329, Y: 36.22}}},
	{"g3", "第三组", subdiv.Triangle{{X: 25.698, Y: 145.976}, {X: 127.338, Y: 91.466}, {X: 25.684, Y: 47.895}}},
}

type levelRow struct {
	N                int     `json:"n"`
	Triangles        int     `json:"triangles"`
	AreaExact        float64 `json:"area_exact_px2"`
	AreaSnappedTotal float64 `json:"area_snapped_total_px2"`
	DeltaArea        float64 `json:"delta_area_px2"`
	AbsDelta         float64 `json:"abs_delta_area_px2"`
	BoundPx2         float64 `json:"bound_3rhoLcap_4n_px2"`
	WithinBound      bool    `json:"within_bound"`
	BudgetPx2        float64 `json:"budget_px2"`
	BoundAcceptable  bool    `json:"bound_acceptable_under_budget"`
}

type covCheck struct {
	OnAnyN1          int  `json:"on_any_n1"`
	OnAnyN2          int  `json:"on_any_n2"`
	AnyCountDiff     int  `json:"any_count_diff_n2_minus_n1"`
	On128N1          int  `json:"on_ge128_n1"`
	On128N2          int  `json:"on_ge128_n2"`
	CountDiff128     int  `json:"count_diff_ge128_n2_minus_n1"`
	BitmapDiffPixels int  `json:"bitmap_diff_pixels_n1_vs_n2"`
	ExactBitmapDiff  int  `json:"exact_bitmap_diff_pixels_n1_vs_n2"`
	CountCheckZero   bool `json:"count_check_diff_equals_zero"`
}

type groupReport struct {
	ID            string       `json:"id"`
	Name          string       `json:"name"`
	VerticesPx    [][2]float64 `json:"vertices_px"`
	Area0Px2      float64      `json:"area0_px2"`
	LongestEdgePx float64      `json:"longest_edge_L0_px"`
	EdgeCapPx     float64      `json:"uniform_edge_cap_Lcap_px"`
	BudgetPx2     float64      `json:"budget_1pct_area_px2"`
	NAcceptable   int          `json:"n_acceptable_by_bound"`
	Levels        []levelRow   `json:"levels"`
	Coverage      covCheck     `json:"coverage_check"`
}

type summary struct {
	ResolutionPx      float64       `json:"coordinate_resolution_rho_px"`
	ResolutionExpr    string        `json:"resolution_expr"`
	CoordErrorCapPx   float64       `json:"rounding_error_cap_rho_over_2_px"`
	Units             string        `json:"units"`
	CanvasPx          [2]int        `json:"canvas_px"`
	Supersample       int           `json:"supersample"`
	BudgetFraction    float64       `json:"budget_fraction_of_area0"`
	CommonNAcceptable int           `json:"common_n_acceptable"`
	PixelCheckAllZero bool          `json:"pixel_check_all_zero"`
	Groups            []groupReport `json:"groups"`
}

func main() {
	outDir := "artifacts"
	if len(os.Args) > 1 {
		outDir = os.Args[1]
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fatal(err)
	}

	sum := summary{
		ResolutionPx:      res,
		ResolutionExpr:    "1/256 px = 0.00390625 px",
		CoordErrorCapPx:   res / 2,
		Units:             "坐标/边长/分辨率单位为像素 px；面积与上界单位为平方像素 px²",
		CanvasPx:          [2]int{width, height},
		Supersample:       ss,
		BudgetFraction:    budget,
		CommonNAcceptable: 1 << 30,
		PixelCheckAllZero: true,
	}

	csvRows := [][]string{{
		"group", "n", "triangles_4n", "area_exact_px2", "area_snapped_total_px2",
		"delta_area_px2", "abs_delta_px2", "bound_3rhoLcap_4n_px2", "within_bound",
		"budget_1pct_area_px2", "bound_acceptable",
	}}

	var pixelLines []string
	pixelLines = append(pixelLines,
		"覆盖像素核对（固定坐标映射，1 坐标单位 = 1 px，4x4 超采样）",
		fmt.Sprintf("坐标分辨率 rho = 1/256 px = %.8f px；阈值 128 与 >0 两种口径", res),
		"")

	for _, g := range groups {
		a0 := subdiv.Area(g.T)
		l0 := subdiv.LongestEdge(g.T)
		lcap := subdiv.EdgeCap(l0, res)
		budgetAbs := budget * a0
		gr := groupReport{
			ID:            g.ID,
			Name:          g.Name,
			VerticesPx:    vertices(g.T),
			Area0Px2:      a0,
			LongestEdgePx: l0,
			EdgeCapPx:     lcap,
			BudgetPx2:     budgetAbs,
			NAcceptable:   subdiv.AcceptableN(res, lcap, budgetAbs),
		}
		if gr.NAcceptable < sum.CommonNAcceptable {
			sum.CommonNAcceptable = gr.NAcceptable
		}

		for n := 0; n <= maxN; n++ {
			exact := subdiv.Subdivide(g.T, n)
			snapped := subdiv.SnapAll(exact, res)
			areaSnap := subdiv.TotalArea(snapped)
			delta := areaSnap - a0
			bound := subdiv.TotalBound(n, res, lcap)
			row := levelRow{
				N:                n,
				Triangles:        len(snapped),
				AreaExact:        a0,
				AreaSnappedTotal: areaSnap,
				DeltaArea:        delta,
				AbsDelta:         math.Abs(delta),
				BoundPx2:         bound,
				WithinBound:      math.Abs(delta) <= bound+1e-12,
				BudgetPx2:        budgetAbs,
				BoundAcceptable:  bound <= budgetAbs,
			}
			gr.Levels = append(gr.Levels, row)
			csvRows = append(csvRows, []string{
				g.ID, itoa(n), itoa(len(snapped)),
				f6(a0, 6), f6(areaSnap, 6), f6(delta, 6), f6(math.Abs(delta), 6),
				f6(bound, 6), yesno(row.WithinBound), f6(budgetAbs, 4), yesno(row.BoundAcceptable),
			})
		}

		// 覆盖图：n=0（细分前）、n=1、n=2（细分两次后），全部量化到同一分辨率。
		ims := map[int]*raster.Image{}
		for _, n := range []int{0, 1, 2} {
			tris := subdiv.SnapAll(subdiv.Subdivide(g.T, n), res)
			im, err := subdiv.Coverage(tris, width, height, ss)
			if err != nil {
				fatal(err)
			}
			base := filepath.Join(outDir, fmt.Sprintf("coverage_%s_%s", g.ID, stageName(n)))
			if err := im.SavePGM(base + ".pgm"); err != nil {
				fatal(err)
			}
			if err := im.SavePNG(base + ".png"); err != nil {
				fatal(err)
			}
			mesh := overlayMesh(im, tris)
			if err := saveRGBA(mesh, filepath.Join(outDir, fmt.Sprintf("mesh_%s_n%d.png", g.ID, n))); err != nil {
				fatal(err)
			}
			ims[n] = im
		}

		im1, im2 := ims[1], ims[2]
		ex1, _ := subdiv.Coverage(subdiv.Subdivide(g.T, 1), width, height, ss)
		ex2, _ := subdiv.Coverage(subdiv.Subdivide(g.T, 2), width, height, ss)
		cc := covCheck{
			OnAnyN1:          im1.CountOn(1),
			OnAnyN2:          im2.CountOn(1),
			AnyCountDiff:     im2.CountOn(1) - im1.CountOn(1),
			On128N1:          im1.CountOn(128),
			On128N2:          im2.CountOn(128),
			CountDiff128:     im2.CountOn(128) - im1.CountOn(128),
			BitmapDiffPixels: subdiv.BitmapDifferentPixels(im1, im2),
			ExactBitmapDiff:  subdiv.BitmapDifferentPixels(ex1, ex2),
		}
		cc.CountCheckZero = cc.CountDiff128 == 0 && cc.AnyCountDiff == 0
		if !cc.CountCheckZero || cc.BitmapDiffPixels != 0 {
			sum.PixelCheckAllZero = false
		}
		gr.Coverage = cc
		sum.Groups = append(sum.Groups, gr)

		pixelLines = append(pixelLines,
			fmt.Sprintf("%s %s: 任意覆盖 n1=%d n2=%d 差=%d；threshold>=128 n1=%d n2=%d 差=%d；位图不同像素=%d；精确细分位图差=%d => %s",
				g.ID, g.Name, cc.OnAnyN1, cc.OnAnyN2, cc.AnyCountDiff,
				cc.On128N1, cc.On128N2, cc.CountDiff128,
				cc.BitmapDiffPixels, cc.ExactBitmapDiff, map[bool]string{true: "PASS(差值为0)", false: "FAIL"}[cc.CountCheckZero]))
	}
	pixelLines = append(pixelLines, "")
	pixelLines = append(pixelLines, fmt.Sprintf("三组 n2-n1 覆盖像素数差值全部为 0：%v", sum.PixelCheckAllZero))

	// 三张“总览条带”：左到右 g1/g2/g3。同时写一份规范命名，便于按固定文件名取用。
	if err := writeStrip(outDir, "coverage_before_all.png", groups, 0); err != nil {
		fatal(err)
	}
	if err := writeStrip(outDir, "coverage_after_all.png", groups, 2); err != nil {
		fatal(err)
	}
	if err := writeStrip(outDir, "coverage_before.png", groups, 0); err != nil {
		fatal(err)
	}
	if err := writeStrip(outDir, "coverage_after.png", groups, 2); err != nil {
		fatal(err)
	}

	// CSV
	cf, err := os.Create(filepath.Join(outDir, "three_groups_table.csv"))
	if err != nil {
		fatal(err)
	}
	w := csv.NewWriter(cf)
	_ = w.WriteAll(csvRows)
	w.Flush()
	cf.Close()

	// JSON
	jb, _ := json.MarshalIndent(sum, "", "  ")
	if err := os.WriteFile(filepath.Join(outDir, "summary.json"), append(jb, '\n'), 0o644); err != nil {
		fatal(err)
	}

	// pixel_check.txt
	if err := os.WriteFile(filepath.Join(outDir, "pixel_check.txt"),
		[]byte(strings.Join(pixelLines, "\n")+"\n"), 0o644); err != nil {
		fatal(err)
	}

	// 分辨率扫描（n=2，最终量化协议）
	sweepRes := []float64{1, 0.5, 0.25, 0.1, 1.0 / 16, 1.0 / 64, 1.0 / 256}
	sweep := resolutionSweep(groups, sweepRes)

	// 粗网格递归取整复现（动机现象，含覆盖差异警示）
	coarse := coarseRecursiveDemo(groups)

	report := buildReport(sum, groups, sweepRes, sweep, coarse, pixelLines)
	if err := os.WriteFile(filepath.Join(outDir, "subdivision_report.md"), []byte(report), 0o644); err != nil {
		fatal(err)
	}

	fmt.Print(strings.Join(pixelLines, "\n") + "\n")
	fmt.Printf("\n产物已写入 %s/\n", outDir)
	if !sum.PixelCheckAllZero {
		fmt.Fprintln(os.Stderr, "覆盖像素核对未全部为零")
		os.Exit(1)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "subdivcheck:", err)
	os.Exit(1)
}

func itoa(n int) string { return strconv.Itoa(n) }
func f6(x float64, prec int) string {
	return strconv.FormatFloat(x, 'f', prec, 64)
}
func yesno(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func vertices(t subdiv.Triangle) [][2]float64 {
	return [][2]float64{{t[0].X, t[0].Y}, {t[1].X, t[1].Y}, {t[2].X, t[2].Y}}
}

func stageName(n int) string {
	switch n {
	case 0:
		return "before_n0"
	case 1:
		return "sub1_n1"
	case 2:
		return "after2_n2"
	default:
		return fmt.Sprintf("n%d", n)
	}
}
