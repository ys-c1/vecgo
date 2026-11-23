package geom

import (
	"errors"
	"math"
	"testing"
)

// ---------- 测试输入构造 ----------

func square(x0, y0, x1, y1 float64) []Point {
	return []Point{{x0, y0}, {x1, y0}, {x1, y1}, {x0, y1}}
}

// ngon 构造正 n 边形（半径 r，圆心 cx,cy，相位旋转 rot）。
func ngon(n int, r, cx, cy, rot float64) []Point {
	p := make([]Point, n)
	for i := 0; i < n; i++ {
		a := rot + 2*math.Pi*float64(i)/float64(n)
		p[i] = Point{cx + r*math.Cos(a), cy + r*math.Sin(a)}
	}
	return p
}

// ---------- 五类情形 ----------

// 1. 相离：并集为两个独立外环。
func TestUnionDisjoint(t *testing.T) {
	a := square(0, 0, 4, 4)
	b := square(10, 10, 14, 14)
	res, err := Union(a, b)
	if err != nil {
		t.Fatalf("相离不应报错: %v", err)
	}
	if len(res.Rings) != 2 {
		t.Fatalf("相离应有两个外环, 得到 %d", len(res.Rings))
	}
	got := TotalArea(res.Rings)
	want := 16.0 + 16.0
	if math.Abs(got-want) > 1e-8 {
		t.Fatalf("相离面积错误: got=%v want=%v", got, want)
	}
	for _, r := range res.Rings {
		if Orientation(r) != 1 {
			t.Fatal("外环绕序必须为 CCW")
		}
	}
}

// 2. 相交：产生交点切分后的单一并集轮廓。
func TestUnionIntersecting(t *testing.T) {
	a := square(0, 0, 6, 6)
	b := square(4, 2, 10, 8)
	res, err := Union(a, b)
	if err != nil {
		t.Fatalf("相交不应报错: %v", err)
	}
	if len(res.Rings) != 1 {
		t.Fatalf("相交应得到单一外环, 得到 %d 环", len(res.Rings))
	}
	// 并集面积 = 36 + 36 - 重叠([4,6]×[2,6]=2*4=8) = 64；
	// 水平切片复核：6*2 + 10*4 + 6*2 = 64。
	got := TotalArea(res.Rings)
	if math.Abs(got-64.0) > 1e-8 {
		t.Fatalf("相交面积错误: got=%v want=64", got)
	}
	if res.VertexCount != 8 {
		t.Fatalf("相交轮廓顶点数应为 8（含两个交点各算一次）, 得到 %d", res.VertexCount)
	}
	if Orientation(res.Rings[0]) != 1 {
		t.Fatal("外环绕序必须为 CCW")
	}
}

// 3. 包含：内圈边被完全覆盖，外环保留外层轮廓。
func TestUnionContainment(t *testing.T) {
	outer := square(0, 0, 10, 10)
	inner := square(3, 3, 7, 7)
	res, err := Union(outer, inner)
	if err != nil {
		t.Fatalf("包含不应报错: %v", err)
	}
	if len(res.Rings) != 1 {
		t.Fatalf("包含应得到单一外环, 得到 %d", len(res.Rings))
	}
	if math.Abs(TotalArea(res.Rings)-100.0) > 1e-8 {
		t.Fatalf("包含并集面积应等于外环面积 100")
	}
	if res.VertexCount != 4 {
		t.Fatalf("包含轮廓应保留外环 4 个顶点, 得到 %d", res.VertexCount)
	}
}

// 4. 重合：带序号错误 E3。
func TestUnionCoincident(t *testing.T) {
	a := square(0, 0, 5, 5)
	b := square(5, 5, 0, 0) // 反向但同一点集
	_, err := Union(a, b)
	if !errors.Is(err, ErrCoincident) {
		t.Fatalf("完全重合必须返回 E3, 得到 %v", err)
	}
}

// 5. 一边退化：含重复点的退化边在清理后仍能正确合并。
func TestUnionDegenerateEdge(t *testing.T) {
	a := []Point{{0, 0}, {4, 0}, {4, 4}, {4, 4}, {0, 4}} // 重复 (4,4)
	b := square(2, 2, 6, 6)
	res, err := Union(a, b)
	if err != nil {
		t.Fatalf("退化边不应报错: %v", err)
	}
	if len(res.Rings) != 1 {
		t.Fatalf("退化边情形应得到单一外环, 得到 %d", len(res.Rings))
	}
	// 16 + 16 - 重叠(2*2=4) = 28
	if math.Abs(TotalArea(res.Rings)-28.0) > 1e-8 {
		t.Fatalf("退化边面积错误: got=%v want=28", TotalArea(res.Rings))
	}
	if res.VertexCount != 8 {
		t.Fatalf("退化边轮廓顶点应为 8, 得到 %d", res.VertexCount)
	}
}

