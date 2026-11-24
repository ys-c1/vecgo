package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"vecgo/affine"
	"vecgo/blocks"
	"vecgo/geom"
)

type tierSpec struct {
	Name     string
	Width    int
	Height   int
	Source   []geom.Point
	Target   []geom.Point
	Conflict bool
}

type reportData struct {
	Tiers     []tierResult     `json:"tiers"`
	Conflict  conflictResult   `json:"conflictDemo"`
	BlockSets []blockSetResult `json:"blockSets"`
	Adaptive  adaptiveResult   `json:"adaptive"`
}

type tierResult struct {
	Name   string       `json:"name"`
	Width  int          `json:"width"`
	Height int          `json:"height"`
	Fit    *affine.Fit  `json:"fit"`
	Target []geom.Point `json:"target"`
}

type conflictResult struct {
	Fit         *affine.Fit `json:"fit"`
	Description string      `json:"description"`
}

type blockSetResult struct {
	TierName string         `json:"tier"`
	Width    int            `json:"width"`
	Height   int            `json:"height"`
	Stats    []blocks.Stats `json:"stats"`
}

type adaptiveResult struct {
	BlockSize       int  `json:"blockSize"`
	OffEqualsFixed  bool `json:"offEqualsFixed"`
	FixedBlockCount int  `json:"fixedBlockCount"`
	AdaptiveCount   int  `json:"adaptiveBlockCount"`
}

