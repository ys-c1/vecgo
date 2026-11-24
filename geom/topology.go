package geom

import (
	"fmt"
	"math"
	"strings"
)

// topoEps 是拓扑判定使用的相对容差。
//
// 判定按坐标量级缩放：坐标量级为 s 时，距离容差取 topoEps*s，
// 叉积（面积）容差取 topoEps*s*s。这样裁剪交点因浮点舍入产生的
// “数学同一点、末位差几 ulp”能归并，而不同顶点不会被误并。
const topoEps = 1e-9

// EulerCounts 是一个环的欧拉计数，约定见 docs/clip-euler-invariant.md：
//
//	V：环上顶点表的长度（重复坐标按多次计，因为 SH 输出里重复顶点
//	   本来就是两次“顶点事件”，这正是要暴露的膨胀来源）；
//	E：不同无向边的条数，零长边不计、同一条边只计一次；
//	F：平面图的 face 数 = 1 个无界面 + 有界面数；零面积环只有无界面。
//
// 不变量是 V - E + F = 2。边界情况：
//   - 简单环：恒有 V=E，F=2，Δ=0；
//   - 相邻重复顶点：V 增 1，E 不变（零长边被排除），Δ=+1；
//   - 单点环：V=1,E=0,F=1，Δ=0；线段环：V=2,E=1,F=1，Δ=0。
//     两者都满足公式却无面积，需另看 HasArea；
//   - 共线中间点：V、E 同时 +1，Δ 仍为 0，必须看 CleanResult.Removed
//     才能发现——这是“欧拉公式自洽 ≠ 无退化”的边界；
//   - 真自交/捏点/T 型接触：Δ 可能仍为 0，由 Crossings/Repeated/
//     TJunctions/Overlaps 单独报出。
type EulerCounts struct {
	V, E, F    int
	Chi        int // V - E + F
	Want       int // 2
	Delta      int // Chi - Want
	Crossings  int // 非相邻边的真交叉数
	Repeated   int // 非相邻重复坐标的重复次数
	TJunctions int // 顶点落在非相邻边内部的次数（T 型接触）
	Overlaps   int // 非相邻边共线重叠的次数
}

func (c EulerCounts) String() string {
	return fmt.Sprintf("V=%d E=%d F=%d χ=%d (期望 %d, 差 %+d)", c.V, c.E, c.F, c.Chi, c.Want, c.Delta)
}

// Applicable 表示不变量 V-E+F=2 对该计数是否有意义。空环没有顶点，
// 不构成平面图，此时返回 false（空结果按“无覆盖”单独报告）。
func (c EulerCounts) Applicable() bool { return c.V > 0 }

// EulerSatisfied 表示欧拉计数自洽，且不存在真自交或非相邻重复（捏点）。
//
// 注意这是“必要不充分”的判据：
//   - 共线冗余点让 V、E 同时 +1，Δ 仍为 0，需看 CleanResult.Removed；
//   - 单点环 V=1,E=0,F=1 与线段环 V=2,E=1,F=1 也都满足 χ=2，
//     但它们不是有效面，需看 ClipDiagnosis.HasArea / Residual；
//   - T 型接触、非相邻边重叠不改变 χ，但已不是简单环，需看对应计数。
func (c EulerCounts) EulerSatisfied() bool {
	return c.Delta == 0 && c.Crossings == 0 && c.Repeated == 0 &&
		c.TJunctions == 0 && c.Overlaps == 0
}

// HasTopologicalDefect 表示残留在清理后、会破坏简单环性质的问题
// （捏点或真自交）。单点/线段/空环属于“无面积”，不算拓扑缺陷。
func HasTopologicalDefect(degs []DegenerateVertex) bool {
	for _, d := range degs {
		switch d.Kind {
		case KindPinch, KindSelfCross, KindTJunction, KindOverlapEdge:
			return true
		}
	}
	return false
}

// DegenerateKind 是退化顶点的类别。
type DegenerateKind string

const (
	KindDuplicate   DegenerateKind = "相邻重复顶点"
	KindCollinear   DegenerateKind = "共线中间顶点"
	KindSpike       DegenerateKind = "折返尖刺顶点"
	KindZeroArea    DegenerateKind = "零面积环冗余顶点"
	KindPinch       DegenerateKind = "非相邻重复（捏点）"
	KindSelfCross   DegenerateKind = "自交顶点"
	KindTJunction   DegenerateKind = "T 型接触顶点"
	KindOverlapEdge DegenerateKind = "共线重叠边"
	KindEmpty       DegenerateKind = "空环"
	KindPointRing   DegenerateKind = "退化环（单点）"
	KindSegmentRing DegenerateKind = "退化环（线段）"
)

