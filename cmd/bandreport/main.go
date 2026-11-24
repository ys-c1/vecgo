// Command bandreport 复现“渐变色彩带”实验：
//   - 两档画幅（默认 64×64 与第二档 256×256），8-bit PGM（256 色阶）；
//   - 每种插值空间各出一张 .pgm 与最近邻放大 .png（含临界接缝红线图）；
//   - 从落盘的 PGM 重新读回像素数带（不是直接数内存）；
//   - 用闭式/量化公式独立预测，打印预测 vs 实测对照，并写 results.json。
//
// 用法：
//
//	go run ./cmd/bandreport [-out artifacts/banding]
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"vecgo/band"
	"vecgo/colorspace"
	"vecgo/config"
	"vecgo/geom"
	"vecgo/pipe"
	"vecgo/post"
	"vecgo/raster"
)

var scene = [][]geom.Point{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}}

type Row struct {
	Size              int     `json:"size"`
	Space             string  `json:"space"`
	Runs              int     `json:"runs"`
	Distinct          int     `json:"distinct"`
	MaxGap            int     `json:"maxGap"`
	Weber02           int     `json:"weber02"`
	Weber05           int     `json:"weber05"`
	Weber10           int     `json:"weber10"`
	DL2               int     `json:"dl2"`
	MaxDL             float64 `json:"maxDL"`
	PredRuns          int     `json:"predRuns"`
	PredWeber10       int     `json:"predWeber10"`
	ContinuousWeber10 int     `json:"continuousWeber10"`
}

func main() {
	outDir := flag.String("out", "artifacts/banding", "产物目录")
	flag.Parse()
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fatal(err)
	}

	sizes := [][2]int{{64, 64}, {256, 256}}
	spaces := []string{"encoded", "linear", "perceptual"}
	var rows []Row

	for _, wh := range sizes {
		for _, sp := range spaces {
			row := runOne(*outDir, wh[0], wh[1], sp)
			rows = append(rows, row)
		}
	}

	printTable(rows)
	writeJSON(filepath.Join(*outDir, "results.json"), rows)
	writeMD(filepath.Join(*outDir, "table.generated.md"), rows)
	fmt.Println("\n图像与结果已写入", *outDir)
}

func runOne(outDir string, w, h int, sp string) Row {
	cfg := config.Default()
	cfg.Width, cfg.Height = w, h
	cfg.Supersample = 2
	cfg.Shading = "gradient"
	cfg.Gamma = 1
	cfg.Blur = 0
	cfg.InterpSpace = sp

	im, err := pipe.Run(scene, cfg)
	if err != nil {
		fatal(err)
	}
	stem := fmt.Sprintf("%dx%d_%s", w, h, sp)
	pgmPath := filepath.Join(outDir, stem+".pgm")
	if err := im.SavePGM(pgmPath); err != nil {
		fatal(err)
	}

	// 从落盘 PGM 读回再数，确保“实测”针对真正产物。
	disk, err := loadPGM(pgmPath)
	if err != nil {
		fatal(err)
	}
	x := disk.W / 2
	col := band.Column(disk, x)
	_, maxGap := band.Gaps(col)

	var lut *[256]uint8
	if config.NormalizeSpace(sp) == "encoded" {
		t := post.GammaTable(1) // 默认链路 pipe 会执行 post.Gamma(gamma=1)
		lut = &t
	}
	pred := band.PredictColumn(h, sp, lut)
	predIm := &raster.Image{W: 1, H: h, Pix: pred}
	predCol := band.Column(predIm, 0)

	r := Row{
		Size:        h,
		Space:       sp,
		Runs:        band.CountRuns(col),
		Distinct:    band.CountDistinct(col),
		MaxGap:      maxGap,
		Weber02:     band.CountWeberCritical(col, 0.02),
		Weber05:     band.CountWeberCritical(col, 0.05),
		Weber10:     band.CountWeberCritical(col, 0.10),
		DL2:         band.CountDeltaLCritical(col, 2.0),
		MaxDL:       round2(band.MaxDeltaL(col)),
		PredRuns:    band.CountRuns(predCol),
		PredWeber10: band.CountWeberCritical(predCol, 0.10),
		// 线性光均匀插值的连续闭式预测 ≈ ceil(1/w)；其它空间不给该闭式值。
		ContinuousWeber10: 0,
	}
	if sp == "linear" {
		r.ContinuousWeber10 = band.LinearSpaceContinuousPrediction(0.10)
	}

	scale := 8
	if w >= 256 {
		scale = 4
	}
	writeGrayPNG(filepath.Join(outDir, stem+"_x.png"), disk, scale, nil)
	// Weber 10% 临界接缝红线叠加图。
	seams := weberSeamRows(col, 0.10)
	writeGrayPNG(filepath.Join(outDir, stem+"_seams.png"), disk, scale, seams)
	return r
}

// predictColumn 的实现已下沉到 band.PredictColumn，供命令与测试共用同一预测口径。

