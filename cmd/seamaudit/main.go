// Command seamaudit 复现“裁剪接缝像素被算两次”的完整对账：
// 端点规则逐类核对、三类裁剪位置的可加性验证、取整残差解释、接缝放大图。
//
//	go run ./cmd/seamaudit -out artifacts/seam
package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"strings"

	"vecgo/seam"
)

const tierGray = 96 // 每被一块覆盖叠一档的灰度贡献

type classSpec struct {
	class seam.Class
	name  string
	s     float64
	k     int
}

func main() {
	out := flag.String("out", "artifacts/seam", "产物输出目录")
	flag.Parse()
	if err := run(*out); err != nil {
		fmt.Fprintln(os.Stderr, "seamaudit 失败:", err)
		os.Exit(1)
	}
}

func run(outDir string) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}

	b := seam.Band{W: 24, H: 16, Y0: 4, Y1: 12}
	classes := []classSpec{
		{seam.ClassInteger, "整数缝", 10, 10},
		{seam.ClassHalf, "半像素缝", 10.5, 10},
		{seam.ClassFrac, "一般小数缝", 10.25, 10},
	}
	rules := seam.AllRules()
	policies := []seam.SnapPolicy{
		seam.SnapFloorShared,
		seam.SnapOutwardHalfOpen,
		seam.SnapInwardTrunc,
		seam.SnapNearestSeparate,
	}

	// ---- 表 A：三类裁剪位置 × 四种端点规则 ----
	var endpointRows []seam.Ledger
	for _, c := range classes {
		for _, r := range rules {
			endpointRows = append(endpointRows, seam.ReconcileRule(b, c.class, c.s, c.k, r))
		}
	}

	// ---- 表 B：三类裁剪位置 × 四种取整口径 ----
	var roundingRows []seam.RoundingRow
	for _, c := range classes {
		for _, p := range policies {
			roundingRows = append(roundingRows, seam.ReconcileSnap(b, c.class, c.s, p))
		}
	}

	// ---- 全量扫描：恒等式 / 半开零重复 / 取整解释 ----
	identityChecks, identityBad := 0, 0
	halfOpenChecks, halfOpenBad := 0, 0
	for k := 1; k < b.W; k++ {
		for _, r := range rules {
			le := seam.ReconcileRule(b, "", float64(k), k, r)
			identityChecks++
			if le.IdentityResidual != 0 {
				identityBad++
			}
			if r == seam.HalfOpenRight || r == seam.HalfOpenLeft {
				halfOpenChecks++
				if le.DupCovered != 0 || le.GapCovered != 0 || le.Delta != 0 {
					halfOpenBad++
				}
			}
		}
	}
	sweepChecks, sweepBad := 0, 0
	fracs := []float64{0.125, 0.2, 0.25, 1.0 / 3.0, 0.375, 0.49, 0.5, 0.51, 0.625, 2.0 / 3.0, 0.75, 0.875}
	for k := 0; k < b.W; k++ {
		for _, f := range fracs {
			s := float64(k) + f
			if s >= float64(b.W) {
				continue
			}
			for _, p := range policies {
				row := seam.ReconcileSnap(b, seam.ClassFrac, s, p)
				sweepChecks++
				if row.Unexplained != 0 {
					sweepBad++
				}
			}
		}
	}

	// ---- 主算例数字 ----
	whole := b.WholeCount()
	cc := seam.ReconcileRule(b, seam.ClassInteger, 10, 10, seam.ClosedClosed)
	ho := seam.ReconcileRule(b, seam.ClassInteger, 10, 10, seam.HalfOpenRight)
	oo := seam.ReconcileRule(b, seam.ClassInteger, 10, 10, seam.OpenOpen)
	grayWhole := whole * tierGray
	grayClosedSum := (cc.Left + cc.Right) * tierGray
	grayDelta := grayClosedSum - grayWhole // = dup*96，无截断（2*96=192<256）

	allPassed := identityBad == 0 && halfOpenBad == 0 && sweepBad == 0 &&
		cc.IdentityResidual == 0 && ho.DupCovered == 0 && ho.Delta == 0 &&
		cc.DupCovered == cc.Delta && grayDelta == cc.DupCovered*tierGray

	audit := map[string]any{
		"schema": "vecgo.seam-audit.v1",
		"model": map[string]any{
			"width": b.W, "height": b.H,
			"band_y0": b.Y0, "band_y1": b.Y1, "band_height": b.BandHeight(),
			"seam_k": 10, "tier_gray": tierGray,
		},
		"headline": map[string]any{
			"whole_covered":      whole,
			"closed_left":        cc.Left,
			"closed_right":       cc.Right,
			"closed_sum":         cc.Left + cc.Right,
			"closed_delta":       cc.Delta,
			"dup_covered":        cc.DupCovered,
			"halfopen_left":      ho.Left,
			"halfopen_right":     ho.Right,
			"halfopen_sum":       ho.Left + ho.Right,
			"halfopen_delta":     ho.Delta,
			"open_delta":         oo.Delta,
			"gap_covered":        oo.GapCovered,
			"gray_whole_sum":     grayWhole,
			"gray_closed_sum":    grayClosedSum,
			"gray_delta":         grayDelta,
			"gray_delta_per_dup": tierGray,
		},
		"endpoint_rows": endpointRows,
		"rounding_rows": roundingRows,
		"sweeps": map[string]any{
			"identity_checks": identityChecks,
			"identity_bad":    identityBad,
			"halfopen_checks": halfOpenChecks,
			"halfopen_bad":    halfOpenBad,
			"rounding_checks": sweepChecks,
			"rounding_bad":    sweepBad,
		},
		"all_checks_passed": allPassed,
	}
	if err := writeJSON(filepath.Join(outDir, "audit.json"), audit); err != nil {
		return err
	}
	if err := writeEndpointCSV(filepath.Join(outDir, "endpoint_checks.csv"), endpointRows); err != nil {
		return err
	}
	if err := writeRoundingCSV(filepath.Join(outDir, "rounding_checks.csv"), roundingRows); err != nil {
		return err
	}
	if err := writeVerification(filepath.Join(outDir, "verification.txt"), audit, classes, endpointRows, roundingRows,
		identityChecks, identityBad, halfOpenChecks, halfOpenBad, sweepChecks, sweepBad); err != nil {
		return err
	}
	if err := renderImages(outDir, b); err != nil {
		return err
	}

	fmt.Printf("all_checks_passed=%v\n", allPassed)
	fmt.Printf("whole=%d closed_sum=%d delta=%+d dup=%d | halfopen_sum=%d delta=%+d | gray_delta=%+d\n",
		whole, cc.Left+cc.Right, cc.Delta, cc.DupCovered, ho.Left+ho.Right, ho.Delta, grayDelta)
	fmt.Printf("artifacts -> %s\n", outDir)
	if !allPassed {
		os.Exit(2)
	}
	return nil
}

