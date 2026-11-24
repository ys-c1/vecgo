package colorspace

import (
	"math"
	"testing"
)

func near(a, b, eps float64) bool { return math.Abs(a-b) <= eps }

func TestSRGBKnownPoints(t *testing.T) {
	// IEC 常数与经典样点。
	if SRGBToLinear(0) != 0 || SRGBToLinear(1) != 1 {
		t.Fatal("sRGB 端点必须为 0/1")
	}
	if got := SRGBToLinear(0.5); !near(got, 0.214041, 1e-5) {
		t.Fatalf("srgb->linear(0.5)=%.6f want ~0.214041", got)
	}
	if got := LinearToSRGB(0.214041); !near(got, 0.5, 1e-4) {
		t.Fatalf("linear->srgb(0.214)=%.5f want 0.5", got)
	}
	// 暗部线性段：c<=0.04045 时 Y=c/12.92。
	if got := SRGBToLinear(0.03); !near(got, 0.03/12.92, 1e-12) {
		t.Fatalf("暗部线性段错误: %.6f", got)
	}
}

func TestRoundTrip(t *testing.T) {
	for _, c := range []float64{0, 0.01, 0.1, 0.25, 0.5, 0.75, 0.9, 1} {
		back := LinearToSRGB(SRGBToLinear(c))
		if !near(back, c, 1e-9) {
			t.Fatalf("sRGB 往返失败 c=%v back=%v", c, back)
		}
	}
	// L* 端点与中灰。
	if LinearToLStar(0) != 0 || LinearToLStar(1) != 100 {
		t.Fatal("L* 端点应为 0/100")
	}
	if got := LinearToLStar(0.214041); !near(got, 53.39, 0.05) {
		t.Fatalf("L*(0.214)=%.2f want ~53.39", got)
	}
	for _, y := range []float64{0.001, 0.01, 0.2, 0.5, 1} {
		if back := LStarToLinear(LinearToLStar(y)); !near(back, y, 1e-9) {
			t.Fatalf("L* 往返失败 y=%v back=%v", y, back)
		}
	}
}
