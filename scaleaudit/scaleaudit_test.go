package scaleaudit

import (
	"math"
	"testing"
)

func TestAreaPredictionUsesSquaredRatio(t *testing.T) {
	// 满档灰度 127：缩到一半，面积比 1/4 → 31.75 ≈ 32；
	// 错误的边长比 1/2 → 63.5 ≈ 64。两者必须不同，且主预测走平方。
	if got := AreaGray(127, 0.5, 1); math.Abs(got-31.75) > 1e-12 {
		t.Fatalf("面积比预测应为 31.75, got %v", got)
	}
	if got := QuantizePred(AreaGray(127, 0.5, 1)); got != 32 {
		t.Fatalf("面积预测取整应为 32, got %d", got)
	}
	if got := QuantizePred(LinearGray(127, 0.5, 1)); got != 64 {
		t.Fatalf("边长预测取整应为 64, got %d", got)
	}
	// 任意相邻半档面积比是边长比的平方关系：(s/s0)^2。
	if got := AreaGray(200, 0.75, 1); math.Abs(got-112.5) > 1e-9 {
		t.Fatalf("0.75 档面积预测应为 112.5, got %v", got)
	}
}

func TestShrinkBreaks(t *testing.T) {
	// 降序（全尺寸→一半）下，缩小却变亮 G[i+1]>G[i] 即破坏。
	if b := ShrinkBreaks([]int{127, 191, 127, 127, 63}); len(b) != 1 || b[0] != 0 {
		t.Fatalf("应只在第 0 对发现一次反升, got %v", b)
	}
	if b := ShrinkBreaks([]int{255, 191, 127, 63, 0}); len(b) != 0 {
		t.Fatalf("单调不增序列不应有破坏, got %v", b)
	}
	if b := ShrinkBreaks([]int{10, 10, 10}); len(b) != 0 {
		t.Fatalf("相等满足非增, 不应报破坏, got %v", b)
	}
}

func TestFixtureTableAndBreakLocation(t *testing.T) {
	rep, err := Build()
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Levels) != 5 {
		t.Fatalf("应有五个缩放档, got %d", len(rep.Levels))
	}
	wantScales := []float64{1, 0.875, 0.75, 0.625, 0.5}
	wantCols := []int{50, 45, 40, 35, 30}
	wantHits := []int{2, 3, 2, 2, 1}
	wantGray := []int{127, 191, 127, 127, 63}
	for i, l := range rep.Levels {
		if math.Abs(l.Scale-wantScales[i]) > 1e-12 {
			t.Fatalf("档%d scale=%v want %v", i, l.Scale, wantScales[i])
		}
		if l.Row != ScanRow || l.Col != wantCols[i] {
			t.Fatalf("档%d 像素=(%d,%d) want col=%d row=%d", i, l.Col, l.Row, wantCols[i], ScanRow)
		}
		if l.Hits != wantHits[i] || l.Den != 4 {
			t.Fatalf("档%d hits=%d/%d want %d/4", i, l.Hits, l.Den, wantHits[i])
		}
		if l.Gray != wantGray[i] {
			t.Fatalf("档%d gray=%d want %d", i, l.Gray, wantGray[i])
		}
	}

	if len(rep.Breaks) != 1 {
		t.Fatalf("主扫描行应恰有 1 处破坏, got %d", len(rep.Breaks))
	}
	br := rep.Breaks[0]
	if br.Pair != 0 || br.Rise != 64 {
		t.Fatalf("破坏对应为档1→2 且反升 64, got pair=%d rise=%d", br.Pair, br.Rise)
	}
	if br.PixelFrom != [2]int{50, 21} || br.GrayFrom != 127 {
		t.Fatalf("破坏较大档端定位错误: %+v", br)
	}
	if br.PixelTo != [2]int{45, 21} || br.GrayTo != 191 {
		t.Fatalf("破坏异常峰定位错误: %+v", br)
	}
	if br.Location == "" {
		t.Fatal("必须给出人话破坏位置")
	}

	// 整条边上的「抗锯齿中间灰」破坏也应只有这一处（row=21）。
	if len(rep.EdgePartialBreaks) != 1 {
		t.Fatalf("抗锯齿边缘灰破坏应只有 1 处, got %d: %+v", len(rep.EdgePartialBreaks), rep.EdgePartialBreaks)
	}
	if rep.EdgePartialBreaks[0].Row != 21 {
		t.Fatalf("破坏行应为 21, got %d", rep.EdgePartialBreaks[0].Row)
	}
}

func TestFixedPixelInequalityHoldsEverywhere(t *testing.T) {
	rep, err := Build()
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.FixedPairs) != 4 {
		t.Fatalf("应有 4 对相邻档, got %d", len(rep.FixedPairs))
	}
	for _, fp := range rep.FixedPairs {
		if fp.BrighterViolations != 0 {
			t.Fatalf("s=%.3f→%.3f 固定像素出现 %d 个缩小反亮，违反覆盖率单调性",
				fp.ScaleA, fp.ScaleB, fp.BrighterViolations)
		}
		if fp.DarkerPixels+fp.EqualPixels != Width*Height {
			t.Fatalf("变暗+不变应覆盖全部像素, %+v", fp)
		}
	}
}

func TestPatchAreaLawBeatsLinearLaw(t *testing.T) {
	rep, err := Build()
	if err != nil {
		t.Fatal(err)
	}
	pc := rep.Patch
	if pc == nil || len(pc.Points) != 5 {
		t.Fatal("缺少面积律受控实验的五个点")
	}
	if !(pc.AreaRMSE < pc.LinearRMSE) {
		t.Fatalf("面积预测 RMSE 应小于边长预测: area=%.2f linear=%.2f", pc.AreaRMSE, pc.LinearRMSE)
	}
	// 固定像素、嵌套伸缩：实测灰度降序必须非增。
	for i := 0; i+1 < len(pc.Points); i++ {
		if pc.Points[i+1].Gray > pc.Points[i].Gray {
			t.Fatalf("覆盖块灰度在 %v→%v 反升", pc.Points[i].Scale, pc.Points[i+1].Scale)
		}
	}
}