// DegenerateVertex 记录一个退化顶点：清理前的下标、坐标与类别。
type DegenerateVertex struct {
	Index int
	P     Point
	Kind  DegenerateKind
}

func (d DegenerateVertex) String() string {
	return fmt.Sprintf("[%d]%s(%.9g,%.9g)", d.Index, d.Kind, d.P.X, d.P.Y)
}

// CleanResult 是清环结果。
//
// Removed 是本次被合并/删除的退化顶点，即“退化顶点数”；
// Residual 是清理后仍存在的、无法靠合并相邻顶点消除的退化
// （空环、单点/线段环、捏点、真自交）。
type CleanResult struct {
	Ring     []Point
	Removed  []DegenerateVertex
	Residual []DegenerateVertex
}

func samePoint(a, b Point) bool {
	s := 1.0
	for _, v := range [...]float64{a.X, a.Y, b.X, b.Y} {
		if av := math.Abs(v); av > s {
			s = av
		}
	}
	tol := topoEps * s
	return math.Abs(a.X-b.X) <= tol && math.Abs(a.Y-b.Y) <= tol
}

// SignedArea 返回环的有向面积；逆时针为正。
func SignedArea(poly []Point) float64 {
	if len(poly) < 3 {
		return 0
	}
	s := 0.0
	for i := range poly {
		j := (i + 1) % len(poly)
		s += poly[i].X*poly[j].Y - poly[j].X*poly[i].Y
	}
	return s / 2
}

func coordScale(poly []Point) float64 {
	s := 1.0
	for _, p := range poly {
		if v := math.Abs(p.X); v > s {
			s = v
		}
		if v := math.Abs(p.Y); v > s {
			s = v
		}
	}
	return s
}

func areaTol(poly []Point) float64 {
	s := coordScale(poly)
	return topoEps * s * s
}

func zeroArea(poly []Point) bool {
	return math.Abs(SignedArea(poly)) <= areaTol(poly)
}

// collinearBetween 判断 b 是否为直边 a—c 上的冗余中间点。
// 只认“落在 a、c 之间”，不认 180° 折返（尖刺），后者交给 spike 环节。
func collinearBetween(a, b, c Point) bool {
	s := coordScale([]Point{a, b, c})
	cr := (b.X-a.X)*(c.Y-a.Y) - (b.Y-a.Y)*(c.X-a.X)
	if math.Abs(cr) > topoEps*s*s {
		return false
	}
	dot := (b.X-a.X)*(c.X-b.X) + (b.Y-a.Y)*(c.Y-b.Y)
	return dot >= 0
}

// spikeAt 判断 b 是否为 a→b→c 的 180° 折返点：a、b、c 共线，
// 且 b 不在 a、c 之间（走 a→b 又原路退回）。这种点不围面积，
// 是 SH 在共线贴边时产生的“尖刺”，必须连同 b 一起删除。
func spikeAt(a, b, c Point) bool {
	s := coordScale([]Point{a, b, c})
	cr := (b.X-a.X)*(c.Y-a.Y) - (b.Y-a.Y)*(c.X-a.X)
	if math.Abs(cr) > topoEps*s*s {
		return false
	}
	// a、b、c 共线且 b 不在 a、c 之间 ⇔ (b-a)·(c-b) < 0。
	dot := (b.X-a.X)*(c.X-b.X) + (b.Y-a.Y)*(c.Y-b.Y)
	return dot < 0
}

// uniqueIDs 把容差内相同的坐标映射到同一个 id（按首次出现顺序编号），
// 返回每个顶点的 id、不同坐标数，以及 id 对应的代表坐标。
func uniqueIDs(poly []Point) (ids []int, reps []Point) {
	ids = make([]int, len(poly))
	for i, p := range poly {
		id := -1
		for k, q := range reps {
			if samePoint(p, q) {
				id = k
				break
			}
		}
		if id < 0 {
			id = len(reps)
			reps = append(reps, p)
		}
		ids[i] = id
	}
	return ids, reps
}

// Edges 返回环上不同无向边的条数（零长边不计）。
func Edges(poly []Point) int {
	ids, _ := uniqueIDs(poly)
	return distinctEdges(ids)
}

