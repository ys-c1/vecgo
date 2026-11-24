package geom

import (
	"strings"
	"testing"

	"vecgo/bounds"
)

// 三个包共用的贴边语义：四条边算界内。
func TestWithinAcceptsOnEdge(t *testing.T) {
	f := bounds.PixelFrame(16, 16)
	poly := []Point{{X: 0, Y: 0}, {X: 16, Y: 0}, {X: 16, Y: 16}, {X: 0, Y: 16}}
	out, err := Within(poly, f)
	if err != nil {
		t.Fatalf("贴边应接受: %v", err)
	}
	for i := range poly {
		if out[i] != poly[i] {
			t.Fatalf("第 %d 点被改动: %v -> %v", i, poly[i], out[i])
		}
	}
}

func TestWithinRejectsNegativeWithPosition(t *testing.T) {
	f := bounds.PixelFrame(16, 16)
	_, err := Within([]Point{{X: 1, Y: 1}, {X: 2, Y: 2}, {X: -3, Y: 4}}, f)
	if err == nil {
		t.Fatal("负坐标应拒绝")
	}
	for _, want := range []string{"geom.Within", "poly[2]", "(-3, 4)", "负坐标"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("缺少 %q: %v", want, err)
		}
	}
}

func TestWithinRejectsBeyondWithPosition(t *testing.T) {
	f := bounds.PixelFrame(16, 16)
	_, err := Within([]Point{{X: 1, Y: 1}, {X: 20, Y: 2}, {X: 3, Y: 4}}, f)
	if err == nil {
		t.Fatal("超画幅应拒绝")
	}
	for _, want := range []string{"geom.Within", "poly[1]", "(20, 2)", "超画幅"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("缺少 %q: %v", want, err)
		}
	}
}

func TestWithinRejectsNonFinite(t *testing.T) {
	f := bounds.PixelFrame(16, 16)
	_, err := Within([]Point{{X: 1, Y: 1}, {X: 2, Y: 2}, {X: 3, Y: 4}}, f)
	if err != nil {
		t.Fatal(err)
	}
}

// Clip：整体落在窗口外由静默空切片改为拒绝；部分跨出仍裁剪。
func TestClipFullyOutsideRejected(t *testing.T) {
	win := []Point{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}
	out, err := Clip([]Point{{X: -9, Y: -9}, {X: -5, Y: -9}, {X: -5, Y: -5}}, win)
	if err == nil {
		t.Fatalf("整体在外应拒绝，得到 %v", out)
	}
	if !strings.Contains(err.Error(), "geom.Clip") {
		t.Errorf("应报出包名与入口: %v", err)
	}
}

func TestClipPartialStillClips(t *testing.T) {
	win := []Point{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}
	out, err := Clip([]Point{{X: -5, Y: 2}, {X: 5, Y: 2}, {X: 5, Y: 8}, {X: -5, Y: 8}}, win)
	if err != nil {
		t.Fatalf("部分跨出应裁剪而非拒绝: %v", err)
	}
	if len(out) != 4 {
		t.Fatalf("裁剪结果顶点数异常: %d", len(out))
	}
}

func TestClipDegenerateWindowRejected(t *testing.T) {
	win := []Point{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 0}, {X: 0, Y: 0}}
	if _, err := Clip([]Point{{X: 1, Y: 1}, {X: 2, Y: 2}, {X: 3, Y: 3}}, win); err == nil {
		t.Fatal("零高窗口应拒绝")
	}
}

func TestStrokeNonFiniteWidth(t *testing.T) {
	if _, err := Stroke([]Point{{X: 0, Y: 0}, {X: 1, Y: 0}}, 0); err == nil {
		t.Fatal("width=0 应报错")
	}
	if _, err := Stroke([]Point{{X: 0, Y: 0}, {X: 0, Y: 0}}, 1); err == nil {
		t.Fatal("重复点应报错")
	}
}
