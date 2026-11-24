//go:build vecgotiming

package timing

import "testing"

func TestReportAccumulatesInStageOrder(t *testing.T) {
	Reset()
	Start(StageRasterize)
	Done(StageRasterize)
	Start(StageGamma)
	Done(StageGamma)

	r := Report()
	if len(r) != 2 {
		t.Fatalf("应记录两个阶段，实际 %d", len(r))
	}
	// Report 必须按管道执行顺序输出，与调用先后无关。
	if r[0].Stage != StageRasterize || r[1].Stage != StageGamma {
		t.Fatalf("阶段顺序错误: %v", r)
	}
	if r[0].TotalNs <= 0 || r[1].TotalNs <= 0 {
		t.Fatal("累计耗时应为正")
	}

	Reset()
	if len(Report()) != 0 {
		t.Fatal("Reset 后应清空累计值")
	}
}
