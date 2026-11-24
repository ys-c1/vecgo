package band

import "testing"

func TestCountRunsAndDistinct(t *testing.T) {
	// 4 个等值段：255,255 | 200 | 10,10,10 | 0
	col := []uint8{255, 255, 200, 10, 10, 10, 0}
	if got := CountRuns(col); got != 4 {
		t.Fatalf("runs=%d want 4", got)
	}
	if got := CountDistinct(col); got != 4 {
		t.Fatalf("distinct=%d want 4", got)
	}
}

func TestCriticalCodeGap(t *testing.T) {
	// m_c(k)=w*k/gamma；w=0.10, gamma=2.2, k=110 -> 5.0
	if got := CriticalCodeGap(110, 0.10, 2.2); got < 4.99 || got > 5.01 {
		t.Fatalf("m_c(110)=%.3f want ~5.0", got)
	}
}

func TestLinearContinuousPrediction(t *testing.T) {
	// 10% Weber 临界 -> 约 1/w = 10，闭式取整为 11（含端步约定）。
	if got := LinearSpaceContinuousPrediction(0.10); got != 11 {
		t.Fatalf("continuous=%d want 11", got)
	}
}

func TestPredictColumnEndpoints(t *testing.T) {
	for _, sp := range []string{"encoded", "linear", "perceptual"} {
		col := PredictColumn(64, sp, nil)
		if col[0] != 255 || col[63] != 0 {
			t.Fatalf("space=%s 端点错误: top=%d bottom=%d", sp, col[0], col[63])
		}
		if CountRuns(col) != 64 {
			t.Fatalf("space=%s H=64 应每行不同，runs=%d", sp, CountRuns(col))
		}
	}
}
