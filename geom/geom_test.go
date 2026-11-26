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

func TestSignedAreaSign(t *testing.T) {
	ccw := []Point{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}
	cw := []Point{{X: 0, Y: 0}, {X: 0, Y: 10}, {X: 10, Y: 10}, {X: 10, Y: 0}}
	if a := SignedArea(ccw); a != 100 {
		t.Fatalf("CCW 有向面积应为 +100, 实际 %v", a)
	}
	if a := SignedArea(cw); a != -100 {
		t.Fatalf("CW 有向面积应为 -100, 实际 %v", a)
	}
}

func TestWindingCancels(t *testing.T) {
	outer := []Point{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}
	hole := []Point{{X: 3, Y: 3}, {X: 3, Y: 7}, {X: 7, Y: 7}, {X: 7, Y: 3}} // CW
	// 实体区：外环 +1，洞环 0
	if w := WindingSum([][]Point{outer, hole}, 1, 1); w != 1 {
		t.Fatalf("实体区绕数应为 1, 实际 %d", w)
	}
	// 洞内：外环 +1 与反向洞环 -1 抵消为 0
	if w := WindingSum([][]Point{outer, hole}, 5, 5); w != 0 {
		t.Fatalf("洞内绕数应抵消为 0, 实际 %d", w)
	}
	// 外部：0
	if w := WindingSum([][]Point{outer, hole}, -5, 5); w != 0 {
		t.Fatalf("外部绕数应为 0, 实际 %d", w)
	}
	// 同向洞环：洞内绕数为 +2，无法抵消
	holeSame := []Point{{X: 3, Y: 3}, {X: 7, Y: 3}, {X: 7, Y: 7}, {X: 3, Y: 7}} // CCW
	if w := WindingSum([][]Point{outer, holeSame}, 5, 5); w != 2 {
		t.Fatalf("同向内环绕数应叠加为 2, 实际 %d", w)
	}
}

func TestCheckOrientation(t *testing.T) {
	outer := []Point{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}
	holeCW := []Point{{X: 3, Y: 3}, {X: 3, Y: 7}, {X: 7, Y: 7}, {X: 7, Y: 3}}
	holeCCW := []Point{{X: 3, Y: 3}, {X: 7, Y: 3}, {X: 7, Y: 7}, {X: 3, Y: 7}}
	if err := CheckOrientation([][]Point{outer, holeCW}); err != nil {
		t.Fatalf("符号相反应通过, 实际报错: %v", err)
	}
	if err := CheckOrientation([][]Point{outer, holeCCW}); err == nil {
		t.Fatal("符号相同应报错")
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
