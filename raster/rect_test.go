package raster

import "testing"

func TestRectBasics(t *testing.T) {
	r := Rect{X0: 2, Y0: 3, X1: 10, Y1: 8}
	if r.Empty() || r.Area() != 40 {
		t.Fatalf("面积错误: %+v", r)
	}
	if (Rect{X0: 5, Y0: 5, X1: 5, Y1: 9}).Empty() == false {
		t.Fatal("退化矩形应为空")
	}
	if (Rect{X0: 0, Y0: 0, X1: 4, Y1: 4}).InBounds(3, 4) {
		t.Fatal("超宽应判越界")
	}
	if !(Rect{X0: 0, Y0: 0, X1: 3, Y1: 4}).InBounds(3, 4) {
		t.Fatal("恰好铺满应合法")
	}
}

func TestRectOverlapVsTouch(t *testing.T) {
	a := Rect{X0: 0, Y0: 0, X1: 4, Y1: 4}
	edge := Rect{X0: 4, Y0: 0, X1: 8, Y1: 4}
	if a.Overlaps(edge) {
		t.Fatal("共边不算重叠")
	}
	if !a.Touches(edge) {
		t.Fatal("共边应可合并")
	}
	if !a.Overlaps(Rect{X0: 3, Y0: 3, X1: 9, Y1: 9}) {
		t.Fatal("面积相交应判重叠")
	}
}

func TestRectUnionClampExpand(t *testing.T) {
	a := Rect{X0: 0, Y0: 0, X1: 4, Y1: 4}
	b := Rect{X0: 10, Y0: 10, X1: 12, Y1: 12}
	u := a.Union(b)
	if u != (Rect{X0: 0, Y0: 0, X1: 12, Y1: 12}) {
		t.Fatalf("并集错误: %+v", u)
	}
	if a.Union(Rect{}) != a {
		t.Fatal("与空矩形并集应保持不变")
	}
	c := Rect{X0: -5, Y0: -5, X1: 100, Y1: 100}.Clamp(10, 10)
	if c != (Rect{X0: 0, Y0: 0, X1: 10, Y1: 10}) {
		t.Fatalf("裁剪错误: %+v", c)
	}
	e := Rect{X0: 2, Y0: 2, X1: 4, Y1: 4}.Expand(2, 10, 10)
	if e != (Rect{X0: 0, Y0: 0, X1: 6, Y1: 6}) {
		t.Fatalf("外扩错误: %+v", e)
	}
}