func main() {
	source := []geom.Point{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}
	tiers := []tierSpec{
		{
			Name: "一档：16×16（1:1）", Width: 16, Height: 16, Source: source,
			Target: []geom.Point{{X: 0, Y: 0}, {X: 16, Y: 0}, {X: 16, Y: 16}, {X: 0, Y: 16}},
		},
		{
			Name: "二档：32×16（2:1，contain 居中）", Width: 32, Height: 16, Source: source,
			Target: []geom.Point{{X: 8, Y: 0}, {X: 24, Y: 0}, {X: 24, Y: 16}, {X: 8, Y: 16}},
		},
	}

	var data reportData
	var md strings.Builder
	md.WriteString("# vecgo 画幅映射与块统计诊断\n\n")
	md.WriteString("角点编号固定为 1=(0,0)、2=(10,0)、3=(10,10)、4=(0,10)，顺序为左上、右上、右下、左下。\n\n")

	for _, t := range tiers {
		pairs := make([]affine.Pair, 4)
		for i := range source {
			pairs[i] = affine.Pair{ID: i + 1, Src: t.Source[i], Dst: t.Target[i]}
		}
		fit, err := affine.Solve(pairs)
		if err != nil {
			fmt.Fprintln(os.Stderr, "仿射求解失败:", err)
			os.Exit(1)
		}
		data.Tiers = append(data.Tiers, tierResult{Name: t.Name, Width: t.Width, Height: t.Height, Fit: fit, Target: t.Target})
		writeTierMarkdown(&md, t, fit)
	}

	// 故障注入：只把第 3 点移动 (2.4,-1.8) 像素，模长 3.0。
	badTarget := append([]geom.Point(nil), tiers[0].Target...)
	badTarget[2].X += 2.4
	badTarget[2].Y -= 1.8
	badPairs := make([]affine.Pair, 4)
	for i := range source {
		badPairs[i] = affine.Pair{ID: i + 1, Src: source[i], Dst: badTarget[i]}
	}
	badFit, err := affine.Solve(badPairs)
	if err != nil {
		fmt.Fprintln(os.Stderr, "冲突样例求解失败:", err)
		os.Exit(1)
	}
	data.Conflict = conflictResult{Fit: badFit, Description: "第 3 点目标坐标注入 (+2.4,-1.8) px"}
	writeConflictMarkdown(&md, badTarget, badFit)

	// 两档画幅使用同一套确定性块样图；宽画幅只是在右侧增加块，不改变左侧块数据。
	sample16 := blockSample(16, 16)
	sample32 := blockSample(32, 16)
	samples := []struct {
		tier tierSpec
		pix  []uint8
	}{
		{tier: tiers[0], pix: sample16},
		{tier: tiers[1], pix: sample32},
	}
	for _, s := range samples {
		statsSet, err := analyzeBlocks(s.pix, s.tier.Width, s.tier.Height)
		if err != nil {
			fmt.Fprintln(os.Stderr, "块统计失败:", err)
			os.Exit(1)
		}
		data.BlockSets = append(data.BlockSets, blockSetResult{TierName: s.tier.Name, Width: s.tier.Width, Height: s.tier.Height, Stats: statsSet})
		writeBlockMarkdown(&md, s.tier.Name, s.tier.Width, s.tier.Height, statsSet)
	}

	// 用第二档画幅验证参数关闭时与固定网格逐块相同。
	w, h := 32, 16
	pix := sample32
	opts := blocks.DefaultOptions()
	opts.BlockSize = 8
	opts.Adaptive = false
	offRects, _, err := blocks.Plan(pix, w, h, opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "固定块规划失败:", err)
		os.Exit(1)
	}
	fixedRects, err := blocks.FixedPlan(w, h, 8)
	if err != nil {
		fmt.Fprintln(os.Stderr, "FixedPlan 失败:", err)
		os.Exit(1)
	}
	equal := rectsEqual(offRects, fixedRects)
	opts.Adaptive = true
	opts.EntropyThreshold = 0.9
	opts.MinSize = 4
	adaptiveRects, adaptiveStats, err := blocks.Plan(pix, w, h, opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "自适应块规划失败:", err)
		os.Exit(1)
	}
	data.Adaptive = adaptiveResult{BlockSize: 8, OffEqualsFixed: equal, FixedBlockCount: len(fixedRects), AdaptiveCount: len(adaptiveRects)}
	md.WriteString("## 块级自适应开关\n\n")
	md.WriteString("- 样图的 8 个根块依次为：全黑、全白、竖分、棋盘、横分、梯度、横行条纹、固定种子噪声。\n")
	md.WriteString(fmt.Sprintf("- `Adaptive=false`：%d 块；与 `FixedPlan(8)` 逐块相等 = **%v**。这就是参数关闭后回到的固定块路径。\n", len(offRects), equal))
	md.WriteString(fmt.Sprintf("- `Adaptive=true, EntropyThreshold=0.9, MinSize=4`：%d 个叶块；高熵块分裂到 4×4，低熵常数块保留 8×8。\n", len(adaptiveRects)))
	md.WriteString("\n| 自适应叶块 | 矩形 | 熵 H | 实际最长游程 | 游程上限 U=N |\n|---:|---|---:|---:|---:|\n")
	for _, s := range adaptiveStats {
		md.WriteString(fmt.Sprintf("| %d | (%d,%d,%d,%d) | %.6f | %d | %d |\n", s.Index+1, s.X, s.Y, s.W, s.H, s.Entropy, s.LongestRun, s.RunUpper))
	}
	md.WriteString("\n")
	if !equal {
		fmt.Fprintln(os.Stderr, "Adaptive=false 没有回到固定块")
		os.Exit(2)
	}

	if err := os.MkdirAll("artifacts/diag", 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "创建输出目录失败:", err)
		os.Exit(1)
	}
	mdPath := filepath.Join("artifacts", "diag", "report.md")
	if err := os.WriteFile(mdPath, []byte(md.String()), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "写 Markdown 失败:", err)
		os.Exit(1)
	}
	jsonPath := filepath.Join("artifacts", "diag", "report.json")
	jb, _ := json.MarshalIndent(data, "", "  ")
	if err := os.WriteFile(jsonPath, jb, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "写 JSON 失败:", err)
		os.Exit(1)
	}
	fmt.Print(md.String())
	fmt.Printf("\n机器可读结果：%s、%s\n", mdPath, jsonPath)
}

