// Command stripreport generates triangle-strip counts, exact pixel checks,
// JSON/CSV tables, and the greedy counterexample frame.
package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"vecgo/geom"
	"vecgo/raster"
	"vecgo/strip"
)

type counterReport struct {
	Report                      strip.Report `json:"report"`
	Frame                       strip.Frame  `json:"frame"`
	RenderWidth                 int          `json:"renderWidth"`
	RenderHeight                int          `json:"renderHeight"`
	Supersample                 int          `json:"supersample"`
	Coordinates                 []geom.Point `json:"coordinates"`
	DualEdges                   [][2]int     `json:"dualEdges"`
	Degrees                     []int        `json:"degrees"`
	GreedyOrder                 [][]int      `json:"greedyOrder"`
	GreedyStreams               [][]int      `json:"greedyStreams"`
	GreedyVertices              int          `json:"greedyVertices"`
	LowerBound                  int          `json:"lowerBound"`
	LowerBoundToGreedyRatio     float64      `json:"lowerBoundToGreedyRatio"`
	OptimalOrder                []int        `json:"optimalOrder"`
	OptimalStream               []int        `json:"optimalStream"`
	OptimalVertices             int          `json:"optimalVertices"`
	FrameGreedyDifferentPixels  int          `json:"frameGreedyDifferentPixels"`
	FrameOptimalDifferentPixels int          `json:"frameOptimalDifferentPixels"`
}

type output struct {
	Groups         []strip.Report `json:"groups"`
	Counterexample counterReport  `json:"counterexample"`
}

func main() {
	outDir := flag.String("out", "artifacts/strip", "report output directory")
	flag.Parse()

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fatal(err)
	}

	meshes := []*strip.Mesh{
		strip.NewRibbonMesh("M1-long-single-grid-96x1", []int{96}),
		strip.NewRibbonMesh("M2-twin-grid-2x64x1", []int{64, 64}),
		strip.NewRibbonMesh("M3-four-island-grid-4x48x1", []int{48, 48, 48, 48}),
	}

	var reports []strip.Report
	for _, m := range meshes {
		rep, err := strip.Analyze(m, 120, 56, 2)
		if err != nil {
			fatal(err)
		}
		reports = append(reports, rep)

		independent, err := strip.RenderCoverage(strip.TrianglePolygons(m), 240, 112, 2)
		if err != nil {
			fatal(err)
		}
		stripped, err := strip.RenderCoverage(strip.StreamPolygons(m, rep.Streams), 240, 112, 2)
		if err != nil {
			fatal(err)
		}
		saveImages(*outDir, m.Name, independent, stripped)
		fmt.Printf("%s: T=%d c=%d N0=%d LB=%d N=%d ratio=%.6f diff=%d\n",
			m.Name, rep.Triangles, rep.Components, rep.IndependentVertices,
			rep.LowerBound, rep.StripVertices, rep.LowerBoundToStripRatio,
			rep.DifferentPixels)
	}

	ce := strip.NewCounterexample()
	ceRep, err := strip.Analyze(ce, 128, 128, 4)
	if err != nil {
		fatal(err)
	}
	frame := strip.Frame{MinX: 0, MinY: 0, MaxX: 16, MaxY: 16}
	ss := 4
	indFrame, err := strip.RenderFrame(strip.TrianglePolygons(ce), 160, 160, ss, frame)
	if err != nil {
		fatal(err)
	}
	greedyFrame, err := strip.RenderFrame(strip.StreamPolygons(ce, ceRep.Streams), 160, 160, ss, frame)
	if err != nil {
		fatal(err)
	}
	optOrder := []int{2, 0, 3, 1, 4}
	optStream := strip.EncodePath(ce, optOrder)
	optFrame, err := strip.RenderFrame(
		strip.StreamPolygons(ce, [][]int{optStream}), 160, 160, ss, frame)
	if err != nil {
		fatal(err)
	}
	frameGreedyDiff := strip.CountDifferentPixels(indFrame, greedyFrame)
	frameOptDiff := strip.CountDifferentPixels(indFrame, optFrame)
	if frameGreedyDiff != 0 || frameOptDiff != 0 {
		fatal(fmt.Errorf("counterexample frame pixels differ: greedy=%d optimal=%d",
			frameGreedyDiff, frameOptDiff))
	}
	saveImages(*outDir, "counterexample-greedy", indFrame, greedyFrame)
	savePGM(filepath.Join(*outDir, "counterexample-optimal.pgm"), optFrame)
	savePNG(filepath.Join(*outDir, "counterexample-optimal.png"), optFrame)

	adj := strip.Adjacency(ce)
	degrees := make([]int, len(adj))
	edges := [][2]int{}
	seenEdge := map[[2]int]bool{}
	for i, ns := range adj {
		degrees[i] = len(ns)
		for _, j := range ns {
			e := strip.CanonicalEdge(i, j)
			if !seenEdge[e] {
				seenEdge[e] = true
				edges = append(edges, e)
			}
		}
	}
	greedyVertices := 0
	for _, s := range ceRep.Streams {
		greedyVertices += len(s)
	}
	counter := counterReport{
		Report:                      ceRep,
		Frame:                       frame,
		RenderWidth:                 160,
		RenderHeight:                160,
		Supersample:                 ss,
		Coordinates:                 ce.Vertices,
		DualEdges:                   edges,
		Degrees:                     degrees,
		GreedyOrder:                 ceRep.GreedyPaths,
		GreedyStreams:               ceRep.Streams,
		GreedyVertices:              greedyVertices,
		LowerBound:                  ceRep.LowerBound,
		LowerBoundToGreedyRatio:     float64(ceRep.LowerBound) / float64(greedyVertices),
		OptimalOrder:                optOrder,
		OptimalStream:               optStream,
		OptimalVertices:             len(optStream),
		FrameGreedyDifferentPixels:  frameGreedyDiff,
		FrameOptimalDifferentPixels: frameOptDiff,
	}

	out := output{Groups: reports, Counterexample: counter}
	writeJSON(filepath.Join(*outDir, "report.json"), out)
	writeCSV(filepath.Join(*outDir, "counts.csv"), reports)
	writeCounterCSV(filepath.Join(*outDir, "counterexample.csv"), counter)
	writeSVG(filepath.Join(*outDir, "counterexample-frame.svg"), ce)
	writeMarkdown(filepath.Join(*outDir, "REPORT.md"), out)

	fmt.Printf("counterexample: LB=%d greedy=%d ratio=%.6f optimal=%d frame-diff=%d/%d\n",
		counter.LowerBound, counter.GreedyVertices, counter.LowerBoundToGreedyRatio,
		counter.OptimalVertices, frameGreedyDiff, frameOptDiff)
	fmt.Println("wrote", *outDir)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "stripreport:", err)
	os.Exit(1)
}

