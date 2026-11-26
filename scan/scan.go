// Package scan 研究凸多边形逐行扫描填充：
//
//   - Recompute：每一行都重新遍历全部边、用除法求区间端点（慢的基准实现）；
//   - Incremental：利用凸多边形的两条 y-单调链，端点按 Δx 增量推进，
//     仅在越界时换边（本题要求的增量实现）；
//   - EvenOdd：收集一条扫描线上的全部交点并按奇偶配对，作为任意
//     简单多边形（含非凸）的正确参照；
//   - FixedDeltaWrong：“首行定 Δ、之后永远 x+=Δ”的最简增量写法，
//     专门用作非凸形状上的反例。
//
// 行坐标约定：第 r 行扫描线取 y = r + 1/2（像素中心），第 c 列中心为
// c + 1/2，中心落入闭区间 [XL, XR] 的像素被点亮。三组实验形状的顶点 y
// 都是整数，因此半整数扫描线不会经过顶点，不存在交点二义性。
package scan

import (
	"math"
	"sort"

	"vecgo/geom"
)

// Span 是第 R 行上的端点区间 [XL, XR]。
type Span struct {
	R      int
	XL, XR float64
}

// Counts 是一次扫描过程中的工作量/边界检查计数。
type Counts struct {
	RowsCovered int   // 实际落在多边形 y 范围内的扫描行数
	EdgeTests   int   // 逐行重算时做的“边是否跨过本行”检查次数
	Divisions   int   // 真正执行端点插值除法的次数
	Adds        int   // 增量路径执行的 x += dxdy 次数
	OOBChecks   int   // 越界判定条件 (x'-xt)(x'-xb) > 0 的求值次数
	Switches    int   // 越界后换到下一条边的次数
	ClipChecks  int   // 写帧缓冲前的列裁剪判定次数（每次写区间 1 次）
	PerRowOOB   []int // 按行统计的越界判定次数，长度为画布高 H
}

func newCounts(h int) Counts {
	return Counts{PerRowOOB: make([]int, h)}
}

// 以下计数助手都允许接收 nil：精简计时路径传 nil，零统计开销。
func (c *Counts) edgeTest() {
	if c != nil {
		c.EdgeTests++
	}
}

func (c *Counts) division() {
	if c != nil {
		c.Divisions++
	}
}

func (c *Counts) add() {
	if c != nil {
		c.Adds++
	}
}

func (c *Counts) spanCovered() {
	if c != nil {
		c.RowsCovered++
	}
}

func (c *Counts) oob(r int) {
	if c == nil {
		return
	}
	c.OOBChecks++
	if r >= 0 && r < len(c.PerRowOOB) {
		c.PerRowOOB[r]++
	}
}

func (c *Counts) switching() {
	if c != nil {
		c.Switches++
	}
}

// ---------- 公共的区间 -> 像素 ----------

// tieEps 用来吸收“精确落在像素中心判定线上”时，除法直算与 Δ 累加
// 之间约 1 ulp 的浮点差异（真实端点距判定线非零时远大于该容差）。
const tieEps = 1e-9

// columns 返回中心落在 [xl, xr] 内的列号闭区间 [lo, hi]（未裁剪）。
func columns(xl, xr float64) (lo, hi int) {
	lo = int(math.Ceil(xl - 0.5 - tieEps))  // 最小的 c 使 c+1/2 >= xl
	hi = int(math.Floor(xr - 0.5 + tieEps)) // 最大的 c 使 c+1/2 <= xr
	return lo, hi
}

// paintSpan 把一个区间画进灰度缓冲（255=亮），并做帧缓冲边界裁剪。
func paintSpan(pix []uint8, w, h int, s Span, c *Counts) {
	lo, hi := columns(s.XL, s.XR)
	if c != nil {
		c.ClipChecks++
	}
	if lo < 0 {
		lo = 0
	}
	if hi >= w {
		hi = w - 1
	}
	if s.R < 0 || s.R >= h { // 行级边界保护
		return
	}
	for x := lo; x <= hi; x++ {
		if x < 0 || x >= w { // 列级边界保护（正常裁剪后不会触发）
			continue
		}
		pix[s.R*w+x] = 255
	}
}

// Paint 用给定区间填充一块 w*h 的灰度缓冲。
func Paint(spans []Span, w, h int) []uint8 {
	pix := make([]uint8, w*h)
	for _, s := range spans {
		paintSpan(pix, w, h, s, nil)
	}
	return pix
}