func writeJSON(path string, v any) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func ruleCN(r seam.Rule) string {
	switch r {
	case seam.ClosedClosed:
		return "双闭[0,k]+[k,W)"
	case seam.OpenOpen:
		return "双开[0,k)+[k+1,W)"
	case seam.HalfOpenRight:
		return "半开(缝归右)[0,k)+[k,W)"
	case seam.HalfOpenLeft:
		return "半开(缝归左)[0,k+1)+[k+1,W)"
	}
	return string(r)
}

func classCN(c seam.Class) string {
	switch c {
	case seam.ClassInteger:
		return "整数缝"
	case seam.ClassHalf:
		return "半像素缝"
	case seam.ClassFrac:
		return "一般小数缝"
	}
	return string(c)
}

func writeEndpointCSV(path string, rows []seam.Ledger) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	if err := w.Write([]string{"class", "s", "k", "rule", "left", "right", "sum", "whole",
		"delta", "dup_covered", "gap_covered", "delta_minus_dup_minus_gap_residual", "pass"}); err != nil {
		return err
	}
	for _, le := range rows {
		pass := le.IdentityResidual == 0
		if (le.Rule == seam.HalfOpenRight || le.Rule == seam.HalfOpenLeft) &&
			(le.DupCovered != 0 || le.GapCovered != 0 || le.Delta != 0) {
			pass = false
		}
		if err := w.Write([]string{
			classCN(le.Class), fmt.Sprint(le.S), fmt.Sprint(le.K), ruleCN(le.Rule),
			fmt.Sprint(le.Left), fmt.Sprint(le.Right), fmt.Sprint(le.Left + le.Right),
			fmt.Sprint(le.Whole), fmt.Sprint(le.Delta), fmt.Sprint(le.DupCovered),
			fmt.Sprint(le.GapCovered), fmt.Sprint(le.IdentityResidual), fmt.Sprintf("%v", pass),
		}); err != nil {
			return err
		}
	}
	return nil
}