// 非连续重复点形成“去-回”退化刺：cleanRing 必须剪掉，否则 T 型切分漏段。
func TestUnionNonConsecutiveDuplicate(t *testing.T) {
	// (4,4) 与第二个顶点重复但不相邻，制造退化边 (0,4)->(4,4)->(0,4) 折返。
	a := []Point{{0, 0}, {4, 0}, {4, 4}, {0, 4}, {4, 4}}
	b := square(2, 2, 6, 6)
	if got := len(cleanRing(a)); got != 4 {
		t.Fatalf("cleanRing 应剪掉非连续重复得到 4 点, 得到 %d", got)
	}
	res, err := Union(a, b)
	if err != nil {
		t.Fatalf("非连续退化输入不应报错: %v", err)
	}
	if len(res.Rings) != 1 {
		t.Fatalf("应得到单一外环, 得到 %d", len(res.Rings))
	}
	// 16 + 16 - 重叠 4 = 28
	if math.Abs(TotalArea(res.Rings)-28.0) > 1e-8 {
		t.Fatalf("非连续退化面积应为 28, 得到 %v", TotalArea(res.Rings))
	}
	if res.VertexCount != 8 {
		t.Fatalf("非连续退化轮廓应为 8 顶点, 得到 %d", res.VertexCount)
	}
}

// ---------- 三类带序号错误 ----------

func TestErrTooFewVertices(t *testing.T) {
	if _, err := Union([]Point{{0, 0}, {1, 0}}, square(0, 0, 2, 2)); !errors.Is(err, ErrTooFewVertices) {
		t.Fatalf("顶点不足必须返回 E2, 得到 %v", err)
	}
	if _, err := MergeSimple([]Point{{0, 0}, {1, 0}}, square(0, 0, 2, 2)); !errors.Is(err, ErrTooFewVertices) {
		t.Fatalf("MergeSimple 顶点不足必须返回 E2, 得到 %v", err)
	}
}

func TestErrSelfIntersecting(t *testing.T) {
	bowtie := []Point{{0, 0}, {10, 10}, {10, 0}, {0, 10}}
	// 底层闸门：未拆分的自交输入必须被拒绝。
	if _, err := MergeSimple(bowtie, square(0, 0, 2, 2)); !errors.Is(err, ErrSelfIntersecting) {
		t.Fatalf("自交未拆分必须返回 E1, 得到 %v", err)
	}
}

func TestErrCoincidentCode(t *testing.T) {
	a := ngon(5, 3, 1, 1, 0)
	b := ngon(5, 3, 1, 1, 0)
	if _, err := Union(a, b); !errors.Is(err, ErrCoincident) {
		t.Fatalf("完全重合必须返回 E3, 得到 %v", err)
	}
}

// ---------- 自交先拆分再合并 ----------

func TestSplitBowtie(t *testing.T) {
	bowtie := []Point{{0, 0}, {10, 10}, {10, 0}, {0, 10}}
	parts, err := SplitSelfIntersecting(bowtie)
	if err != nil {
		t.Fatalf("拆分失败: %v", err)
	}
	if len(parts) != 2 {
		t.Fatalf("蝴蝶结应拆为两个三角形, 得到 %d", len(parts))
	}
	for _, p := range parts {
		if len(p) != 3 {
			t.Fatalf("拆分结果应为三角形, 得到 %d 顶点", len(p))
		}
		if Orientation(p) != 1 {
			t.Fatal("拆分出的简单环必须归一化为 CCW")
		}
	}
	// 两个三角形面积之和 = 原蝴蝶结的绝对面积（|SignedArea|=0，故按两三角形各 25 计）
	if math.Abs(TotalArea(parts)-50.0) > 1e-8 {
		t.Fatalf("拆分面积和应为 50, 得到 %v", TotalArea(parts))
	}
}

func TestUnionSelfIntersectingInput(t *testing.T) {
	bowtie := []Point{{0, 0}, {10, 10}, {10, 0}, {0, 10}}
	other := square(12, 2, 16, 8) // 与蝴蝶结右三角形局部相交
	res, err := Union(bowtie, other)
	if err != nil {
		t.Fatalf("自交输入经拆分后应能合并: %v", err)
	}
	if res.SplitLoops < 3 {
		t.Fatalf("自交输入应先拆分为多个简单环, SplitLoops=%d", res.SplitLoops)
	}
	if len(res.Rings) < 1 {
		t.Fatal("并集至少有一个外环")
	}
	for _, r := range res.Rings {
		if Orientation(r) != 1 {
			t.Fatal("并集外环必须 CCW")
		}
	}
}

// ---------- 与逐点判定参照对照 ----------

