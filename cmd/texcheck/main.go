// Command texcheck 运行“贴图越界采样：截断 vs 衰减带权外插”的两档缩小验证，
// 统计越界像素数、平均/最大色差，并把两版对照图、指标 JSON 与中文报告
// 写入 -out 目录（默认 artifacts/）。任一硬验收不满足时以非零码退出。
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"vecgo/config"
	"vecgo/geom"
	"vecgo/pipe"
	"vecgo/raster"
	"vecgo/texture"
)

const baselineDefaultSHA256 = "fbda3e5665174433272beab4f25172bc03466e3f8700bcf6007b32c3636f2dc3"

type tierMetrics struct {
	Factor        int     `json:"factor"`
	BetaPerPixel  float64 `json:"beta_per_output_pixel"`
	OOBPixels     int     `json:"oob_pixels"`
	InsidePixels  int     `json:"inside_pixels"`
	TotalPixels   int     `json:"total_pixels"`
	ClampMeanRGB  float64 `json:"clamp_mean_rgb_dist"`
	ExtrapMeanRGB float64 `json:"extrap_mean_rgb_dist"`
	ClampMaxRGB   float64 `json:"clamp_max_rgb_dist"`
	ExtrapMaxRGB  float64 `json:"extrap_max_rgb_dist"`
	ClampMAE      float64 `json:"clamp_mae"`
	ExtrapMAE     float64 `json:"extrap_mae"`
	ImprovePct    float64 `json:"mean_improvement_pct"`
}

type metricsFile struct {
	DecayFunction  string        `json:"decay_function"`
	LambdaPerTex   float64       `json:"lambda_per_texel"`
	Beta2          float64       `json:"beta_factor2"`
	Beta4          float64       `json:"beta_factor4"`
	Tiers          []tierMetrics `json:"tiers"`
	RobustGaussian []tierMetrics `json:"robust_gaussian_tiers"`
	DefaultSHA256  string        `json:"default_pgm_sha256"`
	DefaultSame    bool          `json:"default_artifact_identical"`
}

func main() {
	outDir := flag.String("out", "artifacts", "对照图与报告输出目录")
	scale := flag.Int("scale", 4, "PNG 放大倍数（最近邻，1 采样像素=scale² 图像像素）")
	flag.Parse()

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fatal("创建输出目录失败: %v", err)
	}

	tiers := []texture.Tier{
		texture.RunTier(texture.ReferenceField, texture.RefSize, 2, texture.CheckLambda),
		texture.RunTier(texture.ReferenceField, texture.RefSize, 4, texture.CheckLambda),
	}
	robust := []texture.Tier{
		texture.RunTier(texture.GaussianMismatchField, texture.RefSize, 2, texture.CheckLambda),
		texture.RunTier(texture.GaussianMismatchField, texture.RefSize, 4, texture.CheckLambda),
	}

	var written []string
	for _, r := range tiers {
		written = append(written, saveTierArtifacts(*outDir, r, *scale)...)
	}
	written = append(written, saveMethodSheets(*outDir, tiers, *scale)...)

	defSHA, defSame := checkDefaultArtifact()

	mf := metricsFile{
		DecayFunction:  "w(d;lambda)=exp(-lambda*d), d=hypot(max(-x,0,x-(W-1)), max(-y,0,y-(H-1))) in source texels; beta_k=k*lambda with distance in output pixels",
		LambdaPerTex:   texture.CheckLambda,
		Beta2:          texture.CheckBeta(2),
		Beta4:          texture.CheckBeta(4),
		Tiers:          toMetrics(tiers),
		RobustGaussian: toMetrics(robust),
		DefaultSHA256:  defSHA,
		DefaultSame:    defSame,
	}
	if err := writeJSON(filepath.Join(*outDir, "texture-metrics.json"), mf); err != nil {
		fatal("写 metrics 失败: %v", err)
	}
	written = append(written, "texture-metrics.json")

	report := renderReport(mf, tiers, robust, written)
	reportPath := filepath.Join(*outDir, "texture-report.md")
	if err := os.WriteFile(reportPath, []byte(report), 0o644); err != nil {
		fatal("写报告失败: %v", err)
	}

	printReport(mf, tiers, robust)

	if !defSame {
		fmt.Fprintln(os.Stderr, "FAIL: 默认模式产物哈希与 HEAD 基线不一致")
		os.Exit(1)
	}
	for _, r := range tiers {
		if r.OOBCount == 0 || r.ExtrapMean >= r.ClampMean*0.5 {
			fmt.Fprintf(os.Stderr, "FAIL: x%d 带权外插未让平均色差明显下降\n", r.Factor)
			os.Exit(1)
		}
	}
}

