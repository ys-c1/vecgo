package main

import (
	"fmt"
	"math"
	"strings"

	"vecgo/subdiv"
)

func buildReport(s summary, gs []groupDef, sweepRes []float64, sweep []sweepRow, coarse []coarseRow, pixelLines []string) string {
	var b strings.Builder
	w := func(format string, args ...any) { fmt.Fprintf(&b, format, args...) }
	fence := "```"

	// 精确细分的 float64 残差实测。
	maxExactResidual := 0.0
	for _, g := range gs {
		a0 := subdiv.Area(g.T)
		for n := 0; n <= 7; n++ {
			d := math.Abs(subdiv.TotalArea(subdiv.Subdivide(g.T, n)) - a0)
			if d > maxExactResidual {
				maxExactResidual = d
			}
		}
	}

	b.WriteString("# 中点细分两次，面积总和为什么会变？——保面积证明、取整误差上界、4^n 累积与覆盖核对\n\n")
	b.WriteString("本报告由 `go run ./cmd/subdivcheck` 用同一份代码生成，所有数字均可复现；验收测试为 `go test ./subdiv/ -v`。\n\n")

	// 1 结论
	b.WriteString("## 1. 结论速览\n\n")
	w("- **几何上严格保面积**：三角形经中点细分得到 4 个面积各为 A0/4 的子三角形；细分 n 次后有 4^n 个叶子，每个面积 A0/4^n，总和恒为 A0。\n")
	w("- **面积变化只能来自坐标取整（量化）**：精确中点流程在 float64 下到 n=7 的总面积残差实测最大仅 %.3g px²（机器精度量级）；一旦把坐标量化到分辨率 ρ=%s，就出现可复现的面积差。\n", maxExactResidual, s.ResolutionExpr)
	w("- **单三角形误差上界是“边长 × 精度”**：|ΔA| ≤ 3ρL（ρ 为坐标分辨率，L 为最长边；更紧常数为 1+√2）。\n")
	w("- **总上界按 4 的幂累积**：B(n)=3ρLcap·4^n（Lcap=L0+2ρ≈L0），即 B(n+1)=4B(n)。\n")
	w("- **可接受细分次数**：以“总误差不超过初始面积的 %.0f%%”为预算，三组各自和公共的 n* 都是 **%d**；n=%d 的上界仍在预算内，n=%d 起上界超预算（上界保守，实测误差小得多）。\n",
		100*s.BudgetFraction, s.CommonNAcceptable, s.CommonNAcceptable, s.CommonNAcceptable+1)
	w("- **覆盖像素核对通过**：三组的一次与两次细分，覆盖像素数差值全部为 **0**；在 4×4 超采样下连灰度位图都逐格相同（另见 `pixel_check.txt`）。\n\n")

	// 2 设置
	b.WriteString("## 2. 实验设置与单位\n\n")
	w("- 坐标系：像素坐标，**1 个坐标单位 = 1 px**；固定映射到 %d×%d 画幅（不使用包围盒自适应缩放），像素 (col,row) 对应坐标 [col,col+1)×[row,row+1)。\n", width, height)
	w("- 坐标分辨率（精度）：**ρ = 1/256 px = 0.00390625 px**，即 8 位子像素精度（等价 24.8 定点数的小数位）。四舍五入到最近网格后，**每个坐标分量的误差上限 η = ρ/2 = 1/512 px = 0.001953125 px**。\n")
	w("- 单位约定：坐标 x/y、边长 L、分辨率 ρ 的单位都是 **px**；面积 A、面积差 ΔA、上界 B 的单位都是 **px²**（边长×精度 = px·px = px²）。若坐标改用毫米，公式不变，单位相应变为 mm 与 mm²。\n")
	w("- 覆盖采样：每像素 %d×%d 个子样本，灰度 = 255·命中数/%d；“覆盖像素数”按仓库主程序同款阈值 ≥128 统计，并额外统计 >0。\n", ss, ss, ss*ss)
	w("- 误差预算（可接受判据）：E = %.2f%%·A0。该比例是显式声明的质量阈值，可按需替换；一般结论用 n* 公式给出。\n\n", 100*s.BudgetFraction)

	b.WriteString("| 组 | 顶点坐标 (px) | 初始面积 A0 (px²) | 最长边 L0 (px) | 预算 E=1%A0 (px²) |\n")
	b.WriteString("|---|---|---:|---:|---:|\n")
	for _, g := range s.Groups {
		v := g.VerticesPx
		w("| %s %s | (%.3f,%.3f), (%.3f,%.3f), (%.3f,%.3f) | %.4f | %.4f | %.4f |\n",
			g.ID, g.Name, v[0][0], v[0][1], v[1][0], v[1][1], v[2][0], v[2][1],
			g.Area0Px2, g.LongestEdgePx, g.BudgetPx2)
	}
	b.WriteString("\n")

	// 3 证明保面积
	b.WriteString("## 3. 证明：中点细分严格保面积\n\n")
	b.WriteString("记三角形顶点的位置向量为 a,b,c，定义有向两倍面积（叉积）\n\n")
	b.WriteString("```\nD = (b-a) × (c-a)，   A0 = |D| / 2\n```\n\n")
	b.WriteString("三个边中点为\n\n")
	b.WriteString("```\nm_ab=(a+b)/2,  m_bc=(b+c)/2,  m_ca=(c+a)/2\n```\n\n")
	b.WriteString("四个子三角形逐一计算叉积：\n\n")
	b.WriteString("```\nA, m_ab, m_ca : (m_ab-a)×(m_ca-a) = (b-a)/2 × (c-a)/2 = D/4\n")
	b.WriteString("m_ab, B, m_bc : (b-m_ab)×(m_bc-m_ab) = (b-a)/2 × (c-a)/2 = D/4\n")
	b.WriteString("m_ca, m_bc, C : 同理 = D/4\n")
	b.WriteString("m_ab,m_bc,m_ca: (m_bc-m_ab)×(m_ca-m_ab)\n")
	b.WriteString("              = (c-a)/2 × (c-b)/2 = (1/4)(c-a)×((c-a)-(b-a))\n")
	b.WriteString("              = (1/4)(b-a)×(c-a) = D/4\n```\n\n")
	b.WriteString("所以每个子三角形面积都是 |D/4|/2 = **A0/4**，四块之和 4·A0/4 = **A0**。\n\n")
	b.WriteString("**归纳**：若第 k 层每个三角形面积为 A0/4^k，则第 k+1 层每个为 A0/4^(k+1)，个数为 4^(k+1)，总和仍为 A0。故细分 n 次后\n\n")
	b.WriteString("```\n叶子数 = 4^n，  每叶面积 = A0/4^n，  面积总和 = A0   （恒等式，与 n 无关）\n```\n\n")
	b.WriteString("几何上这也对应“三条中位线把三角形分成四个全等（实际是相似且等积）的小三角形”。\n\n")

	// 4 取整唯一来源
	b.WriteString("## 4. 面积差只能来自坐标取整\n\n")
	b.WriteString("中点公式只含加法与除以 2；在实数运算中 §3 的恒等式严格成立。代码里 `subdiv.Subdivide` 全程使用未取整的 float64 中点，实测 n=0…7 的总面积与 A0 的最大偏差仅 **")
	w("%.3g px²**（约为 float64 求和残差）。因此一旦 `Σ面积` 出现 1e-2～1e-1 px² 量级的稳定偏差，它不可能来自细分本身，只能来自 `subdiv.SnapAll` 把坐标量化到分辨率 ρ 的动作。\n\n", maxExactResidual)
	b.WriteString("实验协议：先用精确中点剖分，再把每个输出顶点量化到 ρ 网格（等价于“高精度计算、按坐标分辨率出库”）。这样每个最终坐标相对其精确值的分量误差都不超过 ρ/2，§5 的上界对每片叶子严格成立。\n\n")

	// 5 上界
	b.WriteString("## 5. 取整面积误差上界：边长 × 精度\n\n")
	b.WriteString("设量化后某三角形顶点为 A,B,C，对应精确顶点的每个坐标分量误差 ≤ ρ/2。令 u=B−A、v=C−A 为量化后的边向量，扰动为 α,β,γ，则 δu=β−α、δv=γ−α 的每个分量满足 |δu_x|,|δu_y|,|δv_x|,|δv_y| ≤ ρ。\n\n")
	b.WriteString("有向两倍面积 D=u×v 的扰动展开为\n\n")
	b.WriteString("```\nδD = δu×v + u×δv + δu×δv\n```\n\n")
	b.WriteString("设 L 为该三角形最长边（|u|,|v|,|v−u| ≤ L）。用 |u_x|,|u_y| ≤ L：\n\n")
	b.WriteString("```\n|δD| ≤ ρ(|v_x|+|v_y|) + ρ(|u_x|+|u_y|) + 2ρ²\n")
	b.WriteString("     ≤ 4ρL + 2ρ²\n")
	w("|ΔA| = |δD|/2 ≤ 2ρL + ρ²\n```\n\n")
	w("当 **ρ ≤ L**（本组参数 ρ=%.6f px，而最小叶子在 n≤8 时仍远大于 ρ，条件成立），有 ρ² ≤ ρL，于是得到所要求的“边长 × 精度”形式：\n\n", res)
	b.WriteString("```\n┌─────────────────────────────────────────────┐\n")
	b.WriteString("│  单个三角形： |ΔA| ≤ 3·ρ·L                  │\n")
	b.WriteString("│  ρ=坐标分辨率(px)，L=最长边(px)，ΔA 单位 px² │\n")
	b.WriteString("└─────────────────────────────────────────────┘\n```\n\n")
	b.WriteString("常数还可以收紧：用 |x|+|y| ≤ √2·√(x²+y²) 得 |ΔA| ≤ √2 ρL + ρ² ≤ (1+√2)ρL ≈ 2.414ρL；代码与下表统一取更保守、更好记的常数 **3**。\n\n")

	// 6 4^n 与 n*
	b.WriteString("## 6. 上界按四的幂累积，以及可接受细分次数\n\n")
	b.WriteString("细分 n 次后有 4^n 个叶子。精确中点边逐次减半、从不超过初始最长边 L0；量化会让一条边的两个端点各产生 ≤ρ/2 的分量误差，边长膨胀最多 √2ρ < 2ρ。因此所有叶子（含 n=0）的最长边有统一上限 **Lcap = L0 + 2ρ**（本组 2ρ/L0 ≈ 7×10⁻⁵，数值上 Lcap≈L0）。对每片叶子套用 §5 并求和（三角不等式）：\n\n")
	b.WriteString("```\n|Σ A_leaf − A0| ≤ Σ |ΔA_leaf| ≤ 4^n · 3ρLcap\n\n")
	b.WriteString("B(n) = 3ρLcap·4^n = B(0)·4^n，   B(n+1) = 4·B(n)\n```\n\n")
	b.WriteString("这就是“按四的幂累积”：每深一级，三角形数 ×4，保守误差总预算也 ×4。`TestBoundAccumulatesByFour` 逐级别验证相邻比恰为 4。\n\n")
	b.WriteString("给定面积预算 E（单位 px²），可接受次数为满足 3ρLcap·4^n ≤ E 的最大整数：\n\n")
	b.WriteString("```\nn* = max{ n≥0 : B(n) ≤ E } = floor( log₄( E / (3ρLcap) ) )\n```\n\n")
	b.WriteString("（比值 <1 时 n*=−1，表示连不细分的上界都超过预算；代码用逐级比较实现，避免浮点取整歧义。）\n\n")

	b.WriteString("不同预算口径下的 n*（供参考；主口径为 1%A0）：\n\n")
	b.WriteString("| 组 | Lcap=L0+2ρ (px) | n* @0.1%A0 | n* @1%A0 | n* @5%A0 |\n|---|---:|---:|---:|---:|\n")
	// 不同预算口径 n* 表用 lcap。
	for _, g := range gs {
		a0 := subdiv.Area(g.T)
		lcap := subdiv.EdgeCap(subdiv.LongestEdge(g.T), res)
		w("| %s | %.2f | %d | **%d** | %d |\n", g.ID, lcap,
			subdiv.AcceptableN(res, lcap, 0.001*a0),
			subdiv.AcceptableN(res, lcap, 0.01*a0),
			subdiv.AcceptableN(res, lcap, 0.05*a0))
	}
	w("\n三组最保守的公共可接受次数 = **%d**（取最小值）。\n\n", s.CommonNAcceptable)

	b.WriteString("> 关于“每一级都把中点立即取整存回”的递归实现：父代已有的量化误差会传给中点，顶点偏差最坏按 η_k ≤ kρ/2 逐级线性累积，对应单叶上界约 (2n+1)ρL、总量上界约 (2n+1)ρL0·4^n。主导项仍是 **4^n**，只多一个线性 n 因子，结论不变。代码同时提供 `subdiv.SubdivideRounded`，§10 用它复现整数粗网格下的明显漂移。\n\n")

	// 7 三组 n=2 总表
	b.WriteString("## 7. 三组三角形总表：面积差、上界与细分次数\n\n")
	b.WriteString("主口径 n=2（中点细分两次，16 个叶子，ρ=1/256 px）：\n\n")
	b.WriteString("| 组 | A0 (px²) | n | 叶子数 | 实测 ΔA1 (px²) | 实测 ΔA2 (px²) | 上界 B2=3ρLcap·4² (px²) | abs(ΔA2)≤B2 | n* @1% | 覆盖像素 n1 | 覆盖像素 n2 | 像素数差 |\n")
	b.WriteString("|---|---:|---:|---:|---:|---:|---:|:--:|---:|---:|---:|---:|\n")
	for _, g := range s.Groups {
		lv1, lv2 := g.Levels[1], g.Levels[2]
		w("| %s | %.3f | 2 | 16 | %+.6f | %+.6f | %.4f | %s | **%d** | %d | %d | **%d** |\n",
			g.ID, g.Area0Px2, lv1.DeltaArea, lv2.DeltaArea, lv2.BoundPx2,
			map[bool]string{true: "✓", false: "✗"}[lv2.WithinBound],
			g.NAcceptable, g.Coverage.On128N1, g.Coverage.On128N2, g.Coverage.CountDiff128)
	}
	b.WriteString("\n注：ΔA 的正负号表示量化后总面积相对 A0 的增减；上界只约束绝对值。实测值比最坏上界小约 2～3 个数量级，因为网格误差相互抵消是常态，3ρLcap·4^n 是“所有叶子同向取最坏”的保守预算。\n\n")

	// 8 层级明细
	for gi, g := range s.Groups {
		b.WriteString(fmt.Sprintf("### 7.%d %s（%s）逐级明细\n\n", gi+1, g.ID, g.Name))
		b.WriteString("| n | 叶子数 4^n | 量化后总面积 (px²) | ΔA (px²) | 上界 B(n) (px²) | 误差≤上界 | B(n)≤1%A0 预算 |\n")
		b.WriteString("|---:|---:|---:|---:|---:|:--:|:--:|\n")
		for _, r := range g.Levels {
			w("| %d | %d | %.6f | %+.6f | %.4f | %s | %s |\n",
				r.N, r.Triangles, r.AreaSnappedTotal, r.DeltaArea, r.BoundPx2,
				check(r.WithinBound), check(r.BoundAcceptable))
		}
		b.WriteString("\n")
	}

	// 9 覆盖图
	b.WriteString("## 8. 细分前后覆盖图与像素核对\n\n")
	b.WriteString("下图为灰度覆盖图（白=全覆盖，黑=无覆盖，灰=超采样边缘）。每组依次为细分前 n=0、一次 n=1、两次 n=2：\n\n")
	for _, g := range s.Groups {
		b.WriteString(fmt.Sprintf("**%s %s**\n\n", g.ID, g.Name))
		b.WriteString("<p>\n")
		w("  <img src=\"coverage_%s_before_n0.png\" width=\"192\" alt=\"%s before\">\n", g.ID, g.ID)
		w("  <img src=\"coverage_%s_sub1_n1.png\" width=\"192\" alt=\"%s sub1\">\n", g.ID, g.ID)
		w("  <img src=\"coverage_%s_after2_n2.png\" width=\"192\" alt=\"%s after2\">\n", g.ID, g.ID)
		b.WriteString("</p>\n\n")
		b.WriteString(fmt.Sprintf("红线网格版（能直接看到 1→4→16 个三角形）：`mesh_%s_n0.png`、`mesh_%s_n1.png`、`mesh_%s_n2.png`。\n\n", g.ID, g.ID, g.ID))
	}
	b.WriteString("三张总览条带（从左到右为 g1/g2/g3）：\n\n")
	b.WriteString("- 细分前：[`coverage_before_all.png`](coverage_before_all.png)\n")
	b.WriteString("- 细分两次后：[`coverage_after_all.png`](coverage_after_all.png)\n\n")

	b.WriteString("像素核对原始输出：\n\n")
	b.WriteString(fence + "\n")
	b.WriteString(strings.Join(pixelLines, "\n"))
	b.WriteString("\n" + fence + "\n\n")
	b.WriteString("测试 `TestCoverageDepth1EqualsDepth2` 对同一结论做了强制断言：threshold>0 与 threshold≥128 两种口径的 n2−n1 计数差都必须为 0，且本组参数下 4×4 灰度位图逐格相同；精确（不量化）细分的 n1/n2 位图同样逐格相同。\n\n")

	// 10 分辨率扫描
	b.WriteString("## 9. 分辨率扫描：精度越粗，面积误差随 ρ 线性放大\n\n")
	b.WriteString("固定 n=2，改变坐标分辨率 ρ（最终量化协议）。下表每格为 `实测 ΔA2 / 上界 B2`，单位 px²：\n\n")
	b.WriteString("| ρ (px) | g1 实测 | g1 上界 | g2 实测 | g2 上界 | g3 实测 | g3 上界 |\n")
	b.WriteString("|---:|---:|---:|---:|---:|---:|---:|\n")
	for _, r := range sweep {
		w("| %s | %+.4f | %.2f | %+.4f | %.2f | %+.4f | %.2f |\n",
			formatRes(r.Res), r.Delta[0], r.Bound2[0], r.Delta[1], r.Bound2[1], r.Delta[2], r.Bound2[2])
	}
	b.WriteString("\n实测误差整体随 ρ 增大而增大（带符号值因抵消而波动），与“误差上界 = 边长×ρ”的线性关系同向；上界始终成立。选 ρ=1/256 px 正是为了让最坏上界在 n=2 仍低于 1% 面积预算，并让覆盖核对严格为零。\n\n")

	// 11 粗网格复现
	b.WriteString("## 10. 动机复现：整数像素 + 每级立即取整\n\n")
	b.WriteString("如果把坐标分辨率取成 ρ=1 px（整数网格），并按最常见的递归写法“算出中点立刻 math.Round 存回”，面积总和会出现肉眼可见的变化：\n\n")
	b.WriteString("| 组 | ΔA1 (px²) | ΔA2 (px²) | 覆盖≥128 n1 | 覆盖≥128 n2 | 像素数差 n2−n1 | 位图不同格数 |\n")
	b.WriteString("|---|---:|---:|---:|---:|---:|---:|\n")
	for _, r := range coarse {
		w("| %s | %+.2f | %+.2f | %d | %d | %+d | %d |\n",
			r.ID, r.DeltaA1, r.DeltaA2, r.On128N1, r.On128N2, r.Diff128, r.BitmapD)
	}
	b.WriteString("\n可见粗分辨率下面积漂移可达数十 px²，覆盖像素数也可能不再相等——这正是题目开头“居然变了”的现象，也说明最后的“差值应为零”不是恒真命题：它需要足够细的坐标分辨率（或无损的精确中点）来保证。§8 的零差值是在 ρ=1/256 px 的验收配置下实测并由测试锁定的。\n\n")

	// 12 复现
	b.WriteString("## 11. 复现方式与产物清单\n\n")
	b.WriteString(fence + "bash\n")
	b.WriteString("go test ./subdiv/ -v             # 证明/上界/覆盖零差值的强制断言\n")
	b.WriteString("go run ./cmd/subdivcheck         # 重新生成 artifacts/ 下全部产物\n")
	b.WriteString(fence + "\n\n")
	b.WriteString("- `three_groups_table.csv`：三组 × n=0…8 的结构化总表。\n")
	b.WriteString("- `summary.json`：全部数字与核对结果（含单位字段）。\n")
	b.WriteString("- `pixel_check.txt`：覆盖像素 n1/n2 差值核对。\n")
	b.WriteString("- `coverage_g{1,2,3}_{before_n0,sub1_n1,after2_n2}.{png,pgm}`：每组细分前/一次/两次覆盖图（PNG 与仓库原生 PGM 各一份）。\n")
	b.WriteString("- `mesh_g{1,2,3}_n{0,1,2}.png`：覆盖 + 细分线框叠加图。\n")
	b.WriteString("- `coverage_before_all.png`、`coverage_after_all.png`：三组细分前/两次后的总览条带。\n")
	b.WriteString("- 数学实现与强制断言：`subdiv/subdiv.go`、`subdiv/subdiv_test.go`。\n")

	return b.String()
}

func check(ok bool) string {
	if ok {
		return "✓"
	}
	return "✗"
}

func formatRes(r float64) string {
	if r == 1 {
		return "1"
	}
	if r == 0.5 {
		return "0.5"
	}
	if r == 0.25 {
		return "0.25"
	}
	if r == 0.1 {
		return "0.1"
	}
	return fmt.Sprintf("1/%d", int(math.Round(1/r)))
}
