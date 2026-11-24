package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"vecgo/config"
	"vecgo/geom"
	"vecgo/pipe"
)

// overlapReport 是三类重叠组合的量化结果。理论值来自解析重叠面积，
// 实测值来自逐像素的绘制计数。统计关闭时只做渲染，不产生报告。
type overlapReport struct {
	Combos []comboResult `json:"combos"`
	// Identical 为 true 表示每类组合在“统计开/关”下输出的图像逐字节相同。
	Identical bool `json:"identical_with_stats"`
	// IdenticalBy 记录每类组合两种路径的输出哈希，作为一致的证据。
	IdenticalBy []hashPair `json:"identical_evidence"`
}

type hashPair struct {
	Kind    string `json:"kind"`
	Without string `json:"sha256_stats_off"`
	With    string `json:"sha256_stats_on"`
	Same    bool   `json:"same"`
	Bytes   int    `json:"bytes"`
}

type comboResult struct {
	Kind        string  `json:"kind"`
	Description string  `json:"description"`
	Shapes      int     `json:"shapes"`
	Width       int     `json:"width"`
	Height      int     `json:"height"`
	Supersample int     `json:"supersample"`
	PixelArea   float64 `json:"pixel_area"`
	CanvasArea  float64 `json:"canvas_area"`

	ShapeArea   float64 `json:"shape_area_sum"`
	UnionArea   float64 `json:"union_area"`
	OverlapArea float64 `json:"overlap_area"`
	OverlapFrac float64 `json:"overlap_fraction"`

	// 理论值：由解析重叠面积按面积占比 × 样本数推算。
	TheoryPostsPerPixel  float64 `json:"theory_posts_per_pixel"`
	TheoryRepeatPerPixel float64 `json:"theory_repeat_per_pixel"`
	TheoryPostsTotal     float64 `json:"theory_posts_samples"`
	TheoryRepeatTotal    float64 `json:"theory_repeat_samples"`

	// 实测值：逐像素计数后按像素数平均。不短路口径指“每个包含该样本的图形各写一次”。
	MeasuredPostsPerPixel  float64 `json:"measured_posts_per_pixel"`
	MeasuredRepeatPerPixel float64 `json:"measured_repeat_per_pixel"`
	MeasuredPostsTotal     int     `json:"measured_posts_samples"`
	MeasuredRepeatTotal    int     `json:"measured_repeat_samples"`

	PostsRelDiff  float64 `json:"posts_rel_diff"`
	RepeatRelDiff float64 `json:"repeat_rel_diff"`

	// 现行成像路径在首个命中处短路，故每样本至多一次写入。
	ActualPostsPerPixel  float64 `json:"actual_posts_per_pixel"`
	ActualRepeatPerPixel float64 `json:"actual_repeat_per_pixel"`
	ActualTestsPerPixel  float64 `json:"actual_geom_tests_per_pixel"`
	ActualTestsTotal     int     `json:"actual_geom_tests_total"`

	// 交叉校验：被 ≥2 个图形覆盖的面积占比 vs 实测重叠样本占比。
	TheoryMultiFraction   float64 `json:"theory_multi_fraction"`
	MeasuredMultiFraction float64 `json:"measured_overlap_sample_fraction"`

	// 逐像素重复情况的分布与热点。
	CoveredPixels     int      `json:"covered_pixels"`
	RepeatPixels      int      `json:"pixels_with_repeat"`
	RepeatPixelShare  float64  `json:"repeat_pixel_share"`
	RepeatHistogram   []int    `json:"repeat_histogram"`
	MaxRepeatPerPixel int      `json:"max_repeat_per_pixel"`
	MaxRepeatAt       [2]int   `json:"max_repeat_at_row_col"`
	HotPixels         int      `json:"hot_pixel_count"`
	HotRegion         [4]int   `json:"hot_region_row0_col0_row1_col1"`
	HotMap            []string `json:"hot_map_ascii"`
}

func main() {
	stats := flag.Bool("stats", false, "打开填充统计（默认关闭，关闭时成像与无统计完全一致）")
	jsonOut := flag.String("json", "", "把统计报告写到该文件；为空则只打印摘要")
	flag.Parse()

	// 无论统计开关如何，都先走与改动前完全相同的渲染路径并导出同一产物。
	scene := [][]geom.Point{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}}
	cfg := config.Default()
	im, err := pipe.Run(scene, cfg)
	if err != nil {
		fail(1, "失败", err)
	}
	if len(flag.Args()) > 0 {
		cfg.Output = flag.Arg(0)
	}
	if err := im.SavePGM(cfg.Output); err != nil {
		fail(2, "导出失败", err)
	}
	fmt.Printf("on=%d size=%dx%d\n", im.CountOn(128), im.W, im.H)
	if !*stats {
		return
	}
	rep, err := buildReport()
	if err != nil {
		fail(3, "统计失败", err)
	}
	if err := printReport(os.Stdout, rep); err != nil {
		fail(4, "输出失败", err)
	}
	if *jsonOut != "" {
		if err := writeJSON(*jsonOut, rep); err != nil {
			fail(4, "写出报告失败", err)
		}
	}
}

