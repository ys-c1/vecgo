// scanrule 是独立实验程序：同一多边形的两条路径，在三种扫描线边界起点规则下，
// 分别给出按计数原理闭式推出的理论覆盖像素数与实测覆盖像素数。
// 理论值不调用任何 inside 实现；实测值走与 raster.Image.Fill 相同的采样公式，
// 仅把采样域固定（见 domain），使像素中心精确落在多边形边线上。
// 三种规则 A/B/C 都是实验规则；生产代码 geom.Inside 的半开射线法另作对照输出。
// 本程序不修改任何生产代码。
package main

import (
	"fmt"
	"math"

	"vecgo/geom"
)

const (
	W = 8
	H = 8
	eps = 1e-9
)

// 固定采样域：min=0.5、span=8。Fill 公式 x_c=min+(c+0.5)*span/W，
// 代入得 x_c=0.5+(c+0.5)=c+1，即中心为整数 1,2,...,8；y 同理。
// 实验多边形取 [1,8]^2：左边/上边过中心 1，右边/下边过中心 8，
// 最外圈像素中心精确落在边线上，四角由两边共享。
var domain = struct{ minX, minY, spanX, spanY float64 }{0.5, 0.5, 8, 8}

type rule struct {
	name   string
	cond   string // 判定条件（像素中心落在边界上时算哪边）
	result string // 该条件的结果
	inside func(poly []geom.Point, px, py float64) bool
}

// onSeg 判断 (px,py) 是否落在边 ab 上。
func onSeg(ax, ay, bx, by, px, py float64) bool {
	cross := (bx-ax)*(py-ay) - (by-ay)*(px-ax)
	if math.Abs(cross) > eps {
		return false
	}
	return min(ax, bx)-eps <= px && px <= max(ax, bx)+eps &&
		min(ay, by)-eps <= py && py <= max(ay, by)+eps
}

// ruleExclude：朴素"边界全排除"规则——先显式判边，中心落在任意边线上
// （含角点）即判外部；严格内/外部再走半开射线。
func ruleExclude(poly []geom.Point, px, py float64) bool {
	n := len(poly)
	for i := 0; i < n; i++ {
		ax, ay := poly[i].X, poly[i].Y
		bx, by := poly[(i+1)%n].X, poly[(i+1)%n].Y
		if onSeg(ax, ay, bx, by, px, py) {
			return false
		}
	}
	inside := false
	for i := 0; i < n; i++ {
		ax, ay := poly[i].X, poly[i].Y
		bx, by := poly[(i+1)%n].X, poly[(i+1)%n].Y
		if (ay > py) == (by > py) {
			continue
		}
		t := (py - ay) / (by - ay)
		if ax+(bx-ax)*t > px {
			inside = !inside
		}
	}
	return inside
}

// ruleInclude：闭边规则，边界点一律算内部（显式落边判内部，
// 射线交叉改为 >= px）。
func ruleInclude(poly []geom.Point, px, py float64) bool {
	n := len(poly)
	for i := 0; i < n; i++ {
		ax, ay := poly[i].X, poly[i].Y
		bx, by := poly[(i+1)%n].X, poly[(i+1)%n].Y
		if onSeg(ax, ay, bx, by, px, py) {
			return true
		}
	}
	inside := false
	for i := 0; i < n; i++ {
		ax, ay := poly[i].X, poly[i].Y
		bx, by := poly[(i+1)%n].X, poly[(i+1)%n].Y
		if (ay > py) == (by > py) {
			continue
		}
		t := (py - ay) / (by - ay)
		if ax+(bx-ax)*t >= px {
			inside = !inside
		}
	}
	return inside
}