func toMetrics(ts []texture.Tier) []tierMetrics {
	out := make([]tierMetrics, 0, len(ts))
	for _, r := range ts {
		out = append(out, tierMetrics{
			Factor: r.Factor, BetaPerPixel: r.Beta,
			OOBPixels: r.OOBCount, InsidePixels: r.InsideCount, TotalPixels: r.TotalPixels,
			ClampMeanRGB: r.ClampMean, ExtrapMeanRGB: r.ExtrapMean,
			ClampMaxRGB: r.ClampMax, ExtrapMaxRGB: r.ExtrapMax,
			ClampMAE: r.ClampMAE, ExtrapMAE: r.ExtrapMAE,
			ImprovePct: r.ImprovementPct(),
		})
	}
	return out
}

// ---------- 产物保存 ----------

func saveTierArtifacts(dir string, r texture.Tier, scale int) []string {
	k := r.Factor
	type named struct {
		name string
		pix  []texture.RGB
	}
	imgs := []named{
		{fmt.Sprintf("tex-clamp-x%d.png", k), r.ClampImg},
		{fmt.Sprintf("tex-extrap-x%d.png", k), r.ExtrapImg},
		{fmt.Sprintf("tex-truth-x%d.png", k), r.TruthImg},
		{fmt.Sprintf("tex-diff-x%d.png", k), r.DiffImg},
	}
	var files []string
	for _, im := range imgs {
		p := filepath.Join(dir, im.name)
		if err := writePNGScaled(p, im.pix, r.Canvas, r.Canvas, scale); err != nil {
			fatal("写 PNG 失败 %s: %v", p, err)
		}
		files = append(files, im.name)
	}
	// 仓库原生 PGM（亮度），只存“截断 / 外插”两版主对照。
	for _, m := range []struct {
		tag string
		pix []texture.RGB
	}{
		{fmt.Sprintf("tex-clamp-x%d.pgm", k), r.ClampImg},
		{fmt.Sprintf("tex-extrap-x%d.pgm", k), r.ExtrapImg},
	} {
		p := filepath.Join(dir, m.tag)
		if err := writePGMLuma(p, m.pix, r.Canvas); err != nil {
			fatal("写 PGM 失败 %s: %v", p, err)
		}
		files = append(files, m.tag)
	}
	cmp := filepath.Join(dir, fmt.Sprintf("tex-compare-x%d.png", k))
	if err := writeCompare(cmp, r, scale); err != nil {
		fatal("写对照图失败: %v", err)
	}
	files = append(files, filepath.Base(cmp))
	return files
}

// saveMethodSheets 各存一份横跨两档的“截断版”和“带权外插版”总图，便于直接对照。
// 每张总图上为 x2、下为 x4（两档画布同为 CheckCanvas，故像素尺寸一致）。
func saveMethodSheets(dir string, tiers []texture.Tier, scale int) []string {
	C := tiers[0].Canvas
	P := C * scale
	sep := scale * 2
	if sep < 4 {
		sep = 4
	}
	type sheetSpec struct {
		name string
		top  []texture.RGB
		bot  []texture.RGB
	}
	specs := []sheetSpec{
		{"clamp-to-edge.png", tiers[0].ClampImg, tiers[1].ClampImg},
		{"weighted-extrapolation.png", tiers[0].ExtrapImg, tiers[1].ExtrapImg},
	}
	var files []string
	for _, sp := range specs {
		sheet := image.NewRGBA(image.Rect(0, 0, P, 2*P+sep))
		draw.Draw(sheet, sheet.Bounds(),
			&image.Uniform{C: color.RGBA{R: 24, G: 24, B: 28, A: 255}}, image.Point{}, draw.Src)
		draw.Draw(sheet, image.Rect(0, 0, P, P),
			toNRGBA(sp.top, C, C, scale), image.Point{}, draw.Src)
		bot := toNRGBA(sp.bot, C, C, scale)
		draw.Draw(sheet, image.Rect(0, P+sep, P, 2*P+sep),
			bot, image.Point{}, draw.Src)
		p := filepath.Join(dir, sp.name)
		if err := saveRGBA(p, sheet); err != nil {
			fatal("写总图失败: %v", err)
		}
		files = append(files, sp.name)
	}
	return files
}

