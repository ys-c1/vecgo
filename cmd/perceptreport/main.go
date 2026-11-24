// Command perceptreport 复现“感知均匀色阶分布”的两档画幅数据：
// 参数轴暗部间距方差 vs 闭式/理想预测，并真实运行 pipe、落盘 PGM 后读回复核。
//
//	go run ./cmd/perceptreport -out artifacts/perceptual
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"vecgo/config"
	"vecgo/geom"
	"vecgo/level"
	"vecgo/pipe"
)

var sizes = []int{64, 256}

type axisRow struct {
	Size       int     `json:"size"`
	DarkGaps   int     `json:"darkGaps"`
	VLinear    float64 `json:"vLinear"`
	VPredLin   float64 `json:"vPredLinearClosed"`
	VPerc      float64 `json:"vPerceptual"`
	VPredPerc  float64 `json:"vPredPerceptual"`
	ReductionX float64 `json:"reductionFactor"`
	// CIE L* 交叉验证（视觉坐标单位 ΔL*²）
	VLinLab  float64 `json:"vLinearLab"`
	VPercLab float64 `json:"vPerceptualLab"`
}

type imageRow struct {
	Size          int     `json:"size"`
	Cutoff        int     `json:"cutoff"`
	LinLevels     int     `json:"linearLevels"`
	VLinCode      float64 `json:"vLinearCode"`
	PercLevels    int     `json:"perceptualLevels"`
	VPercCode     float64 `json:"vPerceptualCode"`
	ReductionX    float64 `json:"reductionFactor"`
	PGMLinear     string  `json:"pgmLinear"`
	PGMPerceptual string  `json:"pgmPerceptual"`
}

type report struct {
	Alpha        float64    `json:"alpha"`
	DarkHalfNote string     `json:"darkHalfNote"`
	Axis         []axisRow  `json:"axis"`
	Image        []imageRow `json:"imageReadback"`
}

func scene() [][]geom.Point {
	return [][]geom.Point{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}}
}

func main() {
	out := flag.String("out", "artifacts/perceptual", "产物目录（空串表示只打印不落盘）")
	flag.Parse()

	rep := report{
		Alpha:        level.DefaultAlpha,
		DarkHalfNote: "暗部=由暗端起 m=(H-1)/2 个相邻间隔（约下半段）",
	}

	fmt.Println("参数轴（连续）暗部相邻色阶间距方差 vs 预测")
	fmt.Printf("%-6s %-5s %-13s %-13s %-12s %-8s %-12s\n",
		"画幅H", "m", "线性实测", "线性闭式预测", "感知实测", "感知预测", "降幅")

	var imgRows []imageRow

	for _, h := range sizes {
		mLin, _, vLin, err := level.DarkVariance(h, level.Linear, 2)
		ck(err)
		mCF, _, vPredLin := level.PredictLinearDarkVarianceSq(h)
		if mLin != mCF {
			panic("dark count mismatch")
		}
		_, _, vPerc, err := level.DarkVariance(h, level.Perceptual, 2)
		ck(err)
		tsL, err := level.Positions(h, level.Linear, 0)
		ck(err)
		gL, err := level.HalfDarkGaps(tsL, level.Lab, 0)
		ck(err)
		vLinLab := level.Variance(gL)
		tsP, err := level.Positions(h, level.Lab, 0)
		ck(err)
		gP, err := level.HalfDarkGaps(tsP, level.Lab, 0)
		ck(err)
		vPercLab := level.Variance(gP)

		row := axisRow{
			Size: h, DarkGaps: mLin,
			VLinear: vLin, VPredLin: vPredLin,
			VPerc: vPerc, VPredPerc: level.IdealPerceptualVariance(),
			ReductionX: vLin / maxF(vPerc),
			VLinLab:    vLinLab, VPercLab: vPercLab,
		}
		rep.Axis = append(rep.Axis, row)
		fmt.Printf("%-6d %-5d %-13.6e %-13.6e %-12.3e %-8.0f %.2e×\n",
			h, mLin, vLin, vPredLin, vPerc, 0.0, row.ReductionX)

		// 真实渲染两档画幅的线性 / 感知 PGM，落盘后读回中央列复核。
		if *out != "" {
			ck(os.MkdirAll(*out, 0o755))
		}
		iRow := renderAndMeasure(*out, h)
		imgRows = append(imgRows, iRow)
	}
	rep.Image = imgRows

	fmt.Println()
	fmt.Println("落盘 8-bit PGM 读回的暗部码间距方差（P=(k/255)^2, cutoff=127）")
	fmt.Printf("%-6s %-12s %-13s %-12s %-13s %-10s\n",
		"画幅H", "线性色阶数", "线性方差", "感知色阶数", "感知方差", "降幅")
	for _, r := range imgRows {
		fmt.Printf("%-6d %-12d %-13.6e %-12d %-13.6e %.2f×\n",
			r.Size, r.LinLevels, r.VLinCode, r.PercLevels, r.VPercCode, r.ReductionX)
	}

	if *out != "" {
		writeJSON(*out, rep)
		writeTable(*out, rep)
		fmt.Println("\n已写出:", filepath.Join(*out, "results.json"))
		fmt.Println("已写出:", filepath.Join(*out, "table.generated.md"))
	}
}