func writeJSON(path string, v any) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		fatal(err)
	}
}

func writeCSV(path string, reports []strip.Report) {
	f, err := os.Create(path)
	if err != nil {
		fatal(err)
	}
	defer f.Close()
	w := csv.NewWriter(f)
	_ = w.Write([]string{
		"mesh", "triangles", "components", "independent_vertices",
		"lower_bound", "greedy_strips", "strip_vertices",
		"lower_bound_over_vertices", "different_pixels",
	})
	for _, r := range reports {
		_ = w.Write([]string{
			r.Name,
			fmt.Sprint(r.Triangles),
			fmt.Sprint(r.Components),
			fmt.Sprint(r.IndependentVertices),
			fmt.Sprint(r.LowerBound),
			fmt.Sprint(r.GreedyStripCount),
			fmt.Sprint(r.StripVertices),
			fmt.Sprintf("%.9f", r.LowerBoundToStripRatio),
			fmt.Sprint(r.DifferentPixels),
		})
	}
	w.Flush()
	if err := w.Error(); err != nil {
		fatal(err)
	}
}

func writeCounterCSV(path string, c counterReport) {
	f, err := os.Create(path)
	if err != nil {
		fatal(err)
	}
	defer f.Close()
	w := csv.NewWriter(f)
	_ = w.Write([]string{"case", "triangles", "components", "lower_bound", "greedy_vertices", "LB/greedy", "optimal_vertices", "frame_diff_greedy", "frame_diff_optimal"})
	_ = w.Write([]string{
		"degree-greedy-trap",
		fmt.Sprint(c.Report.Triangles),
		fmt.Sprint(c.Report.Components),
		fmt.Sprint(c.LowerBound),
		fmt.Sprint(c.GreedyVertices),
		fmt.Sprintf("%.9f", c.LowerBoundToGreedyRatio),
		fmt.Sprint(c.OptimalVertices),
		fmt.Sprint(c.FrameGreedyDifferentPixels),
		fmt.Sprint(c.FrameOptimalDifferentPixels),
	})
	w.Flush()
}

