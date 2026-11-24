// Package scan 在整数格点上做奇偶规则（even-odd）扫描线填充，并保证
// 同一个多边形沿正、反两个方向遍历时得到逐像素相同的结果。
//
// 格点约定：单元 (col,row) 覆盖 [col,col+1]×[row,row+1]，采样点取单元
// 中心 (col+1/2,row+1/2)。为了只用整数算术，所有坐标放大两倍：顶点落到
// 偶格点 (2x,2y)，采样中心落到奇格点 (2col+1,2row+1)。采样中心的坐标
// 是奇数，顶点的坐标是偶数，所以采样点永远不会落在顶点上；扫描线只剩
// 一种退化情形——采样点恰好落在某条边的内部。
//
// 翻转规则见 docs/boundary-flip-rule.md：
//
//	基准位 b(a→b) = [dy > 0]（把 +y 定为屏幕向下）
//	补翻转 b*(a→b) = b(a→b) ⊕ [a ≻ b]（≻ 为 (x,y) 字典序）
//
// 补翻转只在构造有向边时触发一次（Edge.Flip），运行时只在“采样点恰在
// 边上”的分支使用它。补与不补的取舍、以及守恒条件，文档里有推导。
package scan

import (
	"errors"
	"fmt"
	"math"

	"vecgo/geom"
)

// 画幅与坐标的上限：保证两倍坐标与交叉相乘都在 int64 内，且不会因
// 误传超大画幅直接耗尽内存。
const (
	MaxSide  = 8192
	MaxCoord = 1 << 20
)

// Direction 是多边形的遍历方向。
type Direction int

const (
	// Forward 按顶点给出的顺序遍历，即边 i 为 v[i]→v[i+1]。
	Forward Direction = iota
	// Reverse 逆序遍历，即边 i 为 v[i+1]→v[i]。
	Reverse
)

func (d Direction) String() string {
	switch d {
	case Forward:
		return "正向"
	case Reverse:
		return "反向"
	default:
		return "未知"
	}
}

// Cell 是一个像素单元，列为 Col、行为 Row，左上角在 (Col,Row)。
type Cell struct{ Col, Row int }

// Mask 是一次填充的结果，1 表示该单元的中心落在多边形内。
type Mask struct {
	W, H int
	Bits []uint8 // 行优先，长度 W*H，取值 0 或 1
}

// At 返回 (col,row) 的覆盖位；越界返回 0。
func (m *Mask) At(col, row int) uint8 {
	if m == nil || col < 0 || row < 0 || col >= m.W || row >= m.H {
		return 0
	}
	return m.Bits[row*m.W+col]
}

// Count 返回被覆盖的单元数。每个单元面积恰为 1，因此这个整数就是
// 光栅化区域的面积（贴边单元按半开约定整格计入或整格不计）。
func (m *Mask) Count() int {
	n := 0
	for _, v := range m.Bits {
		if v != 0 {
			n++
		}
	}
	return n
}

// Differing 返回两张同尺寸掩码中覆盖位不同的单元，行优先升序。
func Differing(a, b *Mask) []Cell {
	if a == nil || b == nil || a.W != b.W || a.H != b.H {
		return nil
	}
	var out []Cell
	for row := 0; row < a.H; row++ {
		for col := 0; col < a.W; col++ {
			i := row*a.W + col
			if a.Bits[i] != b.Bits[i] {
				out = append(out, Cell{Col: col, Row: row})
			}
		}
	}
	return out
}

// lattice 是放大两倍后的整数坐标。
type lattice struct{ x, y int64 }

// Edge 是多边形的一条有向边，Index 是它在正向顶点序中的下标。
// A、B 是遍历方向上的起点与终点。
type Edge struct {
	Index int
	A, B  geom.Point
	// Flip 表示这条有向边的起点按 (x,y) 字典序大于终点，需要补一次翻转。
	Flip bool

	ax, ay, bx, by int64
}

// Edges 按给定遍历方向构造有向边。边的下标始终对应正向顶点序中的
// v[i]→v[i+1]，反向遍历只改变每条边的朝向，不改变编号。
func Edges(poly []geom.Point, dir Direction) ([]Edge, error) {
	lat, err := latticePoly(poly)
	if err != nil {
		return nil, err
	}
	if dir != Forward && dir != Reverse {
		return nil, errors.New("未知遍历方向")
	}
	n := len(poly)
	out := make([]Edge, n)
	for i := 0; i < n; i++ {
		j := (i + 1) % n
		a, b := lat[i], lat[j]
		e := Edge{Index: i, A: poly[i], B: poly[j]}
		if dir == Reverse {
			a, b = b, a
			e.A, e.B = poly[j], poly[i]
		}
		e.ax, e.ay, e.bx, e.by = a.x, a.y, b.x, b.y
		e.Flip = greater(a, b)
		out[i] = e
	}
	return out, nil
}

// greater 是 (x,y) 字典序比较，坐标已放大两倍，比较结果与原坐标一致。
func greater(a, b lattice) bool {
	if a.x != b.x {
		return a.x > b.x
	}
	return a.y > b.y
}

// bitAt 返回该边对采样点 (px,py) 的奇偶位；px、py 是放大两倍的坐标。
// correct=false 时跳过补翻转，用于复现方向相关的旧行为。
//
// 严格内部按射线法：边跨越 py 且交点位于 px 右侧时翻转一次。
// 采样点恰在边上时，基准位取 [dy>0]，再按 correct 决定是否补翻转。
func (e Edge) bitAt(px, py int64, correct bool) uint8 {
	if (e.ay > py) == (e.by > py) {
		return 0
	}
	dy := e.by - e.ay
	dx := e.bx - e.ax
	lhs := e.ax*dy + dx*(py-e.ay) - px*dy
	if lhs == 0 {
		var b uint8
		if dy > 0 {
			b = 1
		}
		if correct && e.Flip {
			b ^= 1
		}
		return b
	}
	if (lhs > 0) == (dy > 0) {
		return 1
	}
	return 0
}

