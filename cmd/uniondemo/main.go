// Command uniondemo 演示真并集轮廓：按交点切分边并重建外轮廓，
// 与逐点判定参照对照，输出顶点表/面积差/绕序检查/耗时等登记信息。
// 它不接入 pipe 绘制路径，现有叠加绘制输出保持不变。
package main

import (
	"errors"
	"fmt"
	"math"
	"os"
	"strings"

	"vecgo/geom"
)

type scenario struct {
	name string
	a, b []geom.Point
}

func sq(x0, y0, x1, y1 float64) []geom.Point {
	return []geom.Point{
		{X: x0, Y: y0}, {X: x1, Y: y0}, {X: x1, Y: y1}, {X: x0, Y: y1},
	}
}

func ngon(n int, r, cx, cy, rot float64) []geom.Point {
	p := make([]geom.Point, n)
	for i := 0; i < n; i++ {
		ang := rot + 2*math.Pi*float64(i)/float64(n)
		p[i] = geom.Point{X: cx + r*math.Cos(ang), Y: cy + r*math.Sin(ang)}
	}
	return p
}

func main() {
	scenarios := []scenario{
		{"1-相离", sq(0, 0, 4, 4), sq(10, 10, 14, 14)},
		{"2-相交", sq(0, 0, 6, 6), sq(4, 2, 10, 8)},
		{"3-包含", sq(0, 0, 10, 10), sq(3, 3, 7, 7)},
		{"4-重合(应报E3)", sq(0, 0, 5, 5), sq(5, 5, 0, 0)},
		{"5-一边退化", append(sq(0, 0, 4, 4), geom.Point{X: 4, Y: 4}), sq(2, 2, 6, 6)},
	}

	var log strings.Builder
	emit := func(format string, args ...any) {
		line := fmt.Sprintf(format, args...)
		fmt.Println(line)
		log.WriteString(line + "\n")
	}

	emit("================ vecgo 真并集轮廓登记 ================")
	for _, sc := range scenarios {
		emit("\n########## 情形 %s ##########", sc.name)
		emitVertexTable(emit, "合并前-图形A", sc.a)
		emitVertexTable(emit, "合并前-图形B", sc.b)

		res, err := geom.Union(sc.a, sc.b)
		if err != nil {
			emit("并集错误: %v", err)
			classifyError(emit, err)
			continue
		}
		for ri, ring := range res.Rings {
			emitVertexTable(emit, fmt.Sprintf("合并后-外环%d", ri), ring)
			emit("  外环%d 轮廓顶点数=%d 有向面积=%.6f 绕序=%s",
				ri, len(ring), geom.SignedArea(ring), orientName(geom.Orientation(ring)))
		}
		diffVertexBySet(emit, sc.a, sc.b, res.Rings)
		emit("轮廓顶点总数=%d 拆分简单环数=%d 耗时=%v", res.VertexCount, res.SplitLoops, res.Elapsed)

		// 逐点判定参照与面积差上界
		ref, rerr := geom.ReferenceUnion([][]geom.Point{sc.a, sc.b}, 400)
		if rerr != nil {
			emit("参照构造失败: %v", rerr)
			continue
		}
		exact := geom.TotalArea(res.Rings)
		diff := math.Abs(exact - ref.GridArea)
		perim := perimeter(sc.a) + perimeter(sc.b)
		bound := perim * ref.GridStep // 具体上界：穿越格数 ≤ 周长/h，每格面积 h²
		emit("精确轮廓面积=%.6f 逐点参照网格面积=%.6f 面积差=%.6f", exact, ref.GridArea, diff)
		emit("面积差上界(具体数值)=周长%.4f × 网格步长%.6f = %.6f；判定 %s",
			perim, ref.GridStep, bound, passFail(diff <= bound))
		emit("顶点数差: 精确轮廓=%d 参照轮廓=%d 差(参照-精确)=%d",
			res.VertexCount, ref.VertexCount, ref.VertexCount-res.VertexCount)

		// 绕序方向检查与反转定位
		for ri, ring := range res.Rings {
			rep := geom.InspectOrientation(ring)
			emit("外环%d 绕序检查: 判定依据=有向面积符号(%.6f) → %s",
				ri, rep.SignedArea, ternary(rep.CCW, "CCW 与外环方向一致", "CW 与外环方向相反"))
			if rep.ReversedAt >= 0 {
				emit("  方向反转定位: 顶点下标=%d 坐标=(%.4f,%.4f) 最小局部叉积=%.6f",
					rep.ReversedAt, rep.ReversedPoint.X, rep.ReversedPoint.Y, rep.MinCross)
			} else {
				emit("  方向正确，无需定位反转顶点（ReversedAt=-1）")
			}
		}
	}

	// 两类额外带序号错误（E1/E2）单独登记
	emit("\n########## 带序号错误专项 ##########")
	bowtie := []geom.Point{
		{X: 0, Y: 0}, {X: 10, Y: 10}, {X: 10, Y: 0}, {X: 0, Y: 10},
	}
	if _, err := geom.MergeSimple(bowtie, sq(0, 0, 2, 2)); err != nil {
		emit("自交未拆分: %v", err)
		classifyError(emit, err)
	}
	if _, err := geom.Union([]geom.Point{{X: 0, Y: 0}, {X: 1, Y: 0}}, sq(0, 0, 2, 2)); err != nil {
		emit("顶点不足三个: %v", err)
		classifyError(emit, err)
	}

	// 两档顶点数：轮廓顶点与耗时
	emit("\n########## 两档顶点数（轮廓顶点与耗时）##########")
	for _, tier := range []struct {
		label string
		n     int
	}{{"低档 6 顶点/环", 6}, {"高档 32 顶点/环", 32}} {
		res, err := geom.Union(ngon(tier.n, 5, 0, 0, 0), ngon(tier.n, 5, 2.5, 0, math.Pi/float64(tier.n)))
		if err != nil {
			emit("%s 错误: %v", tier.label, err)
			continue
		}
		emit("%s: 轮廓顶点=%d 外环数=%d 耗时=%v", tier.label, res.VertexCount, len(res.Rings), res.Elapsed)
	}

	// 自交先拆分再合并的完整链路
	emit("\n########## 自交输入：先拆分再合并 ##########")
	parts, err := geom.SplitSelfIntersecting(bowtie)
	if err != nil {
		emit("拆分失败: %v", err)
	} else {
		emit("蝴蝶结拆分为 %d 个简单环，面积和=%.4f", len(parts), geom.TotalArea(parts))
	}
	res, err := geom.Union(bowtie, sq(12, 2, 16, 8))
	if err != nil {
		emit("自交并集错误: %v", err)
	} else {
		emit("拆分后合并: 外环数=%d 轮廓顶点=%d 总拆分环数=%d 耗时=%v",
			len(res.Rings), res.VertexCount, res.SplitLoops, res.Elapsed)
	}

	// 绕序反转的定位能力：把一个 CCW 环反转为 CW，检查必须报“相反”并定位到顶点。
	emit("\n########## 绕序反转定位（人工翻转一个外环）##########")
	src := sq(0, 0, 6, 6)
	cw := make([]geom.Point, len(src))
	for i := range src {
		cw[i] = src[len(src)-1-i]
	}
	rep := geom.InspectOrientation(cw)
	emit("翻转后有向面积=%.6f → 判定=%s", rep.SignedArea,
		ternary(rep.CCW, "CCW（与外环一致）", "CW（与外环方向相反）"))
	emit("反转定位: ReversedAt 顶点下标=%d 坐标=(%.3f,%.3f) 最小局部叉积=%.6f（该顶点为最顺时针转角处）",
		rep.ReversedAt, rep.ReversedPoint.X, rep.ReversedPoint.Y, rep.MinCross)

	// 登记逐次执行的命令（追加写入，保留历次运行记录）。
	cmdLine := "go run ./cmd/uniondemo"
	if len(os.Args) > 1 {
		cmdLine += " " + strings.Join(os.Args[1:], " ")
	}
	if f, ferr := os.OpenFile("artifacts/union_commands.log",
		os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); ferr == nil {
		fmt.Fprintf(f, "%s\n", cmdLine)
		f.Close()
	}

	path := "artifacts/union_report.txt"
	if err := os.WriteFile(path, []byte(log.String()), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "登记文件写入失败:", err)
		os.Exit(1)
	}
	fmt.Println("\n登记已写入:", path)
	fmt.Println("命令记录已追加: artifacts/union_commands.log")
}