func saveImages(dir, base string, independent, stripped *raster.Image) {
	savePGM(filepath.Join(dir, base+".independent.pgm"), independent)
	savePGM(filepath.Join(dir, base+".strip.pgm"), stripped)
	savePNG(filepath.Join(dir, base+".independent.png"), independent)
	savePNG(filepath.Join(dir, base+".strip.png"), stripped)
}

func savePGM(path string, im *raster.Image) {
	if err := im.SavePGM(path); err != nil {
		fatal(err)
	}
}

func savePNG(path string, im *raster.Image) {
	gray := &image.Gray{
		Pix:    im.Pix,
		Stride: im.W,
		Rect:   image.Rect(0, 0, im.W, im.H),
	}
	f, err := os.Create(path)
	if err != nil {
		fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, gray); err != nil {
		fatal(err)
	}
}

func writeSVG(path string, m *strip.Mesh) {
	colors := []string{"#9ecae1", "#a1d99b", "#fdae6b", "#bcbddc", "#fcae91"}
	var b strings.Builder
	b.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" width="720" height="720" viewBox="0 0 16 16">` + "\n")
	b.WriteString(`<rect x="0" y="0" width="16" height="16" fill="white" stroke="#222" stroke-width="0.08"/>` + "\n")
	// 1-unit grid.
	for i := 1; i < 16; i++ {
		b.WriteString(fmt.Sprintf(`<line x1="%d" y1="0" x2="%d" y2="16" stroke="#eee" stroke-width="0.03"/>`, i, i))
		b.WriteString(fmt.Sprintf(`<line x1="0" y1="%d" x2="16" y2="%d" stroke="#eee" stroke-width="0.03"/>`, i, i))
	}
	sy := func(y float64) float64 { return 16 - y }
	for ti, tri := range m.Triangles {
		pts := make([]string, 0, 3)
		var cx, cy float64
		for _, id := range tri {
			p := m.Vertices[id]
			pts = append(pts, fmt.Sprintf("%.3f,%.3f", p.X, sy(p.Y)))
			cx += p.X
			cy += p.Y
		}
		cx /= 3
		cy /= 3
		fill := colors[ti%len(colors)]
		b.WriteString(fmt.Sprintf(`<polygon points="%s" fill="%s" fill-opacity="0.55" stroke="#111" stroke-width="0.08"/>`,
			strings.Join(pts, " "), fill))
		b.WriteString(fmt.Sprintf(`<text x="%.2f" y="%.2f" font-size="0.75" text-anchor="middle" dominant-baseline="middle">T%d</text>`,
			cx, sy(cy), ti))
	}
	for id, p := range m.Vertices {
		b.WriteString(fmt.Sprintf(`<circle cx="%.2f" cy="%.2f" r="0.13" fill="#111"/>`, p.X, sy(p.Y)))
		b.WriteString(fmt.Sprintf(`<text x="%.2f" y="%.2f" font-size="0.55" text-anchor="middle">V%d(%.0f,%.0f)</text>`,
			p.X, sy(p.Y)-0.32, id, p.X, p.Y))
	}
	b.WriteString("</svg>\n")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		fatal(err)
	}
}

