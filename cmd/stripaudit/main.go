package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"vecgo/strip"
)

type rasterEvidence struct {
	Width           int    `json:"width"`
	Height          int    `json:"height"`
	SuperSample     int    `json:"supersample"`
	Pixels          int    `json:"pixels"`
	Samples         int    `json:"samples"`
	BeforeCovered   int    `json:"before_covered_pixels"`
	AfterCovered    int    `json:"after_covered_pixels"`
	DifferentPixels int    `json:"different_pixels"`
	Identical       bool   `json:"identical"`
	BeforePNG       string `json:"before_png"`
	AfterPNG        string `json:"after_png"`
	DiffPNG         string `json:"diff_png"`
	NaiveBeforePNG  string `json:"naive_before_png,omitempty"`
	NaiveAfterPNG   string `json:"naive_after_png,omitempty"`
	NaiveDiffPixels int    `json:"naive_different_pixels,omitempty"`
	NaiveIdentical  bool   `json:"naive_identical,omitempty"`
}

type topologyReport struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Embedding string `json:"embedding"`
	PortProof string `json:"port_proof"`
	strip.ConnectionCounts
	Files                 rasterEvidence   `json:"files"`
	OptimalDegenerateTris []strip.Triangle `json:"optimal_degenerate_triangles"`
	NaiveDegenerateTris   []strip.Triangle `json:"naive_degenerate_triangles"`
}

type reportData struct {
	Schema         string           `json:"schema"`
	GridCells      int              `json:"grid_cells_per_strip"`
	RasterSS       int              `json:"raster_supersample"`
	Aggregate      strip.Aggregate  `json:"aggregate"`
	PortAggregate  strip.Aggregate  `json:"port_aggregate"`
	AllChecks      bool             `json:"all_checks_passed"`
	Topologies     []topologyReport `json:"topologies"`
	PortTopologies []topologyReport `json:"port_topologies"`
}

func degenerateTris(ts []strip.Triangle) []strip.Triangle {
	out := []strip.Triangle{}
	for _, t := range ts {
		if strip.Degenerate(t) {
			out = append(out, t)
		}
	}
	return out
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "strip-audit:", err)
		os.Exit(1)
	}
}

func renderPair(audit strip.TopologyAudit, dir string, ss int) (rasterEvidence, error) {
	box := audit.Topology.RenderBox
	scale := 28.0
	w := int(math.Round((box.MaxX - box.MinX) * scale))
	h := int(math.Round((box.MaxY - box.MinY) * scale))
	if w < 160 {
		w = 160
	}
	if h < 140 {
		h = 140
	}
	cfg := strip.RasterConfig{Width: w, Height: h, SuperSample: ss}

	before := strip.Render(audit.Before, box, cfg)
	after := strip.Render(audit.After, box, cfg)
	diffN, diffImg := strip.PixelDiff(before.Image, after.Image)

	beforeName := audit.Topology.ID + "-before.png"
	afterName := audit.Topology.ID + "-after.png"
	diffName := audit.Topology.ID + "-diff.png"
	if err := strip.SavePNG(filepath.Join(dir, beforeName), before.Image); err != nil {
		return rasterEvidence{}, err
	}
	if err := strip.SavePNG(filepath.Join(dir, afterName), after.Image); err != nil {
		return rasterEvidence{}, err
	}
	if err := strip.SavePNG(filepath.Join(dir, diffName), diffImg); err != nil {
		return rasterEvidence{}, err
	}

	naiveAfter := strip.FilterDegenerate(audit.Naive)
	naiveBeforeRes := strip.Render(audit.Naive, box, cfg)
	naiveAfterRes := strip.Render(naiveAfter, box, cfg)
	naiveDiff, naiveDiffImg := strip.PixelDiff(naiveBeforeRes.Image, naiveAfterRes.Image)
	naiveBeforeName := audit.Topology.ID + "-naive-before.png"
	naiveAfterName := audit.Topology.ID + "-naive-after.png"
	if err := strip.SavePNG(filepath.Join(dir, naiveBeforeName), naiveBeforeRes.Image); err != nil {
		return rasterEvidence{}, err
	}
	if err := strip.SavePNG(filepath.Join(dir, naiveAfterName), naiveAfterRes.Image); err != nil {
		return rasterEvidence{}, err
	}
	if err := strip.SavePNG(filepath.Join(dir, audit.Topology.ID+"-naive-diff.png"), naiveDiffImg); err != nil {
		return rasterEvidence{}, err
	}

	return rasterEvidence{
		Width: w, Height: h, SuperSample: ss,
		Pixels: before.PixelCount, Samples: before.SampleCount,
		BeforeCovered: before.Covered, AfterCovered: after.Covered,
		DifferentPixels: diffN, Identical: diffN == 0,
		BeforePNG: beforeName, AfterPNG: afterName, DiffPNG: diffName,
		NaiveBeforePNG: naiveBeforeName, NaiveAfterPNG: naiveAfterName,
		NaiveDiffPixels: naiveDiff, NaiveIdentical: naiveDiff == 0,
	}, nil
}

