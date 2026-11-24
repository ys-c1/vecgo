package freqsel

import (
	"math"
	"testing"
)

func sineGray(w, h int, f float64) *Gray {
	g := MustGray(w, h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			g.Pix[y*w+x] = 128 + 90*math.Sin(2*math.Pi*f*float64(x))
		}
	}
	return g
}

func TestSigmaLevelRoundTrip(t *testing.T) {
	for l := 0; l <= 4; l++ {
		s := SigmaLevel(l)
		if got := int(math.Round(LevelForSigma(s))); got != l {
			t.Fatalf("层宽反解失败 l=%d tau=%v", l, LevelForSigma(s))
		}
	}
	if math.Abs(Sigma0-2/math.Pi) > 1e-12 {
		t.Fatalf("Sigma0 常量错误: %v", Sigma0)
	}
}

func TestFrequencyMonotonicAndRecovers(t *testing.T) {
	for _, f := range []float64{0.05, 0.10, 0.16} {
		g := sineGray(160, 80, f)
		fld := EstimateFrequency(g, 15)
		got := fld.MedianFrequency(Rect{20, 10, 140, 70})
		t.Logf("true=%.3f estimated=%.4f ratio=%.2f", f, got, got/f)
		if math.Abs(got-f) > 0.30*f+0.01 {
			t.Fatalf("主频估计偏差过大 true=%v got=%v", f, got)
		}
	}

	// 单调性：频率越高，中位估计越高。
	var meds []float64
	for _, f := range []float64{0.04, 0.09, 0.18} {
		g := sineGray(160, 80, f)
		fld := EstimateFrequency(g, 11)
		meds = append(meds, fld.MedianFrequency(Rect{20, 10, 140, 70}))
	}
	if !(meds[0] < meds[1] && meds[1] < meds[2]) {
		t.Fatalf("主频估计不单调: %v", meds)
	}
}

func TestCardCalibrationLog(t *testing.T) {
	card := NewCard(120)
	for _, win := range []int{5, 11} {
		fld := EstimateFrequency(card.Gray, win)
		t.Logf("window=%d", win)
		for cy := 0; cy < 4; cy++ {
			for cx := 0; cx < 4; cx++ {
				r := Rect{cx * 120, cy * 120, (cx + 1) * 120, (cy + 1) * 120}
				t.Logf("  tile(%d,%d) medianF=%.4f", cx, cy, fld.MedianFrequency(r))
			}
		}
	}
}

func TestAdaptiveHigherEnergyNoRinging(t *testing.T) {
	card := NewCard(120)
	st := BuildStack(card.Gray, 4)
	scales := []float64{0.5, 1.0 / 3.0, 0.25}
	for _, win := range []int{5, 11} {
		fld := EstimateFrequency(card.Gray, win)
		for _, s := range scales {
			sel := NewSelector(fld, s, DefaultOptions())
			ad := DownscaleAdaptive(st, sel)
			gl := DownscaleGlobal(st, s)
			geG, _ := GradientEnergy(gl)
			geA, _ := GradientEnergy(ad.Out)

			ow, oh := gl.W, gl.H
			ve := RingingIndex(gl, ScaleRect(card.VerticalEdge, s, ow, oh), false)
			he := RingingIndex(gl, ScaleRect(card.HorizontalEdge, s, ow, oh), true)
			raG := (ve.MeanOvershoot + he.MeanOvershoot) / 2

			veA := RingingIndex(ad.Out, ScaleRect(card.VerticalEdge, s, ow, oh), false)
			heA := RingingIndex(ad.Out, ScaleRect(card.HorizontalEdge, s, ow, oh), true)
			raA := (veA.MeanOvershoot + heA.MeanOvershoot) / 2

			ctrl := UnsharpControl(ad.Out, 1.2, 1.3)
			veC := RingingIndex(ctrl, ScaleRect(card.VerticalEdge, s, ow, oh), false)
			heC := RingingIndex(ctrl, ScaleRect(card.HorizontalEdge, s, ow, oh), true)
			raC := (veC.MeanOvershoot + heC.MeanOvershoot) / 2

			t.Logf("win=%2d s=%.3f globL=%d τN=%.3f | meanLvl=%.3f meanTau=%.3f meanSig=%.3f below=%.2f | GE g=%.0f a=%.0f (+%.1f%%) | ring g=%.3f a=%.3f ctrl=%.2f",
				win, s, sel.GlobalLevel, sel.TauN, ad.MeanLevel, ad.MeanTau, ad.MeanSigma, ad.BelowGlobal,
				geG, geA, 100*(geA/geG-1), raG, raA, raC)

			if geA <= geG {
				t.Fatalf("自适应梯度能量应高于全局: %.0f <= %.0f", geA, geG)
			}
			if raA > 0.75 {
				t.Fatalf("自适应出现明显振铃过冲: %.3f", raA)
			}
			if raC <= raA+0.5 {
				t.Fatalf("振铃阳性对照应明显更高: ctrl=%.3f adapt=%.3f", raC, raA)
			}
		}
	}
}
