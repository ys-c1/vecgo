package composite

import (
	"fmt"
	"math"
	"strings"
)

// Row 是误差分配表的一行：某一层的权重、反解上界、兑现格式、以及该层
// 实际产生的舍入误差 δ_i 和它衰减到终值后的贡献 w_i·δ_i。
type Row struct {
	Layer  int
	Alpha  float64
	W      float64 // 衰减系数
	Eps    float64 // 该层误差上界（分配表的 ε_i*；等分表的常数 ε）
	Scheme Scheme
	Err    float64 // 兑现的 ε_i = Scheme.Err()
	WErr   float64 // w_i·ε_i
	Delta  float64 // 实际 δ_i（档）
	WDelta float64 // w_i·δ_i
}

// Table 是某一层数下的逐层误差表。Bound 是保证上界 Σ w_i·ε_i；
// Deviation 是这组层叠出来的实际总偏差 Σ w_i·δ_i（档）。
type Table struct {
	Title     string
	Budget    float64 // 本次方案的预算（档）
	Rows      []Row
	Bound     float64
	Deviation float64
}

// WorstError 按最坏情况逐层传播 |e_k| ≤ ε_k + (1−α_k)·|e_{k−1}|，返回每层
// 之后的累积误差上界，长度为 N；最后一项就是终值的保证界。它让“界对最后一层
// 验一次”有逐层可看的中间值。
func WorstError(alphas []float64, schemes []Scheme) ([]float64, error) {
	if err := checkAlphas(alphas); err != nil {
		return nil, err
	}
	if len(schemes) != len(alphas) {
		return nil, fmt.Errorf("%w: %d 层 vs %d 方案", ErrShape, len(alphas), len(schemes))
	}
	out := make([]float64, len(alphas))
	e := 0.0
	for i := range alphas {
		if !schemes[i].Valid() {
			return nil, fmt.Errorf("%w: 第 %d 层 %v", ErrRoundMode, i+1, schemes[i])
		}
		e = schemes[i].Err() + (1-alphas[i])*e
		out[i] = e
	}
	return out, nil
}

// SumWeights 返回 Σ w_i。等分方案下这个数就是误差放大倍数。
func SumWeights(alphas []float64) float64 {
	s := 0.0
	for _, w := range Weights(alphas) {
		s += w
	}
	return s
}

// BuildAlloc 按反解分配 ε_i* = B/(N·w_i) 建表：逐层兑现格式并量出实际误差。
func BuildAlloc(layers []Layer, budget float64) (Table, error) {
	if len(layers) == 0 {
		return Table{}, ErrNoLayers
	}
	alphas := make([]float64, len(layers))
	for i, l := range layers {
		alphas[i] = l.Alpha
	}
	eps, err := Allowed(alphas, budget)
	if err != nil {
		return Table{}, err
	}
	w := Weights(alphas)
	sch := make([]Scheme, len(layers))
	for i := range sch {
		if math.IsInf(eps[i], 1) {
			sch[i] = Scheme{Mode: Truncate, Guard: 0}
			continue
		}
		s, err := Realize(eps[i])
		if err != nil {
			return Table{}, fmt.Errorf("第 %d 层: %w", i+1, err)
		}
		sch[i] = s
	}
	res, err := OverFixed(layers, sch)
	if err != nil {
		return Table{}, err
	}
	t := Table{Title: "反解分配", Budget: budget, Deviation: res.Deviation}
	for i := range layers {
		r := Row{
			Layer: i + 1,
			Alpha: layers[i].Alpha,
			W:     w[i],
			Eps:   eps[i],
			Scheme: sch[i],
			Err:   sch[i].Err(),
			Delta: res.Deltas[i],
		}
		r.WErr = r.W * r.Err
		r.WDelta = r.W * r.Delta
		t.Bound += r.WErr
		t.Rows = append(t.Rows, r)
	}
	return t, nil
}

// BuildEqual 建等分方案的表：每层都用同一个格式（同一个误差上界 ε），
// 因此每层分到的误差额度与它被后面衰减多少无关——这正是它的问题。
func BuildEqual(layers []Layer, s Scheme) (Table, error) {
	if len(layers) == 0 {
		return Table{}, ErrNoLayers
	}
	if !s.Valid() {
		return Table{}, fmt.Errorf("%w: %v", ErrRoundMode, s)
	}
	alphas := make([]float64, len(layers))
	for i, l := range layers {
		alphas[i] = l.Alpha
	}
	w := Weights(alphas)
	sch := make([]Scheme, len(layers))
	for i := range sch {
		sch[i] = s
	}
	res, err := OverFixed(layers, sch)
	if err != nil {
		return Table{}, err
	}
	t := Table{Title: "等分误差", Budget: Budget, Deviation: res.Deviation}
	for i := range layers {
		r := Row{
			Layer:  i + 1,
			Alpha:  layers[i].Alpha,
			W:      w[i],
			Eps:    s.Err(),
			Scheme: s,
			Err:    s.Err(),
			Delta:  res.Deltas[i],
		}
		r.WErr = r.W * r.Err
		r.WDelta = r.W * r.Delta
		t.Bound += r.WErr
		t.Rows = append(t.Rows, r)
	}
	return t, nil
}

// Markdown 把表渲染成 Markdown，末行是合计并同时给出界与偏差。
// 最后一行的“总误差 vs 界”一列就是要求核对的那一格。
func (t Table) Markdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, "### %s（%d 层，预算 %.4f 档）\n\n", t.Title, len(t.Rows), t.Budget)
	b.WriteString("| 层 | α | w_i | ε_i* | 格式 | ε_i | w_i·ε_i | δ_i | w_i·δ_i |\n")
	b.WriteString("|---:|---:|---:|---:|:--:|---:|---:|---:|---:|\n")
	for _, r := range t.Rows {
		eps := fmtS(r.Eps)
		fmt.Fprintf(&b, "| %d | %.2f | %.4f | %s | %s | %.4f | %.4f | %+.4f | %+.4f |\n",
			r.Layer, r.Alpha, r.W, eps, r.Scheme, r.Err, r.WErr, r.Delta, r.WDelta)
	}
	verdict := "不超"
	if t.Bound > Budget*(1+1e-12) {
		verdict = "**超出**"
	}
	fmt.Fprintf(&b, "| 合计 | | | | | | **%.4f** | | **%+.4f** |\n\n", t.Bound, t.Deviation)
	fmt.Fprintf(&b, "保证界 Σw_i·ε_i = %.4f 档，%s一档；实测总偏差 Σw_i·δ_i = %+.4f 档（|偏差| %.4f 档）。\n",
		t.Bound, verdict, t.Deviation, math.Abs(t.Deviation))
	return b.String()
}

func fmtS(x float64) string {
	if math.IsInf(x, 1) {
		return "+∞"
	}
	return fmt.Sprintf("%.4f", x)
}
