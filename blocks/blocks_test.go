package blocks

import (
	"math"
	"reflect"
	"testing"
)

func TestAnalyzeEntropyAndRunUpper(t *testing.T) {
	// 2x2: 第一行全 0，第二行全 255。两个符号等概率，H=1；最长游程为 2。
	pix := []uint8{0, 0, 255, 255}
	got := Analyze(pix, 2, Rect{X: 0, Y: 0, W: 2, H: 2})
	if math.Abs(got.Entropy-1) > 1e-12 {
		t.Fatalf("Entropy = %v, 期望 1", got.Entropy)
	}
	if got.LongestRun != 2 || got.RunUpper != 4 || got.Runs != 2 {
		t.Fatalf("游程统计错误: %+v", got)
	}
	if got.WorstCaseBytes != 8 {
		t.Fatalf("最坏 RLE 上界 = %d, 期望 8", got.WorstCaseBytes)
	}
}

func TestRunCountCappedAt255(t *testing.T) {
	pix := make([]uint8, 300)
	for i := range pix {
		pix[i] = 7
	}
	got := Analyze(pix, 300, Rect{X: 0, Y: 0, W: 300, H: 1})
	if got.LongestRun != 300 || got.RunUpper != 300 {
		t.Fatalf("最长游程/上限错误: %+v", got)
	}
	if got.RLEPairs != 2 || got.RLEBytes != 4 {
		t.Fatalf("300 像素应拆成 ceil(300/255)=2 对，实际 pairs=%d bytes=%d", got.RLEPairs, got.RLEBytes)
	}

	body := EncodeRLE(pix)
	back, err := DecodeRLE(body)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(back, pix) {
		t.Fatal("长游程 RLE 往返后像素不一致")
	}
}

func TestFixedPlanEdges(t *testing.T) {
	rects, err := FixedPlan(5, 3, 2)
	if err != nil {
		t.Fatal(err)
	}
	want := []Rect{
		{Index: 0, X: 0, Y: 0, W: 2, H: 2},
		{Index: 1, X: 2, Y: 0, W: 2, H: 2},
		{Index: 2, X: 4, Y: 0, W: 1, H: 2},
		{Index: 3, X: 0, Y: 2, W: 2, H: 1},
		{Index: 4, X: 2, Y: 2, W: 2, H: 1},
		{Index: 5, X: 4, Y: 2, W: 1, H: 1},
	}
	if !reflect.DeepEqual(rects, want) {
		t.Fatalf("固定块网格错误:\n got %+v\nwant %+v", rects, want)
	}
}

func TestDisabledAdaptiveReturnsFixedBlocks(t *testing.T) {
	pix := make([]uint8, 16*16)
	for i := range pix {
		pix[i] = uint8(i % 251)
	}
	opts := DefaultOptions()
	opts.BlockSize = 8
	opts.Adaptive = false

	got, _, err := Plan(pix, 16, 16, opts)
	if err != nil {
		t.Fatal(err)
	}
	want, err := FixedPlan(16, 16, 8)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("关闭自适应时必须回到固定块:\n got %+v\nwant %+v", got, want)
	}
}

func TestAdaptiveSplitsHighEntropyBlock(t *testing.T) {
	pix := make([]uint8, 8*8)
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			if (x+y)%2 == 0 {
				pix[y*8+x] = 0
			} else {
				pix[y*8+x] = 255
			}
		}
	}
	opts := Options{BlockSize: 8, Adaptive: true, EntropyThreshold: 0.9, MinSize: 4}
	rects, stats, err := Plan(pix, 8, 8, opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(rects) != 4 || len(stats) != 4 {
		t.Fatalf("棋盘块应分裂为 4 个 4x4 叶块，实际 %d", len(rects))
	}
	for _, r := range rects {
		if r.W != 4 || r.H != 4 {
			t.Fatalf("叶块尺寸错误: %+v", r)
		}
	}
}

func TestPlanRejectsPixelLengthMismatch(t *testing.T) {
	if _, _, err := Plan([]uint8{1}, 2, 2, DefaultOptions()); err == nil {
		t.Fatal("像素长度不匹配应报错")
	}
}