// Fill 把区间画进调用方复用的缓冲（计时热路径用，不做计数）。
// 调用前缓冲应已清零；同一组区间重复 Fill 是幂等的。
func Fill(pix []uint8, w, h int, spans []Span) {
	for _, s := range spans {
		paintSpan(pix, w, h, s, nil)
	}
}

// ---------- 基准：逐行重算 ----------

// edgeX 在 y 处对边 a-b 做线性插值求 x；调用前需确认边跨过 y。
func edgeX(a, b geom.Point, y float64, c *Counts) float64 {
	c.division()
	t := (y - a.Y) / (b.Y - a.Y)
	return a.X + (b.X-a.X)*t
}

// RecomputeIntervals 是带工作量统计的“逐行重算”。
func RecomputeIntervals(poly []geom.Point, w, h int) ([]Span, Counts) {
	c := newCounts(h)
	spans := recompute(poly, w, h, &c)
	return spans, c
}

// RecomputeSpans 是无统计的精简热路径，供计时使用。
func RecomputeSpans(poly []geom.Point, w, h int) []Span {
	return recompute(poly, w, h, nil)
}

// recompute 每行遍历全部边、从头求两个端点。
// 对凸多边形，一条扫描线至多两个交点，取 min/max 即区间。
func recompute(poly []geom.Point, w, h int, c *Counts) []Span {
	spans := make([]Span, 0, h)
	n := len(poly)
	for r := 0; r < h; r++ {
		y := float64(r) + 0.5
		var xl, xr float64
		have := false
		for i := 0; i < n; i++ {
			a := poly[i]
			b := poly[(i+1)%n]
			c.edgeTest()
			if (a.Y > y) == (b.Y > y) {
				continue // 两点在扫描线同侧（含水平边），无交点
			}
			x := edgeX(a, b, y, c)
			if !have {
				xl, xr, have = x, x, true
			} else {
				if x < xl {
					xl = x
				}
				if x > xr {
					xr = x
				}
			}
		}
		if have {
			c.spanCovered()
			spans = append(spans, Span{R: r, XL: xl, XR: xr})
		}
	}
	return spans
}

// Recompute 是“逐行重算 + 填充”的完整基准路径（带统计）。
func Recompute(poly []geom.Point, w, h int) ([]uint8, Counts) {
	c := newCounts(h)
	spans := recompute(poly, w, h, &c)
	pix := make([]uint8, w*h)
	for _, s := range spans {
		paintSpan(pix, w, h, s, &c)
	}
	return pix, c
}

// ---------- 增量：两条 y-单调链 ----------

// chain 是从 y 最小顶点沿环的一个方向走到 y 最大顶点的 y-单调链。
// 它直接以顶点索引沿环步进，不预建边表、不产生堆分配。
type chain struct {
	poly     []geom.Point
	cur, end int // 当前待考察的顶点索引；到达 end(iMax) 后链尽
	step, n  int // 环上步进方向 +1 / -1
	x        float64
	xt, xb   float64 // 当前边上下端点 x
	yt, yb   float64 // yb > yt
	dxdy     float64
	primed   bool
}

func makeChain(poly []geom.Point, start, end, step int) chain {
	return chain{poly: poly, cur: start, end: end, step: step, n: len(poly)}
}

// nextEdge 把游标移到下一条非水平边并按 y 定向；链尽返回 false。
func (ch *chain) nextEdge(c *Counts) bool {
	for ch.cur != ch.end {
		i := ch.cur
		j := (i + ch.step + ch.n) % ch.n
		a, b := ch.poly[i], ch.poly[j]
		ch.cur = j
		if a.Y == b.Y {
			continue // 水平边与半整数扫描线不相交，跳过
		}
		if a.Y > b.Y {
			a, b = b, a
		}
		ch.xt, ch.yt = a.X, a.Y
		ch.xb, ch.yb = b.X, b.Y
		ch.dxdy = (ch.xb - ch.xt) / (ch.yb - ch.yt) // 每条非水平边一次除法
		c.division()
		return true
	}
	return false
}