func distinctEdges(ids []int) int {
	n := len(ids)
	if n < 2 {
		return 0
	}
	type key [2]int
	seen := make(map[key]struct{}, n)
	for i := 0; i < n; i++ {
		a, b := ids[i], ids[(i+1)%n]
		if a == b {
			continue
		}
		if a > b {
			a, b = b, a
		}
		seen[key{a, b}] = struct{}{}
	}
	return len(seen)
}

func orient(a, b, c Point) int {
	s := coordScale([]Point{a, b, c})
	v := (b.X-a.X)*(c.Y-a.Y) - (b.Y-a.Y)*(c.X-a.X)
	switch {
	case v > topoEps*s*s:
		return 1
	case v < -topoEps*s*s:
		return -1
	}
	return 0
}

func properIntersect(a, b, c, d Point) bool {
	o1, o2 := orient(a, b, c), orient(a, b, d)
	o3, o4 := orient(c, d, a), orient(c, d, b)
	return o1*o2 < 0 && o3*o4 < 0
}

// onSegmentInterior 判断 p 是否落在 a—b 的内部（不含两端）。
func onSegmentInterior(a, b, p Point) bool {
	if samePoint(p, a) || samePoint(p, b) {
		return false
	}
	if orient(a, b, p) != 0 {
		return false
	}
	s := coordScale([]Point{a, b, p})
	tol := topoEps * s
	return p.X >= math.Min(a.X, b.X)-tol && p.X <= math.Max(a.X, b.X)+tol &&
		p.Y >= math.Min(a.Y, b.Y)-tol && p.Y <= math.Max(a.Y, b.Y)+tol
}

// collinearOverlap 判断两条非相邻边共线且有长度大于容差的重叠。
func collinearOverlap(a, b, c, d Point) bool {
	if orient(a, b, c) != 0 || orient(a, b, d) != 0 {
		return false
	}
	s := coordScale([]Point{a, b, c, d})
	tol := topoEps * s
	// 投影到较长轴。
	useX := math.Abs(b.X-a.X) >= math.Abs(b.Y-a.Y)
	get := func(p Point) float64 {
		if useX {
			return p.X
		}
		return p.Y
	}
	lo := math.Max(math.Min(get(a), get(b)), math.Min(get(c), get(d)))
	hi := math.Min(math.Max(get(a), get(b)), math.Max(get(c), get(d)))
	return hi-lo > tol
}

// contactDefects 统计 T 型接触与非相邻边共线重叠。
func contactDefects(poly []Point, ids []int) (tjunct, overlaps int) {
	n := len(poly)
	if n < 3 {
		return 0, 0
	}
	for i := 0; i < n; i++ {
		i2 := (i + 1) % n
		for j := 0; j < n; j++ {
			j2 := (j + 1) % n
			if adjacencySkip(n, j, i) {
				continue
			}
			// 顶点 poly[j] 落在非相邻边 i—i2 内部。
			if ids[j] != ids[i] && ids[j] != ids[i2] && onSegmentInterior(poly[i], poly[i2], poly[j]) {
				tjunct++
			}
			// 只对 j > i 判一次重叠。
			if j > i && !adjacencySkip(n, i, j) && collinearOverlap(poly[i], poly[i2], poly[j], poly[j2]) {
				overlaps++
			}
		}
	}
	return tjunct, overlaps
}

func adjacencySkip(n, i, j int) bool {
	// 边 i—i+1 与边 j—j+1 共享端点时不是真交叉。
	i2, j2 := (i+1)%n, (j+1)%n
	return i == j || i2 == j || j2 == i
}

func countCrossings(poly []Point, ids []int) int {
	n := len(poly)
	if n < 4 {
		return 0
	}
	c := 0
	for i := 0; i < n; i++ {
		i2 := (i + 1) % n
		for j := i + 1; j < n; j++ {
			if adjacencySkip(n, i, j) {
				continue
			}
			j2 := (j + 1) % n
			if ids[i] == ids[j] || ids[i] == ids[j2] || ids[i2] == ids[j] || ids[i2] == ids[j2] {
				continue // 共享同一坐标，不是真交叉
			}
			if properIntersect(poly[i], poly[i2], poly[j], poly[j2]) {
				c++
			}
		}
	}
	return c
}