func fail(code int, msg string, err error) {
	fmt.Fprintln(os.Stderr, msg+":", err)
	os.Exit(code)
}

type combo struct {
	kind  string
	desc  string
	cfg   config.Config
	scene [][]geom.Point
}

// combos 构造三类重叠。三者画幅面积与像素面积都相同，理论值可直接横向比较：
// 画幅 30×20 / 20×20 场景单位，80 像素/单位 → 每像素 0.0625 场景面积。
func combos() []combo {
	base := config.Default()
	base.Supersample = 4
	base.Scale, base.Rotate = 1, 0
	base.Blur = 0

	square20 := []geom.Point{{X: 0, Y: 0}, {X: 20, Y: 0}, {X: 20, Y: 20}, {X: 0, Y: 20}}
	square20b := []geom.Point{{X: 10, Y: 0}, {X: 30, Y: 0}, {X: 30, Y: 20}, {X: 10, Y: 20}}
	inner10 := []geom.Point{{X: 5, Y: 5}, {X: 15, Y: 5}, {X: 15, Y: 15}, {X: 5, Y: 15}}

	dup := base
	dup.Width, dup.Height = 80, 80
	dupScene := [][]geom.Point{square20, square20}

	part := base
	part.Width, part.Height = 120, 80
	partScene := [][]geom.Point{square20, square20b}

	nest := base
	nest.Width, nest.Height = 80, 80
	nestScene := [][]geom.Point{square20, inner10}

	return []combo{
		{"duplicate", "同一 20×20 正方形重复绘制两次（完全重叠）", dup, dupScene},
		{"partial", "两个 20×20 正方形部分重叠（交集 10×20）", part, partScene},
		{"contained", "10×10 正方形完全落在大正方形内", nest, nestScene},
	}
}

func buildReport() (*overlapReport, error) {
	rep := &overlapReport{Identical: true}
	for _, c := range combos() {
		offim, err := pipe.Run(c.scene, c.cfg)
		if err != nil {
			return nil, fmt.Errorf("%s 无统计: %w", c.kind, err)
		}
		st := &pipe.Stats{Enabled: true, KeepPerPixel: true}
		onim, err := pipe.RunWithStats(c.scene, c.cfg, st)
		if err != nil {
			return nil, fmt.Errorf("%s 有统计: %w", c.kind, err)
		}
		offHash, onHash := sha(offim.Pix), sha(onim.Pix)
		pair := hashPair{Kind: c.kind, Without: offHash, With: onHash, Same: offHash == onHash, Bytes: len(offim.Pix)}
		if !pair.Same {
			rep.Identical = false
		}
		rep.IdenticalBy = append(rep.IdenticalBy, pair)

		r := comboResult{
			Kind:        c.kind,
			Description: c.desc,
			Shapes:      len(c.scene),
			Width:       onim.W,
			Height:      onim.H,
			Supersample: st.Supersample,
			PixelArea:   st.PixelArea,
			CanvasArea:  st.CanvasArea,
			ShapeArea:   st.ShapeSumArea,
			UnionArea:   st.UnionArea,
			OverlapArea: st.OverlapArea,
			OverlapFrac: st.OverlapFraction,
		}
		r.TheoryPostsPerPixel = st.TheoryPostsPerPixel()
		r.TheoryRepeatPerPixel = st.TheoryRepeatPerPixel()
		r.TheoryPostsTotal = st.TheoryPostsTotal()
		r.TheoryRepeatTotal = st.TheoryRepeatTotal()
		r.MeasuredPostsPerPixel = st.MeasuredNaivePostsPerPixel()
		r.MeasuredRepeatPerPixel = st.MeasuredNaiveRepeatPerPixel()
		r.MeasuredPostsTotal = st.Fill.Posts
		r.MeasuredRepeatTotal = st.Fill.ExtraPosts
		r.PostsRelDiff = pipe.RelDiff(r.MeasuredPostsPerPixel, r.TheoryPostsPerPixel)
		r.RepeatRelDiff = pipe.RelDiff(r.MeasuredRepeatPerPixel, r.TheoryRepeatPerPixel)
		r.ActualPostsPerPixel = st.MeasuredActualPostsPerPixel()
		r.ActualRepeatPerPixel = st.MeasuredActualRepeatPerPixel()
		r.ActualTestsPerPixel = float64(st.Fill.Tests) / float64(onim.W*onim.H)
		r.ActualTestsTotal = st.Fill.Tests
		r.TheoryMultiFraction = st.MultiFraction()
		r.MeasuredMultiFraction = st.MeasuredOverlapFraction()

		r.CoveredPixels = st.Fill.Pixels
		r.RepeatPixels = st.Fill.RepeatAtLeast(1)
		if r.CoveredPixels > 0 {
			r.RepeatPixelShare = float64(r.RepeatPixels) / float64(r.CoveredPixels)
		}
		r.RepeatHistogram = append([]int(nil), st.Fill.Repeat...)
		r.MaxRepeatPerPixel = st.Fill.MaxRepeat
		r.MaxRepeatAt = st.Fill.MaxAt
		r.HotPixels = st.Fill.RepeatAtLeast(st.Fill.MaxRepeat)
		r0, c0, r1, c1, _ := st.Fill.HotRegion()
		r.HotRegion = [4]int{r0, c0, r1, c1}
		r.HotMap = st.Fill.HotMap(hotCell(onim.W, onim.H))
		rep.Combos = append(rep.Combos, r)
	}
	return rep, nil
}

