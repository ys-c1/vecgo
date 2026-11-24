package post

import "testing"

func TestSolveSRGBBreakpoints(t *testing.T) {
	eps := 0.5 / 255
	pw, err := NewSRGBPiecewise(eps)
	if err != nil {
		t.Fatal(err)
	}
	if len(pw.Xs) < 3 {
		t.Fatalf("段数过少: %d", len(pw.Xs)-1)
	}
	if pw.Xs[0] != 0 || pw.Xs[len(pw.Xs)-1] != 1 {
		t.Fatalf("端点应为 0 和 1: %v %v", pw.Xs[0], pw.Xs[len(pw.Xs)-1])
	}
	for i := 1; i < len(pw.Xs); i++ {
		if pw.Xs[i] <= pw.Xs[i-1] {
			t.Fatalf("断点应严格递增: %v", pw.Xs)
		}
	}
	// 稠密采样上偏差不得超过 eps（允许 1e-9 数值余量）
	if _, dev := pw.MaxDev(100000); dev > eps+1e-9 {
		t.Fatalf("最大偏差 %v 超过 eps %v", dev, eps)
	}
}

func TestPiecewiseTableCloseToExact(t *testing.T) {
	pw, err := NewSRGBPiecewise(0.5 / 255)
	if err != nil {
		t.Fatal(err)
	}
	exact, approx := SRGBTable(), pw.Table()
	for i := 0; i < 256; i++ {
		d := int(exact[i]) - int(approx[i])
		if d < 0 {
			d = -d
		}
		if d > 1 {
			t.Fatalf("表项 %d 差 %d 级，超过 1", i, d)
		}
	}
}
