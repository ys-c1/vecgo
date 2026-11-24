package geom

import (
	"math"
	"testing"
)

func near(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("%s: got %v want %v", name, got, want)
	}
}

func TestAreaSquare(t *testing.T) {
	sq := []Point{{X: 0, Y: 0}, {X: 20, Y: 0}, {X: 20, Y: 20}, {X: 0, Y: 20}}
	near(t, "面积", Area(sq), 400)
	near(t, "逆序面积", Area([]Point{sq[3], sq[2], sq[1], sq[0]}), 400)
}

func TestIntersectionArea(t *testing.T) {
	a := []Point{{X: 0, Y: 0}, {X: 20, Y: 0}, {X: 20, Y: 20}, {X: 0, Y: 20}}
	b := []Point{{X: 10, Y: 0}, {X: 30, Y: 0}, {X: 30, Y: 20}, {X: 10, Y: 20}}
	near(t, "部分重叠", IntersectionArea(a, b), 200)
	c := []Point{{X: 5, Y: 5}, {X: 15, Y: 5}, {X: 15, Y: 15}, {X: 5, Y: 15}}
	near(t, "包含", IntersectionArea(a, c), 100)
	d := []Point{{X: 50, Y: 50}, {X: 60, Y: 50}, {X: 60, Y: 60}, {X: 50, Y: 60}}
	near(t, "分离", IntersectionArea(a, d), 0)
	near(t, "自身", IntersectionArea(a, a), 400)
}

func TestMeasureCombos(t *testing.T) {
	sq := []Point{{X: 0, Y: 0}, {X: 20, Y: 0}, {X: 20, Y: 20}, {X: 0, Y: 20}}
	near(t, "重复重叠", func() float64 { c, _ := OverlapExcess([][]Point{sq, sq}); return c }(), 400)

	a := []Point{{X: 0, Y: 0}, {X: 20, Y: 0}, {X: 20, Y: 20}, {X: 0, Y: 20}}
	b := []Point{{X: 10, Y: 0}, {X: 30, Y: 0}, {X: 30, Y: 20}, {X: 10, Y: 20}}
	c, err := Measure([][]Point{a, b})
	if err != nil {
		t.Fatal(err)
	}
	near(t, "部分 和", c.Sum, 800)
	near(t, "部分 并集", c.Union, 600)
	near(t, "部分 重叠", c.Excess, 200)
	near(t, "部分 多重覆盖", c.Multi, 200)

	outer := []Point{{X: 0, Y: 0}, {X: 20, Y: 0}, {X: 20, Y: 20}, {X: 0, Y: 20}}
	inner := []Point{{X: 5, Y: 5}, {X: 15, Y: 5}, {X: 15, Y: 15}, {X: 5, Y: 15}}
	c, err = Measure([][]Point{outer, inner})
	if err != nil {
		t.Fatal(err)
	}
	near(t, "包含 和", c.Sum, 500)
	near(t, "包含 并集", c.Union, 400)
	near(t, "包含 重叠", c.Excess, 100)
	near(t, "包含 多重覆盖", c.Multi, 100)
}

func TestMeasureThreeWay(t *testing.T) {
	// 两个 20×20 加一个 10×20 竖条：Σ=1000。
	a := []Point{{X: 0, Y: 0}, {X: 20, Y: 0}, {X: 20, Y: 20}, {X: 0, Y: 20}}
	b := []Point{{X: 10, Y: 0}, {X: 30, Y: 0}, {X: 30, Y: 20}, {X: 10, Y: 20}}
	// 与 a、b 都相交的竖条，但只交 a 的右半、b 的左半。
	c := []Point{{X: 15, Y: 10}, {X: 25, Y: 10}, {X: 25, Y: 30}, {X: 15, Y: 30}}
	m, err := Measure([][]Point{a, b, c})
	if err != nil {
		t.Fatal(err)
	}
	// a∩b=200，a∩c=50，b∩c=100，三交=50 → 并集=1000−350+50=700。
	near(t, "三 并集", m.Union, 700)
	// 重复覆盖面积按重数计权：恰两覆盖 200×1 + 三覆盖 50×2 = 300；
	// 被 ≥2 个覆盖的面积（不计重数）是 200+50=250。
	near(t, "三 重叠", m.Excess, 300)
	near(t, "三 多重覆盖面积", m.Multi, 250)
}

func TestMeasureNonConvexRejected(t *testing.T) {
	non := []Point{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 5, Y: 3}, {X: 10, Y: 10}, {X: 0, Y: 10}}
	if _, err := Measure([][]Point{non}); err == nil {
		t.Fatal("凹多边形应报错而不是给错误面积")
	}
}