func renderAndMeasure(outDir string, h int) imageRow {
	cutoff := 127
	mk := func(dist string) *config.Config {
		c := config.Default()
		c.Width, c.Height = h, h
		c.Shading = "gradient"
		c.LevelDist = dist
		return &c
	}
	scene := scene()

	lin, err := pipe.Run(scene, *mk("linear"))
	ck(err)
	perc, err := pipe.Run(scene, *mk("perceptual"))
	ck(err)

	linPath := fmt.Sprintf("linear_%d.pgm", h)
	percPath := fmt.Sprintf("perceptual_%d.pgm", h)
	if outDir != "" {
		ck(lin.SavePGM(filepath.Join(outDir, linPath)))
		ck(perc.SavePGM(filepath.Join(outDir, percPath)))
		linCodes := readCenterColumn(filepath.Join(outDir, linPath), h)
		percCodes := readCenterColumn(filepath.Join(outDir, percPath), h)
		nL, _, vL, err := level.DarkCodeVariance(linCodes, 2, cutoff)
		ck(err)
		nP, _, vP, err := level.DarkCodeVariance(percCodes, 2, cutoff)
		ck(err)
		red := 0.0
		if vP > 0 {
			red = vL / vP
		}
		return imageRow{Size: h, Cutoff: cutoff, LinLevels: nL, VLinCode: vL,
			PercLevels: nP, VPercCode: vP, ReductionX: red,
			PGMLinear: linPath, PGMPerceptual: percPath}
	}
	// 不落盘时直接用内存像素。
	linCodes := level.ColumnCodes(lin.Pix, lin.W, lin.H, lin.W/2)
	percCodes := level.ColumnCodes(perc.Pix, perc.W, perc.H, perc.W/2)
	nL, _, vL, _ := level.DarkCodeVariance(linCodes, 2, cutoff)
	nP, _, vP, _ := level.DarkCodeVariance(percCodes, 2, cutoff)
	red := 0.0
	if vP > 0 {
		red = vL / vP
	}
	return imageRow{Size: h, Cutoff: cutoff, LinLevels: nL, VLinCode: vL,
		PercLevels: nP, VPercCode: vP, ReductionX: red}
}

// readCenterColumn 从落盘的 P5 PGM 读回并取中央列，证明统计基于真实产物而非内存理想值。
func readCenterColumn(path string, expectH int) []uint8 {
	data, err := os.ReadFile(path)
	ck(err)
	br := bufio.NewReader(bytes.NewReader(data))
	var magic string
	if _, err := fmt.Fscan(br, &magic); err != nil {
		panic(err)
	}
	if magic != "P5" {
		panic("not P5: " + magic)
	}
	var w, h, maxv int
	_, _ = fmt.Fscan(br, &w)
	_, _ = fmt.Fscan(br, &h)
	_, _ = fmt.Fscan(br, &maxv)
	if _, err := br.ReadByte(); err != nil { // 吃掉 maxval 后的单个空白
		panic(err)
	}
	pix := make([]byte, w*h)
	if _, err := io.ReadFull(br, pix); err != nil {
		panic(err)
	}
	if h != expectH {
		panic("height mismatch")
	}
	return level.ColumnCodes(pix, w, h, w/2)
}

func writeJSON(dir string, rep report) {
	b, err := json.MarshalIndent(rep, "", "  ")
	ck(err)
	ck(os.WriteFile(filepath.Join(dir, "results.json"), b, 0o644))
}

func writeTable(dir string, rep report) {
	var b bytes.Buffer
	fmt.Fprintln(&b, "| 画幅 H | 暗部间隔 m | 默认线性·实测方差 | 线性闭式预测 | 感知分布·实测方差 | 感知预测 | 降幅 |")
	fmt.Fprintln(&b, "|---:|---:|---:|---:|---:|---:|---:|")
	for _, r := range rep.Axis {
		fmt.Fprintf(&b, "| %d | %d | %.6e | %.6e | %.3e | %g | %.2e× |\n",
			r.Size, r.DarkGaps, r.VLinear, r.VPredLin, r.VPerc, r.VPredPerc, r.ReductionX)
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "CIE L* 交叉验证（视觉坐标 ΔL*²，按序列前半间隔）：")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "| 画幅 H | 线性 L* 方差 | 感知(L*)方差 |")
	fmt.Fprintln(&b, "|---:|---:|---:|")
	for _, r := range rep.Axis {
		fmt.Fprintf(&b, "| %d | %.6f | %.3e |\n", r.Size, r.VLinLab, r.VPercLab)
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "落盘 8-bit PGM 读回（P=(k/255)^2，暗部 cutoff=127）：")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "| 画幅 H | 线性色阶数 | 线性码方差 | 感知色阶数 | 感知码方差 | 降幅 |")
	fmt.Fprintln(&b, "|---:|---:|---:|---:|---:|---:|")
	for _, r := range rep.Image {
		fmt.Fprintf(&b, "| %d | %d | %.6e | %d | %.6e | %.2f× |\n",
			r.Size, r.LinLevels, r.VLinCode, r.PercLevels, r.VPercCode, r.ReductionX)
	}
	ck(os.WriteFile(filepath.Join(dir, "table.generated.md"), b.Bytes(), 0o644))
}

func maxF(v float64) float64 {
	if v < 1e-300 {
		return 1e-300
	}
	return v
}

func ck(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		os.Exit(1)
	}
}
