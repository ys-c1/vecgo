package frame

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"vecgo/geom"
)

func square(x, y, size float64) []geom.Point {
	return []geom.Point{
		{X: x, Y: y},
		{X: x + size, Y: y},
		{X: x + size, Y: y + size},
		{X: x, Y: y + size},
	}
}

func almost(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// 单图形：正方形偏移放置，窗口四边须与手算一致。
func TestSolveSingleShape(t *testing.T) {
	scene := [][]geom.Point{square(2, 3, 10)}
	spec := Spec{Aspect: 1, Margin: 0.1, FillMin: 0.5, FillMax: 0.7}
	w, err := Solve(scene, spec)
	if err != nil {
		t.Fatal(err)
	}
	// bbox 10x10，inner=0.8，窗口边长 12.5，中心 (7,8)
	if !almost(w.MinX, 0.75) || !almost(w.MaxX, 13.25) || !almost(w.MinY, 1.75) || !almost(w.MaxY, 14.25) {
		t.Fatalf("窗口四边错误: %+v", w)
	}
	c, err := Verify(scene, w, spec)
	if err != nil {
		t.Fatal(err)
	}
	if !almost(c.FillRatio, 0.64) { // 0.8^2
		t.Fatalf("占屏比例错误: %v", c.FillRatio)
	}
	if !c.FillOK {
		t.Fatal("占屏比例应落在区间内")
	}
	if !c.NoClipOK {
		t.Fatalf("不应裁掉顶点, clipped=%d", c.Clipped)
	}
}

// 多图形：散布的多个图形，所有顶点须落在窗口内。
func TestSolveMultiShape(t *testing.T) {
	scene := [][]geom.Point{
		square(0, 0, 4),
		square(20, 10, 2),
		square(-6, 14, 3),
	}
	spec := Spec{Aspect: 16.0 / 9.0, Margin: 0.05, FillMin: 0.1, FillMax: 0.9}
	w, err := Solve(scene, spec)
	if err != nil {
		t.Fatal(err)
	}
	c, err := Verify(scene, w, spec)
	if err != nil {
		t.Fatal(err)
	}
	if !c.FillOK {
		t.Fatalf("占屏比例应落在区间内: %v", c.FillRatio)
	}
	if !c.NoClipOK {
		t.Fatalf("不应裁掉顶点, clipped=%d", c.Clipped)
	}
	// 独立复算占屏比例
	lo, hi, _ := BBox(scene)
	want := (hi.X - lo.X) * (hi.Y - lo.Y) / (w.SpanX() * w.SpanY())
	if !almost(c.FillRatio, want) {
		t.Fatalf("占屏比例与复算不符: %v != %v", c.FillRatio, want)
	}
}

// 极端长条：10000x1，窗口须完整覆盖且不裁顶点。
func TestSolveLongStrip(t *testing.T) {
	scene := [][]geom.Point{{{X: 0, Y: 0}, {X: 10000, Y: 0}, {X: 10000, Y: 1}, {X: 0, Y: 1}}}
	spec := Spec{Aspect: 1, Margin: 0.1, FillMin: 1e-6, FillMax: 1}
	w, err := Solve(scene, spec)
	if err != nil {
		t.Fatal(err)
	}
	// 受限方向为 x：窗口边长 10000/0.8=12500
	if !almost(w.SpanX(), 12500) || !almost(w.SpanY(), 12500) {
		t.Fatalf("窗口边长错误: %v x %v", w.SpanX(), w.SpanY())
	}
	c, err := Verify(scene, w, spec)
	if err != nil {
		t.Fatal(err)
	}
	if !c.NoClipOK {
		t.Fatalf("长条不应裁掉顶点, clipped=%d", c.Clipped)
	}
	if !almost(c.FillRatio, 10000.0/(12500*12500)) {
		t.Fatalf("占屏比例错误: %v", c.FillRatio)
	}
}

// 退化包围盒：单点与零高折线均须报出字段 bbox。
func TestSolveDegenerateBBox(t *testing.T) {
	spec := Spec{Aspect: 1, Margin: 0.1, FillMin: 0.1, FillMax: 1}
	point := [][]geom.Point{{{X: 1, Y: 1}}}
	if _, err := Solve(point, spec); err == nil || !strings.Contains(err.Error(), "bbox") {
		t.Fatalf("单点应报 bbox 退化, got %v", err)
	}
	line := [][]geom.Point{{{X: 0, Y: 5}, {X: 9, Y: 5}, {X: 3, Y: 5}}}
	if _, err := Solve(line, spec); err == nil || !strings.Contains(err.Error(), "bbox") {
		t.Fatalf("零高折线应报 bbox 退化, got %v", err)
	}
}

// 空场景须报出字段 scene。
func TestSolveEmptyScene(t *testing.T) {
	spec := Spec{Aspect: 1, Margin: 0.1, FillMin: 0.1, FillMax: 1}
	if _, err := Solve(nil, spec); err == nil || !strings.Contains(err.Error(), "scene") {
		t.Fatalf("空场景应报 scene, got %v", err)
	}
	if _, err := Solve([][]geom.Point{{}}, spec); err == nil || !strings.Contains(err.Error(), "scene") {
		t.Fatalf("无顶点场景应报 scene, got %v", err)
	}
}

// 留白越界须报出字段 margin。
func TestSolveBadMargin(t *testing.T) {
	scene := [][]geom.Point{square(0, 0, 1)}
	for _, m := range []float64{-0.1, 0.5, 1} {
		spec := Spec{Aspect: 1, Margin: m, FillMin: 0.1, FillMax: 1}
		if _, err := Solve(scene, spec); err == nil || !strings.Contains(err.Error(), "margin") {
			t.Fatalf("margin=%v 应报 margin, got %v", m, err)
		}
	}
}

// 超出画布：占屏比例达不到下限时报 FillOK=false，且不裁顶点核验须独立通过。
func TestVerifyRatioOutOfInterval(t *testing.T) {
	scene := [][]geom.Point{{{X: 0, Y: 0}, {X: 10000, Y: 0}, {X: 10000, Y: 1}, {X: 0, Y: 1}}}
	spec := Spec{Aspect: 1, Margin: 0.1, FillMin: 0.9, FillMax: 1}
	w, err := Solve(scene, spec)
	if err != nil {
		t.Fatal(err)
	}
	c, err := Verify(scene, w, spec)
	if err != nil {
		t.Fatal(err)
	}
	if c.FillOK {
		t.Fatalf("占屏比例 %v 不应落入 [0.9,1]", c.FillRatio)
	}
	if !c.NoClipOK {
		t.Fatal("不裁顶点核验应独立通过")
	}
}

// 两档场景数：各报占屏比例与耗时。
func TestFrameScales(t *testing.T) {
	for _, n := range []int{100, 10000} {
		scene := gridScene(n)
		spec := Spec{Aspect: 1, Margin: 0.1, FillMin: 0.5, FillMax: 0.9}
		start := time.Now()
		w, err := Solve(scene, spec)
		if err != nil {
			t.Fatal(err)
		}
		c, err := Verify(scene, w, spec)
		if err != nil {
			t.Fatal(err)
		}
		elapsed := time.Since(start)
		if !c.FillOK || !c.NoClipOK {
			t.Fatalf("n=%d 核验失败: %+v", n, c)
		}
		t.Logf("n=%-6d 窗口=[%.2f, %.2f, %.2f, %.2f] 占屏比例=%.4f 耗时=%v",
			n, w.MinX, w.MinY, w.MaxX, w.MaxY, c.FillRatio, elapsed)
	}
}

// gridScene 生成约 n 个单位正方形，间隔排布。
func gridScene(n int) [][]geom.Point {
	k := int(math.Ceil(math.Sqrt(float64(n))))
	scene := make([][]geom.Point, 0, n)
	for i := 0; i < n; i++ {
		x := float64(i%k) * 3
		y := float64(i/k) * 3
		scene = append(scene, square(x, y, 1))
	}
	return scene
}

func BenchmarkSolveVerify(b *testing.B) {
	for _, n := range []int{100, 10000} {
		scene := gridScene(n)
		spec := Spec{Aspect: 1, Margin: 0.1, FillMin: 0.5, FillMax: 0.9}
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				w, err := Solve(scene, spec)
				if err != nil {
					b.Fatal(err)
				}
				if _, err := Verify(scene, w, spec); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