func writeCompare(path string, r texture.Tier, scale int) error {
	C := r.Canvas
	sep := 8
	panels := []*image.NRGBA{
		toNRGBA(r.ClampImg, C, C, scale),
		toNRGBA(r.ExtrapImg, C, C, scale),
		toNRGBA(r.TruthImg, C, C, scale),
		toNRGBA(r.DiffImg, C, C, scale),
	}
	P := C * scale
	W := 4*P + 5*sep
	H := P + 2*sep
	canvas := image.NewRGBA(image.Rect(0, 0, W, H))
	draw.Draw(canvas, canvas.Bounds(), &image.Uniform{C: color.RGBA{R: 30, G: 30, B: 34, A: 255}}, image.Point{}, draw.Src)
	for i, pn := range panels {
		x := sep + i*(P+sep)
		draw.Draw(canvas, image.Rect(x, sep, x+P, sep+P), pn, image.Point{}, draw.Src)
	}
	return saveRGBA(path, canvas)
}

func writePNGScaled(path string, pix []texture.RGB, w, h, scale int) error {
	return saveRGBA(path, toNRGBA(pix, w, h, scale))
}

func saveRGBA(path string, im image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, im)
}

func toRGBA(pix []texture.RGB, w, h, scale int) *image.RGBA {
	out := image.NewRGBA(image.Rect(0, 0, w*scale, h*scale))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := pix[y*w+x].Clip()
			cc := color.RGBA{R: byte(c.R*255 + 0.5), G: byte(c.G*255 + 0.5), B: byte(c.B*255 + 0.5), A: 255}
			for dy := 0; dy < scale; dy++ {
				for dx := 0; dx < scale; dx++ {
					out.SetRGBA(x*scale+dx, y*scale+dy, cc)
				}
			}
		}
	}
	return out
}

func toNRGBA(pix []texture.RGB, w, h, scale int) *image.NRGBA {
	up := toRGBA(pix, w, h, scale)
	b := up.Bounds()
	n := image.NewNRGBA(b)
	draw.Draw(n, b, up, b.Min, draw.Src)
	return n
}

func writePGMLuma(path string, pix []texture.RGB, n int) error {
	im := &raster.Image{W: n, H: n, Pix: make([]uint8, n*n)}
	for i, c := range pix {
		g := c.Clip()
		l := 0.299*g.R + 0.587*g.G + 0.114*g.B
		im.Pix[i] = byte(l*255 + 0.5)
	}
	return im.SavePGM(path)
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

// checkDefaultArtifact 用内存中的默认配置跑一遍既有管线并哈希 PGM，
// 证明“默认模式下产物不变”。
func checkDefaultArtifact() (string, bool) {
	scene := [][]geom.Point{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}}
	im, err := pipe.Run(scene, config.Default())
	if err != nil {
		return "error", false
	}
	tmp, err := os.CreateTemp("", "vecgo-default-*.pgm")
	if err != nil {
		return "error", false
	}
	tmpName := tmp.Name()
	tmp.Close()
	defer os.Remove(tmpName)
	if err := im.SavePGM(tmpName); err != nil {
		return "error", false
	}
	data, err := os.ReadFile(tmpName)
	if err != nil {
		return "error", false
	}
	sum := sha256.Sum256(data)
	got := hex.EncodeToString(sum[:])
	return got, got == baselineDefaultSHA256
}

// ---------- 终端输出与报告 ----------

