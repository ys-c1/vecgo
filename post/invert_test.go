package post

import (
	"math"
	"testing"
)

// 三组目标半径：反解组合须与穷举最优一致，且串联残差低于单次大核对照。
func TestSolveCascadePair(t *testing.T) {
	for _, R := range []int{2, 4, 8} {
		sigma := SigmaForRadius(R)
		half := KernelHalf(sigma, 16)
		r1, r2, cont, res := SolveCascadePair(sigma, half)
		e1, e2, _ := ExhaustiveCascadePair(sigma, half, 16)
		if r1 != e1 || r2 != e2 {
			t.Errorf("R=%d: 反解 (%d,%d) 与穷举最优 (%d,%d) 不一致", R, r1, r2, e1, e2)
		}
		ctrl := ControlResidual(R, sigma, half)
		if res >= ctrl {
			t.Errorf("R=%d: 串联残差 %e 应小于对照 %e", R, res, ctrl)
		}
		t.Logf("R=%2d σ=%.4f 连续解=(%.4f,%.4f) 组合=(%d,%d) 穷举=(%d,%d) E串联=%.6e E对照=%.6e 比值=%.3f",
			R, sigma, cont[0], cont[1], r1, r2, e1, e2, res, ctrl, res/ctrl)
	}
}

// 图像级验证：串联结果与目标核直接卷积的差，应小于单次大核路径。
func TestCascadeBeatsControlOnImage(t *testing.T) {
	const w, h = 40, 40
	img := make([]float64, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			v := float64(x+y) / 80 // 缓变梯度
			if x >= 12 && x < 28 && y >= 12 && y < 28 {
				v = 1 // 亮方块（硬边）
			}
			img[y*w+x] = v
		}
	}
	img[5*w+5] = 1 // 孤立亮点
	for _, R := range []int{2, 4, 8} {
		sigma := SigmaForRadius(R)
		half := KernelHalf(sigma, 16)
		r1, r2, _, _ := SolveCascadePair(sigma, half)
		target := applySep(img, w, h, gaussKernel1D(sigma, half))
		cascade := applySep(applySep(img, w, h, boxKernel1D(float64(r1), half)), w, h, boxKernel1D(float64(r2), half))
		control := applySep(img, w, h, boxKernel1D(float64(R), half))
		rmsCas, rmsCtrl := rms(cascade, target), rms(control, target)
		if rmsCas >= rmsCtrl {
			t.Errorf("R=%d: 串联与目标卷积的 RMS %f 应小于对照 %f", R, rmsCas, rmsCtrl)
		}
		t.Logf("R=%2d 组合=(%d,%d) RMS串联=%.5f RMS对照=%.5f", R, r1, r2, rmsCas, rmsCtrl)
	}
}

// applySep 水平、垂直两遍施加归一化一维核，边界按有效权重归一（同 Blur）。
func applySep(pix []float64, w, h int, k []float64) []float64 {
	r := len(k) / 2
	tmp := make([]float64, w*h)
	out := make([]float64, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			s, ws := 0.0, 0.0
			for j := -r; j <= r; j++ {
				if nx := x + j; nx >= 0 && nx < w {
					s += pix[y*w+nx] * k[j+r]
					ws += k[j+r]
				}
			}
			tmp[y*w+x] = s / ws
		}
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			s, ws := 0.0, 0.0
			for j := -r; j <= r; j++ {
				if ny := y + j; ny >= 0 && ny < h {
					s += tmp[ny*w+x] * k[j+r]
					ws += k[j+r]
				}
			}
			out[y*w+x] = s / ws
		}
	}
	return out
}

func rms(a, b []float64) float64 {
	s := 0.0
	for i := range a {
		d := a[i] - b[i]
		s += d * d
	}
	return math.Sqrt(s / float64(len(a)))
}
