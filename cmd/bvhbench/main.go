// Command bvhbench 为五类三角形场景建树并采样查询，核验两条约束：
//
//  1. 剪枝查询结果与暴力遍历逐项一致；
//  2. 实测树高不超过给定上界，且不超过由三角形数反解的期望树高。
//
// 输出「树高与访问节点数」总表（两档三角形规模各一行），另附分场景表，
// 并为聚集场景单列细目（按点查询 / 小矩形 / 大矩形分列）。
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"vecgo/bvh"
)

// kindStats 是一类采样查询的累计统计。
type kindStats struct {
	queries    int
	visitedSum int
	visitedMax int
	prunedSum  int
	testedSum  int
	matchedSum int
}

func (k kindStats) avgVisited() float64 { return float64(k.visitedSum) / float64(k.queries) }
func (k kindStats) avgPruned() float64  { return float64(k.prunedSum) / float64(k.queries) }
func (k kindStats) avgTested() float64  { return float64(k.testedSum) / float64(k.queries) }
func (k kindStats) avgMatched() float64 { return float64(k.matchedSum) / float64(k.queries) }

// sceneResult 是一个场景在一个规模档上的全部实测结果。
type sceneResult struct {
	scene     string
	n         int
	requested int
	height    int
	expected  int
	bound     int
	nodes     int
	leaves    int
	byKind    [3]kindStats
	mismatch  int
}

func (r sceneResult) all() kindStats {
	var a kindStats
	for _, k := range r.byKind {
		a.queries += k.queries
		a.visitedSum += k.visitedSum
		a.prunedSum += k.prunedSum
		a.testedSum += k.testedSum
		a.matchedSum += k.matchedSum
		if k.visitedMax > a.visitedMax {
			a.visitedMax = k.visitedMax
		}
	}
	return a
}

func main() {
	small := flag.Int("small", 1000, "小档三角形数")
	large := flag.Int("large", 10000, "大档三角形数")
	nqueries := flag.Int("queries", 300, "每场景每档采样查询数（点/小矩形/大矩形按 1:1:1 分摊）")
	out := flag.String("out", "artifacts/bvh-report.md", "报告输出路径")
	flag.Parse()

	if *small < 1 || *large < *small || *nqueries < 3 {
		fmt.Fprintln(os.Stderr, "参数非法：需要 1 <= small <= large，queries >= 3")
		os.Exit(2)
	}

	report := &strings.Builder{}
	emit := func(format string, args ...any) {
		fmt.Fprintf(report, format, args...)
		fmt.Printf(format, args...)
	}

	ok, err := run([]int{*small, *large}, *nqueries, emit)
	if err != nil {
		fmt.Fprintln(os.Stderr, "失败:", err)
		os.Exit(1)
	}

	if dir := filepath.Dir(*out); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			fmt.Fprintln(os.Stderr, "创建报告目录失败:", err)
			os.Exit(1)
		}
	}
	if err := os.WriteFile(*out, []byte(report.String()), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "写报告失败:", err)
		os.Exit(1)
	}
	if !ok {
		os.Exit(1)
	}
}

