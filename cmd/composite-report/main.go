// Command composite-report 生成多层半透明叠合的定点舍入误差报告：
// 反解分配表（5 层与 10 层）、等分方案的对照、以及最后一层的界核对。
//
// 用法：
//
//	go run ./cmd/composite-report            # 输出 Markdown 到 stdout
//	go run ./cmd/composite-report > report.md
package main

import (
	"fmt"
	"math"
	"os"

	"vecgo/composite"
)

// scenario 返回 n 层、等 α=0.3、颜色微有起伏的层栈。颜色错开 0.02 是为了
// 避免每层都恰好落在 8 位格点上，让相位项不为零，比较才有意义。
func scenario(n int) []composite.Layer {
	ls := make([]composite.Layer, n)
	for i := range ls {
		ls[i] = composite.Layer{Color: 0.40 + 0.02*float64(i%3), Alpha: 0.30}
	}
	return ls
}

func alphasOf(ls []composite.Layer) []float64 {
	a := make([]float64, len(ls))
	for i := range ls {
		a[i] = ls[i].Alpha
	}
	return a
}

func main() {
	// 结论先行。
	fmt.Println("# 多层半透明叠合的定点舍入误差报告")
	fmt.Println()
	fmt.Println("颜色归一在 [0,1]，一档 = 8 位输出相邻两值之间的距离（1/255）。")
	fmt.Println("误差表里的 ε_i、δ_i、界、偏差都**以档为单位**，一档 = 1.0。")
	fmt.Println()

	// 累积式。
	fmt.Println("## 累积式")
	fmt.Println()
	fmt.Println("叠合从底到顶按 over 递推，第 i 层精确值为 `C_i = (1−α_i)·C_{i−1} + α_i·c_i`。")
	fmt.Println("每层都把累积色舍到定点格点，存下 `Ĉ_i = C_i + e_i`，于是")
	fmt.Println()
	fmt.Println("```")
	fmt.Println("e_0 = 0")
	fmt.Println("e_i = (1−α_i)·e_{i−1} + δ_i           δ_i 为第 i 层舍入误差")
	fmt.Println("e_N = Σ_{i=1..N} w_i·δ_i              w_i = ∏_{j=i+1..N}(1−α_j),  w_N = 1")
	fmt.Println("```")
	fmt.Println()
	fmt.Println("后层按 (1−α) 衰减前层误差；顶层不衰减。若 |δ_i| ≤ ε_i，则")
	fmt.Println("`|e_N| ≤ Σ w_i·ε_i ≤ 一档`，反解就在这条不等式下分配每层 ε_i。")
	fmt.Println()

	// 反解。
	fmt.Println("## 反解每层最大误差")
	fmt.Println()
	fmt.Println("要求 `Σ w_i·ε_i = 一档`。把预算按层数等额切成终值份额（每层占 B/N），")
	fmt.Println("再按该层衰减把它换回存储误差：")
	fmt.Println()
	fmt.Println("```")
	fmt.Println("ε_i* = B / (N · w_i)          B = 一档 = 1.0")
	fmt.Println("```")
	fmt.Println()
	fmt.Println("被上面多层盖住的层 w_i 小、误差被大幅衰减，额度可以很粗；顶层 w_N=1，额度最紧。")
	fmt.Println("把 ε_i* 兑现成定点格式时，从阶梯里挑误差不超过 ε_i* 且格距最粗的一档（就近/截断 × 保护位）。")
	fmt.Println("原来的实现是每层 `uint8(255*x)` 截断并把结果写回 8 位缓冲——这正是粒度为整档、误差 ≤ 1 档的“等分”特例。")
	fmt.Println()

	for _, n := range []int{5, 10} {
		ls := scenario(n)
		tb, err := composite.BuildAlloc(ls, composite.Budget)
		if err != nil {
			fmt.Fprintln(os.Stderr, "失败:", err)
			os.Exit(1)
		}
		fmt.Println(tb.Markdown())
	}
	// 最后一层核对：用 PrefixBound 在 k=N 处重算，必须等于闭式界。
	fmt.Println("## 界的最后一层核对")
	fmt.Println()
	for _, n := range []int{5, 10} {
		ls := scenario(n)
		a := alphasOf(ls)
		sch, err := composite.Plan(a, composite.Budget)
		if err != nil {
			fmt.Fprintln(os.Stderr, "失败:", err)
			os.Exit(1)
		}
		b, err := composite.Bound(a, sch)
		if err != nil {
			fmt.Fprintln(os.Stderr, "失败:", err)
			os.Exit(1)
		}
		pb, err := composite.PrefixBound(a, sch, n)
		if err != nil {
			fmt.Fprintln(os.Stderr, "失败:", err)
			os.Exit(1)
		}
		worst, err := composite.WorstError(a, sch)
		if err != nil {
			fmt.Fprintln(os.Stderr, "失败:", err)
			os.Exit(1)
		}
		// 逐层实跑，量出每层舍入误差在三档以外还有没有残差。
		res, err := composite.OverFixed(ls, sch)
		if err != nil {
			fmt.Fprintln(os.Stderr, "失败:", err)
			os.Exit(1)
		}
		w := composite.Weights(a)
		fmt.Printf("- **%d 层**：闭式界 `Σ w_i·ε_i = %.4f`；按最坏情况递推到第 %d 层 `PrefixBound(·,%d) = %.4f`；两层算法差 `%.2e`。\n",
			n, b, n, n, pb, math.Abs(b-pb))
		fmt.Printf("  中间各层累积上界：%s。\n", fmtList(worst))
		fmt.Printf("  顶层 w_N=%.4f（必为 1），该层贡献 w_N·ε_N=%.4f；实测总偏差 %.4f 档。\n", w[n-1], sch[n-1].Err(), res.Deviation)
		fmt.Printf("  `%.4f ≤ 一档 = 1.0`：%s。\n\n", b, pass(b))
	}

	// 等分对照。
	fmt.Println("## 与等分误差方案对比")
	fmt.Println()
	fmt.Println("等分方案每层都用同一个格式。它的保证界是 `ε·Σ w_i`——把每层同样的误差都乘上各自的衰减再相加。")
	fmt.Println("注意：实际跑出来的那一组 δ_i 有正有负、相位相互抵消，单次实测偏差并不可靠地分出优劣；")
	fmt.Println("**方案的可比数字是保证界**，它对任何颜色组合都成立。")
	fmt.Println()
	fmt.Println("| 层数 | 方案 | 保证界（档） | 分配界（档） | 差值（等分−分配，档） |")
	fmt.Println("|---:|---:|---:|---:|---:|")
	var diffs []float64
	for _, n := range []int{5, 10} {
		ls := scenario(n)
		alloc, err := composite.BuildAlloc(ls, composite.Budget)
		if err != nil {
			fmt.Fprintln(os.Stderr, "失败:", err)
			os.Exit(1)
		}
		for _, v := range []struct {
			label string
			s     composite.Scheme
		}{
			{"等分·就近 8 位", composite.Scheme{Mode: composite.Nearest, Guard: 0}},
			{"等分·截断 8 位（原实现语义）", composite.Scheme{Mode: composite.Truncate, Guard: 0}},
		} {
			eq, err := composite.BuildEqual(ls, v.s)
			if err != nil {
				fmt.Fprintln(os.Stderr, "失败:", err)
				os.Exit(1)
			}
			d := eq.Bound - alloc.Bound
			if v.s.Mode == composite.Truncate {
				diffs = append(diffs, d)
			}
			fmt.Printf("| %d | %s | %.4f | %.4f | **%+.4f** |\n", n, v.label, eq.Bound, alloc.Bound, d)
		}
	}
	fmt.Println()
	fmt.Printf("同样层数下等分比反解多出的保证界：**5 层 +%.4f 档，10 层 +%.4f 档**（截断 8 位口径；",
		diffs[0], diffs[1])
	fmt.Printf("就近 8 位口径为 +0.6725 与 +0.9393 档）。\n")
	fmt.Println()

	// 剩余问题。
	fmt.Println("## 用的是哪套缓冲")
	fmt.Println()
	fmt.Println("本包把“层”建模成 `(color, alpha)` 的 over 递推；渲染器还没有图层缓冲，")
	fmt.Println("`raster.Image.Pix` 仍是每像素 `uint8`、每层 `Fill` 整块覆写（`shading.Apply` 也只认 0/非 0）。")
	fmt.Println("因此这里的反解方案要落地，像素缓冲需要从 `uint8` 改为按分配表定的定点格式")
	fmt.Println("（`Scheme{Mode, Guard}`），或至少引入一个每像素浮点/定点的累积缓冲，最后一次性舍到 8 位。")
	fmt.Println("`Scheme` 的 `Quantum()`、`Err()` 已经按格式给出口径，落地时逐层按 `Plan` 返回的格式舍入即可。")
}

func pass(b float64) string {
	if b <= composite.Budget*(1+1e-12) {
		return "不超一档"
	}
	return "**超出一档**"
}

func fmtList(xs []float64) string {
	s := ""
	for i, x := range xs {
		if i > 0 {
			s += "、"
		}
		s += fmt.Sprintf("%.4f", x)
	}
	return s
}