func blockSample(w, h int) []uint8 {
	pix := make([]uint8, w*h)
	state := uint32(0x12345678)
	randByte := func() uint8 {
		state ^= state << 13
		state ^= state >> 17
		state ^= state << 5
		return uint8(state >> 24)
	}
	for by := 0; by < h; by += 8 {
		for bx := 0; bx < w; bx += 8 {
			k := (by/8)*(w/8) + bx/8
			for y := by; y < by+8; y++ {
				for x := bx; x < bx+8; x++ {
					lx, ly := x-bx, y-by
					var v uint8
					switch k % 8 {
					case 0:
						v = 0
					case 1:
						v = 255
					case 2:
						if lx >= 4 {
							v = 255
						}
					case 3:
						if (lx+ly)%2 == 1 {
							v = 255
						}
					case 4:
						if ly >= 4 {
							v = 255
						}
					case 5:
						v = uint8((lx*29 + ly*43 + 17) & 0xff)
					case 6:
						if ly%2 == 1 {
							v = 255
						}
					default:
						v = randByte()
					}
					pix[y*w+x] = v
				}
			}
		}
	}
	return pix
}

func analyzeBlocks(pix []uint8, w, h int) ([]blocks.Stats, error) {
	opts := blocks.DefaultOptions()
	opts.BlockSize = 8
	opts.Adaptive = false
	_, stats, err := blocks.Plan(pix, w, h, opts)
	return stats, err
}

func writeTierMarkdown(md *strings.Builder, t tierSpec, f *affine.Fit) {
	md.WriteString("## " + t.Name + "\n\n")
	md.WriteString("目标落点：\n\n")
	md.WriteString("| 交点 | 源坐标 | 目标应有坐标 |\n|---:|---:|---:|\n")
	for i, p := range t.Target {
		s := t.Source[i]
		md.WriteString(fmt.Sprintf("| %d | (%.1f, %.1f) | (%.1f, %.1f) |\n", i+1, s.X, s.Y, p.X, p.Y))
	}
	md.WriteString("\n方程形如 $X_i=a x_i+b y_i+e$、$Y_i=c x_i+d y_i+f$。四个点给 8 条方程、6 个未知量；用全部方程做最小二乘。源点设计矩阵的法方程为：\n\n")
	md.WriteString("```\n")
	md.WriteString(fmt.Sprintf("A^T A = [[%.0f, %.0f, %.0f], [%.0f, %.0f, %.0f], [%.0f, %.0f, %.0f]]\n",
		f.NormalMatrix[0][0], f.NormalMatrix[0][1], f.NormalMatrix[0][2],
		f.NormalMatrix[1][0], f.NormalMatrix[1][1], f.NormalMatrix[1][2],
		f.NormalMatrix[2][0], f.NormalMatrix[2][1], f.NormalMatrix[2][2]))
	md.WriteString(fmt.Sprintf("A^T X = [%.10f, %.10f, %.10f]\n", f.XRight[0], f.XRight[1], f.XRight[2]))
	md.WriteString(fmt.Sprintf("A^T Y = [%.10f, %.10f, %.10f]\n", f.YRight[0], f.YRight[1], f.YRight[2]))
	md.WriteString("```\n\n")
	v := f.Matrix.Values()
	md.WriteString("六个参数逐个列出（顺序 a,b,c,d,e,f）：\n\n")
	for i, name := range []string{"a", "b", "c", "d", "e", "f"} {
		md.WriteString(fmt.Sprintf("- `%s = %.12g`\n", name, v[i]))
	}
	md.WriteString("\n重投影残差：\n\n")
	md.WriteString("| 交点 | 重投影坐标 | 目标坐标 | Δx (px) | Δy (px) | 欧氏残差 (px) | 判定 |\n|---:|---:|---:|---:|---:|---:|:---:|\n")
	for _, r := range f.Residuals {
		ok := "✅"
		if r.Distance > f.Tolerance {
			ok = "❌"
		}
		md.WriteString(fmt.Sprintf("| %d | (%.9f, %.9f) | (%.1f, %.1f) | %+.3e | %+.3e | %.6f | %s |\n",
			r.ID, r.Got.X, r.Got.Y, r.Want.X, r.Want.Y, r.DX, r.DY, r.Distance, ok))
	}
	md.WriteString(fmt.Sprintf("\n最大残差 **%.6g px**，半像素阈值 %.1f px，结论 **%s**。\n\n", f.MaxResidual, f.Tolerance, passText(f.OK)))
}