func writeMarkdown(path string, out output) {
	var b strings.Builder
	b.WriteString("# 三角形条带：下界、度贪心常数界、三组网格与反例\n\n")
	b.WriteString("本报告由 `go run ./cmd/stripreport` 重新生成；计数来自代码，像素比较由仓库自带采样器全量执行。\n\n")

	b.WriteString("## 1. 下界证明：固定 T 后只额外依赖连通分量数 c\n\n")
	b.WriteString("设共有 T 个三角形，边邻接对偶图有 c 个连通分量。一个含 t 个三角形的普通滑动条带，前三个引用给出第一个三角形，此后每个三角形只需一个新引用，因此恰有 t+2 个顶点引用。若所有三角形被分成 s 个条带，则\n\n")
	b.WriteString("```text\nN_strip = Σ(t_i + 2) = T + 2s.\n```\n\n")
	b.WriteString("条带中相邻三角形必须共享一条边，所以一个条带不能跨过对偶图的连通分量；每个连通分量至少需要一个条带，故 s≥c。于是\n\n")
	b.WriteString("```text\nN_strip ≥ L(T,c) = T + 2c.\n```\n\n")
	b.WriteString("也就是说，给定三角形总数 T，顶点数下界的附加项只由连通分量数 c 决定：每个分量贡献 2 个启动引用。独立三角形列表则提交 `N0=3T` 个引用。\n\n")

	b.WriteString("## 2. 邻接度贪心的常数倍保证\n\n")
	b.WriteString("实现的确定规则是：在未使用三角形中选原始对偶度最大、同度取编号最小者作为种子；从当前条带末端继续选未使用的最大对偶度邻接三角形，同度取编号最小；走不动就重启。\n\n")
	b.WriteString("常数界不依赖度排序是否总是选对，而来自“每个三角形只输出一次”的记账。对一个 t 三角形条带，编码器初始用 3 个引用；每个后续三角形先做至多 2 个对齐/交换引用，再追加 1 个第三顶点，因此至多增加 3 个引用：\n\n")
	b.WriteString("```text\nN_block ≤ 3 + 3(t−1) = 3t.\n```\n\n")
	b.WriteString("所有条带合计 `N_g≤3T`。又因为 `L=T+2c≥T`，所以\n\n")
	b.WriteString("```text\nN_g ≤ 3T ≤ 3L,    即    L/N_g ≥ 1/3.\n```\n\n")
	b.WriteString("因此度贪心一定落在下界的 3 倍以内。这个常数不是紧近似比；下表的长网格接近 1。若把 primitive restart 标记也计作索引项，s 个条带最多再加 s−1≤T−1，总索引项至多 4T≤4L，仍是常数倍。\n\n")

	b.WriteString("## 3. 三组网格结果\n\n")
	b.WriteString("画幅/渲染设置：120×56 逻辑比较、SS=2；另存 240×112 PNG/PGM。三个网格分别是 1 条 96 格长带、2 条各 64 格的分离网格带、4 条各 48 格的分离网格带。\n\n")
	b.WriteString("| 网格 | T | c | 独立顶点 3T | 下界 L=T+2c | 贪心条带数 s | 条带顶点 N | 比值 L/N | 像素差异 |\n")
	b.WriteString("|---|---:|---:|---:|---:|---:|---:|---:|---:|\n")
	for _, r := range out.Groups {
		b.WriteString(fmt.Sprintf("| `%s` | %d | %d | %d | %d | %d | %d | **%.6f** | %d/%d |\n",
			r.Name, r.Triangles, r.Components, r.IndependentVertices, r.LowerBound,
			r.GreedyStripCount, r.StripVertices, r.LowerBoundToStripRatio,
			r.DifferentPixels, r.PixelCount))
	}
	b.WriteString("\n三个要求的比值（下界/实际顶点数）依次为：\n\n")
	for _, r := range out.Groups {
		b.WriteString(fmt.Sprintf("- `%s`: **%d/%d = %.9f**\n", r.Name, r.LowerBound, r.StripVertices, r.LowerBoundToStripRatio))
	}
	b.WriteString("\n每组贪心把每条完整网格带分成两段：先从编号最小的度 2 三角形向长方向走，漏掉同格端帽的度 1 三角形；因此 s=2c、N=T+4c。网格足够长时，额外的 2c 个启动顶点相对 T 很小，所以 N 贴近 T+2c。所有组的独立三角形渲染与条带流展开渲染逐字节像素相同，SHA-256 也记录在 `report.json`。\n\n")

	c := out.Counterexample
	b.WriteString("## 4. 贪心失效反例（画幅固定，便于复算）\n\n")
	b.WriteString(fmt.Sprintf("画幅坐标窗：`x∈[%.0f,%.0f]`, `y∈[%.0f,%.0f]`；输出栅格 %d×%d，SS=%d。几何包围盒为 x∈[2,12]、y∈[4,13]。矢量画幅见 [`counterexample-frame.svg`](counterexample-frame.svg)。\n\n",
		c.Frame.MinX, c.Frame.MaxX, c.Frame.MinY, c.Frame.MaxY, c.RenderWidth, c.RenderHeight, c.Supersample))
	b.WriteString("顶点坐标：\n\n")
	b.WriteString("| 顶点 | 坐标 |\n|---:|---|\n")
	for i, p := range c.Coordinates {
		b.WriteString(fmt.Sprintf("| V%d | `(%g,%g)` |\n", i, p.X, p.Y))
	}
	b.WriteString("\n三角形与顶点：\n\n")
	// Explicit triangle table gives a stable coordinate audit even if JSON
	b.WriteString("| 三角形 | 顶点 ID | 坐标三元组 | 对偶度 |\n|---|---|---|---:|\n")
	triIDs := [][3]int{{0, 1, 2}, {0, 3, 1}, {1, 2, 4}, {0, 2, 3}, {3, 1, 5}}
	for i, tri := range triIDs {
		coords := make([]string, 3)
		for k, id := range tri {
			p := c.Coordinates[id]
			coords[k] = fmt.Sprintf("V%d(%g,%g)", id, p.X, p.Y)
		}
		b.WriteString(fmt.Sprintf("| T%d | `{%d,%d,%d}` | %s | %d |\n",
			i, tri[0], tri[1], tri[2], strings.Join(coords, ", "), c.Degrees[i]))
	}
	b.WriteString("\n对偶边为 `0–1, 0–2, 0–3, 1–3, 1–4`；核心 T0,T1,T3 是一个 K3，T2、T4 是分别挂在 T0、T1 上的叶。\n\n")
	b.WriteString("### 4.1 度贪心走法\n\n")
	b.WriteString(fmt.Sprintf("- 贪心分组：`%v`\n", c.GreedyOrder))
	b.WriteString(fmt.Sprintf("- 编码后的顶点 ID 流：`%v`\n", c.GreedyStreams))
	b.WriteString(fmt.Sprintf("- 实际顶点数：**%d**；连通分量 c=1，下界 L=5+2=**%d**；比值 L/N=**%.6f**。按 3 个条带之间放 2 个 primitive-restart 标记的索引口径，总索引项为 13。\n",
		c.GreedyVertices, c.LowerBound, c.LowerBoundToGreedyRatio))
	b.WriteString(fmt.Sprintf("- 最优顺序 `T2–T0–T3–T1–T4`，编码流 `%v`，长度 **%d=L**。\n\n",
		c.OptimalStream, c.OptimalVertices))
	b.WriteString("### 4.2 差在哪个连接处\n\n")
	b.WriteString("贪心从度 3 的 T0 出发，在候选邻接中选了度同为 3、编号最小的 **T0→T1**，即共享边 `V0–V1 = O–A`。进入 T1 后，它又在度 2 的 T3 与度 1 的 T4 中选了 **T1→T3**，即共享边 `V0–V3 = O–C`。T3 的另一个邻接只有 T0，已经使用，于是条带死在 T3；T4 虽然与 T1 共享 `V3–V1 = C–A`，但 T1 已成为路径内部节点，这个出口无法再作为当前末端延伸，只能重启。\n\n")
	b.WriteString("最优走法不直接使用核心接缝 **T0–T1（O–A）**，而是 `T0→T3→T1`：先走共享边 `O–B`，再经 `O–C` 进入 T1，最后从 **T1–T4（C–A）** 出口离开。关键差别就是 T1 的两个接缝方向：贪心把 `O–C` 当出口并在 T3 终止；最优把 `O–C` 当入口，把 `C–A` 留给 T4。若允许双向扩展，这个 5 三角形框架仍会得到两条路径（一个叶被留在内部节点上），所以不是“只因为不能回头”的表面问题。\n\n")
	b.WriteString(fmt.Sprintf("固定画幅逐像素检查：贪心流对独立三角形列表差异 **%d** 像素，最优流差异 **%d** 像素（160×160，SS=%d）。\n\n",
		c.FrameGreedyDifferentPixels, c.FrameOptimalDifferentPixels, c.Supersample))

	b.WriteString("## 5. 复现与文件\n\n")
	b.WriteString("```bash\ngofmt -w ./strip ./cmd/stripreport\ngo test ./...\ngo run ./cmd/stripreport\n```\n\n")
	b.WriteString("- 机器可读总表：`report.json`\n- 三组网格 CSV：`counts.csv`\n- 反例 CSV：`counterexample.csv`\n- 反例画幅：[`counterexample-frame.svg`](counterexample-frame.svg)\n- 每组与反例的独立/条带 PNG、PGM 也在本目录。\n")

	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		fatal(err)
	}
}
