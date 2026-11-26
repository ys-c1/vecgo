package geom

import "sort"

// Cross 返回向量 (a-o)×(b-o) 的 z 分量。
//
//	>0：a→b 左转，b 在有向边 o→a 的左侧（CCW 凸包的内侧）
//	=0：三点共线
//	<0：右转，b 在右侧（外侧）
//
// 可见边判定完全依赖这个符号：对 CCW 凸包 H 的边 v_i→v_{i+1}，
// cross(v_i, v_{i+1}, p) < 0 当且仅当 p 在该边的外侧半平面，即该边对 p 可见。
func Cross(o, a, b Point) float64 {
	return (a.X-o.X)*(b.Y-o.Y) - (a.Y-o.Y)*(b.X-o.X)
}

// Hull 是全量凸包：Andrew 单调链算法。
// 返回从字典序最小点开始、按 CCW 排列的凸包顶点；
// 去重，并丢弃共线边中间的点（与增量维护采用同一约定：叉积为 0 的边不算可见）。
func Hull(pts []Point) []Point {
	if len(pts) <= 1 {
		return append([]Point{}, pts...)
	}
	sorted := append([]Point{}, pts...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].X != sorted[j].X {
			return sorted[i].X < sorted[j].X
		}
		return sorted[i].Y < sorted[j].Y
	})
	uniq := make([]Point, 0, len(sorted))
	for _, q := range sorted {
		if len(uniq) > 0 && q == uniq[len(uniq)-1] {
			continue
		}
		uniq = append(uniq, q)
	}
	if len(uniq) <= 2 {
		return uniq
	}

	lower := make([]Point, 0, len(uniq))
	for _, p := range uniq {
		// <= 0：弹出右转点与共线中间点，只保留极点。
		for len(lower) >= 2 && Cross(lower[len(lower)-2], lower[len(lower)-1], p) <= 0 {
			lower = lower[:len(lower)-1]
		}
		lower = append(lower, p)
	}
	upper := make([]Point, 0, len(uniq))
	for i := len(uniq) - 1; i >= 0; i-- {
		p := uniq[i]
		for len(upper) >= 2 && Cross(upper[len(upper)-2], upper[len(upper)-1], p) <= 0 {
			upper = upper[:len(upper)-1]
		}
		upper = append(upper, p)
	}
	// 上下链各自的首尾点重复，去掉端点后拼接。
	return append(lower[:len(lower)-1], upper[:len(upper)-1]...)
}

// IncrementalHull 维护动态点集 S 的凸包，支持逐个加入顶点。
// 不变量：h 始终是 hull(S) 的 CCW 顶点序列，且从字典序最小点开始，
// 因此与 Hull 的输出可以逐点比较。
type IncrementalHull struct {
	h []Point
}

// NewIncrementalHull 用初始点集的全量凸包打底，之后靠 Add 增量维护。
func NewIncrementalHull(seed []Point) *IncrementalHull {
	return &IncrementalHull{h: Hull(seed)}
}

// Points 返回当前凸包顶点的拷贝。
func (ih *IncrementalHull) Points() []Point {
	return append([]Point{}, ih.h...)
}

// Add 把 p 动态加入点集并增量更新凸包。
//
// 正确性依据（充要条件）：被删除的边必须恰好是可见链
// {e_i : cross(v_i,v_{i+1},p) < 0}，再用两条切边 v_s→p、p→v_{t+1} 替换。
// 叉积为 0 的共线边仅当夹在可见链中间时一并移除（p 落在边的延长线上），
// p 落在边段上时没有任何严格可见边，直接忽略，与全量算法丢弃共线点的约定一致。
func (ih *IncrementalHull) Add(p Point) {
	h := ih.h
	switch len(h) {
	case 0:
		ih.h = []Point{p}
		return
	case 1:
		if h[0] != p {
			ih.h = Hull([]Point{h[0], p})
		}
		return
	case 2:
		// 三点以下直接全量重算，代价为常数；同时统一共线/重复点约定。
		ih.h = Hull([]Point{h[0], h[1], p})
		return
	}

	n := len(h)
	c := make([]float64, n)
	first := -1
	for i := 0; i < n; i++ {
		c[i] = Cross(h[i], h[(i+1)%n], p)
		if c[i] < 0 && first < 0 {
			first = i
		}
	}
	if first < 0 {
		// 所有边叉积 >= 0：p 在凸包内部或某条边段上，凸包不变。
		return
	}

	// 可见边在循环下标上是一个连续区间。从任一可见边出发向两侧扩展，
	// 叉积 <= 0 的共线边并入链中（处理 p 在边延长线上的情形）。
	s := first
	for steps := 0; steps < n && c[(s-1+n)%n] <= 0; steps++ {
		s = (s - 1 + n) % n
	}
	t := first
	for steps := 0; steps < n && c[(t+1)%n] <= 0; steps++ {
		t = (t + 1) % n
	}

	// 保留链首切点 h[s]，删除 h[s+1..t]，插入 p，再接回链尾切点 h[t+1..]。
	// 关键：两个切点都必须保留，切片错位一个就会漏掉极点。
	out := make([]Point, 0, n+1)
	out = append(out, h[s])
	out = append(out, p)
	for k := (t + 1) % n; k != s; k = (k + 1) % n {
		out = append(out, h[k])
	}
	ih.h = rotateToLexMin(out)
}

// rotateToLexMin 把循环序列旋转到字典序最小点开头，与 Hull 输出对齐。
func rotateToLexMin(poly []Point) []Point {
	k := 0
	for i := 1; i < len(poly); i++ {
		if poly[i].X < poly[k].X || (poly[i].X == poly[k].X && poly[i].Y < poly[k].Y) {
			k = i
		}
	}
	out := make([]Point, 0, len(poly))
	out = append(out, poly[k:]...)
	out = append(out, poly[:k]...)
	return out
}