// repeatedNonAdjacent 统计非相邻位置的重复坐标次数（首尾相邻不算）。
func repeatedNonAdjacent(ids []int) int {
	n := len(ids)
	if n == 0 {
		return 0
	}
	first := make(map[int]int, n)
	extra := 0
	for i, id := range ids {
		j, ok := first[id]
		if !ok {
			first[id] = i
			continue
		}
		if i == j+1 || (j == 0 && i == n-1) {
			continue
		}
		extra++
	}
	return extra
}

// boundedLoops 用栈把闭合走线拆成若干带回路的段，统计非零面积回路数。
// 用于自接触（捏点）环的面数：捏点处把走线剖成两个环。
func boundedLoops(reps []Point, ids []int) int {
	if len(ids) == 0 {
		return 0
	}
	stack := make([]int, 0, len(ids))
	pos := make(map[int]int, len(ids))
	loops := 0
	for k := 0; k <= len(ids); k++ {
		id := ids[k%len(ids)]
		if at, ok := pos[id]; ok {
			pts := make([]Point, 0, len(stack)-at)
			for _, sid := range stack[at:] {
				pts = append(pts, reps[sid])
			}
			if len(pts) >= 3 && !zeroArea(pts) {
				loops++
			}
			for m := at + 1; m < len(stack); m++ {
				delete(pos, stack[m])
			}
			stack = stack[:at+1]
			continue
		}
		pos[id] = len(stack)
		stack = append(stack, id)
	}
	return loops
}

// EulerOf 计算一个闭合环的欧拉计数。
//
// 面数约定：非零面积环至少有 1 个有界面；有自接触时按捏点拆分后
// 逐个数有界回路；每个真交叉额外贡献 1 个有界面（交叉点没有计入 V，
// 因此 Δ 会变正——这正是要暴露的“顶点数/边数对不上”）。
func EulerOf(poly []Point) EulerCounts {
	ids, reps := uniqueIDs(poly)
	n := len(poly)
	c := EulerCounts{V: n, Want: 2, Crossings: countCrossings(poly, ids), Repeated: repeatedNonAdjacent(ids)}
	c.TJunctions, c.Overlaps = contactDefects(poly, ids)
	c.E = distinctEdges(ids)
	switch {
	case n == 0:
		c.F = 1 // 只有无界面
	case len(reps) == 1:
		c.F = 1 // 单点环：无界面
	case zeroArea(poly):
		c.F = 1 // 线段/尖刺：无界面
	default:
		loops := 1
		if c.Repeated > 0 {
			loops = boundedLoops(reps, ids)
		}
		c.F = 1 + loops + c.Crossings
	}
	c.Chi = c.V - c.E + c.F
	if n == 0 {
		// 空环不构成平面图：Δ 无意义，记为 0，由 Applicable() 判定。
		c.Chi, c.Delta = 0, 0
		return c
	}
	c.Delta = c.Chi - c.Want
	return c
}

