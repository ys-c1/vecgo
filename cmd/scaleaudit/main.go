package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"vecgo/scaleaudit"
)

func main() {
	outDir := flag.String("out", "artifacts", "CSV/JSON 产物目录")
	flag.Parse()

	rep, err := scaleaudit.Build()
	if err != nil {
		fmt.Fprintln(os.Stderr, "审计失败:", err)
		os.Exit(1)
	}

	printMarkdown(rep)

	if err := writeArtifacts(*outDir, rep); err != nil {
		fmt.Fprintln(os.Stderr, "写产物失败:", err)
		os.Exit(2)
	}
	fmt.Fprintf(os.Stderr, "\n已写出 %s/scale_audit_table.csv、scale_audit.json 与 frames/*.pgm\n", *outDir)
}

func printMarkdown(rep *scaleaudit.Report) {
	var b strings.Builder
	fmt.Fprintf(&b, "<!-- 由 go run ./cmd/scaleaudit 生成；夹具 θ=%.4f° row=%d -->\n\n", rep.AngleDeg, scaleaudit.ScanRow)

	b.WriteString("## 五档同一条边：灰度列 vs 面积比预测列（档1=全尺寸 → 档5=一半）\n\n")
	b.WriteString("| 档 | 缩放 s | 边缘像素 (col,row) | 命中 | 灰度 G（一列） | 面积预测 G₁·(s/1)²（一列） | 错误的边长预测 G₁·s | 判定 |\n")
	b.WriteString("|---:|---:|---|---:|---:|---:|---:|---|\n")
	breakTo := map[int]scaleaudit.Break{}
	breakFrom := map[int]scaleaudit.Break{}
	for _, br := range rep.Breaks {
		breakFrom[br.Pair] = br
		breakTo[br.Pair+1] = br
	}
	for _, l := range rep.Levels {
		where := fmt.Sprintf("(%d,%d)", l.Col, l.Row)
		judge := "OK"
		if br, ok := breakTo[l.Index]; ok {
			judge = fmt.Sprintf("**⚠ 单调性破坏档：较上一档反升 %d**", br.Rise)
		} else if _, ok := breakFrom[l.Index]; ok {
			judge = "破坏对的较大档端"
		}
		fmt.Fprintf(&b, "| %d | %.3f | `%s` | %d/%d | **%d** | %.2f → **%d** | %.1f → %d | %s |\n",
			l.Dang, l.Scale, where, l.Hits, l.Den, l.Gray,
			l.PredRaw, l.PredGray, l.LinearRaw, l.LinearGray, judge)
	}

	b.WriteString("\n### 单调性破坏位置（精确）\n\n")
	if len(rep.Breaks) == 0 {
		b.WriteString("未发现破坏：缩小方向灰度序列单调不增。\n")
	}
	for _, br := range rep.Breaks {
		fmt.Fprintf(&b, "- %s\n", br.Location)
	}

	b.WriteString("\n### 以半档（档5, G=63）为基准的面积预测（稳健性对照，非主预测列）\n\n")
	b.WriteString("| 档 | s | 实测 G | 以半档为基准 63·(s/0.5)² → 取整 |\n|---:|---:|---:|---:|\n")
	half := rep.Levels[len(rep.Levels)-1].Gray
	for _, l := range rep.Levels {
		raw := scaleaudit.AreaGray(half, l.Scale, scaleaudit.HalfScale)
		fmt.Fprintf(&b, "| %d | %.3f | %d | %.2f → **%d** |\n", l.Dang, l.Scale, l.Gray, raw, scaleaudit.QuantizePred(raw))
	}

	fmt.Print(b.String())

	fmt.Println("\n## 同一条近 4:1 边：抗锯齿边缘灰(0<G<255)的破坏位置")
	if len(rep.EdgePartialBreaks) == 0 {
		fmt.Println("未发现两档均为中间灰的反升。")
	}
	for _, sp := range rep.EdgePartialBreaks {
		fmt.Printf("- row=%d: %s\n", sp.Row, sp.Break.Location)
	}
	fmt.Printf("\n另有 %d 处扫描线探针落到纯内/外像素(含 0/255)的二值跳变，"+
		"属探针身份跳变而非抗锯齿灰反转，完整坐标见 artifacts/scale_audit.json 的 ProbeJumps。\n", len(rep.ProbeJumps))

	fmt.Println("\n## 固定像素全图单调性核对（降序 1→0.5，期望 Pix[小档] ≤ Pix[大档]）")
	fmt.Println("| 相邻档 | sA→sB | 变暗像素(符合) | 不变 | 变亮破坏像素 |")
	fmt.Println("|---:|---|---:|---:|---:|")
	for _, fp := range rep.FixedPairs {
		fmt.Printf("| %d→%d | %.3f→%.3f | %d | %d | **%d** |\n",
			fp.I+1, fp.I+2, fp.ScaleA, fp.ScaleB, fp.DarkerPixels, fp.EqualPixels, fp.BrighterViolations)
	}

	if pc := rep.Patch; pc != nil {
		fmt.Printf("\n## 面积律受控验证：固定像素 (col=%d,row=%d) 内的相似伸缩三角块（SS=%d）\n", pc.Col, pc.Row, pc.SS)
		fmt.Println("| s | 命中 | 实测灰度 | 解析 255·C(s)=255·0.32s² | 面积预测 G₁s²→取整 | 边长预测 G₁s→取整 |")
		fmt.Println("|---:|---|---:|---:|---:|---:|")
		for _, p := range pc.Points {
			fmt.Printf("| %.3f | %d/%d | **%d** | %.1f | %.1f→**%d** | %.1f→%d |\n",
				p.Scale, p.Hits, p.Den, p.Gray, p.ExactGray, p.AreaRaw, p.AreaGray, p.LinearRaw, p.LinearGray)
		}
		fmt.Printf("\n最大绝对误差：面积预测 %.0f vs 边长预测 %.0f；RMSE：面积 %.2f vs 边长 %.2f（灰度级）。\n",
			pc.AreaMaxErr, pc.LinearMaxErr, pc.AreaRMSE, pc.LinearRMSE)
	}

	fmt.Println("\n## 破坏处邻域灰度（同一扫描行 row=21，边向右移动）")
	for _, n := range rep.Neighborhood {
		fmt.Printf("- s=%.3f cols=%v\n  gray=%v\n", n.Scale, n.Cols, n.Grays)
	}
}