// ruleTL：左/上闭、右/下开规则（与顶点走向无关的几何规则）。
func ruleTL(poly []geom.Point, px, py float64) bool {
	minx, miny := poly[0].X, poly[0].Y
	maxx, maxy := minx, miny
	for _, p := range poly {
		minx = math.Min(minx, p.X)
		maxx = math.Max(maxx, p.X)
		miny = math.Min(miny, p.Y)
		maxy = math.Max(maxy, p.Y)
	}
	onLeft := math.Abs(px-minx) <= eps && miny-eps <= py && py <= maxy+eps
	onRight := math.Abs(px-maxx) <= eps && miny-eps <= py && py <= maxy+eps
	onTop := math.Abs(py-miny) <= eps && minx-eps <= px && px <= maxx+eps
	onBottom := math.Abs(py-maxy) <= eps && minx-eps <= px && px <= maxx+eps
	switch {
	case onLeft && onTop: // 左上角：两闭边
		return true
	case onLeft && onBottom: // 左下角：左闭、下开
		return false
	case onRight && onTop: // 右上角：上闭、右开
		return false
	case onRight && onBottom: // 右下角：两开边
		return false
	case onLeft, onTop:
		return true
	case onRight, onBottom:
		return false
	}
	return ruleExclude(poly, px, py) // 不在边界上：与默认规则相同
}

// raster 用与 raster.Image.Fill 相同的采样公式，但采样域固定为 domain，
// Inside 函数可替换。返回每行亮像素数和总数（实测值）。
func raster(poly []geom.Point, inside func([]geom.Point, float64, float64) bool) (rows []int, total int) {
	rows = make([]int, H)
	for row := 0; row < H; row++ {
		for col := 0; col < W; col++ {
			px := domain.minX + (float64(col)+0.5)*domain.spanX/W
			py := domain.minY + (float64(row)+0.5)*domain.spanY/H
			if inside(poly, px, py) {
				rows[row]++
				total++
			}
		}
	}
	return rows, total
}

// theory 独立闭式推导，不调用 inside。
//
// 中心网格 (c,r) -> 坐标 (c+1, r+1)，c,r=0..7，即 1..8。
// 多边形 [1,8]^2：c=0 命中左边、c=7 命中右边、r=0 命中上边、r=7 命中下边，
// 内部 6x6，四角共享。
//
// 规则 A（边界全排除）：仅严格内部的 6x6 中心亮，
// 每行 r0、r7 为 0，r1..r6 为 6，总计 36。
//
// 规则 B（边界全包含）：8x8 全亮，每行 8，总计 64。
//
// 规则 C（左/上闭，右/下开）：
//   - r1..r6：左竖边中心算内、右竖边中心算外，每行 7；
//   - r0（上边行）：左上角两闭边算内、右上角含开边算外，整行 7；
//   - r7（下边行）：下横边全开，整行 0；
//   总计 7*7 = 49。
func theory(kind string) (rows []int, total int) {
	rows = make([]int, H)
	for r := 0; r < H; r++ {
		for c := 0; c < W; c++ {
			onLeft := c == 0
			onRight := c == 7
			onTop := r == 0
			onBottom := r == 7
			interior := !onLeft && !onRight && !onTop && !onBottom
			var hit bool
			switch kind {
			case "A": // 边界全排除：仅严格内部
				hit = interior
			case "B": // 边界全包含
				hit = true
			case "C": // 显式左/上闭、右/下开，角点含开边即外
				switch {
				case onLeft && onTop:
					hit = true
				case onLeft && onBottom, onRight && onTop, onRight && onBottom:
					hit = false
				case onLeft, onTop:
					hit = true
				case onRight, onBottom:
					hit = false
				default:
					hit = true
				}
			}
			if hit {
				rows[r]++
				total++
			}
		}
	}
	return rows, total
}

// square 与固定采样域对齐：[1,8]^2，边线穿过整数中心 1 与 8。
var squareP1 = []geom.Point{
	{X: 1, Y: 1}, {X: 8, Y: 1}, {X: 8, Y: 8}, {X: 1, Y: 8},
}