func writeRoundingCSV(path string, rows []seam.RoundingRow) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	if err := w.Write([]string{"policy", "class", "s", "left_interval", "right_interval",
		"left", "right", "sum", "whole", "delta", "rounding_term", "unexplained", "dup", "gap", "pass"}); err != nil {
		return err
	}
	for _, r := range rows {
		pass := r.Unexplained == 0
		if err := w.Write([]string{
			string(r.Policy), classCN(r.Class), fmt.Sprint(r.S),
			fmt.Sprintf("[%d,%d)", r.LeftInterval[0], r.LeftInterval[1]),
			fmt.Sprintf("[%d,%d)", r.RightInterval[0], r.RightInterval[1]),
			fmt.Sprint(r.Left), fmt.Sprint(r.Right), fmt.Sprint(r.Sum), fmt.Sprint(r.Whole),
			fmt.Sprint(r.Delta), fmt.Sprint(r.RoundingTerm), fmt.Sprint(r.Unexplained),
			fmt.Sprint(r.DupCovered), fmt.Sprint(r.GapCovered), fmt.Sprintf("%v", pass),
		}); err != nil {
			return err
		}
	}
	return nil
}

func writeVerification(path string, audit map[string]any, classes []classSpec,
	epRows []seam.Ledger, rdRows []seam.RoundingRow,
	idN, idBad, hoN, hoBad, swN, swBad int) error {
	var sb strings.Builder
	fmt.Fprintf(&sb, "all_checks_passed=%v\n", audit["all_checks_passed"])
	sb.WriteString("schema=vecgo.seam-audit.v1\n")
	sb.WriteString("model=24x16 band rows[4,12) height=8 seam_k=10 tier_gray=96\n")
	hl := audit["headline"].(map[string]any)
	for _, k := range []string{"whole_covered", "closed_left", "closed_right", "closed_sum",
		"closed_delta", "dup_covered", "halfopen_left", "halfopen_right", "halfopen_sum",
		"halfopen_delta", "open_delta", "gap_covered", "gray_whole_sum", "gray_closed_sum",
		"gray_delta", "gray_delta_per_dup"} {
		fmt.Fprintf(&sb, "%s=%v\n", k, hl[k])
	}
	fmt.Fprintf(&sb, "sweep identity checks=%d bad=%d\n", idN, idBad)
	fmt.Fprintf(&sb, "sweep halfopen checks=%d bad=%d\n", hoN, hoBad)
	fmt.Fprintf(&sb, "sweep rounding checks=%d bad=%d\n", swN, swBad)

	sb.WriteString("\n[endpoint: class x rule -> L+R vs whole, dup/gap, residual]\n")
	for _, le := range epRows {
		fmt.Fprintf(&sb, "%s s=%-4v %-34s L=%3d R=%3d sum=%3d whole=%3d Δ=%+3d dup=%2d gap=%2d residual=%d\n",
			classCN(le.Class), le.S, ruleCN(le.Rule), le.Left, le.Right, le.Left+le.Right,
			le.Whole, le.Delta, le.DupCovered, le.GapCovered, le.IdentityResidual)
	}
	sb.WriteString("\n[rounding: class x policy -> delta vs rounding term]\n")
	for _, r := range rdRows {
		fmt.Fprintf(&sb, "%s s=%-5v %-26s Δ=%+3d term=%+3d unexplained=%d dup=%2d gap=%2d\n",
			classCN(r.Class), r.S, r.Policy, r.Delta, r.RoundingTerm, r.Unexplained,
			r.DupCovered, r.GapCovered)
	}
	return os.WriteFile(path, []byte(sb.String()), 0o644)
}

