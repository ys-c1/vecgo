package geom

import (
	"testing"
)

func TestBezierEnds(t *testing.T) {
	a := Bezier(Point{X: 0, Y: 0}, Point{X: 0, Y: 1}, Point{X: 1, Y: 1}, Point{X: 1, Y: 0}, 0)
	if a.X != 0 || a.Y != 0 {
		t.Fatalf("起点错误: %v", a)
	}
}

func TestFlattenCount(t *testing.T) {
	pts, err := Flatten(Point{X: 0, Y: 0}, Point{X: 0, Y: 10}, Point{X: 10, Y: 10}, Point{X: 10, Y: 0}, 8)
	if err != nil || len(pts) != 9 {
		t.Fatalf("细分点数错误: %v %d", err, len(pts))
	}
	if Length(pts) <= 10 {
		t.Fatal("长度应大于弦长")
	}
}

func TestInsideSquare(t *testing.T) {
	sq := []Point{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}
	if !Inside(sq, 5, 5) || Inside(sq, 15, 5) {
		t.Fatal("内外判定错误")
	}
}

func TestClipShrinks(t *testing.T) {
	sq := []Point{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}
	win := []Point{{X: 2, Y: 2}, {X: 8, Y: 2}, {X: 8, Y: 8}, {X: 2, Y: 8}}
	out, err := Clip(sq, win)
	if err != nil || len(out) < 3 {
		t.Fatalf("裁剪结果异常: %v %d", err, len(out))
	}
}

func TestStrokeRejectsZero(t *testing.T) {
	if _, err := Stroke([]Point{{X: 0, Y: 0}, {X: 1, Y: 0}}, 0); err == nil {
		t.Fatal("width=0 应报错")
	}
	if _, err := Stroke([]Point{{X: 0, Y: 0}, {X: 0, Y: 0}}, 1); err == nil {
		t.Fatal("重复点应报错")
	}
}

func TestAreaAndPerimeter(t *testing.T) {
	sq := []Point{{X: 0, Y: 0}, {X: 2, Y: 0}, {X: 2, Y: 2}, {X: 0, Y: 2}}
	if a := Area(sq); a != 4 {
		t.Fatalf("面积应为 4，实为 %v", a)
	}
	if p := Perimeter(sq); p != 8 {
		t.Fatalf("周长应为 8，实为 %v", p)
	}
	// 有向面积随环绕方向变号，面积取绝对值。
	rev := []Point{{X: 0, Y: 2}, {X: 2, Y: 2}, {X: 2, Y: 0}, {X: 0, Y: 0}}
	if SignedArea(rev) >= 0 {
		t.Fatal("反向环绕的有向面积应为负")
	}
	if Area(rev) != 4 {
		t.Fatal("面积不应依赖环绕方向")
	}
}

func TestIsSimple(t *testing.T) {
	sq := []Point{{X: 0, Y: 0}, {X: 2, Y: 0}, {X: 2, Y: 2}, {X: 0, Y: 2}}
	if !IsSimple(sq) {
		t.Fatal("正方形应为简单多边形")
	}
	bow := []Point{{X: 0, Y: 0}, {X: 2, Y: 2}, {X: 2, Y: 0}, {X: 0, Y: 2}}
	if IsSimple(bow) {
		t.Fatal("自相交的蝴蝶结不应判为简单")
	}
	dup := []Point{{X: 0, Y: 0}, {X: 0, Y: 0}, {X: 1, Y: 0}, {X: 1, Y: 1}}
	if IsSimple(dup) {
		t.Fatal("零长边不应判为简单")
	}
}