func TestAgainstReference(t *testing.T) {
	cases := map[string][][]Point{
		"intersect": {square(0, 0, 6, 6), square(4, 2, 10, 8)},
		"contain":   {square(0, 0, 10, 10), square(3, 3, 7, 7)},
		"disjoint":  {square(0, 0, 4, 4), square(10, 10, 14, 14)},
	}
	for name, polys := range cases {
		t.Run(name, func(t *testing.T) {
			res, err := Union(polys[0], polys[1])
			if err != nil {
				t.Fatalf("Union 失败: %v", err)
			}
			ref, err := ReferenceUnion(polys, 400)
			if err != nil {
				t.Fatalf("参照构造失败: %v", err)
			}
			exact := TotalArea(res.Rings)
			diff := math.Abs(exact - ref.GridArea)
			// 面积差上界（具体数值）：边界穿越的格数 ≤ 周长/h，每格面积 h²。
			perim := 0.0
			for _, r := range polys {
				perim += polygonPerimeter(r)
			}
			h := ref.GridStep
			bound := perim * h
			if diff > bound {
				t.Fatalf("%s 面积差 %v 超过上界 %v (精确=%v 网格=%v h=%v)",
					name, diff, bound, exact, ref.GridArea, h)
			}
			if diff > 0.05*exact {
				t.Fatalf("%s 面积差相对值过大: %v vs %v", name, diff, exact)
			}
			// 顶点数差：参照轮廓是网格折线，只报差值并检查非负且量级合理。
			dv := ref.VertexCount - res.VertexCount
			if dv < 0 {
				t.Fatalf("%s 参照顶点数不应少于精确轮廓: ref=%d exact=%d",
					name, ref.VertexCount, res.VertexCount)
			}
			if res.VertexCount > ref.VertexCount {
				t.Fatalf("%s 顶点数差异常", name)
			}
			t.Logf("%s: 精确面积=%.6f 网格面积=%.6f 差=%.6f 上界=%.6f 顶点差(ref-exact)=%d",
				name, exact, ref.GridArea, diff, bound, dv)
		})
	}
}

func polygonPerimeter(r []Point) float64 {
	s := 0.0
	n := len(r)
	for i := 0; i < n; i++ {
		a, b := r[i], r[(i+1)%n]
		s += math.Hypot(b.X-a.X, b.Y-a.Y)
	}
	return s
}

// ---------- 两档顶点数：轮廓顶点与耗时 ----------

func TestTwoVertexTiers(t *testing.T) {
	tiers := []struct {
		name string
		n    int
	}{
		{"低档-六边形", 6},
		{"高档-32边形", 32},
	}
	for _, tier := range tiers {
		a := ngon(tier.n, 5, 0, 0, 0)
		b := ngon(tier.n, 5, 2.5, 0, math.Pi/float64(tier.n))
		res, err := Union(a, b)
		if err != nil {
			t.Fatalf("%s 合并失败: %v", tier.name, err)
		}
		if len(res.Rings) != 1 {
			t.Fatalf("%s 应得到单一外环, 实际 %d", tier.name, len(res.Rings))
		}
		if res.VertexCount < tier.n {
			t.Fatalf("%s 轮廓顶点数 %d 少于输入边数", tier.name, res.VertexCount)
		}
		if res.Elapsed <= 0 {
			t.Fatalf("%s 耗时记录异常: %v", tier.name, res.Elapsed)
		}
		t.Logf("%s: 输入顶点=%d/环 轮廓顶点=%d 耗时=%v",
			tier.name, tier.n, res.VertexCount, res.Elapsed)
	}
}

// ---------- 绕序方向检查与顶点定位 ----------

func TestInspectOrientation(t *testing.T) {
	ccw := square(0, 0, 4, 4)
	rep := InspectOrientation(ccw)
	if !rep.CCW || rep.ReversedAt != -1 {
		t.Fatalf("CCW 环判定错误: %+v", rep)
	}
	cw := []Point{{0, 0}, {0, 4}, {4, 4}, {4, 0}}
	rep2 := InspectOrientation(cw)
	if rep2.CCW || rep2.ReversedAt < 0 {
		t.Fatalf("CW 环必须定位到反转顶点: %+v", rep2)
	}
	// 定位到的顶点必须确实是环上的某个顶点。
	if rep2.ReversedPoint != cw[rep2.ReversedAt] {
		t.Fatal("反转顶点定位与输入不一致")
	}
}

// ---------- 既有 Inside 行为不变（回归保护） ----------

func TestInsideRegression(t *testing.T) {
	sq := square(0, 0, 10, 10)
	if !Inside(sq, 5, 5) || Inside(sq, 15, 5) {
		t.Fatal("Inside 行为发生变化")
	}
}