func writeArtifacts(dir string, rep *scaleaudit.Report) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	// CSV：五档主表，灰度一列、面积预测一列（另附原始未取整列）。
	csvPath := filepath.Join(dir, "scale_audit_table.csv")
	f, err := os.Create(csvPath)
	if err != nil {
		return err
	}
	w := csv.NewWriter(f)
	_ = w.Write([]string{"dang", "scale", "col", "row", "hits", "den", "gray",
		"pred_area_raw", "pred_area_round", "pred_linear_round", "monotonic_break_here"})
	breakTo := map[int]bool{}
	for _, br := range rep.Breaks {
		breakTo[br.Pair+1] = true
	}
	for _, l := range rep.Levels {
		mark := ""
		if breakTo[l.Index] {
			mark = "rise_vs_prev"
		}
		_ = w.Write([]string{
			fmt.Sprint(l.Dang), fmt.Sprintf("%.3f", l.Scale), fmt.Sprint(l.Col), fmt.Sprint(l.Row),
			fmt.Sprint(l.Hits), fmt.Sprint(l.Den), fmt.Sprint(l.Gray),
			fmt.Sprintf("%.4f", l.PredRaw), fmt.Sprint(l.PredGray), fmt.Sprint(l.LinearGray), mark,
		})
	}
	w.Flush()
	if err := w.Error(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}

	js, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "scale_audit.json"), js, 0o644); err != nil {
		return err
	}

	// 五档原始覆盖率灰度 PGM，供直接查看同一条边。
	frameDir := filepath.Join(dir, "frames")
	if err := os.MkdirAll(frameDir, 0o755); err != nil {
		return err
	}
	for i, im := range rep.Images {
		if im == nil {
			continue
		}
		p := filepath.Join(frameDir, fmt.Sprintf("dang%d_s%.3f.pgm", i+1, rep.Levels[i].Scale))
		if err := im.SavePGM(p); err != nil {
			return err
		}
	}
	return nil
}