func run(scales []int, nqueries int, emit func(string, ...any)) (bool, error) {
	emit("# 三角形包围盒层次树：树高与访问节点数\n\n")
	emit("生成命令：\n\n```\ngo run ./cmd/bvhbench -small %d -large %d -queries %d\n```\n\n",
		scales[0], scales[1], nqueries)
	emit("建树规则：每个节点沿包围盒最长轴按三角形质心中位数二分，递归到单三角形叶子；\n")
	emit("树高上界取 `MaxDepth = 0`，即由三角形数反解为 `ceil(log2(n))+1`（根为第 1 层）。\n")
	emit("采样查询按点 / 小矩形 / 大矩形轮转，种子固定，结果可复现；表中为每类查询的平均值。\n\n")

	var results []sceneResult
	byScene := map[string][]sceneResult{}
	eqOK, heightOK := true, true
	for _, sc := range bvh.Scenes() {
		for _, n := range scales {
			if sc.Name == "single" && n != scales[0] {
				continue
			}
			r, err := measure(sc, n, nqueries)
			if err != nil {
				return false, fmt.Errorf("%s n=%d: %w", sc.Name, n, err)
			}
			results = append(results, r)
			byScene[sc.Name] = append(byScene[sc.Name], r)
			if r.mismatch > 0 {
				eqOK = false
			}
			if r.height > r.bound || r.height > r.expected {
				heightOK = false
			}
		}
	}

	emit("## 总表：树高与访问节点数（两档规模各一行）\n\n")
	emit("以均匀场景为代表，两档三角形规模各一行；其余场景与聚集细目见后两节。\n")
	emit("暴力遍历每次查询要做 n 次三角形判定；下表的「精确判定/次」是剪枝后真正落到三角形的次数。\n\n")
	emit("| 规模档 | 三角形数 | 实测树高 | 反解期望 | 上界 | 节点数 | 访问节点/次 | 访问占比 | 精确判定/次 | 暴力等值 |\n")
	emit("|---|---:|---:|---:|---:|---:|---:|---:|---:|:---:|\n")
	for i, r := range byScene["uniform"] {
		label := "小档"
		if i == 1 {
			label = "大档"
		}
		a := r.all()
		eq := "一致"
		if r.mismatch > 0 {
			eq = fmt.Sprintf("不一致(%d)", r.mismatch)
		}
		emit("| %s | %d | %d | %d | %d | %d | %.2f | %.2f%% | %.2f | %s |\n",
			label, r.n, r.height, r.expected, r.bound, r.nodes,
			a.avgVisited(), 100*a.avgVisited()/float64(r.nodes), a.avgTested(), eq)
	}
	emit("\n")

	emit("## 分场景表\n\n")
	emit("| 场景 | 三角形数 | 实测树高 | 反解期望 | 点查询访问/次 | 小矩形访问/次 | 大矩形访问/次 | 精确判定/次 | 命中/次 | 暴力等值 |\n")
	emit("|---|---:|---:|---:|---:|---:|---:|---:|---:|:---:|\n")
	for _, r := range results {
		eq := "一致"
		if r.mismatch > 0 {
			eq = fmt.Sprintf("不一致(%d)", r.mismatch)
		}
		emit("| %s | %d | %d | %d | %.2f | %.2f | %.2f | %.2f | %.2f | %s |\n",
			r.scene, r.n, r.height, r.expected,
			r.byKind[bvh.KindPoint].avgVisited(),
			r.byKind[bvh.KindSmallBox].avgVisited(),
			r.byKind[bvh.KindLargeBox].avgVisited(),
			r.all().avgTested(), r.all().avgMatched(), eq)
	}
	emit("\n「精确判定/次」是叶子内实际做三角形判定的次数，可与暴力遍历的每次 n 次对照；\n")
	emit("大矩形常常覆盖大半个场景，剪枝空间有限，因此其访问节点数明显高于点查询。\n\n")

	emit("## 聚集场景细目\n\n")
	emit("| 三角形数 | 实测树高 | 反解期望 | 点查询访问/次 | 小矩形访问/次 | 大矩形访问/次 | 最大访问 | 整棵剪枝/次 | 精确判定/次 | 命中/次 |\n")
	emit("|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|\n")
	for _, r := range byScene["clustered"] {
		emit("| %d | %d | %d | %.2f | %.2f | %.2f | %d | %.2f | %.2f | %.2f |\n",
			r.n, r.height, r.expected,
			r.byKind[bvh.KindPoint].avgVisited(),
			r.byKind[bvh.KindSmallBox].avgVisited(),
			r.byKind[bvh.KindLargeBox].avgVisited(),
			r.all().visitedMax, r.all().avgPruned(), r.all().avgTested(), r.all().avgMatched())
	}
	emit("\n聚集场景的簇心故意不均：一半输入挤在左下角 200×200 区域，一半散在 1000×1000 范围；\n")
	emit("同一簇内三角形包围盒彼此交叠，剪枝只对簇外查询有效，因此点查询访问节点数\n")
	emit("比均匀场景低得多（簇内只有一小组候选），大矩形仍会铺开访问多个簇。\n")
	emit("树高由中位数二分保证，与分布无关，仍与反解一致。\n\n")

	emit("## 两条约束的核验\n\n")
	if eqOK {
		emit("- **查询结果与暴力遍历完全一致**：所有场景、两档规模、全部采样查询逐项比对通过。\n")
	} else {
		emit("- **查询结果与暴力遍历完全一致**：存在不一致，见总表「暴力等值」列。\n")
	}
	if heightOK {
		emit("- **树高不超过给定上界**：所有建树实测树高均 ≤ 上界，且 ≤ 由三角形数反解的期望树高。\n")
	} else {
		emit("- **树高不超过给定上界**：存在越界，见分场景表。\n")
	}
	emit("\n两条约束在 `bvh` 包的测试里分别核验：\n")
	emit("`TestScenesQueryMatchesBruteForce` 与 `TestLargeScalesQueryMatchesBruteForce` 只比查询结果；\n")
	emit("`TestHeightWithinUpperBound` 只比树高上界；`TestExpectedHeightInvertsTriangleCount` 只比反解结果。\n\n")

	emit("## 场景说明\n\n")
	emit("| 场景 | 构造 |\n|---|---|\n")
	emit("| uniform | 1000×1000 内均匀随机三角形 |\n")
	emit("| clustered | 12 个簇心，一半挤在左下角、一半散开，簇内 ±25 抖动 |\n")
	emit("| elongated | x 方向 300~400、y 方向不足 1 的长条，沿 y 铺开 |\n")
	emit("| overlapping | 全部三角形围绕中心 (500,500) 的 ±60 区域，彼此大量重叠 |\n")
	emit("| single | 一个固定三角形，叶子即根 |\n")
	emit("\n三类输入错误的节点号由 `bvh.BuildError.NodeIndex()` 给出，分别由\n")
	emit("`TestBuildEmptyTriangles`、`TestBuildDegenerateBoundsReportsNode`、\n")
	emit("`TestBuildDepthExceededReportsNode` 核验空输入、退化包围盒、深度越界。\n")

	return eqOK && heightOK, nil
}

