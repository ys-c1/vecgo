package geom

import (
	"errors"
	"math"
	"testing"

	"vecgo/param"
)

func TestBezierRejectsNaN(t *testing.T) {
	if _, err := Bezier(Point{}, Point{}, Point{}, Point{}, math.NaN()); err == nil {
		t.Fatal("t=NaN 应报错")
	}
}

func TestBezierEnds(t *testing.T) {
	a, err := Bezier(Point{X: 0, Y: 0}, Point{X: 0, Y: 1}, Point{X: 1, Y: 1}, Point{X: 1, Y: 0}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if a.X != 0 || a.Y != 0 {
		t.Fatalf("起点错误: %v", a)
	}
}

func TestFlattenCount(t *testing.T) {
	pts, err := Flatten(Point{X: 0, Y: 0}, Point{X: 0, Y: 10}, Point{X: 10, Y: 10}, Point{X: 10, Y: 0}, 8)
	if err != nil || len(pts) != 9 {
		t.Fatalf("细分点数错误: %v %d", err, len(pts))
	}
	l, err := Length(pts)
	if err != nil || l <= 10 {
		t.Fatal("长度应大于弦长")
	}
}

func TestInsideSquare(t *testing.T) {
	sq := []Point{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}
	ok, err := Inside(sq, 5, 5)
	if err != nil || !ok {
		t.Fatalf("内部点应判为内: %v %v", ok, err)
	}
	ok, err = Inside(sq, 15, 5)
	if err != nil || ok {
		t.Fatalf("外部点应判为外: %v %v", ok, err)
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

// 这些输入以前会被静默接受或直接 panic，现在必须返回带字段名的错误。
func TestInvalidInputsNameField(t *testing.T) {
	sq := []Point{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 1, Y: 1}}
	cases := []struct {
		name  string
		want  string
		run   func() error
		panic bool
	}{
		{
			name: "Flatten NaN 控制点",
			want: "geom.Flatten.p1",
			run: func() error {
				_, err := Flatten(Point{X: 0, Y: 0}, Point{X: math.NaN(), Y: 1}, Point{}, Point{}, 4)
				return err
			},
		},
		{
			name: "Flatten segments 溢出",
			want: "geom.Flatten.segments",
			run: func() error {
				_, err := Flatten(Point{}, Point{}, Point{}, Point{}, math.MaxInt)
				return err
			},
		},
		{
			name: "Inside 两个顶点",
			want: "geom.Inside.poly",
			run: func() error {
				_, err := Inside([]Point{{X: 0, Y: 0}, {X: 1, Y: 1}}, 0, 0)
				return err
			},
		},
		{
			name: "Clip 窗口含 NaN",
			want: "geom.Clip.window[0]",
			run: func() error {
				_, err := Clip(sq, []Point{{X: math.NaN()}, {X: 1, Y: 0}, {X: 1, Y: 1}})
				return err
			},
		},
		{
			name: "Stroke 单点",
			want: "geom.Stroke.pts",
			run: func() error {
				_, err := Stroke([]Point{{X: 1, Y: 1}}, 1)
				return err
			},
		},
		{
			name: "Stroke width NaN",
			want: "geom.Stroke.width",
			run: func() error {
				_, err := Stroke(sq, math.NaN())
				return err
			},
		},
		{
			name: "Stroke 重复点带下标",
			want: "geom.Stroke.pts[1]",
			run: func() error {
				_, err := Stroke([]Point{{X: 0, Y: 0}, {X: 0, Y: 0}, {X: 1, Y: 1}}, 1)
				return err
			},
		},
		{
			name: "Length 含 NaN",
			want: "geom.Length.pts[0]",
			run: func() error {
				_, err := Length([]Point{{X: math.NaN(), Y: 0}})
				return err
			},
		},
		{
			name: "Bezier t=NaN",
			want: "geom.Bezier.t",
			run: func() error {
				_, err := Bezier(Point{}, Point{}, Point{}, Point{}, math.NaN())
				return err
			},
		},
	}
	for _, tc := range cases {
		err := tc.run()
		var pe *param.Error
		if !errors.As(err, &pe) || pe.Field != tc.want {
			t.Fatalf("%s: 期望字段 %s，实际 %v", tc.name, tc.want, err)
		}
	}
}

// 空折线仍然是合法的（长度为 0），不因校验而改变。
func TestLengthEmptyOK(t *testing.T) {
	l, err := Length(nil)
	if err != nil || l != 0 {
		t.Fatalf("空折线应返回 0: %v %v", l, err)
	}
}
