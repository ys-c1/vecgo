// Command shadowlab 渲染点/小面/大面三档光源的影边，按灰度过半位置
// 实测半影宽度，与相似三角形预测值列表对照，并把图像/剖面/表格写入目录。
//
//	go run ./cmd/shadowlab -out artifacts
package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"text/tabwriter"

	"vecgo/geom"
	"vecgo/shadow"
)

type caseSpec struct {
	Key      string  `json:"key"`
	Label    string  `json:"label"`
	Size     float64 `json:"light_size_world"`
	SamplesX int     `json:"samples_x"`
}

type edgeRecord struct {
	XHalfGray   float64 `json:"x_half_gray_px"`
	MeasWidthPx float64 `json:"meas_width_px"`
	TanHalfPx   float64 `json:"tan_halfpx_width_px"`
	Width1090Px float64 `json:"width_10_90_px"`
	Width2575Px float64 `json:"width_25_75_px"`
	Slope       float64 `json:"slope_gray_per_px"`
}

// Record 是一行完整的实验/测量/对照数据。
type Record struct {
	Key   string `json:"key"`
	Label string `json:"label"`

	LightSize float64 `json:"light_size_world"`
	Gap       float64 `json:"gap_h_world"`
	Dist      float64 `json:"dist_H_world"`
	Ratio     float64 `json:"ratio_h_over_HminusH"`
	PxPerUnit float64 `json:"px_per_unit"`

	PredWidthPx float64  `json:"pred_width_px"`
	MeasWidthPx float64  `json:"penumbra_width_meas_px"`
	AbsErrorPx  float64  `json:"abs_error_px"`
	RelErrorPct *float64 `json:"rel_error_pct"` // 点光源基准为 0 -> null

	Left  edgeRecord `json:"left_edge"`
	Right edgeRecord `json:"right_edge"`

	// 两个“灰度过半位置”之间的距离（半灰轮廓宽），独立对照量。
	HalfGraySpanPx float64 `json:"halfgray_span_px"`
	SpanPredPx     float64 `json:"halfgray_span_pred_px"`
	SpanErrorPx    float64 `json:"halfgray_span_error_px"`

	GrayDark float64 `json:"gray_dark"`
	GrayLit  float64 `json:"gray_lit"`

	ImagePGM string `json:"image_pgm"`
	ImagePNG string `json:"image_png"`
}