func emitVertexTable(emit func(string, ...any), title string, ring []geom.Point) {
	emit("【%s】顶点表（共 %d 点，闭合环末点连回首点）", title, len(ring))
	emit("  序号 |      X |      Y")
	for i, p := range ring {
		emit("  %4d | %6.3f | %6.3f", i, p.X, p.Y)
	}
}

// diffVertexBySet 基于顶点集合逐点比对合并前后：标注每个外环顶点来自
// 图形A、图形B（坐标容差匹配）还是新增的交点/切分点，并统计被剔除的原顶点。
func diffVertexBySet(emit func(string, ...any), a, b []geom.Point, rings [][]geom.Point) {
	emit("【逐点比对（顶点集合匹配，容差 1e-6）】")
	const tol = 1e-6
	findIn := func(p geom.Point, ring []geom.Point) (int, bool) {
		for i, q := range ring {
			if math.Abs(p.X-q.X) <= tol && math.Abs(p.Y-q.Y) <= tol {
				return i, true
			}
		}
		return -1, false
	}
	keptA, keptB, newPts := 0, 0, 0
	for ri, ring := range rings {
		for i, p := range ring {
			if idx, ok := findIn(p, a); ok {
				emit("  外环%d 下标%2d (%.3f,%.3f) ← 图形A 顶点%d", ri, i, p.X, p.Y, idx)
				keptA++
			} else if idx, ok := findIn(p, b); ok {
				emit("  外环%d 下标%2d (%.3f,%.3f) ← 图形B 顶点%d", ri, i, p.X, p.Y, idx)
				keptB++
			} else {
				emit("  外环%d 下标%2d (%.3f,%.3f) ← 新增（边交点/切分点）", ri, i, p.X, p.Y)
				newPts++
			}
		}
	}
	droppedA, droppedB := 0, 0
	allMerged := func() []geom.Point {
		var out []geom.Point
		for _, r := range rings {
			out = append(out, r...)
		}
		return out
	}()
	for _, p := range a {
		if _, ok := findIn(p, allMerged); !ok {
			droppedA++
		}
	}
	for _, p := range b {
		if _, ok := findIn(p, allMerged); !ok {
			droppedB++
		}
	}
	emit("  汇总: 保留A=%d 保留B=%d 新增交点=%d；被剔除(并入内部) A=%d B=%d；输入顶点=%d 输出顶点=%d",
		keptA, keptB, newPts, droppedA, droppedB, len(a)+len(b), keptA+keptB+newPts)
}

func classifyError(emit func(string, ...any), err error) {
	switch {
	case errors.Is(err, geom.ErrSelfIntersecting):
		emit("  → 错误序号 E1（自交未拆分）")
	case errors.Is(err, geom.ErrTooFewVertices):
		emit("  → 错误序号 E2（顶点不足三个）")
	case errors.Is(err, geom.ErrCoincident):
		emit("  → 错误序号 E3（两图形完全重合）")
	default:
		emit("  → 未分类错误: %v", err)
	}
}

func perimeter(r []geom.Point) float64 {
	s := 0.0
	for i := range r {
		a, b := r[i], r[(i+1)%len(r)]
		s += math.Hypot(b.X-a.X, b.Y-a.Y)
	}
	return s
}

func orientName(o int) string {
	if o > 0 {
		return "CCW（逆时针，外环标准方向）"
	}
	if o < 0 {
		return "CW（顺时针）"
	}
	return "退化"
}

func passFail(ok bool) string {
	if ok {
		return "通过"
	}
	return "不通过"
}

func ternary(ok bool, a, b string) string {
	if ok {
		return a
	}
	return b
}