// ---------------- 图像 ----------------

func renderImages(dir string, b seam.Band) error {
	k := 10
	lCC, rCC := seam.RuleSpans(b.W, k, seam.ClosedClosed)
	lHO, rHO := seam.RuleSpans(b.W, k, seam.HalfOpenRight)
	lOO, rOO := seam.RuleSpans(b.W, k, seam.OpenOpen)

	grayBuggy := accumulation(b, lCC, rCC, tierGray)
	grayFixed := accumulation(b, lHO, rHO, tierGray)
	grayGap := accumulation(b, lOO, rOO, tierGray)

	red := color.RGBA{R: 235, G: 64, B: 52, A: 255}
	green := color.RGBA{R: 46, G: 204, B: 113, A: 255}
	orange := color.RGBA{R: 240, G: 160, B: 40, A: 255}
	white := color.RGBA{R: 235, G: 238, B: 245, A: 255}
	dim := color.RGBA{R: 150, G: 158, B: 170, A: 255}
	bg := color.RGBA{R: 18, G: 20, B: 26, A: 255}

	// 全图（带像素网格，z=24）。
	for _, item := range []struct {
		name string
		g    *image.Gray
		mk   color.RGBA
	}{
		{"seam-buggy.png", grayBuggy, red},
		{"seam-fixed.png", grayFixed, green},
		{"seam-gap.png", grayGap, orange},
	} {
		p := scalePanel(item.g, 0, b.W, 0, b.H, 24, k, item.mk)
		if err := savePNG(filepath.Join(dir, item.name), withBackground(p, bg)); err != nil {
			return err
		}
	}

	// ---- 主产物：接缝处放大图（错误双闭 vs 正确半开），单独存为 seam-zoom.png ----
	const z = 64
	x0, x1, y0, y1 := 8, 13, 3, 13 // 列 8..12，行 3..12
	pBug := scalePanel(grayBuggy, x0, x1, y0, y1, z, k, red)
	pFix := scalePanel(grayFixed, x0, x1, y0, y1, z, k, green)

	pad, headH, footH := 44, 120, 96
	panelW, panelH := (x1-x0)*z, (y1-y0)*z
	W := pad*3 + panelW*2
	H := headH + panelH + footH
	hero := image.NewRGBA(image.Rect(0, 0, W, H))
	fillRect(hero, 0, 0, W, H, bg)

	xBug, xFix := pad, pad*2+panelW
	yPan := headH
	paste(hero, pBug, xBug, yPan)
	paste(hero, pFix, xFix, yPan)

	// 列号。
	for _, cx := range []int{xBug, xFix} {
		for x := x0; x < x1; x++ {
			label := fmt.Sprint(x)
			tw := textWidth(label, 2)
			drawText(hero, label, cx+(x-x0)*z+z/2-tw/2, yPan-34, 2, dim)
		}
	}
	// 标题与结论。
	drawTextCentered(hero, "CLOSED-CLOSED: K IN BOTH TILES", xBug, panelW, 26, 2, white)
	drawTextCentered(hero, "DUP=8 DELTA=+8", xBug, panelW, 62, 3, red)
	drawTextCentered(hero, "HALF-OPEN [0,K)+[K,W)", xFix, panelW, 26, 2, white)
	drawTextCentered(hero, "DUP=0 DELTA=0", xFix, panelW, 62, 3, green)
	drawTextCentered(hero, "K COLUMN X2: ONE EXTRA TIER", xBug, panelW, yPan+panelH+26, 2, dim)
	drawTextCentered(hero, "GRAY SUMS DIFFER +768", xBug, panelW, yPan+panelH+54, 2, red)
	drawTextCentered(hero, "K OWNED BY RIGHT TILE ONLY", xFix, panelW, yPan+panelH+26, 2, dim)
	drawTextCentered(hero, "GRAY SUMS EQUAL", xFix, panelW, yPan+panelH+54, 2, green)

	if err := savePNG(filepath.Join(dir, "seam-zoom.png"), hero); err != nil {
		return err
	}

	// ---- 三联放大图：双闭 / 半开 / 双开 ----
	const z3 = 44
	pCC := scalePanel(grayBuggy, x0, x1, y0, y1, z3, k, red)
	pHO := scalePanel(grayFixed, x0, x1, y0, y1, z3, k, green)
	pOO := scalePanel(grayGap, x0, x1, y0, y1, z3, k, orange)
	pw3, ph3 := (x1-x0)*z3, (y1-y0)*z3
	gap3 := 28
	W3 := pad*2 + pw3*3 + gap3*2
	H3 := headH + ph3 + footH
	tri := image.NewRGBA(image.Rect(0, 0, W3, H3))
	fillRect(tri, 0, 0, W3, H3, bg)
	xs := []int{pad, pad + pw3 + gap3, pad + (pw3+gap3)*2}
	for i, p := range []*image.RGBA{pCC, pHO, pOO} {
		paste(tri, p, xs[i], yPan)
		for x := x0; x < x1; x++ {
			label := fmt.Sprint(x)
			tw := textWidth(label, 2)
			drawText(tri, label, xs[i]+(x-x0)*z3+z3/2-tw/2, yPan-32, 2, dim)
		}
	}
	drawTextCentered(tri, "CLOSED-CLOSED", xs[0], pw3, 26, 2, red)
	drawTextCentered(tri, "DUP=8 D=+8", xs[0], pw3, 60, 3, red)
	drawTextCentered(tri, "HALF-OPEN", xs[1], pw3, 26, 2, green)
	drawTextCentered(tri, "DUP=0 D=0", xs[1], pw3, 60, 3, green)
	drawTextCentered(tri, "OPEN-OPEN", xs[2], pw3, 26, 2, orange)
	drawTextCentered(tri, "GAP=8 D=-8", xs[2], pw3, 60, 3, orange)
	caps := []string{"SUM=200 VS WHOLE=192", "SUM=192 = WHOLE=192", "SUM=184 VS WHOLE=192"}
	capCols := []color.RGBA{red, green, orange}
	for i, c := range caps {
		drawTextCentered(tri, c, xs[i], pw3, yPan+ph3+26, 2, capCols[i])
	}
	if err := savePNG(filepath.Join(dir, "seam-triptych.png"), tri); err != nil {
		return err
	}
	return nil
}

func drawTextCentered(img *image.RGBA, s string, x0, width, y, scale int, c color.RGBA) {
	tw := textWidth(s, scale)
	x := x0 + (width-tw)/2
	if x < x0 {
		x = x0
	}
	drawText(img, s, x, y, scale, c)
}

// withBackground 把带网格的面板合成到不透明背景上（面板本身已不透明，这里仅统一底色）。
func withBackground(p *image.RGBA, bg color.RGBA) *image.RGBA {
	out := image.NewRGBA(p.Bounds())
	fillRect(out, 0, 0, out.Bounds().Dx(), out.Bounds().Dy(), bg)
	paste(out, p, 0, 0)
	return out
}