func main() {
	outDir := flag.String("out", "artifacts", "产物输出目录")
	imgW := flag.Int("width", 520, "实验图宽（像素）")
	imgH := flag.Int("height", 260, "实验图高（像素）")
	flag.Parse()

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fatal(err)
	}

	// 固定几何：H=100, h=20 -> r=h/(H-h)=20/80=1/4；
	// s=4 px/世界单位 -> w_px = s*r*L = L，三档预测宽度即 0 / 12 / 40 px。
	const (
		H        = 100.0
		h        = 20.0
		s        = 4.0
		xLeft    = 180.0
		xRight   = 340.0
		litGray  = 238
		darkGray = 16
	)
	cases := []caseSpec{
		{Key: "point", Label: "点光源", Size: 0, SamplesX: 1},
		{Key: "small", Label: "小面光源", Size: 12, SamplesX: 256},
		{Key: "large", Label: "大面光源", Size: 40, SamplesX: 512},
	}

	// 贯穿画面上下的遮挡条截面：只出现左、右两条竖直影边，
	// 中部有充足本影平台，两侧有充足受光平台。
	strip := [][]geom.Point{{
		{X: xLeft, Y: -4000},
		{X: xRight, Y: -4000},
		{X: xRight, Y: 5000},
		{X: xLeft, Y: 5000},
	}}

	var records []Record
	profiles := map[string]shadow.Profile{}

	for _, c := range cases {
		geo := shadow.Geometry{Size: c.Size, Gap: h, Dist: H}
		pred, err := geo.PenumbraPixels(s)
		if err != nil {
			fatal(err)
		}
		im, err := shadow.Render(strip, geo, shadow.Options{
			Width: *imgW, Height: *imgH,
			PxPerUnit: s, SamplesX: c.SamplesX, SamplesY: 1,
			Lit: litGray, Dark: darkGray,
		})
		if err != nil {
			fatal(err)
		}

		pgmPath := filepath.Join(*outDir, "shadow_"+c.Key+".pgm")
		pngPath := filepath.Join(*outDir, "shadow_"+c.Key+".png")
		if err := im.SavePGM(pgmPath); err != nil {
			fatal(err)
		}
		if err := saveGrayPNG(pngPath, im.W, im.H, im.Pix); err != nil {
			fatal(err)
		}

		// 取中间 60% 行带：两条竖直边在那里不受任何上下边界影响。
		y0, y1 := im.H/5, 4*im.H/5
		prof, err := shadow.ColumnProfile(im, y0, y1)
		if err != nil {
			fatal(err)
		}
		profiles[c.Key] = prof

		search := math.Max(pred, 2) + 4
		ml, err := shadow.MeasureEdge(prof, xLeft, search)
		if err != nil {
			fatal(fmt.Errorf("%s 左边测边失败: %w", c.Key, err))
		}
		mr, err := shadow.MeasureEdge(prof, xRight, search)
		if err != nil {
			fatal(fmt.Errorf("%s 右边测边失败: %w", c.Key, err))
		}

		meas := (ml.WidthHalfGray + mr.WidthHalfGray) / 2
		absErr := meas - pred
		var relPtr *float64
		if pred > 0 {
			v := 100 * absErr / pred
			relPtr = &v
		}
		span := shadow.HalfGraySpan(ml, mr)
		spanPred := xRight - xLeft

		rec := Record{
			Key: c.Key, Label: c.Label,
			LightSize: c.Size, Gap: h, Dist: H, Ratio: h / (H - h), PxPerUnit: s,
			PredWidthPx: pred, MeasWidthPx: meas,
			AbsErrorPx: absErr, RelErrorPct: relPtr,
			Left:           toEdgeRecord(ml),
			Right:          toEdgeRecord(mr),
			HalfGraySpanPx: span, SpanPredPx: spanPred, SpanErrorPx: span - spanPred,
			GrayDark: ml.GrayDark, GrayLit: ml.GrayLit,
			ImagePGM: filepath.Base(pgmPath), ImagePNG: filepath.Base(pngPath),
		}
		records = append(records, rec)
		rel := "n/a"
		if relPtr != nil {
			rel = fmt.Sprintf("%+.2f%%", *relPtr)
		}
		fmt.Printf("%-6s L=%-4.0f pred=%6.2f meas=%6.3f (L %.3f / R %.3f) err=%+.3f px %s span=%.2f\n",
			c.Key, c.Size, pred, meas, ml.WidthHalfGray, mr.WidthHalfGray, absErr, rel, span)
	}

	if err := writeProfileCSV(filepath.Join(*outDir, "profiles.csv"), profiles, cases); err != nil {
		fatal(err)
	}
	if err := writeJSON(filepath.Join(*outDir, "penumbra_measurements.json"), records); err != nil {
		fatal(err)
	}
	if err := writeTableCSV(filepath.Join(*outDir, "penumbra_table.csv"), records); err != nil {
		fatal(err)
	}
	if err := writeTableMarkdown(filepath.Join(*outDir, "penumbra_table.md"), records); err != nil {
		fatal(err)
	}
	writeStdoutTable(records)
	fmt.Println("产物已写入", *outDir)
}

func toEdgeRecord(m shadow.EdgeMeasure) edgeRecord {
	return edgeRecord{
		XHalfGray: m.XHalfGray, MeasWidthPx: m.WidthHalfGray,
		TanHalfPx: m.WidthTanHalfPx, Width1090Px: m.Width1090, Width2575Px: m.Width2575,
		Slope: m.SlopeAtHalf,
	}
}