func weberSeamRows(col []uint8, w float64) map[int]bool {
	m := map[int]bool{}
	for i := 1; i < len(col); i++ {
		ya := colorspace.ByteToLinear(col[i-1])
		yb := colorspace.ByteToLinear(col[i])
		lo := minF(ya, yb)
		d := absF(ya - yb)
		var c float64
		if lo <= 1e-15 {
			if d > 1e-15 {
				c = 1e9
			}
		} else {
			c = d / lo
		}
		if c >= w {
			m[i] = true
		}
	}
	return m
}

func writeGrayPNG(path string, im *raster.Image, scale int, seams map[int]bool) {
	rect := image.Rect(0, 0, im.W*scale, im.H*scale)
	img := image.NewRGBA(rect)
	for y := 0; y < im.H; y++ {
		for x := 0; x < im.W; x++ {
			v := im.Pix[y*im.W+x]
			c := color.RGBA{R: v, G: v, B: v, A: 255}
			for dy := 0; dy < scale; dy++ {
				for dx := 0; dx < scale; dx++ {
					img.SetRGBA(x*scale+dx, y*scale+dy, c)
				}
			}
		}
	}
	// 接缝只在两行之间的边界画细红线，不把整条色带涂红。
	if len(seams) > 0 {
		thick := 2
		if scale >= 8 {
			thick = 3
		}
		red := color.RGBA{R: 230, G: 0, B: 0, A: 255}
		for row := range seams {
			y0 := row * scale
			for t := 0; t < thick; t++ {
				for sx := 0; sx < im.W*scale; sx++ {
					img.SetRGBA(sx, y0+t, red)
				}
			}
		}
	}
	f, err := os.Create(path)
	if err != nil {
		fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		fatal(err)
	}
}

// loadPGM 读入 P5 binary PGM（maxval 255）。报告工具只需要这一种。
func loadPGM(path string) (*raster.Image, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	pos := 0
	tok := func() string {
		for pos < len(b) && (b[pos] == ' ' || b[pos] == '\n' || b[pos] == '\t' || b[pos] == '\r') {
			pos++
		}
		start := pos
		for pos < len(b) && b[pos] > ' ' {
			pos++
		}
		return string(b[start:pos])
	}
	if tok() != "P5" {
		return nil, fmt.Errorf("不是 P5: %s", path)
	}
	w := atoi(tok())
	h := atoi(tok())
	mv := atoi(tok())
	if mv != 255 {
		return nil, fmt.Errorf("只支持 maxval=255，得到 %d", mv)
	}
	pos++ // 单个空白符后即像素数据
	if pos+w*h > len(b) {
		return nil, fmt.Errorf("像素数据不足")
	}
	im, err := raster.New(w, h)
	if err != nil {
		return nil, err
	}
	copy(im.Pix, b[pos:pos+w*h])
	return im, nil
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		n = n*10 + int(c-'0')
	}
	return n
}

func printTable(rows []Row) {
	fmt.Println("画幅 | 空间 | 等值段 | 不同码 | 最大码差 | Weber≥2% | ≥5% | ≥10% | ΔL*≥2 | 最大ΔL* | 预测段 | 预测W10 | 连续W10")
	fmt.Println(strings.Repeat("-", 120))
	for _, r := range rows {
		fmt.Printf("%3d | %-12s | %4d | %4d | %4d | %4d | %4d | %4d | %4d | %5.2f | %4d | %4d | %4d\n",
			r.Size, r.Space, r.Runs, r.Distinct, r.MaxGap, r.Weber02, r.Weber05, r.Weber10,
			r.DL2, r.MaxDL, r.PredRuns, r.PredWeber10, r.ContinuousWeber10)
	}
}

func writeMD(path string, rows []Row) {
	var b strings.Builder
	b.WriteString("| 画幅 H | 空间 | 实测等值段 | 预测段 | 实测 Weber≥0.10 接缝 | 量化预测 | 连续闭式 |\n")
	b.WriteString("|---:|---|---:|---:|---:|---:|---:|\n")
	for _, r := range rows {
		fmt.Fprintf(&b, "| %d | %s | %d | %d | %d | %d | %s |\n",
			r.Size, r.Space, r.Runs, r.PredRuns, r.Weber10, r.PredWeber10, dash0(r.ContinuousWeber10))
	}
	_ = os.WriteFile(path, []byte(b.String()), 0o644)
}

func writeJSON(path string, rows []Row) {
	b, _ := json.MarshalIndent(rows, "", "  ")
	_ = os.WriteFile(path, b, 0o644)
}

func dash0(v int) string {
	if v == 0 {
		return "—"
	}
	return fmt.Sprintf("%d", v)
}
func round2(v float64) float64 { return float64(int(v*100+0.5)) / 100 }
func minF(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
func absF(a float64) float64 {
	if a < 0 {
		return -a
	}
	return a
}
func fatal(err error) { fmt.Fprintln(os.Stderr, "错误:", err); os.Exit(1) }
