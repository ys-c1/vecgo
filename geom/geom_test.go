package geom

import (
	"math"
	"testing"

	"vecgo/verr"
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

func TestErrorsCarryGeomKinds(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want verr.Kind
	}{
		{"flatten", mustErr(Flatten(Point{}, Point{}, Point{}, Point{}, 0)), verr.KindGeomSegments},
		{"clip", mustErr(Clip(nil, nil)), verr.KindGeomClipVerts},
		{"stroke-width", mustErr(Stroke([]Point{{X: 0, Y: 0}, {X: 1, Y: 0}}, 0)), verr.KindGeomStrokeW},
		{"stroke-dup", mustErr(Stroke([]Point{{X: 0, Y: 0}, {X: 0, Y: 0}}, 1)), verr.KindGeomDupPoint},
		{"stroke-single", mustErr(Stroke([]Point{{X: 1, Y: 1}}, 1)), verr.KindGeomTooFewPts},
		{"poly-few", ValidPolygon([]Point{{X: 0, Y: 0}, {X: 1, Y: 1}}), verr.KindGeomFewVerts},
		{"poly-inf", ValidPolygon([]Point{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: math.Inf(1), Y: 1}}), verr.KindGeomNonFinite},
		{"poly-nan", ValidPolygon([]Point{{X: math.NaN(), Y: 0}, {X: 1, Y: 0}, {X: 1, Y: 1}}), verr.KindGeomNonFinite},
		{"empty", RequirePoints(nil), verr.KindGeomEmptyPts},
	}
	for _, tc := range cases {
		k, ok := verr.Of(tc.err)
		if !ok {
			t.Fatalf("%s 未归一: %v", tc.name, tc.err)
		}
		if k != tc.want {
			t.Fatalf("%s 编码: 期望 %s 实际 %s", tc.name, tc.want.Hex(), k.Hex())
		}
		if k.Stage() != verr.StageGeom {
			t.Fatalf("%s 段不对: %v", tc.name, k.Stage())
		}
	}
}

func mustErr(_ any, err error) error { return err }