// CleanPolygon 迭代删除退化顶点：
//
//  1. 合并相邻重复点（含首尾相接）；
//  2. 删除共线中间点；
//  3. 零面积环收缩为线段（保留相距最远两点）或单点。
//
// 非相邻重复与真自交无法靠合并相邻顶点消除，保留在 Residual。
// 合并/删除的每个顶点都记入 Removed，保留原始下标与坐标。
func CleanPolygon(poly []Point) CleanResult {
	out := append([]Point{}, poly...)
	idx := make([]int, len(poly))
	for i := range idx {
		idx[i] = i
	}
	res := CleanResult{}

	// 1+2. 反复删除相邻重复点、共线中间点与折返尖刺，直到不再变化。
	//
	// 必须合并成同一个不动点循环：删除一个共线/尖刺顶点后，它两侧的
	// 点可能变成新的相邻重复（例如 {7,3} X {7,3} 删掉 X 后就重复了），
	// 若步骤分两轮顺序执行就漏掉这类二次退化。
	for {
		n := len(out)
		changed := false

		// 相邻重复点（含首尾相接）。
		if n > 1 && samePoint(out[0], out[n-1]) {
			res.Removed = append(res.Removed, DegenerateVertex{idx[n-1], out[n-1], KindDuplicate})
			out, idx = out[:n-1], idx[:n-1]
			changed = true
			continue
		}
		for i := 0; i+1 < len(out); i++ {
			if samePoint(out[i], out[i+1]) {
				res.Removed = append(res.Removed, DegenerateVertex{idx[i+1], out[i+1], KindDuplicate})
				out = append(out[:i+1], out[i+2:]...)
				idx = append(idx[:i+1], idx[i+2:]...)
				changed = true
				break
			}
		}
		if changed {
			continue
		}

		// 共线中间点与折返尖刺。
		if len(out) > 2 {
			m := len(out)
			for i := 0; i < m; i++ {
				a, b, c := out[(i-1+m)%m], out[i], out[(i+1)%m]
				if samePoint(a, b) || samePoint(b, c) {
					continue
				}
				switch {
				case collinearBetween(a, b, c):
					res.Removed = append(res.Removed, DegenerateVertex{idx[i], out[i], KindCollinear})
				case spikeAt(a, b, c):
					res.Removed = append(res.Removed, DegenerateVertex{idx[i], out[i], KindSpike})
				default:
					continue
				}
				out = append(out[:i], out[i+1:]...)
				idx = append(idx[:i], idx[i+1:]...)
				changed = true
				break
			}
		}
		if !changed {
			break
		}
	}

	// 3. 零面积环。
	if len(out) >= 3 && zeroArea(out) {
		bi, bj, maxd := 0, 1, -1.0
		for i := 0; i < len(out); i++ {
			for j := i + 1; j < len(out); j++ {
				d := math.Hypot(out[j].X-out[i].X, out[j].Y-out[i].Y)
				if d > maxd {
					maxd, bi, bj = d, i, j
				}
			}
		}
		var keep []Point
		var keepIdx []int
		if bi > bj {
			bi, bj = bj, bi
		}
		if maxd <= topoEps*coordScale(out) {
			keep, keepIdx = []Point{out[0]}, []int{idx[0]}
		} else {
			keep, keepIdx = []Point{out[bi], out[bj]}, []int{idx[bi], idx[bj]}
		}
		for i, p := range out {
			if (len(keep) == 1 && i == 0) || (len(keep) == 2 && (i == bi || i == bj)) {
				continue
			}
			res.Removed = append(res.Removed, DegenerateVertex{idx[i], p, KindZeroArea})
		}
		out, idx = keep, keepIdx
	}

	res.Ring = out
	res.Residual = residualDefects(out)
	return res
}

// residualDefects 报告清理后仍无法消除的退化。
func residualDefects(poly []Point) []DegenerateVertex {
	ids, reps := uniqueIDs(poly)
	var out []DegenerateVertex
	switch {
	case len(poly) == 0:
		return []DegenerateVertex{{Index: -1, Kind: KindEmpty}}
	case len(reps) == 1:
		out = append(out, DegenerateVertex{Index: 0, P: poly[0], Kind: KindPointRing})
	case len(reps) == 2:
		out = append(out, DegenerateVertex{Index: 0, P: poly[0], Kind: KindSegmentRing})
	}
	first := make(map[int]int, len(ids))
	for i, id := range ids {
		j, ok := first[id]
		if !ok {
			first[id] = i
			continue
		}
		n := len(ids)
		if i == j+1 || (j == 0 && i == n-1) {
			continue
		}
		out = append(out, DegenerateVertex{Index: i, P: poly[i], Kind: KindPinch})
	}
	for i := 0; i < len(poly); i++ {
		i2 := (i + 1) % len(poly)
		for j := i + 1; j < len(poly); j++ {
			if adjacencySkip(len(poly), i, j) {
				continue
			}
			j2 := (j + 1) % len(poly)
			if ids[i] == ids[j] || ids[i] == ids[j2] || ids[i2] == ids[j] || ids[i2] == ids[j2] {
				continue
			}
			if properIntersect(poly[i], poly[i2], poly[j], poly[j2]) {
				out = append(out, DegenerateVertex{Index: i, P: poly[i], Kind: KindSelfCross})
			}
		}
	}
	// T 型接触：顶点落在非相邻边内部，是“顶点与边”的退化接触。
	seenT := make(map[int]bool, len(poly))
	for i := 0; i < len(poly); i++ {
		i2 := (i + 1) % len(poly)
		for j := 0; j < len(poly); j++ {
			if adjacencySkip(len(poly), j, i) || ids[j] == ids[i] || ids[j] == ids[i2] {
				continue
			}
			if onSegmentInterior(poly[i], poly[i2], poly[j]) && !seenT[j] {
				seenT[j] = true
				out = append(out, DegenerateVertex{Index: j, P: poly[j], Kind: KindTJunction})
			}
		}
	}
	// 非相邻边共线重叠：两条边共享一段正长度，仍不是简单环。
	for i := 0; i < len(poly); i++ {
		i2 := (i + 1) % len(poly)
		for j := i + 1; j < len(poly); j++ {
			if adjacencySkip(len(poly), i, j) {
				continue
			}
			j2 := (j + 1) % len(poly)
			if collinearOverlap(poly[i], poly[i2], poly[j], poly[j2]) {
				out = append(out, DegenerateVertex{Index: i, P: poly[i], Kind: KindOverlapEdge})
			}
		}
	}
	return out
}