// update 返回链在扫描线 y（第 r 行）处的端点 x。
//
// 核心只有两件事：
//  1. 同行增量：ch.x += ch.dxdy（更新量就是 Δx=(x2-x1)/(y2-y1)，行距为 1）；
//  2. 越界判定（实现中使用的一行式子，对竖直边同样成立）：
//     oob := y >= ch.yb
//     即当前扫描线到达/越过边的下端点，该换边了。
//     对非竖直边，它等价于 x 越出端点区间的代数判定
//     (ch.x-ch.xt)*(ch.x-ch.xb) > 0；竖直边 xt==xb 时该乘积退化，
//     所以统一用 y 式。
func (ch *chain) update(y float64, r int, c *Counts) (float64, bool) {
	if !ch.primed {
		if !ch.nextEdge(c) {
			return 0, false
		}
		ch.x = ch.xt + (y-ch.yt)*ch.dxdy // 首行用预计算斜率定位（乘法，无除法）
		ch.primed = true
	} else {
		ch.x += ch.dxdy
		c.add()
	}
	for {
		oob := y >= ch.yb // 越界判定条件（一行）：扫描线越过边的下端 y 边界
		c.oob(r)
		if !oob {
			return ch.x, true
		}
		// 越过当前边的下端点：凸性保证下一条边恰好从该端点开始。
		c.switching()
		if !ch.nextEdge(c) {
			return 0, false
		}
		ch.x = ch.xt + (y-ch.yt)*ch.dxdy // 仅换边时重新定位一次（乘法）
	}
}

// buildChains 用 y 最小、y 最大的两个顶点把环切成两条链。
// 凸多边形的两条链各自 y 单调不减，因此链内边序列只需单向前进。
func buildChains(poly []geom.Point) [2]chain {
	n := len(poly)
	iMin, iMax := 0, 0
	for i := 1; i < n; i++ {
		if poly[i].Y < poly[iMin].Y {
			iMin = i
		}
		if poly[i].Y > poly[iMax].Y {
			iMax = i
		}
	}
	return [2]chain{makeChain(poly, iMin, iMax, 1), makeChain(poly, iMin, iMax, -1)}
}

// yRange 给出半整数扫描线严格落在 (ymin, ymax) 内的首、末行。
func rowRange(ymin, ymax float64, h int) (int, int) {
	rFirst := int(math.Floor(ymin-0.5)) + 1 // r+1/2 > ymin
	rLast := int(math.Ceil(ymax-0.5)) - 1   // r+1/2 < ymax
	if rFirst < 0 {
		rFirst = 0
	}
	if rLast >= h {
		rLast = h - 1
	}
	return rFirst, rLast
}

// IncrementalIntervals 是带统计的增量版区间计算。
func IncrementalIntervals(poly []geom.Point, w, h int) ([]Span, Counts) {
	c := newCounts(h)
	spans := incremental(poly, w, h, &c)
	return spans, c
}

// IncrementalSpans 是无统计的精简热路径，供计时使用。
func IncrementalSpans(poly []geom.Point, w, h int) []Span {
	return incremental(poly, w, h, nil)
}

// incremental 是两条 y-单调链 + Δ 增量推进的实现本体。
func incremental(poly []geom.Point, w, h int, c *Counts) []Span {
	spans := make([]Span, 0, h)
	if len(poly) < 3 {
		return spans
	}
	ymin, ymax := poly[0].Y, poly[0].Y
	for _, p := range poly[1:] {
		if p.Y < ymin {
			ymin = p.Y
		}
		if p.Y > ymax {
			ymax = p.Y
		}
	}
	rFirst, rLast := rowRange(ymin, ymax, h)
	chains := buildChains(poly)
	for r := rFirst; r <= rLast; r++ {
		y := float64(r) + 0.5
		x0, ok0 := chains[0].update(y, r, c)
		x1, ok1 := chains[1].update(y, r, c)
		if !ok0 || !ok1 {
			continue
		}
		xl, xr := math.Min(x0, x1), math.Max(x0, x1)
		c.spanCovered()
		spans = append(spans, Span{R: r, XL: xl, XR: xr})
	}
	return spans
}

// Incremental 是“增量扫描 + 填充”的完整路径（带统计）。
func Incremental(poly []geom.Point, w, h int) ([]uint8, Counts) {
	c := newCounts(h)
	spans := incremental(poly, w, h, &c)
	pix := make([]uint8, w*h)
	for _, s := range spans {
		paintSpan(pix, w, h, s, &c)
	}
	return pix, c
}

// ---------- 任意简单多边形的正确参照：全交点 + 奇偶配对 ----------