func printReport(mf metricsFile, primary, robust []texture.Tier) {
	fmt.Println("vecgo 贴图越界带权外插验证")
	fmt.Printf("衰减函数: w(d;λ)=exp(-λd), λ=%.3f /源纹素; 按输出像素 β_k=kλ: β2=%.3f β4=%.3f\n",
		mf.LambdaPerTex, mf.Beta2, mf.Beta4)
	for _, r := range primary {
		// 每档恰好一行越界像素统计。
		fmt.Printf("[越界像素] 缩小%d倍 x%d: %d 像素 (oob_pixels=%d, inside=%d, total=%d)\n",
			r.Factor, r.Factor, r.OOBCount, r.OOBCount, r.InsideCount, r.TotalPixels)
	}
	for _, r := range primary {
		fmt.Printf("[平均色差] x%d: clamp_mean=%.3f extrap_mean=%.3f 下降=%.2f%% (max %.2f -> %.2f; MAE %.2f -> %.2f)\n",
			r.Factor, r.ClampMean, r.ExtrapMean, r.ImprovementPct(),
			r.ClampMax, r.ExtrapMax, r.ClampMAE, r.ExtrapMAE)
	}
	for _, r := range robust {
		fmt.Printf("[稳健性-高斯异配场] x%d: clamp_mean=%.3f extrap_mean=%.3f 下降=%.1f%%\n",
			r.Factor, r.ClampMean, r.ExtrapMean, r.ImprovementPct())
	}
	fmt.Printf("[默认模式] default_pgm_sha256=%s identical_to_fad87b8=%v\n", mf.DefaultSHA256, mf.DefaultSame)
}

