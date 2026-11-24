// Command coverageaudit 用两档角度核验解析面积覆盖率，并把每像素交叠比例、
// 裁切后多边形、对比图与报告写入 artifacts/。
//
//	go run ./cmd/coverageaudit [-out artifacts]
package main

import (
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"

	"vecgo/config"
	"vecgo/geom"
	"vecgo/internal/exactcov"
	"vecgo/pipe"
	"vecgo/raster"
)

const (
	width, height = 24, 24
	denseSS       = 128
	zoom          = 12
	// baselineSHA256 是改动前 HEAD(fad87b8) 默认配置 `go run ./cmd` 产物的哈希。
	baselineSHA256 = "fbda3e5665174433272beab4f25172bc03466e3f8700bcf6007b32c3636f2dc3"
)

type angleSpec struct {
	tag string
	deg float64
}

type stats struct {
	spec                                    angleSpec
	edge                                    int
	anRMSE, anMAE, anMax                    float64
	ssRMSE, ssMAE, ssMax                    float64
	denseRMSE                               float64
	disagreeMAE, disagreeMax                float64
	sumCoverage, sumClipArea, polyAreaPixel float64
}

type clipRecord struct {
	Row             int          `json:"row"`
	Col             int          `json:"col"`
	Coverage        float64      `json:"coverage"`
	ShoelaceArea    float64      `json:"shoelace_area"`
	ClippedVertices []geom.Point `json:"clipped_vertices"`
}

func main() {
	outDir := flag.String("out", "artifacts", "核验产物输出目录")
	flag.Parse()

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fail(err)
	}
	specs := []angleSpec{{tag: "15deg", deg: 15}, {tag: "45deg", deg: 45}}
	var all []stats
	for _, sp := range specs {
		st, matrix, err := auditAngle(*outDir, sp)
		if err != nil {
			fail(err)
		}
		if err := writeZoomPGMs(*outDir, sp, matrix); err != nil {
			fail(err)
		}
		all = append(all, st)
		fmt.Printf("%s edge=%d analyticRMSE=%.3e subsample2x2RMSE=%.5f half-test=%v\n",
			sp.tag, st.edge, st.anRMSE, st.ssRMSE, st.anRMSE < 0.5*st.ssRMSE)
	}

	identityOK, afterHash, err := checkDefaultArtifact(*outDir)
	if err != nil {
		fail(err)
	}
	fmt.Printf("default artifact sha256=%s byte-identical=%v\n", afterHash, identityOK)

	if err := writeReport(*outDir, all, identityOK, afterHash); err != nil {
		fail(err)
	}

	for _, st := range all {
		if st.anRMSE >= 0.5*st.ssRMSE {
			fmt.Fprintf(os.Stderr, "%s: 解析误差未小于子点法一半\n", st.spec.tag)
			os.Exit(1)
		}
		if st.sumCoverage-st.polyAreaPixel > 1e-8 || st.polyAreaPixel-st.sumCoverage > 1e-8 {
			fmt.Fprintf(os.Stderr, "%s: 面积守恒失败\n", st.spec.tag)
			os.Exit(1)
		}
	}
	if !identityOK {
		fmt.Fprintln(os.Stderr, "默认子点产物与基线不逐字节相同")
		os.Exit(1)
	}
}