// IsConvex 判断环是否为逆时针或顺时针的严格凸环（无共线折返）。
// Sutherland–Hodgman 只对凸多边形主体是正确的；凹多边形会被裁成
// 带桥接的环，出现捏点/T 型接触，这是算法前提被违反，不是清理缺陷。
func IsConvex(poly []Point) bool {
	n := len(poly)
	if n < 3 {
		return false
	}
	sign := 0
	for i := 0; i < n; i++ {
		o := orient(poly[(i-1+n)%n], poly[i], poly[(i+1)%n])
		if o == 0 {
			continue // 共线三点不改变凸性
		}
		if sign == 0 {
			sign = o
		} else if o != sign {
			return false
		}
	}
	return sign != 0
}
func KindCounts(degs []DegenerateVertex) map[DegenerateKind]int {
	m := make(map[DegenerateKind]int, len(degs))
	for _, d := range degs {
		m[d.Kind]++
	}
	return m
}

// Explain 用一句话说明欧拉计数是否满足、差多少、差在哪一类。
func Explain(c EulerCounts, degs []DegenerateVertex) string {
	if c.EulerSatisfied() && !HasTopologicalDefect(degs) {
		if len(degs) == 0 {
			return "满足 V-E+F=2；无退化顶点"
		}
		return fmt.Sprintf("满足 V-E+F=2；另有 %d 个退化顶点计入统计", len(degs))
	}
	parts := make([]string, 0, 4)
	for _, k := range sortedKinds(degs) {
		parts = append(parts, fmt.Sprintf("%s×%d", k, KindCounts(degs)[k]))
	}
	detail := "无已列出的退化顶点"
	if len(parts) > 0 {
		detail = strings.Join(parts, "、")
	}
	reasons := make([]string, 0, 3)
	if c.Delta != 0 {
		reasons = append(reasons, fmt.Sprintf("欧拉差 %+d", c.Delta))
	}
	if c.Crossings != 0 {
		reasons = append(reasons, fmt.Sprintf("真自交 %d 处", c.Crossings))
	}
	if c.Repeated != 0 {
		reasons = append(reasons, fmt.Sprintf("捏点重复 %d 处", c.Repeated))
	}
	if c.TJunctions != 0 {
		reasons = append(reasons, fmt.Sprintf("T 型接触 %d 处", c.TJunctions))
	}
	if c.Overlaps != 0 {
		reasons = append(reasons, fmt.Sprintf("非相邻边重叠 %d 处", c.Overlaps))
	}
	if HasTopologicalDefect(degs) {
		reasons = append(reasons, "清理后仍有拓扑缺陷")
	}
	return fmt.Sprintf("不满足 V-E+F=2（%s）；类别: %s", strings.Join(reasons, "，"), detail)
}

// Coords 取前 limit 个退化顶点的坐标文本，最多 limit 个。
func Coords(degs []DegenerateVertex, limit int) string {
	if len(degs) == 0 {
		return "无"
	}
	if limit <= 0 || limit > len(degs) {
		limit = len(degs)
	}
	parts := make([]string, 0, limit)
	for _, d := range degs[:limit] {
		parts = append(parts, fmt.Sprintf("(%.9g,%.9g)", d.P.X, d.P.Y))
	}
	if limit < len(degs) {
		return fmt.Sprintf("%s …（共 %d 个，仅列前 %d）", strings.Join(parts, " "), len(degs), limit)
	}
	return strings.Join(parts, " ")
}

func sortedKinds(degs []DegenerateVertex) []DegenerateKind {
	counts := KindCounts(degs)
	kinds := make([]DegenerateKind, 0, len(counts))
	for k := range counts {
		kinds = append(kinds, k)
	}
	for i := 1; i < len(kinds); i++ {
		for j := i; j > 0 && kinds[j] < kinds[j-1]; j-- {
			kinds[j], kinds[j-1] = kinds[j-1], kinds[j]
		}
	}
	return kinds
}