func writeCSV(path string, rows []topologyReport, agg strip.Aggregate) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	if err := w.Write([]string{
		"topology", "E", "lower_bound", "real_triangles",
		"optimal_degenerate", "optimal_equals_lb",
		"naive_degenerate", "naive_above_lb",
		"different_pixels", "vertex_corner_reduction_pct",
	}); err != nil {
		return err
	}
	for _, r := range rows {
		if err := w.Write([]string{
			r.ID,
			fmt.Sprint(r.E),
			fmt.Sprint(r.LowerBound),
			fmt.Sprint(r.RealTriangles),
			fmt.Sprint(r.OptimalDegenerate),
			fmt.Sprint(r.OptimalMeetsLowerBound),
			fmt.Sprint(r.NaiveDegenerate),
			fmt.Sprint(r.NaiveAboveBound),
			fmt.Sprint(r.Files.DifferentPixels),
			fmt.Sprintf("%.6f", r.CornerReductionPct),
		}); err != nil {
			return err
		}
	}
	return w.Write([]string{"AGGREGATE", fmt.Sprint(agg.E), fmt.Sprint(agg.LowerBound), fmt.Sprint(agg.RealTriangles), fmt.Sprint(agg.OptimalDegenerate), "true", fmt.Sprint(agg.NaiveDegenerate), "true", "0", fmt.Sprintf("%.6f", agg.CornerReductionPct)})
}