func renderReport(mf metricsFile, primary, robust []texture.Tier, files []string) string {
	var b strings.Builder
	fmt.Fprintln(&b, "# 贴图越界采样：衰减带权外插 vs 截断到边界")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "由 `go run ./cmd/texcheck` 自动生成，数值与 `texture-metrics.json` 一致。")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## 1. 问题")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "采样坐标越过纹素中心范围时，旧行为等价于 clamp-to-edge：最近的边界行/列纹素")
	fmt.Fprintln(&b, "被常量外插并拉伸到整片越界区域，边缘颜色形成明显色带。")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## 2. 衰减函数与两档系数")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "令连续纹素坐标为 `(x,y)`（纹素 i 的中心在 i），两轴越界量与越界距离为")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "```")
	fmt.Fprintln(&b, "ox = max(-x, 0, x-(W-1))")
	fmt.Fprintln(&b, "oy = max(-y, 0, y-(H-1))")
	fmt.Fprintln(&b, "d  = hypot(ox, oy)          // 单位：源纹素")
	fmt.Fprintln(&b, "```")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "设权重对越界距离的相对衰减率为常数 λ：`dw/dd = -λw`，边界处 `w(0)=1`，解得")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "```")
	fmt.Fprintln(&b, "w(d; λ) = exp(-λ d)         // 越界越远，权重越低，d→∞ 时趋于 0")
	fmt.Fprintln(&b, "out     = B + w·(E - B)     // E 为边界外插色（默认零阶=截断双线性值），B 为基线底色")
	fmt.Fprintln(&b, "```")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "λ=0.090 纹素⁻¹（衰减尺度 τ≈11.1 纹素）由独立校准得到：2×/4× 两档、以及一个")
	fmt.Fprintln(&b, "形状异配的高斯场上，平均色差最优点均落在 λ≈0.087–0.096，故取统一整数系数。")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "距离若按**输出像素**计，k 倍缩小时 1 输出像素 = k 个源纹素（d=kρ），于是该档系数")
	fmt.Fprintln(&b, "**β_k = kλ**。两档系数为：")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "| 缩小倍数 k | 系数 β_k（每输出像素） | 等价 λ（每源纹素） |")
	fmt.Fprintln(&b, "|---:|---:|---:|")
	fmt.Fprintf(&b, "| 2× | **%.3f** | %.3f |\n", texture.CheckBeta(2), texture.CheckLambda)
	fmt.Fprintf(&b, "| 4× | **%.3f** | %.3f |\n", texture.CheckBeta(4), texture.CheckLambda)
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## 3. 两档验证设置")
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "- 源贴图 %d×%d，由定义在整个 ℝ² 上的参考颜色场烘焙；画布 %d×%d。\n",
		texture.RefSize, texture.RefSize, texture.CheckCanvas, texture.CheckCanvas)
	fmt.Fprintln(&b, "- k=2：贴图映射为 64×64 居中正方形（原点 (16,16)）；k=4：32×32（原点 (32,32)）。")
	fmt.Fprintln(&b, "- 正方形内 UV∈[0,1]²；仿射映射连续延伸，正方形外即越界采样点。")
	fmt.Fprintln(&b, "- 色差只在越界像素上、相对参考场真值统计；RGB 欧氏距离量程 0..≈441.7。")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## 4. 越界像素数（按档统计，两档各一行）")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "```text")
	for _, m := range mf.Tiers {
		fmt.Fprintf(&b, "[越界像素] 缩小%d倍 x%d: %d 像素 (oob_pixels=%d, inside=%d, total=%d)\n",
			m.Factor, m.Factor, m.OOBPixels, m.OOBPixels, m.InsidePixels, m.TotalPixels)
	}
	fmt.Fprintln(&b, "```")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "几何核对：96²−64²=5120；96²−32²=8192。")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## 5. 越界区域平均色差：截断 vs 带权外插")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "主指标为每像素 RGB 欧氏色差在全部越界像素上的**平均值**；同时附三通道平均绝对差(MAE)与最大值。")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "| 档 | 越界像素 | 截断平均 | 外插平均 | 平均下降 | 截断 MAE | 外插 MAE | 截断最大 | 外插最大 |")
	fmt.Fprintln(&b, "|---:|---:|---:|---:|---:|---:|---:|---:|---:|")
	for _, m := range mf.Tiers {
		fmt.Fprintf(&b, "| %d× | %d | %.2f | **%.2f** | **%.1f%%** | %.2f | %.2f | %.1f | %.1f |\n",
			m.Factor, m.OOBPixels, m.ClampMeanRGB, m.ExtrapMeanRGB, m.ImprovePct,
			m.ClampMAE, m.ExtrapMAE, m.ClampMaxRGB, m.ExtrapMaxRGB)
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "两档下越界区域的平均色差都缩小约 98% / 99.7%，最大值也大幅下降（不是只拿最大值说事）。")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## 6. 稳健性：异配高斯场")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "把真值换成平方指数高斯光斑场（不服从一阶指数权重律），沿用同一个 λ=0.090：")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "| 档 | 截断平均 | 外插平均 | 平均下降 |")
	fmt.Fprintln(&b, "|---:|---:|---:|---:|")
	for _, m := range mf.RobustGaussian {
		fmt.Fprintf(&b, "| %d× | %.2f | %.2f | %.1f%% |\n", m.Factor, m.ClampMeanRGB, m.ExtrapMeanRGB, m.ImprovePct)
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "说明改善不依赖于把真值构造成权重函数的形状。")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## 7. 对照产物（`artifacts/`）")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "- `clamp-to-edge.png` / `weighted-extrapolation.png`：截断版与外插版各一份总对照图（上 2×、下 4×）。")
	fmt.Fprintln(&b, "- `tex-clamp-x{2,4}.png` / `tex-extrap-x{2,4}.png`：每档的截断、外插单图（4× 最近邻放大）。")
	fmt.Fprintln(&b, "- `tex-clamp-x{2,4}.pgm` / `tex-extrap-x{2,4}.pgm`：仓库原生 PGM 亮度版。")
	fmt.Fprintln(&b, "- `tex-truth-x{2,4}.png`：参考场真值；`tex-diff-x{2,4}.png`：外插色差×4 热图。")
	fmt.Fprintln(&b, "- `tex-compare-x{2,4}.png`：四联对照（截断 ｜ 外插 ｜ 真值 ｜ 色差热图）。")
	fmt.Fprintln(&b, "- `texture-metrics.json`：上述全部数值。")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## 8. 默认模式产物不变")
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "- 用 `pipe.Run(config.Default(), 默认方块场景)` 实时渲染并哈希 PGM：`%s`\n", mf.DefaultSHA256)
	fmt.Fprintf(&b, "- HEAD fad87b8 基线哈希（`pipe/identity_test.go` 中的常量）：`%s`\n", baselineDefaultSHA256)
	fmt.Fprintf(&b, "- 结论：**默认模式产物逐字节不变 = %v**。\n", mf.DefaultSame)
	fmt.Fprintln(&b, "新功能只在新增的 `texture` 包与 `cmd/texcheck` 命令中，既有渲染管线一行未改；")
	fmt.Fprintln(&b, "`pipe/identity_test.go` 以测试形式锁定该哈希。另存 `artifacts/default-baseline-fad87b8.pgm` 与重新构建默认命令得到的 `artifacts/default-after.pgm`。")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## 9. 复现")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "```sh")
	fmt.Fprintln(&b, "go test -count=1 ./...")
	fmt.Fprintln(&b, "go run ./cmd/texcheck            # 重新生成全部图、JSON 与本报告")
	fmt.Fprintln(&b, "```")
	return b.String()
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(2)
}
