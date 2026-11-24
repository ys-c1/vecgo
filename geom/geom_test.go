package geom

import (
	"math"
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

func TestLengthDeterministic(t *testing.T) {
	pts := []Point{{X: 0, Y: 0}, {X: 1.1, Y: 2.2}, {X: 3.3, Y: 4.4},
		{X: 5.5, Y: 6.6}, {X: 7.7, Y: 8.8}, {X: 9.9, Y: 10.1}}
	first := math.Float64bits(Length(pts))
	for i := 0; i < 20; i++ {
		if math.Float64bits(Length(pts)) != first {
			t.Fatalf("第 %d 次 Length 位模式不一致", i)
		}
	}
	// 空输入与单点仍按原接口返回 0，签名行为不变。
	if Length(nil) != 0 || Length([]Point{{X: 1, Y: 1}}) != 0 {
		t.Fatal("Length 边界行为被改变")
	}
}
