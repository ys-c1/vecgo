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

func TestLengthKahanAccuracy(t *testing.T) {
	// 万段等长微小线段：朴素顺序求和会持续丢失低位，Kahan 补偿后应贴近理论总长。
	pts := make([]Point, 10001)
	for i := range pts {
		pts[i] = Point{X: float64(i) * 1e-4, Y: 0}
	}
	got := Length(pts)
	want := 1.0
	if d := got - want; d < -1e-12 || d > 1e-12 {
		t.Fatalf("补偿求和精度异常: got=%v want=%v", got, want)
	}
}