// EvenOdd 收集每行全部交点、排序后两两配对，对凸/非凸简单多边形都正确。
func EvenOdd(poly []geom.Point, w, h int) ([]uint8, Counts) {
	c := newCounts(h)
	pix := make([]uint8, w*h)
	n := len(poly)
	for r := 0; r < h; r++ {
		y := float64(r) + 0.5
		xs := make([]float64, 0, n)
		for i := 0; i < n; i++ {
			a := poly[i]
			b := poly[(i+1)%n]
			c.EdgeTests++
			if (a.Y > y) == (b.Y > y) {
				continue
			}
			xs = append(xs, edgeX(a, b, y, &c))
		}
		sort.Float64s(xs)
		for k := 0; k+1 < len(xs); k += 2 {
			s := Span{R: r, XL: xs[k], XR: xs[k+1]}
			paintSpan(pix, w, h, s, &c)
		}
		if len(xs) >= 2 {
			c.RowsCovered++
		}
	}
	return pix, c
}

// ---------- 反例专用：最朴素的“一次定 Δ，永远 x+=Δ” ----------

// FixedDeltaWrong 只在首行根据当时相交的边确定左右 Δ，之后不再检查、
// 不换边。三角形/平行四边形上它恰好正确；在带侧凹的非凸形状上必错。
func FixedDeltaWrong(poly []geom.Point, w, h int) []uint8 {
	pix := make([]uint8, w*h)
	n := len(poly)
	var xl, xr, dl, dr float64
	primed := false
	for r := 0; r < h; r++ {
		y := float64(r) + 0.5
		type hit struct{ x, d float64 }
		var hits []hit
		for i := 0; i < n; i++ {
			a := poly[i]
			b := poly[(i+1)%n]
			if (a.Y > y) == (b.Y > y) {
				continue
			}
			t := (y - a.Y) / (b.Y - a.Y)
			x := a.X + (b.X-a.X)*t
			d := (b.X - a.X) / (b.Y - a.Y)
			hits = append(hits, hit{x: x, d: d})
		}
		if len(hits) < 2 {
			if primed {
				xl += dl
				xr += dr
				paintSpan(pix, w, h, Span{R: r, XL: xl, XR: xr}, nil)
			}
			continue
		}
		sort.Slice(hits, func(i, j int) bool { return hits[i].x < hits[j].x })
		if !primed {
			xl, xr = hits[0].x, hits[len(hits)-1].x
			dl, dr = hits[0].d, hits[len(hits)-1].d
			primed = true
		} else {
			xl += dl
			xr += dr
		}
		paintSpan(pix, w, h, Span{R: r, XL: xl, XR: xr}, nil)
	}
	return pix
}

// LeftEndpointSeries 返回逐行重算得到的 min 端点序列（只含有交点的行），
// 用来直接展示非凸形状上端点 x 的方向反转。
func LeftEndpointSeries(poly []geom.Point, h int) []float64 {
	var out []float64
	n := len(poly)
	for r := 0; r < h; r++ {
		y := float64(r) + 0.5
		var xl float64
		have := false
		for i := 0; i < n; i++ {
			a := poly[i]
			b := poly[(i+1)%n]
			if (a.Y > y) == (b.Y > y) {
				continue
			}
			t := (y - a.Y) / (b.Y - a.Y)
			x := a.X + (b.X-a.X)*t
			if !have || x < xl {
				xl, have = x, true
			}
		}
		if have {
			out = append(out, xl)
		}
	}
	return out
}

// DiffPixels 返回两块等大缓冲中不同像素的坐标（行,列）。
func DiffPixels(a, b []uint8, w, h int) [][2]int {
	var d [][2]int
	for r := 0; r < h; r++ {
		for c := 0; c < w; c++ {
			if a[r*w+c] != b[r*w+c] {
				d = append(d, [2]int{r, c})
			}
		}
	}
	return d
}

// ASCII 把缓冲渲染成字符画（# 亮，. 暗），便于肉眼核对反例。
func ASCII(pix []uint8, w, h int) string {
	buf := make([]byte, 0, h*(w+1))
	for r := 0; r < h; r++ {
		for c := 0; c < w; c++ {
			if pix[r*w+c] >= 128 {
				buf = append(buf, '#')
			} else {
				buf = append(buf, '.')
			}
		}
		buf = append(buf, '\n')
	}
	return string(buf)
}