func sha(pix []uint8) string {
	h := sha256.Sum256(pix)
	return hex.EncodeToString(h[:])
}

func printReport(w io.Writer, rep *overlapReport) error {
	fmt.Fprintln(w, "\n== 重叠重复绘制：理论(解析面积) vs 实测(逐像素计数) ==")
	fmt.Fprintln(w, "理论次数 = 重叠面积/画幅面积 × 每像素样本数（超采样²）；不采样估计。")
	for _, r := range rep.Combos {
		fmt.Fprintf(w, "\n[%s] %s\n", r.Kind, r.Description)
		fmt.Fprintf(w, "  画幅 %dx%d 超采样 %d，像素面积 %.4f；图形面积和 %.0f，并集 %.0f，重叠 %.0f (%.2f%%)\n",
			r.Width, r.Height, r.Supersample, r.PixelArea, r.ShapeArea, r.UnionArea, r.OverlapArea, 100*r.OverlapFrac)
		fmt.Fprintf(w, "  %-18s %14s %14s %10s\n", "每像素", "理论", "实测", "相对差")
		fmt.Fprintf(w, "  %-18s %14.4f %14.4f %+9.2f%%\n", "写入次数", r.TheoryPostsPerPixel, r.MeasuredPostsPerPixel, 100*r.PostsRelDiff)
		fmt.Fprintf(w, "  %-18s %14.4f %14.4f %+9.2f%%\n", "重复绘制次数", r.TheoryRepeatPerPixel, r.MeasuredRepeatPerPixel, 100*r.RepeatRelDiff)
		fmt.Fprintf(w, "  总数(样本口径): 写入 理论 %.0f / 实测 %d；重复 理论 %.0f / 实测 %d\n",
			r.TheoryPostsTotal, r.MeasuredPostsTotal, r.TheoryRepeatTotal, r.MeasuredRepeatTotal)
		fmt.Fprintf(w, "  交叉校验（≥2 个图形覆盖的面积占比）: 理论 %.4f / 实测 %.4f\n", r.TheoryMultiFraction, r.MeasuredMultiFraction)
		fmt.Fprintf(w, "  现行成像路径（首命中短路）: 写入 %.4f 次/像素，重复 %.4f 次/像素，几何判定 %.4f 次/像素（共 %d 次）\n",
			r.ActualPostsPerPixel, r.ActualRepeatPerPixel, r.ActualTestsPerPixel, r.ActualTestsTotal)
		fmt.Fprintf(w, "  覆盖像素 %d，其中 %d 个有重复 (%.1f%%)；单像素最高重复 %d 次\n",
			r.CoveredPixels, r.RepeatPixels, 100*r.RepeatPixelShare, r.MaxRepeatPerPixel)
		fmt.Fprintf(w, "  重复最多区域: rows %d..%d cols %d..%d，共 %d 像素（首个峰值 row=%d col=%d）\n",
			r.HotRegion[0], r.HotRegion[2], r.HotRegion[1], r.HotRegion[3], r.HotPixels, r.MaxRepeatAt[0], r.MaxRepeatAt[1])
		if len(r.HotMap) > 0 {
			fmt.Fprintln(w, "  标记图（X=重复次数最高的像素，. = 无覆盖）：")
			for _, line := range r.HotMap {
				fmt.Fprintf(w, "    %s\n", line)
			}
		}
	}
	fmt.Fprintf(w, "\n统计开关下图像逐字节一致: %v\n", rep.Identical)
	for _, p := range rep.IdenticalBy {
		fmt.Fprintf(w, "  %-10s off=%s on=%s same=%v bytes=%d\n", p.Kind, p.Without[:16], p.With[:16], p.Same, p.Bytes)
	}
	return nil
}

// hotCell 选择让字符图不超过约 48 列、24 行的 cell 尺寸。
func hotCell(w, h int) int {
	cell := 1
	for (w+cell-1)/cell > 48 || (h+cell-1)/cell > 24 {
		cell++
	}
	return cell
}

func writeJSON(path string, rep *overlapReport) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(rep)
}
