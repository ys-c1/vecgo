package shading

import (
	"testing"

	"vecgo/ramp"
	"vecgo/raster"
)

func customerStops() []ramp.Stop {
	return []ramp.Stop{
		{X: 0.00, Y: 0.970}, {X: 0.15, Y: 0.850}, {X: 0.30, Y: 0.625}, {X: 0.45, Y: 0.385},
		{X: 0.58, Y: 0.177}, {X: 0.73, Y: 0.161}, {X: 0.88, Y: 0.143}, {X: 1.00, Y: 0.133},
	}
}

func TestApplyCubicRampBeforeAfter(t *testing.T) {
	// 事故版本必须复现过冲。
	im0, err := raster.New(64, 64)
	if err != nil {
		t.Fatal(err)
	}
	c0, err := ApplyCubicRamp(im0, customerStops(), false)
	if err != nil {
		t.Fatal(err)
	}
	if px, _, _, _ := c0.OvershootPixels(im0); px == 0 {
		t.Fatal("朴素切线应当产生过冲像素")
	}

	// 修复版本过冲必须为零，且亮部控制点偏离为零。
	im1, err := raster.New(64, 64)
	if err != nil {
		t.Fatal(err)
	}
	c1, err := ApplyCubicRamp(im1, customerStops(), true)
	if err != nil {
		t.Fatal(err)
	}
	if px, rows, _, _ := c1.OvershootPixels(im1); px != 0 || rows != 0 {
		t.Fatalf("修复后仍有过冲 px=%d rows=%d", px, rows)
	}
	if bad, ex := c1.OvershootDense(800); bad != 0 || ex != 0 {
		t.Fatalf("连续过冲未清零 bad=%d ex=%v", bad, ex)
	}
	if d := c1.KnotDeviation([]int{0, 1, 2, 3}); d > 1e-12 {
		t.Fatalf("亮部控制点偏离 %v", d)
	}
}