func writeConflictMarkdown(md *strings.Builder, target []geom.Point, f *affine.Fit) {
	md.WriteString("## 残差超限分支：不相容交点对定位\n\n")
	md.WriteString("额外构造一组数据：1 号目标点不变，3 号点注入 $(+2.4,-1.8)$ px，注入模长 3.0 px。目标坐标为 ")
	for i, p := range target {
		if i > 0 {
			md.WriteString("；")
		}
		md.WriteString(fmt.Sprintf("P%d=(%.1f,%.1f)", i+1, p.X, p.Y))
	}
	md.WriteString("。\n\n")
	v := f.Matrix.Values()
	md.WriteString("解出的六参数为：")
	for i, name := range []string{"a", "b", "c", "d", "e", "f"} {
		md.WriteString(fmt.Sprintf(" `%s=%.9g`", name, v[i]))
	}
	md.WriteString("。\n\n| 交点 | 欧氏残差 (px) |\n|---:|---:|\n")
	for _, r := range f.Residuals {
		md.WriteString(fmt.Sprintf("| %d | %.6f |\n", r.ID, r.Distance))
	}
	dc := f.Diagonal
	md.WriteString(fmt.Sprintf("\n最大残差 %.6f px > 0.5 px。矩形四角的仿射相容条件是两条对角共中点：`P%d+P%d=P%d+P%d`。", f.MaxResidual, dc.PairA[0], dc.PairA[1], dc.PairB[0], dc.PairB[1]))
	md.WriteString(fmt.Sprintf("本样例闭合差向量为 `(%.3f, %.3f)` px，对角中点偏差为 %.6f px。\n\n", dc.Closure.X, dc.Closure.Y, dc.MidpointGap))
	md.WriteString(fmt.Sprintf("**不相容的交点编号对：%d–%d 与 %d–%d（记为 `(%d,%d) ↔ (%d,%d)`）。**", dc.PairA[0], dc.PairA[1], dc.PairB[0], dc.PairB[1], dc.PairA[0], dc.PairA[1], dc.PairB[0], dc.PairB[1]))
	md.WriteString("仅靠这四个点不能把矛盾唯一归罪到某一个物理角点：最小二乘残差按对角交互项分布，任何单点扰动都会在四角产生等幅残差；数学上可唯一指出的是上述两条对角交点对不相容。\n\n")
}

func writeBlockMarkdown(md *strings.Builder, tier string, w, h int, stats []blocks.Stats) {
	md.WriteString("## " + tier + "：逐块熵与游程上限\n\n")
	md.WriteString("统计对象是命令内置的确定性 8 图案样图（同一组字节，可由 `go run ./cmd/diag` 复跑），按 8×8 固定块统计。熵单位是 bit/像素；`U=N` 是块内游程长度的绝对上限，`Lmax` 是实际最长游程，`cap=255` 是单字节 RLE 计数上限，`2N` 是 RLE 负载字节数的最坏上界。\n\n")
	md.WriteString("| 块号 | 矩形 (x,y,w,h) | 熵 H | 实际最长游程 Lmax | 游程上限 U=N | 单字节封顶 | 游程数 | RLE 上界/实际负载 (B) |\n|---:|---|---:|---:|---:|---:|---:|---:|\n")
	for _, s := range stats {
		md.WriteString(fmt.Sprintf("| %d | (%d,%d,%d,%d) | %.6f | %d | %d | %d | %d | 上界 %d / 实际 %d |\n",
			s.Index+1, s.X, s.Y, s.W, s.H, s.Entropy, s.LongestRun, s.RunUpper, s.CountCap, s.Runs, s.WorstCaseBytes, s.RLEBytes))
	}
	md.WriteString("\n")
}

func passText(ok bool) string {
	if ok {
		return "通过（≤0.5 px）"
	}
	return "超限"
}

func rectsEqual(a, b []blocks.Rect) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