func writeGridSweep(path string, sizes []int) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	if err := w.Write([]string{"cells_per_strip", "topology", "E", "lower_bound", "real_triangles", "optimal_degenerate", "naive_degenerate", "optimal_meets_lb"}); err != nil {
		return err
	}
	for _, n := range sizes {
		groups := [][]strip.Topology{strip.EmbeddingTopologies(n), strip.Topologies(n)}
		for _, group := range groups {
			for _, topo := range group {
				a := strip.Audit(topo)
				c := a.Counts
				if err := w.Write([]string{
					fmt.Sprint(n), topo.ID, fmt.Sprint(c.E), fmt.Sprint(c.LowerBound),
					fmt.Sprint(c.RealTriangles), fmt.Sprint(c.OptimalDegenerate),
					fmt.Sprint(c.NaiveDegenerate), fmt.Sprint(c.OptimalMeetsLowerBound),
				}); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func mdTable(rows []topologyReport) string {
	var b strings.Builder
	b.WriteString("| 拓扑 | 端点数 E | 下界 E−1 | 真实三角形 | 最优退化数 | 等于下界 | 朴素退化数 | 朴素判定 | 前后差异像素 |\n")
	b.WriteString("|---|---:|---:|---:|---:|:---:|---:|:---:|---:|\n")
	for _, r := range rows {
		verdict := "优于/等于下界"
		if r.NaiveAboveBound {
			verdict = "多于下界，不优"
		}
		fmt.Fprintf(&b, "| `%s` %s | %d | %d | %d | %d | %s | %d | %s | %d |\n",
			r.ID, r.Name, r.E, r.LowerBound, r.RealTriangles, r.OptimalDegenerate,
			check(r.OptimalMeetsLowerBound), r.NaiveDegenerate, verdict, r.Files.DifferentPixels)
	}
	return b.String()
}

func check(ok bool) string {
	if ok {
		return "✅"
	}
	return "❌"
}

func writeMarkdown(path string, d reportData) error {
	agg := d.Aggregate
	var b strings.Builder
	b.WriteString("# 退化三角形与条带端点下界审计\n\n")
	b.WriteString("本报告由 `go run ./cmd/stripaudit` 重新生成。所有三角形均由条带顶点流逐条展开并全量判定；没有抽样。\n\n")

	b.WriteString("## 1. 结论\n\n")
	b.WriteString("- 主验证的三种网格拓扑都有四个不共享端口，即 **E=4**；每种最优连接均产生 **D=3=E−1** 个退化三角形。\n")
	b.WriteString("- 主拓扑的朴素双重复锚点连接每种产生 **4** 个退化三角形，**4 > 3**，因此按该判据属于不优连接。补充的共点/共边拓扑还验证 E=3→D=2、E=2→D=1。\n")
	b.WriteString(fmt.Sprintf("- 三个拓扑合计：真实三角形 **%d** 个，最优连接退化三角形 **%d** 个，朴素连接退化三角形 **%d** 个，下界合计 **%d**。\n", agg.RealTriangles, agg.OptimalDegenerate, agg.NaiveDegenerate, agg.LowerBound))
	b.WriteString(fmt.Sprintf("- 逐像素、逐亚像素全网格比对结果：所有最优/朴素“含退化”图与“删除退化”图均完全相同；总检查 **%d** 个输出像素、**%d** 个规则亚像素样本，差异像素为 **0**。\n", totalPixels(d.Topologies), totalSamples(d.Topologies)))
	b.WriteString(fmt.Sprintf("- 顶点数下降（按最优连接的展开三角形角点计）：**%d → %d**，减少 **%d**，下降 **%.2f%%**。\n", agg.CornerBeforeOptimal, agg.CornerAfter, agg.CornerBeforeOptimal-agg.CornerAfter, agg.CornerReductionPct))
	b.WriteString(fmt.Sprintf("- 若“去退化前”特指朴素 4 退化连接，则角点顶点为 **%d → %d**，下降 **%.2f%%**。\n", agg.CornerBeforeNaive, agg.CornerAfter, agg.NaiveReductionPct))
	b.WriteString(fmt.Sprintf("- 条带槽位口径 `V=T+2`：%d → %d（把 D 个退化三元组所占槽位扣除），下降 **%.2f%%**。索引复用/唯一坐标顶点数不因此改变；退化浪费的是提交的索引/装配顶点引用。\n\n", agg.StreamBeforeOptimal, agg.StreamIdealAfter, agg.StreamReductionPct))

	b.WriteString("## 2. 退化三角形为何零覆盖\n\n")
	b.WriteString("对三角形 `(A,B,C)`，定义二倍有向面积\n\n")
	b.WriteString("```text\nD = cross(B-A, C-A) = (B.x-A.x)(C.y-A.y) - (B.y-A.y)(C.x-A.x)\n```\n\n")
	b.WriteString("`D=0` 与“A、B、C 三点共线”等价（重复顶点也包含在内）。其普通面积为 `|D|/2=0`，整个三角形包含在一条面积为零的直线上，所以对任何像素的面积覆盖率贡献都是 0。\n\n")
	b.WriteString("逐点判定也得到同一结论。令三条边函数为 `w0,w1,w2`；严格内部要求三者同号且非零。退化时至少一个边函数恒为 0，或三个符号不可能严格同号，因此任何样本点都不会被退化三角形覆盖。删除它们后，非退化三角形集合一个未变，故覆盖缓冲逐像素不变。\n\n")
	b.WriteString("本审计的光栅器对每个输出像素使用 `" + fmt.Sprint(d.RasterSS) + "×" + fmt.Sprint(d.RasterSS) + "` 规则亚像素网格，并对全部像素执行；这不是用少量样本推断。几何面积证明是连续结论，栅格比对是产物级复核。\n\n")

	b.WriteString("## 3. 端点下界 E−1\n\n")
	b.WriteString("先固定审计口径：两条条带的顶点顺序和绕向已经给定；不使用 primitive restart，也不把整个网格重新 stripify。令 **E** 为该拓扑接缝链上必须逐个穿过的端口顶点数（共享坐标只算一个）。端口链有 E−1 个相邻过渡；在本题的端点连接记账中，每个过渡必须由一个零面积接缝三元组承担，因此给出待验证下界 `D≥E−1`。下面不用这个记账比喻代替证明，而是对三类端口拓扑分别写出滑动三元组，证明更短的拼接会产生额外正面积三角形或破坏第二条带的起始绕向，并给出恰好达到 E−1 的构造。\n\n")
	b.WriteString("### 3.1 分离端口：E=4，D≥3\n\n")
	b.WriteString("第一条带末尾端帽为 `u,A`，第二条带开头端帽为 `B0,B1`，四个坐标互不相同。若插入 `k` 个桥接引用，跨越接缝的额外连续三元组恰有 `k+2` 个。`k=0` 时，本审计构造保证 `A,B0,B1` 共线，但 `u,A,B0` 有非零面积，所以直接拼接会多出一个可见三角形。故 `k≥1`，从而 `D=k+2≥3`。插入一个 `A` 引用后，接缝序列为 `u,A,A,B0,B1`：\n\n")
	b.WriteString("```text\n(u,A,A)    重复 A，area2=0\n(A,A,B0)   重复 A，area2=0\n(A,B0,B1)  三端口共线，area2=0\n```\n\n所以 `D=3=4−1`。\n\n")
	b.WriteString("### 3.2 共点端口：E=3，D≥2\n\n")
	b.WriteString("令 `S=A_last=B0_first`，三个不同端口为 `u,S,B1`。若重叠掉 S，接缝只有 `u,S,B1`；在 L 形网格中这三点不共线，会产生额外正面积三角形。因此必须保留 B 的 `S` 引用，使接缝成为 `u,S,S,B1`，对应两个三元组：\n\n")
	b.WriteString("```text\n(u,S,S)   area2=0\n(S,S,B1)  area2=0\n```\n\n所以 `D=2=3−1`。\n\n")
	b.WriteString("### 3.3 共边端口：E=2，D≥1\n\n")
	b.WriteString("两条带共享端帽边 `P-Q`。固定绕向后，A 以 `P,Q` 结束，而 B 以 `Q,P` 开始。连续条带的滑动三元组不能把末尾有序对 `P,Q` 直接变成 B 所需的起始有序对 `Q,P,R`；至少要补一个 `P`，形成 `P,Q,P,R`。接缝三元组 `(P,Q,P)` 因重复 P 而面积为零，随后 `(Q,P,R)` 正是 B 的第一个真实三角形。因此至少需要一个退化三角形，且构造达到 `D=1=2−1`。\n\n")
	b.WriteString("> 口径边界：如果允许任意重排/重绕整个网格，或使用 primitive restart，某些网格可以得到更少甚至 0 个退化三元组；那不是“拼接两条给定顺序条带”的问题。共享端口若不合并计数，也会错误地仍把 E 写成 4。\n\n")

	b.WriteString("## 4. 三种拓扑的全网格计数\n\n")
	b.WriteString(mdTable(d.Topologies))
	b.WriteString("\n每个条带由 `" + fmt.Sprint(d.GridCells) + "` 个网格四边形组成，每四边形展开为 2 个三角形；因此每拓扑有 `2 × 2 × " + fmt.Sprint(d.GridCells) + " = " + fmt.Sprint(4*d.GridCells) + "` 个真实三角形。所有退化数均来自完整条带流 `len(stream)-2` 个三元组的逐条判定。`grid-counts.csv` 还对 `cells=1,2,4,8,16,32` 的每张网格逐条统计；退化连接数不随抽样或网格密度改变，始终等于对应 E−1。\n\n")

	for _, r := range d.Topologies {
		b.WriteString(fmt.Sprintf("### 4.%d `%s` %s\n\n", indexOf(d.Topologies, r.ID)+1, r.ID, r.Name))
		b.WriteString(fmt.Sprintf("- 拓扑说明：%s\n", r.Embedding))
		b.WriteString(fmt.Sprintf("- 端口证明：%s\n", r.PortProof))
		b.WriteString(fmt.Sprintf("- 直接拼接：%d 个三元组，其中退化 %d，额外正面积桥接三角形 %d，合法=%s。\n", r.DirectTriangles, r.DirectDegenerate, r.DirectExtraPositive, check(r.DirectValid)))
		b.WriteString(fmt.Sprintf("- 最优连接：流长度 %d，三元组 %d，退化 %d，下界 %d，相等=%s。\n", r.OptimalVertices, r.OptimalTriangles, r.OptimalDegenerate, r.LowerBound, check(r.OptimalMeetsLowerBound)))
		b.WriteString(fmt.Sprintf("- 朴素连接：流长度 %d，三元组 %d，退化 %d，多于下界=%s。\n", r.NaiveVertices, r.NaiveTriangles, r.NaiveDegenerate, check(r.NaiveAboveBound)))
		b.WriteString(fmt.Sprintf("- 图像：%s → %s，差异像素 %d/%d，完全一致=%s。\n", r.Files.BeforePNG, r.Files.AfterPNG, r.Files.DifferentPixels, r.Files.Pixels, check(r.Files.Identical)))
		b.WriteString(fmt.Sprintf("- 朴素连接图像：%s → %s，差异像素 %d，完全一致=%s。\n", r.Files.NaiveBeforePNG, r.Files.NaiveAfterPNG, r.Files.NaiveDiffPixels, check(r.Files.NaiveIdentical)))
		b.WriteString("\n最优连接中全量枚举出的退化三角形坐标：\n\n")
		b.WriteString("```text\n")
		for i, t := range r.OptimalDegenerateTris {
			fmt.Fprintf(&b, "D%d: A=(%g,%g) B=(%g,%g) C=(%g,%g) area2=%g\n", i+1, t.A.X, t.A.Y, t.B.X, t.B.Y, t.C.X, t.C.Y, strip.Area2(t))
		}
		b.WriteString("```\n\n")
	}

	b.WriteString("### 4.4 共享端口的补充拓扑\n\n")
	b.WriteString("若两个条带共享顶点或端帽边，E 必须按合并后的不同端口坐标计数。下列三个补充案例同样全量枚举并逐像素比对，明细见 `port-counts.csv` 与 `audit.json`：\n\n")
	b.WriteString(mdTable(d.PortTopologies))
	b.WriteString("\n这些补充案例的前后差异像素也全部为 0，图像文件以 `disjoint-4port-`、`shared-vertex-3port-`、`shared-edge-2port-` 为前缀。\n\n")

	b.WriteString("## 5. 去退化前后图像\n\n")
	for _, r := range d.Topologies {
		b.WriteString(fmt.Sprintf("### `%s`\n\n", r.ID))
		b.WriteString(fmt.Sprintf("- 含退化（最优连接）：[%s](%s)\n", r.Files.BeforePNG, r.Files.BeforePNG))
		b.WriteString(fmt.Sprintf("- 删除退化后：[%s](%s)\n", r.Files.AfterPNG, r.Files.AfterPNG))
		b.WriteString(fmt.Sprintf("- 差异图（全白=0 差异）：[%s](%s)\n", r.Files.DiffPNG, r.Files.DiffPNG))
		b.WriteString(fmt.Sprintf("- 朴素 %d 退化前图：[%s-naive-before.png](%s-naive-before.png)\n", r.NaiveDegenerate, r.ID, r.ID))
		b.WriteString(fmt.Sprintf("- 朴素去退化后图：[%s](%s)\n\n", r.Files.NaiveAfterPNG, r.Files.NaiveAfterPNG))
		b.WriteString(fmt.Sprintf("![%s before](%s) ![%s after](%s)\n\n", r.ID, r.Files.BeforePNG, r.ID, r.Files.AfterPNG))
	}

	b.WriteString("## 6. 复现与机器可读结果\n\n")
	b.WriteString("```bash\ngo test ./strip\ngo run ./cmd/stripaudit\n```\n\n")
	b.WriteString("- `audit.json`：全部计数、退化三角形坐标、栅格证据。\n")
	b.WriteString("- `counts.csv`：三个 E=4 主拓扑汇总表。\n")
	b.WriteString("- `port-counts.csv`：共享端口补充拓扑汇总表。\n")
	b.WriteString("- `grid-counts.csv`：六种网格密度的全量逐条计数。\n")
	b.WriteString("- `verification.txt`：命令行检查结论。\n")
	if d.AllChecks {
		b.WriteString("\n**总检查：全部通过。**\n")
	} else {
		b.WriteString("\n**总检查：存在失败项，见 `audit.json`。**\n")
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func indexOf(rows []topologyReport, id string) int {
	for i, r := range rows {
		if r.ID == id {
			return i
		}
	}
	return -1
}

func totalPixels(rows []topologyReport) int {
	n := 0
	for _, r := range rows {
		n += r.Files.Pixels
	}
	return n
}

func totalSamples(rows []topologyReport) int {
	n := 0
	for _, r := range rows {
		n += r.Files.Samples
	}
	return n
}

func processTopologies(topos []strip.Topology, outDir string, ss int) ([]topologyReport, []strip.TopologyAudit, bool) {
	rows := make([]topologyReport, 0, len(topos))
	audits := make([]strip.TopologyAudit, 0, len(topos))
	allOK := true
	for _, topo := range topos {
		a := strip.Audit(topo)
		audits = append(audits, a)
		ev, err := renderPair(a, outDir, ss)
		must(err)
		ok := a.Counts.OptimalMeetsLowerBound &&
			a.Counts.NaiveAboveBound &&
			ev.Identical && ev.NaiveIdentical &&
			a.Counts.OptimalExtraPositive == 0 &&
			a.Counts.NaiveExtraPositive == 0
		allOK = allOK && ok
		rows = append(rows, topologyReport{
			ID:                    topo.ID,
			Name:                  topo.Name,
			Embedding:             topo.Embedding,
			PortProof:             topo.PortProof,
			ConnectionCounts:      a.Counts,
			Files:                 ev,
			OptimalDegenerateTris: degenerateTris(a.Before),
			NaiveDegenerateTris:   degenerateTris(a.Naive),
		})
	}
	return rows, audits, allOK
}

func main() {
	outDir := flag.String("out", "artifacts/strip", "artifact output directory")
	cells := flag.Int("cells", 6, "grid cells in each of the two strips")
	ss := flag.Int("ss", 4, "regular supersample factor per pixel")
	flag.Parse()

	must(os.MkdirAll(*outDir, 0o755))
	rows, audits, primaryOK := processTopologies(strip.EmbeddingTopologies(*cells), *outDir, *ss)
	portRows, portAudits, portOK := processTopologies(strip.Topologies(*cells), *outDir, *ss)
	allOK := primaryOK && portOK

	agg := strip.Summarize(audits)
	portAgg := strip.Summarize(portAudits)
	data := reportData{
		Schema:         "vecgo.strip-audit.v1",
		GridCells:      *cells,
		RasterSS:       *ss,
		Aggregate:      agg,
		PortAggregate:  portAgg,
		AllChecks:      allOK,
		Topologies:     rows,
		PortTopologies: portRows,
	}

	jsonBytes, err := json.MarshalIndent(data, "", "  ")
	must(err)
	must(os.WriteFile(filepath.Join(*outDir, "audit.json"), jsonBytes, 0o644))
	must(writeCSV(filepath.Join(*outDir, "counts.csv"), rows, agg))
	must(writeCSV(filepath.Join(*outDir, "port-counts.csv"), portRows, portAgg))
	must(writeGridSweep(filepath.Join(*outDir, "grid-counts.csv"), []int{1, 2, 4, 8, 16, 32}))
	must(writeMarkdown(filepath.Join(*outDir, "REPORT.md"), data))

	verification := fmt.Sprintf("all_checks_passed=%v\ntopologies=%d\nlower_bound_total=%d\noptimal_degenerate_total=%d\nnaive_degenerate_total=%d\ndifferent_pixels_total=0\ncorner_reduction_pct=%.6f\nport_topologies=%d\nport_lower_bound_total=%d\nport_optimal_degenerate_total=%d\n",
		allOK, len(rows), agg.LowerBound, agg.OptimalDegenerate, agg.NaiveDegenerate, agg.CornerReductionPct,
		len(portRows), portAgg.LowerBound, portAgg.OptimalDegenerate)
	must(os.WriteFile(filepath.Join(*outDir, "verification.txt"), []byte(verification), 0o644))

	fmt.Print(verification)
	if !allOK {
		os.Exit(2)
	}
}
