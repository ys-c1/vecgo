package precision

import (
	"math"
	"testing"

	"vecgo/geom"
)

func approx(a, b float64) bool { return math.Abs(a-b) <= 1e-12*math.Max(1, math.Abs(b)) }

func TestStepNoise(t *testing.T) {
	if !approx(StepNoise(2), 10.0/1.4142135623730951/1000) {
		t.Fatalf("StepNoise(2)=%v", StepNoise(2))
	}
}

func TestAllocateDecisive(t *testing.T) {
	// 轻载、中载、重载三档与报告一致，锁住表里的数值。
	cases := []struct {
		scale float64
		eps   float64
		want  [3]int
	}{
		{0.5, 1e-2, [3]int{2, 2, 3}},
		{8, 1e-2, [3]int{3, 4, 3}},
		{25, 1e-2, [3]int{4, 4, 3}},
		{0.5, 1e-3, [3]int{3, 3, 4}},
		{8, 1e-3, [3]int{4, 5, 4}},
		{25, 1e-3, [3]int{5, 5, 4}},
		{0.5, 1e-4, [3]int{4, 4, 5}},
		{8, 1e-4, [3]int{5, 6, 5}},
		{25, 1e-4, [3]int{6, 6, 5}},
	}
	for _, c := range cases {
		w := [3]float64{c.scale, c.scale, 1}
		got, err := Allocate(w, c.eps)
		if err != nil {
			t.Fatalf("Allocate(s=%g,eps=%g): %v", c.scale, c.eps, err)
		}
		if got != c.want {
			t.Fatalf("Allocate(s=%g,eps=%g)=%v, want %v", c.scale, c.eps, got, c.want)
		}
		if b := Bound(w, got); b > c.eps {
			t.Fatalf("界 %v 超过 eps %v", b, c.eps)
		}
		// 等权重时平局取舍可能不同，但总位数与界应一致。
		if g, err := AllocateGreedy(w, c.eps); err != nil ||
			g[0]+g[1]+g[2] != got[0]+got[1]+got[2] ||
			!approx(Bound(w, g), Bound(w, got)) {
			t.Fatalf("贪心 %v (%v) 与穷举 %v 不等价", g, err, got)
		}
	}
}

func TestAllocateBeatsEqual(t *testing.T) {
	for _, scale := range []float64{0.5, 8, 25} {
		for _, eps := range []float64{1e-2, 1e-3, 1e-4} {
			w := [3]float64{scale, scale, 1}
			dg, err := Allocate(w, eps)
			if err != nil {
				t.Fatal(err)
			}
			eq, err := EqualDigits(w, eps)
			if err != nil {
				t.Fatal(err)
			}
			allocTotal := dg[0] + dg[1] + dg[2]
			if allocTotal >= 3*eq {
				t.Fatalf("s=%g eps=%g: 分配总位数 %d 未少于等精度 %d", scale, eps, allocTotal, 3*eq)
			}
			if b := Bound(w, dg); b > eps {
				t.Fatalf("分配超出上界: %v > %v", b, eps)
			}
			if b := Bound(w, [3]int{eq, eq, eq}); b > eps {
				t.Fatalf("等精度方案不满足上界: %v > %v", b, eps)
			}
		}
	}
}

func TestMeasuredWithinBound(t *testing.T) {
	ch := Chain{Scale: 25, Rotate: 2.0, Shift: geom.Point{X: 40.125, Y: -33.5}}
	pts := Grid(21, 10)
	for _, eps := range []float64{1e-2, 1e-3, 1e-4} {
		dg, err := Allocate(ch.Weights(), eps)
		if err != nil {
			t.Fatal(err)
		}
		dev, coord := ch.MaxDeviation(pts, dg)
		if dev > eps {
			t.Fatalf("实测偏差 %v 超过上界 %v", dev, eps)
		}
		if coord > dev {
			t.Fatalf("单坐标偏差 %v 不应大于欧氏偏差 %v", coord, dev)
		}
	}
}

func TestEqualDigitsIsTight(t *testing.T) {
	w := [3]float64{8, 8, 1}
	eq, err := EqualDigits(w, 1e-3)
	if err != nil {
		t.Fatal(err)
	}
	if Bound(w, [3]int{eq, eq, eq}) > 1e-3 {
		t.Fatal("等精度位数应满足上界")
	}
	if eq > 0 && Bound(w, [3]int{eq - 1, eq - 1, eq - 1}) <= 1e-3 {
		t.Fatal("等精度位数应是最少的")
	}
}

func TestAllocateRejectsBadInput(t *testing.T) {
	if _, err := Allocate([3]float64{1, 1, 1}, 0); err == nil {
		t.Fatal("eps=0 应报错")
	}
	if _, err := Allocate([3]float64{1, 1, 0}, 1e-2); err == nil {
		t.Fatal("零权重应报错")
	}
}