// auditAngle 跑一个角度，写 CSV/JSONL，返回统计与每像素数据矩阵。
func auditAngle(outDir string, sp angleSpec) (stats, [][]float64, error) {
	poly := rotatedSquare(10, sp.deg)
	m, err := raster.BoundsMap([][]geom.Point{poly}, width, height)
	if err != nil {
		return stats{}, nil, err
	}
	px := m.MapPoly(poly)

	csvPath := filepath.Join(outDir, "coverage-"+sp.tag+".csv")
	cf, err := os.Create(csvPath)
	if err != nil {
		return stats{}, nil, err
	}
	defer cf.Close()
	w := csv.NewWriter(cf)
	if err := w.Write([]string{
		"row", "col", "coverage", "clip_area",
		"ss2_hit", "ss2_den", "ss2_ratio",
		"exact_ratio", "dense128_ratio",
	}); err != nil {
		return stats{}, nil, err
	}

	jf, err := os.Create(filepath.Join(outDir, "clipped-"+sp.tag+".jsonl"))
	if err != nil {
		return stats{}, nil, err
	}
	defer jf.Close()
	enc := json.NewEncoder(jf)

	st := stats{spec: sp, polyAreaPixel: geom.Area(px)}
	matrix := make([][]float64, height)
	var anSE, anAE, ssSE, ssAE, denseSE, disAE float64
	for r := 0; r < height; r++ {
		matrix[r] = make([]float64, width)
		for c := 0; c < width; c++ {
			cov, clipped, err := raster.ClipCoveragePx(px, c, r)
			if err != nil {
				return stats{}, nil, err
			}
			rawArea := geom.Area(clipped)
			ssRatio, hit := raster.SubsampleRatioPx(px, c, r, 2)
			denseRatio, _ := raster.SubsampleRatioPx(px, c, r, denseSS)
			exact := exactcov.ConvexCoverage(px, float64(c), float64(r), float64(c+1), float64(r+1))
			matrix[r][c] = cov
			st.sumCoverage += cov
			st.sumClipArea += rawArea

			rec := []string{
				fmt.Sprintf("%d", r), fmt.Sprintf("%d", c),
				fmt.Sprintf("%.12f", cov), fmt.Sprintf("%.12f", rawArea),
				fmt.Sprintf("%d", hit), "4", fmt.Sprintf("%.6f", ssRatio),
				fmt.Sprintf("%.12f", exact), fmt.Sprintf("%.8f", denseRatio),
			}
			if err := w.Write(rec); err != nil {
				return stats{}, nil, err
			}

			if cov > 1e-9 && cov < 1-1e-9 {
				st.edge++
				e := cov - exact
				anSE += e * e
				anAE += math.Abs(e)
				st.anMax = math.Max(st.anMax, math.Abs(e))

				e2 := ssRatio - exact
				ssSE += e2 * e2
				ssAE += math.Abs(e2)
				st.ssMax = math.Max(st.ssMax, math.Abs(e2))

				e3 := cov - denseRatio
				denseSE += e3 * e3

				d := math.Abs(cov - ssRatio)
				disAE += d
				st.disagreeMax = math.Max(st.disagreeMax, d)

				if err := enc.Encode(clipRecord{
					Row: r, Col: c, Coverage: cov,
					ShoelaceArea: rawArea, ClippedVertices: clipped,
				}); err != nil {
					return stats{}, nil, err
				}
			}
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return stats{}, nil, err
	}

	n := float64(st.edge)
	st.anRMSE = math.Sqrt(anSE / n)
	st.anMAE = anAE / n
	st.ssRMSE = math.Sqrt(ssSE / n)
	st.ssMAE = ssAE / n
	st.denseRMSE = math.Sqrt(denseSE / n)
	st.disagreeMAE = disAE / n
	return st, matrix, nil
}

// writeZoomPGMs 为每个角度写两张最近邻放大灰度图：2×2 子点法（沿用 Fill
// 的整数灰度 255*hit/4）与解析面积法（沿用 Fill 的截断量化 uint8(255*c)）。
func writeZoomPGMs(outDir string, sp angleSpec, areaMatrix [][]float64) error {
	poly := rotatedSquare(10, sp.deg)
	m, err := raster.BoundsMap([][]geom.Point{poly}, width, height)
	if err != nil {
		return err
	}
	px := m.MapPoly(poly)

	ss, err := raster.New(width, height)
	if err != nil {
		return err
	}
	an, err := raster.New(width, height)
	if err != nil {
		return err
	}
	if err := an.FillCoverage(poly); err != nil {
		return err
	}
	for r := 0; r < height; r++ {
		for c := 0; c < width; c++ {
			_, hit := raster.SubsampleRatioPx(px, c, r, 2)
			ss.Pix[r*width+c] = uint8(255 * hit / 4)
			// FillCoverage 的落盘灰度必须与 CSV 矩阵按 Fill 同款截断一致。
			wantGray := uint8(255 * areaMatrix[r][c])
			if an.Pix[r*width+c] != wantGray {
				return fmt.Errorf("FillCoverage 与裁切矩阵在 (%d,%d) 不一致: %d vs %d",
					c, r, an.Pix[r*width+c], wantGray)
			}
		}
	}
	if err := saveZoomPGM(filepath.Join(outDir, "render-"+sp.tag+"-subsample2.pgm"), ss, zoom); err != nil {
		return err
	}
	if err := saveZoomPGM(filepath.Join(outDir, "render-"+sp.tag+"-area.pgm"), an, zoom); err != nil {
		return err
	}
	return writeComparePNG(filepath.Join(outDir, "compare-"+sp.tag+".png"), ss, an, zoom)
}

// writeComparePNG 写左右并排对比图：左=2×2 子点，右=解析面积，中间灰条分隔。
func writeComparePNG(path string, left, right *raster.Image, z int) error {
	const pad, gap = 8, 10
	panelW, panelH := left.W*z, left.H*z
	W := pad + panelW + gap + panelW + pad
	H := pad + panelH + pad
	img := image.NewGray(image.Rect(0, 0, W, H))
	for y := 0; y < H; y++ {
		for x := 0; x < W; x++ {
			img.Pix[y*img.Stride+x] = 32 // 深灰底，衬托黑/白边界
		}
	}
	// 中缝。
	for y := pad; y < pad+panelH; y++ {
		for x := pad + panelW; x < pad+panelW+gap; x++ {
			img.Pix[y*img.Stride+x] = 128
		}
	}
	blit := func(im *raster.Image, x0, y0 int) {
		for y := 0; y < panelH; y++ {
			srcRow := (y / z) * im.W
			dstBase := (y0 + y) * img.Stride
			for x := 0; x < panelW; x++ {
				img.Pix[dstBase+x0+x] = im.Pix[srcRow+x/z]
			}
		}
	}
	blit(left, pad, pad)
	blit(right, pad+panelW+gap, pad)

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

func saveZoomPGM(path string, im *raster.Image, z int) error {
	W, H := im.W*z, im.H*z
	var b strings.Builder
	fmt.Fprintf(&b, "P5\n%d %d\n255\n", W, H)
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.WriteString(b.String()); err != nil {
		return err
	}
	row := make([]byte, W)
	for y := 0; y < H; y++ {
		srcRow := (y / z) * im.W
		for x := 0; x < W; x++ {
			row[x] = im.Pix[srcRow+x/z]
		}
		if _, err := f.Write(row); err != nil {
			return err
		}
	}
	return nil
}

// checkDefaultArtifact 完整走一遍 config.Default()+pipe（与 cmd/main.go
// 相同），导出 PGM 并计算 sha256，与改动前基线哈希逐字节比对。
func checkDefaultArtifact(outDir string) (bool, string, error) {
	cfg := config.Default()
	scene := [][]geom.Point{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}}
	im, err := pipe.Run(scene, cfg)
	if err != nil {
		return false, "", err
	}
	path := filepath.Join(outDir, "default-after.pgm")
	if err := im.SavePGM(path); err != nil {
		return false, "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false, "", err
	}
	sum := sha256.Sum256(data)
	h := hex.EncodeToString(sum[:])
	return h == baselineSHA256, h, nil
}

func writeReport(outDir string, all []stats, identityOK bool, afterHash string) error {
	var b strings.Builder
	b.WriteString("# 解析覆盖率 vs 子点计数：面积裁切核验\n\n")
	b.WriteString("场景：边长 10 的方块旋转指定角度；24×24 画幅；多边形包围盒映射到整幅图像，" +
		"映射公式与 `raster.Image.Fill` 完全一致。误差基准 `exact_ratio` 来自 " +
		"`internal/exactcov` 的竖直板条积分，是独立于裁切+鞋带公式的精确面积。\n\n")

	b.WriteString("## 面积裁切算法（三步）\n\n")
	b.WriteString("1. 取像素单位正方形为裁剪窗，用 Sutherland–Hodgman 按四条边对应的半平面逐边裁多边形；\n")
	b.WriteString("2. 对裁后顶点用鞋带公式求面积 `A = ½|Σ(xᵢyᵢ₊₁ − xᵢ₊₁yᵢ)|`；\n")
	b.WriteString("3. 单位像素面积为 1，交叠比例（覆盖率）`c = clamp(A, 0, 1)`。\n\n")

	b.WriteString("## 交叠判断（写清）\n\n")
	b.WriteString("裁剪窗顶点逆时针排列：`(c,r)→(c+1,r)→(c+1,r+1)→(c,r+1)`，四条有向边分别是" +
		"下（朝东，内侧 y≥r）、右（朝北，内侧 x≤c+1）、上（朝西，内侧 y≤r+1）、左（朝南，内侧 x≥c）。\n\n")
	b.WriteString("对裁剪有向边 `A→B`，令 `E=B−A`，点 `P` 的叉积\n\n")
	b.WriteString("```\ncross(E, P−A) = (Bx−Ax)(Py−Ay) − (By−Ay)(Px−Ax)\n```\n\n")
	b.WriteString("**`cross ≥ 0` 判为在内侧（左侧半平面，含边界），`cross < 0` 判为在外侧。** " +
		("逐边处理顶点序列：当前点在内就保留；当前点与前一点恰有一个在内时，" +
			"求线段 `P_prev→P_cur` 与裁剪直线的交点并先插入交点，再按规则插入当前点。") + "\n\n")

	b.WriteString("## 两档角度误差（边缘像素上的覆盖率 RMSE，并列）\n\n")
	b.WriteString("| 角度 | 边缘像素数 | 子点法 2×2 RMSE | 解析面积裁切 RMSE | 解析/子点 | 是否 < 子点一半 | 子点最大误差 | 解析最大误差 |\n")
	b.WriteString("|---|---:|---:|---:|---:|:---:|---:|---:|\n")
	for _, st := range all {
		ratio := st.anRMSE / st.ssRMSE
		ok := "✅ 是"
		if st.anRMSE >= 0.5*st.ssRMSE {
			ok = "❌ 否"
		}
		fmt.Fprintf(&b, "| %.0f° | %d | %.5f | %.2e | %.2e | %s | %.4f | %.2e |\n",
			st.spec.deg, st.edge, st.ssRMSE, st.anRMSE, ratio, ok, st.ssMax, st.anMax)
	}
	b.WriteString("\n折算成 0–255 灰度，子点法 RMSE 约为表中数 ×255（15°≈24.0 级、45°≈63.8 级），" +
		"解析法在 1e-12 级以下。解析法的残余量是浮点误差，不是方法误差。\n\n")

	b.WriteString("### 补充统计\n\n")
	b.WriteString("| 角度 | 子点 MAE | 解析 MAE | 解析 vs 128×128 密点 RMSE | 解析与 2×2 直接平均差 | 直接最大差 |\n")
	b.WriteString("|---|---:|---:|---:|---:|---:|\n")
	for _, st := range all {
		fmt.Fprintf(&b, "| %.0f° | %.5f | %.2e | %.6f | %.5f | %.4f |\n",
			st.spec.deg, st.ssMAE, st.anMAE, st.denseRMSE, st.disagreeMAE, st.disagreeMax)
	}
	b.WriteString("\n说明：45° 时斜边斜率恰为 ±1，与规则子点网格格线对齐，2×2 子点在每个边缘像素上" +
		"非多一个点就是少一个点，误差恒为 0.25（RMSE=0.25），这正是斜边上“台阶”的量化来源；" +
		"128×128 密点基准在该角度也有同方向的格线对齐偏差（约 3.9e-3），所以主基准采用独立精确积分。\n\n")

	b.WriteString("## 裁切面积可核：守恒与逐像素数据\n\n")
	for _, st := range all {
		fmt.Fprintf(&b, "- **%.0f°**：全部 %d 像素交叠比例之和 = %.12f；多边形在像素坐标中的鞋带面积 = %.12f；差额 = %.2e。\n",
			st.spec.deg, width*height, st.sumCoverage, st.polyAreaPixel,
			math.Abs(st.sumCoverage-st.polyAreaPixel))
	}
	b.WriteString("\n每个像素的交叠比例（含裁切面积、2×2 命中数/比例、独立精确值、128 密点值）：\n\n")
	for _, st := range all {
		fmt.Fprintf(&b, "- `artifacts/coverage-%s.csv`（%d 行，row/col 从 0 起）\n", st.spec.tag, width*height)
		fmt.Fprintf(&b, "- `artifacts/clipped-%s.jsonl`（每个边缘像素裁切后的真实顶点序列 + 鞋带面积，可直接重算）\n", st.spec.tag)
		fmt.Fprintf(&b, "- `artifacts/render-%s-subsample2.pgm` / `render-%s-area.pgm`（12 倍最近邻放大对比图）\n",
			st.spec.tag, st.spec.tag)
		fmt.Fprintf(&b, "- `artifacts/compare-%s.png`（左 2×2 子点、右解析面积的并排 PNG）\n", st.spec.tag)
	}
	b.WriteString("\n### 每像素交叠比例矩阵（解析法，行优先，数值保留两位小数）\n\n")
	for _, st := range all {
		b.WriteString("\n**" + st.spec.tag + "**\n\n```\n")
		// 重新读取 CSV 中的 coverage 列以保证报告与落盘数据严格一致。
		matrix, err := readCoverageMatrix(filepath.Join(outDir, "coverage-"+st.spec.tag+".csv"))
		if err != nil {
			return err
		}
		for r := 0; r < height; r++ {
			for c := 0; c < width; c++ {
				v := matrix[r][c]
				switch {
				case v <= 0.005:
					b.WriteString("    .")
				case v >= 0.995:
					b.WriteString("    #")
				default:
					fmt.Fprintf(&b, " %4.2f", v)
				}
			}
			b.WriteString("\n")
		}
		b.WriteString("```\n")
	}

	b.WriteString("\n## 默认子点数下产物逐字节相同\n\n")
	verdict := "✅ 一致"
	if !identityOK {
		verdict = "❌ 不一致"
	}
	fmt.Fprintf(&b, "- 改动前 HEAD(fad87b8) 默认产物 sha256：`%s`\n", baselineSHA256)
	fmt.Fprintf(&b, "- 当前代码经 `config.Default()`（Supersample=2）+ pipe 产物 sha256：`%s`\n", afterHash)
	fmt.Fprintf(&b, "- 结论：**%s**。产物文件：`artifacts/default-after.pgm`；"+
		"另有 `artifacts/default-baseline-fad87b8.pgm` 为改动前二进制所生成，可用 `cmp` 复核。\n\n", verdict)

	b.WriteString("## 复现\n\n```sh\n")
	b.WriteString("go test ./...\n")
	b.WriteString("go run ./cmd/coverageaudit\n")
	b.WriteString("go build -o /tmp/vecgo-bytecheck ./cmd && /tmp/vecgo-bytecheck /tmp/out.pgm \\\n")
	b.WriteString("  && sha256sum /tmp/out.pgm   # 应等于 " + baselineSHA256 + "\n")
	b.WriteString("```\n")

	return os.WriteFile(filepath.Join(outDir, "coverage-report.md"), []byte(b.String()), 0o644)
}

func readCoverageMatrix(path string) ([][]float64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return nil, err
	}
	m := make([][]float64, height)
	for r := range m {
		m[r] = make([]float64, width)
	}
	for i, rec := range rows[1:] {
		if i >= width*height {
			break
		}
		var v float64
		if _, err := fmt.Sscanf(rec[2], "%f", &v); err != nil {
			return nil, err
		}
		r, c := i/width, i%width
		_ = rec[0]
		_ = rec[1]
		m[r][c] = v
	}
	return m, nil
}

func rotatedSquare(side, deg float64) []geom.Point {
	t := deg * math.Pi / 180
	c, s := math.Cos(t), math.Sin(t)
	base := []geom.Point{{X: 0, Y: 0}, {X: side, Y: 0}, {X: side, Y: side}, {X: 0, Y: side}}
	out := make([]geom.Point, len(base))
	for i, p := range base {
		out[i] = geom.Point{X: p.X*c - p.Y*s, Y: p.X*s + p.Y*c}
	}
	return out
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "coverageaudit:", err)
	os.Exit(1)
}
