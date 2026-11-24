package affine

import (
	"math"
	"testing"

	"vecgo/geom"
)

func rectanglePairs(dst [4]geom.Point) []Pair {
	src := []geom.Point{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}
	out := make([]Pair, 4)
	for i := range src {
		out[i] = Pair{ID: i + 1, Src: src[i], Dst: dst[i]}
	}
	return out
}

func TestSolveTwoAspectFrames(t *testing.T) {
	cases := []struct {
		name string
		dst  [4]geom.Point
		want [6]float64 // a,b,c,d,e,f
	}{
		{
			name: "16x16",
			dst:  [4]geom.Point{{X: 0, Y: 0}, {X: 16, Y: 0}, {X: 16, Y: 16}, {X: 0, Y: 16}},
			want: [6]float64{1.6, 0, 0, 1.6, 0, 0},
		},
		{
			name: "32x16 contain",
			dst:  [4]geom.Point{{X: 8, Y: 0}, {X: 24, Y: 0}, {X: 24, Y: 16}, {X: 8, Y: 16}},
			want: [6]float64{1.6, 0, 0, 1.6, 8, 0},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fit, err := Solve(rectanglePairs(tc.dst))
			if err != nil {
				t.Fatal(err)
			}
			got := fit.Matrix.Values()
			for i := range got {
				if math.Abs(got[i]-tc.want[i]) > 1e-9 {
					t.Fatalf("参数 %d = %.12g, 期望 %.12g；全部=%v", i, got[i], tc.want[i], got)
				}
			}
			if !fit.OK || fit.MaxResidual > 0.5 {
				t.Fatalf("残差超限: max=%g ok=%v", fit.MaxResidual, fit.OK)
			}
		})
	}
}

func TestSolveRecoversAllSixParameters(t *testing.T) {
	want := Matrix{A: 1.125, B: 0.25, C: -0.0625, D: 0.875, E: 4.5, F: -2.25}
	src := []geom.Point{{X: 1, Y: 2}, {X: 7, Y: -1}, {X: 3, Y: 9}, {X: -4, Y: 5}}
	pairs := make([]Pair, len(src))
	for i, p := range src {
		pairs[i] = Pair{ID: i + 1, Src: p, Dst: want.Apply(p)}
	}
	fit, err := Solve(pairs)
	if err != nil {
		t.Fatal(err)
	}
	got := fit.Matrix.Values()
	wantVals := want.Values()
	for i := range got {
		if math.Abs(got[i]-wantVals[i]) > 1e-9 {
			t.Fatalf("参数 %d = %.12g, 期望 %.12g", i, got[i], wantVals[i])
		}
	}
	if fit.MaxResidual > 1e-9 {
		t.Fatalf("精确对应不应有残差: %g", fit.MaxResidual)
	}
}

func TestIncompatibleCornersIdentifyDiagonalPairs(t *testing.T) {
	dst := [4]geom.Point{{X: 0, Y: 0}, {X: 16, Y: 0}, {X: 18.4, Y: 14.2}, {X: 0, Y: 16}}
	fit, err := Solve(rectanglePairs(dst))
	if err != nil {
		t.Fatal(err)
	}
	if fit.OK {
		t.Fatal("注入 3.0px 扰动后应判定残差超限")
	}
	if math.Abs(fit.MaxResidual-0.75) > 1e-9 {
		t.Fatalf("最大残差 = %.12g, 期望 0.75", fit.MaxResidual)
	}
	if fit.Diagonal.PairA != [2]int{1, 3} || fit.Diagonal.PairB != [2]int{2, 4} {
		t.Fatalf("冲突交点对错误: %+v", fit.Diagonal)
	}
	if math.Abs(fit.Diagonal.Closure.X-2.4) > 1e-9 || math.Abs(fit.Diagonal.Closure.Y+1.8) > 1e-9 {
		t.Fatalf("闭合差错误: %+v", fit.Diagonal.Closure)
	}
	if len(fit.BadIDs) != 4 {
		t.Fatalf("四角残差都应越限，实际 BadIDs=%v", fit.BadIDs)
	}
}

func TestSolveRejectsDuplicateID(t *testing.T) {
	pairs := rectanglePairs([4]geom.Point{})
	pairs[1].ID = pairs[0].ID
	if _, err := Solve(pairs); err == nil {
		t.Fatal("重复编号应报错")
	}
}