func saveGrayPNG(path string, w, h int, pix []uint8) error {
	img := image.NewGray(image.Rect(0, 0, w, h))
	copy(img.Pix, pix)
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

func writeProfileCSV(path string, profiles map[string]shadow.Profile, cases []caseSpec) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	header := []string{"x_px"}
	for _, c := range cases {
		header = append(header, "gray_"+c.Key)
	}
	if err := w.Write(header); err != nil {
		return err
	}
	n := len(profiles[cases[0].Key].X)
	for i := 0; i < n; i++ {
		row := []string{fmt.Sprintf("%.2f", profiles[cases[0].Key].X[i])}
		for _, c := range cases {
			row = append(row, fmt.Sprintf("%.3f", profiles[c.Key].G[i]))
		}
		if err := w.Write(row); err != nil {
			return err
		}
	}
	return nil
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func relText(p *float64) string {
	if p == nil {
		return ""
	}
	return fmt.Sprintf("%.3f", *p)
}

func writeTableCSV(path string, recs []Record) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	if err := w.Write([]string{
		"key", "label", "light_size", "pred_width_px", "meas_width_px",
		"abs_error_px", "rel_error_pct",
		"x_half_gray_left_px", "x_half_gray_right_px", "halfgray_span_px",
		"span_pred_px", "span_error_px",
		"left_width_px", "right_width_px", "width_10_90_right_px",
	}); err != nil {
		return err
	}
	for _, r := range recs {
		if err := w.Write([]string{
			r.Key, r.Label, fmt.Sprintf("%.3f", r.LightSize),
			fmt.Sprintf("%.4f", r.PredWidthPx), fmt.Sprintf("%.4f", r.MeasWidthPx),
			fmt.Sprintf("%+.4f", r.AbsErrorPx), relText(r.RelErrorPct),
			fmt.Sprintf("%.4f", r.Left.XHalfGray), fmt.Sprintf("%.4f", r.Right.XHalfGray),
			fmt.Sprintf("%.4f", r.HalfGraySpanPx), fmt.Sprintf("%.4f", r.SpanPredPx),
			fmt.Sprintf("%+.4f", r.SpanErrorPx),
			fmt.Sprintf("%.4f", r.Left.MeasWidthPx), fmt.Sprintf("%.4f", r.Right.MeasWidthPx),
			fmt.Sprintf("%.4f", r.Right.Width1090Px),
		}); err != nil {
			return err
		}
	}
	return nil
}

func writeTableMarkdown(path string, recs []Record) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	fmt.Fprintln(f, "| 光源档位 | 光源尺寸 L | 半影宽实测 (px) | 预测宽 w (px) | 误差 (px) | 相对误差 | 灰度过半位置 左/右 (px) |")
	fmt.Fprintln(f, "|---|---:|---:|---:|---:|---:|---|")
	for _, r := range recs {
		rel := "—（基准为 0）"
		if r.RelErrorPct != nil {
			rel = fmt.Sprintf("%+.2f%%", *r.RelErrorPct)
		}
		fmt.Fprintf(f, "| %s（%s） | %.0f | %.2f | %.2f | %+.2f | %s | %.2f / %.2f |\n",
			r.Label, r.Key, r.LightSize, r.MeasWidthPx, r.PredWidthPx, r.AbsErrorPx, rel,
			r.Left.XHalfGray, r.Right.XHalfGray)
	}
	fmt.Fprintln(f)
	fmt.Fprintln(f, "实测口径：取中间行带的列灰度剖面 → 用 2%/98% 分位定暗、亮平台并取中点灰度 →")
	fmt.Fprintln(f, "线性插值找到“灰度过半”的亚像素位置 x50 → 在 x50 附近灰度 20%–80% 的中心线性段")
	fmt.Fprintln(f, "拟合切线 → 半影宽 = 明暗灰度差 / |斜率|；左右两边各测一次取均值。")
	fmt.Fprintln(f, "点光源一档实测约 1 px，是离散像素采样的固有基底（理想盒滤波硬边的过渡支撑恰为 1 px），")
	fmt.Fprintln(f, "几何预测严格为 0；连续极限 L→0 时该基底也随采样/分辨率设计趋于零。")
	fmt.Fprintln(f)
	fmt.Fprintln(f, "两个半灰位置的间距（半灰轮廓宽）：")
	fmt.Fprintln(f)
	fmt.Fprintln(f, "| 档位 | x50 左 | x50 右 | 半灰间距实测 | 中心投影预测 | 误差 |")
	fmt.Fprintln(f, "|---|---:|---:|---:|---:|---:|")
	for _, r := range recs {
		fmt.Fprintf(f, "| %s | %.2f | %.2f | %.2f | %.2f | %+.2f |\n",
			r.Key, r.Left.XHalfGray, r.Right.XHalfGray, r.HalfGraySpanPx, r.SpanPredPx, r.SpanErrorPx)
	}
	fmt.Fprintln(f)
	fmt.Fprintln(f, "该间距对应“光源中心光线”的投影，与 L 无关；它和半影过渡宽 w=Lh/(H-h) 是两个不同几何量，并列于此。")
	return nil
}

func writeStdoutTable(recs []Record) {
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "档位\tL\t实测px\t预测px\t误差px\t相对%")
	for _, r := range recs {
		rel := "n/a"
		if r.RelErrorPct != nil {
			rel = fmt.Sprintf("%+.3f", *r.RelErrorPct)
		}
		fmt.Fprintf(tw, "%s\t%.0f\t%.3f\t%.3f\t%+.3f\t%s\n", r.Key, r.LightSize, r.MeasWidthPx, r.PredWidthPx, r.AbsErrorPx, rel)
	}
	tw.Flush()
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "shadowlab:", err)
	os.Exit(1)
}
