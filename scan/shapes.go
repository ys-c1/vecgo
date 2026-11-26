package scan

import (
	"math"

	"vecgo/geom"
)

// Group 是题目所说的“一组形状”：Rep 用于展示更新量/边界检查计数，
// Corpus 用于计时与更大范围的逐像素等价性核对。
type Group struct {
	Name   string
	Rep    []geom.Point
	Corpus [][]geom.Point
}

// CanvasW / CanvasH 是三组形状统一使用的画幅。
const (
	CanvasW = 200
	CanvasH = 200
)

// Groups 返回固定的三组凸形状（坐标均为整数，保证扫描线不经过顶点）：
//
//	A 三角形：两条边各一个固定 Δ；
//	B 平行四边形：左右 Δ 相同；
//	C 凸十六边形：每条边一个 Δ，越界时多次换边。
func Groups() []Group {
	tri := []geom.Point{
		{X: 100, Y: 20},
		{X: 60, Y: 180},
		{X: 140, Y: 180},
	}
	para := []geom.Point{
		{X: 70, Y: 30},
		{X: 170, Y: 30},
		{X: 140, Y: 150},
		{X: 40, Y: 150},
	}
	poly16 := cleanPolygon16()

	return []Group{
		{Name: "A_三角形", Rep: tri, Corpus: triangleCorpus()},
		{Name: "B_平行四边形", Rep: para, Corpus: quadCorpus()},
		{Name: "C_凸十六边形", Rep: poly16, Corpus: ngonCorpus(48)},
	}
}

// cleanPolygon16 是左右对称的严格凸十六边形。右侧链自上而下的
// Δx 依次为 4, 2, 1/2, 1/4, -1/4, -1/2, -2, -4，全是干净小数，
// 且所有端点到像素中心判定线的距离至少 1/8，没有平局。
func cleanPolygon16() []geom.Point {
	return []geom.Point{
		{X: 100, Y: 20},
		{X: 132, Y: 28},
		{X: 164, Y: 44},
		{X: 172, Y: 60},
		{X: 182, Y: 100},
		{X: 172, Y: 140},
		{X: 164, Y: 156},
		{X: 132, Y: 172},
		{X: 100, Y: 180},
		{X: 68, Y: 172},
		{X: 36, Y: 156},
		{X: 28, Y: 140},
		{X: 18, Y: 100},
		{X: 28, Y: 60},
		{X: 36, Y: 44},
		{X: 68, Y: 28},
	}
}

// triangleCorpus 用确定性参数生成一批形状各异的凸三角形。
func triangleCorpus() [][]geom.Point {
	var out [][]geom.Point
	for k := 0; k < 64; k++ {
		topX := 60 + float64((k*37)%80)
		topY := 12 + float64((k*13)%30)
		lx := 10 + float64((k*29)%55)
		rx := 135 + float64((k*17)%55)
		by := 160 + float64((k*7)%30)
		out = append(out, []geom.Point{
			{X: topX, Y: topY},
			{X: lx, Y: by},
			{X: rx, Y: by + float64(k%3)},
		})
	}
	return out
}

// quadCorpus 生成一批凸四边形（梯形/平行四边形变体）。
func quadCorpus() [][]geom.Point {
	var out [][]geom.Point
	for k := 0; k < 64; k++ {
		top := 25 + float64((k*11)%25)
		topR := 140 + float64((k*19)%35)
		botR := 120 + float64((k*23)%45)
		bot := 15 + float64((k*31)%30)
		y0 := 25 + float64((k*7)%20)
		y1 := 150 + float64((k*5)%35)
		out = append(out, []geom.Point{
			{X: top + float64(k%5)*4, Y: y0},
			{X: topR, Y: y0 + float64(k%4)*2},
			{X: botR, Y: y1},
			{X: bot, Y: y1 - float64(k%3)*2},
		})
	}
	return out
}

// ngonCorpus 生成一批旋转/半径不同的凸 N 边形（正多边形必凸）。
func ngonCorpus(n int) [][]geom.Point {
	var out [][]geom.Point
	for k := 0; k < 32; k++ {
		rx := 60 + float64((k*13)%30)
		ry := 55 + float64((k*17)%30)
		rot := float64(k) * 0.137
		pts := make([]geom.Point, 0, n)
		for i := 0; i < n; i++ {
			a := rot + 2*math.Pi*float64(i)/float64(n)
			pts = append(pts, geom.Point{
				X: 100 + rx*math.Cos(a),
				Y: 100 + ry*math.Sin(a),
			})
		}
		out = append(out, pts)
	}
	return out
}

// NotchPolygon 是左侧带三角形内凹的八边形（实为七顶点）：
//
//	(0,0)──(8,0)
//	  │       │
//	  │       │
//	(0,2)     │
//	  ╲       │
//	  (3,4)   │
//	  ╱       │
//	(0,6)     │
//	  │       │
//	(0,8)──(8,8)
//
// 左端点随行号为 0,0,.75,2.25,2.25,.75,0,0 —— 先向右再向左，
// x 单调性被凹口破坏。
func NotchPolygon() []geom.Point {
	return []geom.Point{
		{X: 0, Y: 0},
		{X: 8, Y: 0},
		{X: 8, Y: 8},
		{X: 0, Y: 8},
		{X: 0, Y: 6},
		{X: 3, Y: 4},
		{X: 0, Y: 2},
	}
}

// OverhangPolygon 是右缘带“悬挑”的凹多边形，从最高点到最低点的
// 右侧链在 A(9,3)->N(6,2) 处 y 反向（3 退回 2）。y=2.5 一行与四条边
// 相交，正确结果是两个区间，而凸性增量法只保留两个端点、抹掉凹档。
//
// 顶点顺序：T, A, N, D, B, L2, L1。
func OverhangPolygon() []geom.Point {
	return []geom.Point{
		{X: 5, Y: 0},  // T 最高点
		{X: 9, Y: 3},  // A
		{X: 6, Y: 2},  // N 悬挑内尖：y=2 < 3，链在此 y 反向
		{X: 9, Y: 6},  // D
		{X: 5, Y: 10}, // B 最低点
		{X: 2, Y: 7},  // L2
		{X: 1, Y: 3},  // L1
	}
}