// Fill 用补翻转规则填充，正反两个方向的掩码逐像素相同。
func Fill(poly []geom.Point, w, h int, dir Direction) (*Mask, error) {
	return fill(poly, w, h, dir, true)
}

// FillUncorrected 省掉补翻转，仅用于复现“同一场景反向遍历像素数不同”
// 的缺陷与诊断；正式渲染请用 Fill。
func FillUncorrected(poly []geom.Point, w, h int, dir Direction) (*Mask, error) {
	return fill(poly, w, h, dir, false)
}

func fill(poly []geom.Point, w, h int, dir Direction, correct bool) (*Mask, error) {
	if w <= 0 || h <= 0 || w > MaxSide || h > MaxSide {
		return nil, fmt.Errorf("画幅必须在 1..%d 内: %dx%d", MaxSide, w, h)
	}
	edges, err := Edges(poly, dir)
	if err != nil {
		return nil, err
	}
	m := &Mask{W: w, H: h, Bits: make([]uint8, w*h)}
	for row := 0; row < h; row++ {
		py := int64(2*row + 1)
		for col := 0; col < w; col++ {
			px := int64(2*col + 1)
			var parity uint8
			for _, e := range edges {
				parity ^= e.bitAt(px, py, correct)
			}
			m.Bits[row*w+col] = parity
		}
	}
	return m, nil
}

// CellDiff 是一个正反遍历覆盖位不同的单元。
type CellDiff struct {
	Col, Row int
	Forward  uint8 // 正向遍历时该单元的覆盖位
	Reverse  uint8 // 反向遍历时该单元的覆盖位
	// Edges 是采样中心恰好落在其上的边，编号为正向顶点序的下标。
	// 差异只可能出现在贴边单元上，所以这里至少有一条边。
	Edges []int
}

// Diagnose 比较正、反两个方向的填充结果，列出所有覆盖位不同的单元，
// 并指出每个单元的采样中心落在哪条边上。correct=false 用来定位缺陷；
// correct=true 时结果应为空。
func Diagnose(poly []geom.Point, w, h int, correct bool) ([]CellDiff, error) {
	fwd, err := fill(poly, w, h, Forward, correct)
	if err != nil {
		return nil, err
	}
	rev, err := fill(poly, w, h, Reverse, correct)
	if err != nil {
		return nil, err
	}
	lat, err := latticePoly(poly)
	if err != nil {
		return nil, err
	}
	var out []CellDiff
	for _, c := range Differing(fwd, rev) {
		i := c.Row*w + c.Col
		out = append(out, CellDiff{
			Col:     c.Col,
			Row:     c.Row,
			Forward: fwd.Bits[i],
			Reverse: rev.Bits[i],
			Edges:   edgesAt(lat, c.Col, c.Row),
		})
	}
	return out, nil
}

// edgesAt 返回采样中心恰好落在其上的边的正向下标。坐标全部是整数，
// 共线判断用交叉积 == 0，不引入浮点误差。
func edgesAt(lat []lattice, col, row int) []int {
	px, py := int64(2*col+1), int64(2*row+1)
	var out []int
	n := len(lat)
	for i := 0; i < n; i++ {
		a := lat[i]
		b := lat[(i+1)%n]
		dx, dy := b.x-a.x, b.y-a.y
		if dx*(py-a.y)-dy*(px-a.x) != 0 {
			continue
		}
		if px < min64(a.x, b.x) || px > max64(a.x, b.x) {
			continue
		}
		if py < min64(a.y, b.y) || py > max64(a.y, b.y) {
			continue
		}
		out = append(out, i)
	}
	return out
}

// Area2 返回两倍的有符号多边形面积，用整数鞋带公式逐项累加，
// 全程没有浮点运算、没有除法。正向与反向遍历的返回值互为相反数，
// 和恒为 0（不是接近 0），这正是整数格点上的面积守恒。
func Area2(poly []geom.Point) (int64, error) {
	if _, err := latticePoly(poly); err != nil {
		return 0, err
	}
	var sum int64
	n := len(poly)
	for i := 0; i < n; i++ {
		j := (i + 1) % n
		x0, y0 := int64(poly[i].X), int64(poly[i].Y)
		x1, y1 := int64(poly[j].X), int64(poly[j].Y)
		sum += x0*y1 - x1*y0
	}
	return sum, nil
}

// latticePoly 校验每个顶点都是有限、绝对值不超限的整数，并转成两倍坐标。
func latticePoly(poly []geom.Point) ([]lattice, error) {
	if len(poly) < 3 {
		return nil, errors.New("多边形至少三个顶点")
	}
	out := make([]lattice, len(poly))
	for i, p := range poly {
		if math.IsNaN(p.X) || math.IsNaN(p.Y) || math.IsInf(p.X, 0) || math.IsInf(p.Y, 0) {
			return nil, fmt.Errorf("顶点 %d 坐标必须有限: (%v,%v)", i, p.X, p.Y)
		}
		if p.X != math.Trunc(p.X) || p.Y != math.Trunc(p.Y) {
			return nil, fmt.Errorf("顶点 %d 坐标必须是整数: (%v,%v)", i, p.X, p.Y)
		}
		if math.Abs(p.X) > MaxCoord || math.Abs(p.Y) > MaxCoord {
			return nil, fmt.Errorf("顶点 %d 坐标超出 ±%d: (%v,%v)", i, MaxCoord, p.X, p.Y)
		}
		out[i] = lattice{x: int64(p.X) * 2, y: int64(p.Y) * 2}
	}
	return out, nil
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