// 路径 P2：同一正方形，起点轮换且顺时针（反向遍历）。
var squareP2 = []geom.Point{
	{X: 1, Y: 1}, {X: 1, Y: 8}, {X: 8, Y: 8}, {X: 8, Y: 1},
}

func main() {
	rules := []rule{
		{"A 边界全排除", "像素中心落在任一边界上（含角点）", "算外部（显式判边，落边即外）", ruleExclude},
		{"B 边界全包含", "像素中心落在任一边界上", "算内部（闭边，落边即内、>= 判定）", ruleInclude},
		{"C 左/上闭，右/下开", "中心落在左竖边/上横边 与 右竖边/下横边", "前者算内部，后者算外部；角点含开边即外", ruleTL},
	}

	fmt.Println("规则表（像素中心落在边界上时算哪边）")
	fmt.Println("规则 | 判定条件 | 结果")
	for _, r := range rules {
		fmt.Printf("%s | %s | %s\n", r.name, r.cond, r.result)
	}
	fmt.Println()
	fmt.Println("实验多边形：正方形 [1,8]^2（边长 7）；固定采样域 min=0.5、span=8，")
	fmt.Println("画幅 8x8、supersample=1；像素中心为整数 1..8，最外圈中心精确落边。")
	fmt.Println()

	paths := []struct {
		name string
		poly []geom.Point
	}{
		{"P1 逆时针", squareP1},
		{"P2 起点轮换+顺时针", squareP2},
	}

	fmt.Println("三次规则的覆盖像素数（按行排列，便于横比）")
	fmt.Println("规则 | 路径 | 每行亮像素数 r0..r7 | 总数")
	type cell struct {
		theoryRows, measRows []int
		theoryTot, measTot   int
	}
	results := map[string]map[string]cell{}
	for idx, r := range rules {
		results[r.name] = map[string]cell{}
		thRows, thTot := theory(string(byte('A' + idx)))
		for _, p := range paths {
			mRows, mTot := raster(p.poly, r.inside)
			results[r.name][p.name] = cell{thRows, mRows, thTot, mTot}
			fmt.Printf("%s | %s | 理论%v 实测%v | 理论%4d 实测%4d\n",
				r.name, p.name, thRows, mRows, thTot, mTot)
		}
	}
	fmt.Println()

	fmt.Println("理论值与实测值差值（单位：像素，实测-理论）")
	fmt.Println("规则 | 路径 | 逐行差 | 总差")
	for _, r := range rules {
		for _, p := range paths {
			c := results[r.name][p.name]
			rowDiff := make([]int, H)
			big := false
			for i := range rowDiff {
				rowDiff[i] = c.measRows[i] - c.theoryRows[i]
				if rowDiff[i] != 0 {
					big = true
				}
			}
			totDiff := c.measTot - c.theoryTot
			note := ""
			if big || totDiff != 0 {
				note = "  <- 超过一格，需解释"
			}
			fmt.Printf("%s | %s | %v | %d%s\n", r.name, p.name, rowDiff, totDiff, note)
		}
	}
	fmt.Println()

	fmt.Println("同一多边形两条路径的面积对比（实测覆盖像素数）")
	for _, r := range rules {
		a := results[r.name][paths[0].name].measTot
		b := results[r.name][paths[1].name].measTot
		pct := 0.0
		if a != 0 {
			pct = float64(b-a) / float64(a) * 100
		}
		fmt.Printf("%s: P1=%d P2=%d 差值=%d 像素 (%.4f%%)\n", r.name, a, b, b-a, pct)
	}
	fmt.Println()

	// 对照：生产代码 geom.Inside（半开射线，严格 >）在同一网格上的实测。
	fmt.Println("对照：生产 geom.Inside（默认规则，未改一行生产代码）")
	for _, p := range paths {
		rows, tot := raster(p.poly, geom.Inside)
		fmt.Printf("geom.Inside | %s | 每行%v | 总数 %d\n", p.name, rows, tot)
	}
}
