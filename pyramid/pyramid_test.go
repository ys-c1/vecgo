package pyramid

import (
	"errors"
	"math"
	"testing"

	"vecgo/raster"
)

func rampImage(w, h int) *raster.Image {
	im, _ := raster.New(w, h)
	for y := 0; y < h; y++ {
		v := uint8(255 * (h - 1 - y) / (h - 1))
		for x := 0; x < w; x++ {
			im.Pix[y*w+x] = v
		}
	}
	return im
}

func constantImage(w, h int, v uint8) *raster.Image {
	im, _ := raster.New(w, h)
	for i := range im.Pix {
		im.Pix[i] = v
	}
	return im
}

func TestMaxDepth(t *testing.T) {
	cases := [][3]int{
		{256, 256, 9}, {64, 64, 7}, {1, 1, 1}, {16, 8, 5}, {0, 10, 0},
	}
	for _, c := range cases {
		if got := MaxDepth(c[0], c[1]); got != c[2] {
			t.Errorf("MaxDepth(%d,%d)=%d, 期望 %d", c[0], c[1], got, c[2])
		}
	}
}

func TestExactReconstructionAtFullDepth(t *testing.T) {
	src := rampImage(64, 64)
	p, err := FromImage(src)
	if err != nil {
		t.Fatal(err)
	}
	if p.Depth != 7 {
		t.Fatalf("Depth=%d, 期望 7", p.Depth)
	}
	got, err := p.Reconstruct(p.Depth)
	if err != nil {
		t.Fatal(err)
	}
	for i := range src.Pix {
		if got.Pix[i] != src.Pix[i] {
			t.Fatalf("第 %d 像素在全 %d 层后不一致: %d vs %d", i, p.Depth, got.Pix[i], src.Pix[i])
		}
	}
	f, _ := p.ReconstructFloat(p.Depth)
	if e := RMS(f, p.src); e > 1e-9 {
		t.Fatalf("全层浮点重建误差应为 0, 实际 %g", e)
	}
}

func TestErrorMonotonicAndEnergyIdentity(t *testing.T) {
	src := rampImage(128, 128)
	p, _ := FromImage(src)
	m := FitDecay(p.LayerErrors())
	m.Layers = p.Depth
	sweep, err := p.Sweep(m)
	if err != nil {
		t.Fatal(err)
	}
	prev := math.MaxFloat64
	for _, s := range sweep {
		if s.Measured > prev+1e-9 {
			t.Fatalf("n=%d 误差非单调: %g > 前值 %g", s.N, s.Measured, prev)
		}
		prev = s.Measured
		// Haar 正交能量恒等式：实测 RMS == sqrt(Σ 省略细节 δ²)。
		if d := math.Abs(s.Measured - s.EnergyRMSE); d > 1e-6 {
			t.Fatalf("n=%d 能量恒等式偏差 %g (meas=%g energy=%g)", s.N, d, s.Measured, s.EnergyRMSE)
		}
	}
	if last := sweep[p.Depth-1]; last.Measured != 0 {
		t.Fatalf("最末层误差应为 0, 实际 %g", last.Measured)
	}
}

func TestMinLevelsFormula(t *testing.T) {
	m := Model{A: 148, Q: 0.5, Layers: 9}
	cases := []struct {
		eps  float64
		want int
	}{
		{20, 3}, {5, 5}, {1.5, 7}, {12, 4}, {3, 6}, {0.8, 8},
		{1000, 1}, // 比最粗误差还宽松 -> 1
		{0, 9},    // 零误差目标 -> 精确层
	}
	for _, c := range cases {
		if got := m.MinLevels(c.eps); got != c.want {
			t.Errorf("eps=%v MinLevels=%d, 期望 %d (real root %.3f)",
				c.eps, got, c.want, m.RequiredN(c.eps))
		}
	}
}

func TestAnalyticMatchesExhaustiveOnRamp(t *testing.T) {
	src := rampImage(256, 256)
	p, _ := FromImage(src)
	m := FitDecay(p.LayerErrors())
	m.Layers = p.Depth
	if !(m.Q > 0.48 && m.Q < 0.52) {
		t.Fatalf("渐变衰减率应接近 0.5, 实际 q=%.4f", m.Q)
	}
	for _, eps := range []float64{20, 5, 1.5, 12, 3, 0.8} {
		a := m.MinLevels(eps)
		e := p.ExhaustiveMinLevels(eps)
		if a != e {
			t.Errorf("eps=%v 解析层 %d != 穷举层 %d", eps, a, e)
		}
	}
}

func TestConstantImageDegenerate(t *testing.T) {
	for _, v := range []uint8{0, 128, 255} {
		p, _ := FromImage(constantImage(32, 32, v))
		m := FitDecay(p.LayerErrors())
		m.Layers = p.Depth
		if m.Usable() {
			t.Fatalf("常量图不应给出可用衰减模型: q=%v A=%v", m.Q, m.A)
		}
		if n := m.MinLevels(5); n != 1 {
			t.Fatalf("常量图任意正目标应在第 1 层满足, got %d", n)
		}
		if n := p.ExhaustiveMinLevels(5); n != 1 {
			t.Fatalf("常量图穷举最小层应为 1, got %d", n)
		}
		rec, err := p.Reconstruct(1)
		if err != nil {
			t.Fatal(err)
		}
		if rec.Pix[0] != v {
			t.Fatalf("常量重建值 %d != %d", rec.Pix[0], v)
		}
	}
}

func TestDepthErrors(t *testing.T) {
	src := rampImage(16, 16)
	if _, err := Build(src, 99); err == nil {
		t.Fatal("超过 MaxDepth 的 Build 应报错")
	}
	p, _ := FromImage(src)
	if _, err := p.Reconstruct(0); err == nil {
		t.Fatal("Reconstruct(0) 应报错")
	}
	if _, err := p.Reconstruct(p.Depth + 1); err == nil {
		t.Fatal("Reconstruct(Depth+1) 应报错")
	}
	var de *DepthError
	if _, err := Build(src, 99); !errors.As(err, &de) {
		t.Fatalf("应为 *DepthError, got %T", err)
	}
}
