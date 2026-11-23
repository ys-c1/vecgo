// Package mesh 提供三角网格的顶点重排（顶点缓存优化）、
// 缓存未命中模拟与未命中次数的估计式。
//
// 背景：顶点被反复提交时，若相邻三角形引用的顶点编号分散，
// 每次取顶点都落在不同的缓存行上，缓存形同虚设。Reorder 按
// 三角形邻接关系重新编号，使相邻三角形尽量落在同一条缓存行。
package mesh

import (
	"fmt"

	"vecgo/geom"
)

// Mesh 是索引三角网格：V 为顶点表，T 中每个三角形是三个顶点下标。
type Mesh struct {
	V []geom.Point
	T [][3]int
}

// Validate 校验网格，拦下三种错误并点出出错索引：
// 空网格、三角形索引越界、同一三角形内顶点重复。
func Validate(m Mesh) error {
	if len(m.V) == 0 || len(m.T) == 0 {
		return fmt.Errorf("空网格: 顶点 %d 个, 三角形 %d 个", len(m.V), len(m.T))
	}
	for ti, tri := range m.T {
		for slot, idx := range tri {
			if idx < 0 || idx >= len(m.V) {
				return fmt.Errorf("三角形 %d 第 %d 个顶点索引越界: %d (顶点数 %d)", ti, slot, idx, len(m.V))
			}
		}
		if tri[0] == tri[1] || tri[1] == tri[2] || tri[0] == tri[2] {
			return fmt.Errorf("三角形 %d 顶点重复: 索引 %d", ti, tri[1])
		}
	}
	return nil
}

// Reorder 按三角形邻接关系（共享顶点即相邻）对三角形做广度优先遍历，
// 顶点按首次被引用的顺序重新编号，三角形按遍历顺序重排，
// 使相邻三角形的顶点尽量落在同一条缓存行。
// 输入网格不被修改；未被任何三角形引用的顶点原样保留在末尾。
// 重排只改编号与顺序，不改变几何，绘制结果与原顺序逐字节一致。
func Reorder(m Mesh) (Mesh, error) {
	if err := Validate(m); err != nil {
		return Mesh{}, err
	}
	adj := make([][]int, len(m.V))
	for ti, tri := range m.T {
		for _, v := range tri {
			adj[v] = append(adj[v], ti)
		}
	}
	visited := make([]bool, len(m.T))
	order := make([]int, 0, len(m.T))
	var queue []int
	for seed := 0; seed < len(m.T); seed++ {
		if visited[seed] {
			continue
		}
		visited[seed] = true
		queue = append(queue, seed)
		for len(queue) > 0 {
			ti := queue[0]
			queue = queue[1:]
			order = append(order, ti)
			for _, v := range m.T[ti] {
				for _, nb := range adj[v] {
					if !visited[nb] {
						visited[nb] = true
						queue = append(queue, nb)
					}
				}
			}
		}
	}
	newID := make([]int, len(m.V))
	for i := range newID {
		newID[i] = -1
	}
	next := 0
	out := Mesh{T: make([][3]int, 0, len(m.T))}
	for _, ti := range order {
		var tri [3]int
		for slot, v := range m.T[ti] {
			if newID[v] < 0 {
				newID[v] = next
				next++
			}
			tri[slot] = newID[v]
		}
		out.T = append(out.T, tri)
	}
	for v := range m.V {
		if newID[v] < 0 {
			newID[v] = next
			next++
		}
	}
	out.V = make([]geom.Point, len(m.V))
	for v, p := range m.V {
		out.V[newID[v]] = p
	}
	return out, nil
}

// Polygons 把网格按当前三角形顺序展开为多边形列表，可直接交给 pipe.Run 绘制。
// 顶点顺序接口保持不变：不重排时调用本函数即得原有顺序的绘制输入。
func Polygons(m Mesh) [][]geom.Point {
	out := make([][]geom.Point, len(m.T))
	for i, tri := range m.T {
		out[i] = []geom.Point{m.V[tri[0]], m.V[tri[1]], m.V[tri[2]]}
	}
	return out
}

// Cache 是顶点缓存模型：Lines 条缓存行，每行容纳 LineVerts 个连续编号的
// 顶点，LRU 置换。顶点 v 落在第 v/LineVerts 条缓存行。
type Cache struct {
	Lines     int
	LineVerts int
}

func NewCache(lines, lineVerts int) (Cache, error) {
	if lines < 1 || lineVerts < 1 {
		return Cache{}, fmt.Errorf("缓存参数必须为正: lines=%d lineVerts=%d", lines, lineVerts)
	}
	return Cache{Lines: lines, LineVerts: lineVerts}, nil
}

func (c Cache) lineOf(v int) int { return v / c.LineVerts }

// Misses 模拟按顺序提交三角形顶点时的缓存未命中次数（LRU，实测值）。
func (c Cache) Misses(tris [][3]int) int {
	in := make(map[int]bool, c.Lines)
	order := make([]int, 0, c.Lines+1) // 队首为最近使用
	miss := 0
	touch := func(line int) {
		if !in[line] {
			miss++
			in[line] = true
			order = append(order, line)
			if len(order) > c.Lines {
				delete(in, order[0])
				order = order[1:]
			}
			return
		}
		for i, l := range order {
			if l == line {
				copy(order[i:], order[i+1:])
				order[len(order)-1] = line
				break
			}
		}
	}
	for _, tri := range tris {
		for _, v := range tri {
			touch(c.lineOf(v))
		}
	}
	return miss
}

// Estimate 是未命中次数的估计式：被引用的不同缓存行数。
// 每条被引用的缓存行至少要装入一次，故 E ≤ 实测值。
// 重排后被引用的 U 个顶点编号为 0..U-1，估计式即 E = ceil(U / LineVerts)。
func (c Cache) Estimate(m Mesh) int {
	seen := make(map[int]bool)
	for _, tri := range m.T {
		for _, v := range tri {
			seen[c.lineOf(v)] = true
		}
	}
	return len(seen)
}

// MissBound 给出实测未命中次数的上界，用于约束实测与估计的偏差：
// 把缓存行引用序列划分为“相位”（每个相位是含不超过 Lines 个不同
// 缓存行的极大区间），LRU 在每个相位内至多未命中 Lines 次，
// 故实测 M ≤ Lines × 相位个数，偏差 M - Estimate ≤ MissBound - Estimate。
func (c Cache) MissBound(tris [][3]int) int {
	phases := 0
	distinct := make(map[int]bool)
	for _, tri := range tris {
		for _, v := range tri {
			l := c.lineOf(v)
			if !distinct[l] && len(distinct) == c.Lines {
				phases++
				distinct = make(map[int]bool)
			}
			distinct[l] = true
		}
	}
	if len(distinct) > 0 {
		phases++
	}
	return phases * c.Lines
}