func measure(sc bvh.Scene, n, nqueries int) (sceneResult, error) {
	tris := sc.Generate(n)
	tree, err := bvh.Build(tris, bvh.Options{})
	if err != nil {
		return sceneResult{}, err
	}
	r := sceneResult{
		scene:     sc.Name,
		n:         len(tris),
		requested: n,
		height:    tree.Height,
		expected:  bvh.ExpectedHeight(len(tris)),
		bound:     tree.MaxDepth,
		nodes:     tree.NodeCount(),
		leaves:    tree.LeafCount(),
	}
	for _, q := range bvh.SampleQueries(tris, nqueries, uint64(n)*97+3) {
		var got, want []int
		st := bvh.Stats{}
		switch q.Kind {
		case bvh.KindPoint:
			got = tree.QueryPoint(q.Point, &st)
			want = bvh.BruteForcePoint(tris, q.Point)
		default:
			got = tree.QueryBox(q.Box, &st)
			want = bvh.BruteForceBox(tris, q.Box)
		}
		if !slices.Equal(got, want) {
			r.mismatch++
		}
		k := &r.byKind[int(q.Kind)]
		k.queries++
		k.visitedSum += st.VisitedNodes
		if st.VisitedNodes > k.visitedMax {
			k.visitedMax = st.VisitedNodes
		}
		k.prunedSum += st.PrunedNodes
		k.testedSum += st.Tested
		k.matchedSum += st.Matched
	}
	return r, nil
}
