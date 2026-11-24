package geom

import "testing"

func TestTopLeftSquareCorners(t *testing.T) {
	// 屏幕坐标：py 向下。两种绕向结果必须一致。
	sqCCW := []Point{{X: 0, Y: 0}, {X: 0, Y: 10}, {X: 10, Y: 10}, {X: 10, Y: 0}} // 面积负
	sqCW := []Point{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}   // 面积正
	cases := []struct {
		x, y    float64
		want    bool
		note    string
	}{
		{0, 0, true, "视觉左上：顶边+左边，算内"},
		{10, 0, true, "视觉右上：顶边算内（右边算外，OR）"},
		{10, 10, false, "视觉右下：右边+底边，都算外"},
		{0, 10, true, "视觉左下：左边算内（底边算外，OR）"},
	}
	for _, c := range cases {
		if got := InsideWith(sqCCW, c.x, c.y, RuleHalfPixel); got != c.want {
			t.Fatalf("CCW 角点(%.0f,%.0f)=%v 期望 %v // %s", c.x, c.y, got, c.want, c.note)
		}
		if got := InsideWith(sqCW, c.x, c.y, RuleHalfPixel); got != c.want {
			t.Fatalf("CW  角点(%.0f,%.0f)=%v 期望 %v // %s", c.x, c.y, got, c.want, c.note)
		}
	}
}

func TestTopLeftEdgeMidpoints(t *testing.T) {
	sq := []Point{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}} // 面积正
	cases := []struct {
		x, y float64
		want bool
		note string
	}{
		{5, 0, true, "顶边中点算内"},
		{0, 5, true, "左边中点算内"},
		{10, 5, false, "右边中点算外"},
		{5, 10, false, "底边中点算外"},
		{5, 5, true, "严格内部算内"},
		{50, 50, false, "严格外部算外"},
	}
	for _, c := range cases {
		if got := InsideWith(sq, c.x, c.y, RuleHalfPixel); got != c.want {
			t.Fatalf("(%.0f,%.0f)=%v 期望 %v // %s", c.x, c.y, got, c.want, c.note)
		}
	}
}

func TestLegacyEqualsInside(t *testing.T) {
	polys := [][]Point{
		{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}},
		{{X: 2, Y: 2}, {X: 14, Y: 2}, {X: 2, Y: 12}},
		{{X: 0, Y: 0}, {X: 16, Y: 16}, {X: 8.5, Y: 8.5}},
	}
	for _, p := range polys {
		for k := 0; k <= 20; k++ {
			for l := 0; l <= 20; l++ {
				x, y := float64(k)*0.5, float64(l)*0.5
				if InsideWith(p, x, y, RuleLegacy) != Inside(p, x, y) {
					t.Fatalf("legacy 与历史 Inside 在 (%.1f,%.1f) 不一致", x, y)
				}
			}
		}
	}
}

func TestWindingIndependence(t *testing.T) {
	a := []Point{{X: 0, Y: 0}, {X: 16, Y: 16}, {X: 8.5, Y: 8.5}}
	b := []Point{{X: 0, Y: 0}, {X: 8.5, Y: 8.5}, {X: 16, Y: 16}} // 反向绕序
	for k := 0; k <= 32; k++ {
		for l := 0; l <= 32; l++ {
			x, y := float64(k)*0.5, float64(l)*0.5
			if InsideWith(a, x, y, RuleHalfPixel) != InsideWith(b, x, y, RuleHalfPixel) {
				t.Fatalf("绕向改变结果 (%.1f,%.1f)", x, y)
			}
		}
	}
}
