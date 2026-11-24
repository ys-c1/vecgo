package pipe

import (
	"strings"
	"testing"
	"time"

	"vecgo/config"
	"vecgo/diag"
	"vecgo/geom"
	"vecgo/verr"
)

// 用写死的快照序列把“诊断记录的时间顺序”钉住：一次成功渲染的完整骨架。
// 每段都显式标了阶段（AddStage），所以 stage-ok 也能回答“是哪一段”。
// 这些数值槽的含义见 diag.Record 的注释；测试同时是格式文档。
func TestDiagRecordShapeSuccess(t *testing.T) {
	d := diag.New(true, func() time.Time { return time.Unix(1000, 0) })
	_ = RunEx(testScene(), testCfg(), RunOpts{Diag: d})
	var b strings.Builder
	for _, r := range d.Records() {
		b.WriteString(r.Event.String())
		b.WriteString("|")
		b.WriteString(r.Stage.String())
		b.WriteString("|")
		b.WriteString(r.Kind.Hex())
		b.WriteString("\n")
	}
	got := b.String()
	want := "" +
		"start|none|0x0000\n" +
		"stage-ok|config|0x0000\n" +
		"stage-ok|raster|0x0301\n" +
		"stage-ok|geom|0x0000\n" +
		"stage-ok|raster|0x0000\n" +
		"stage-ok|shading|0x0000\n" +
		"stage-ok|post|0x0000\n" +
		"stage-ok|post|0x0000\n" +
		"finish|none|0x0000\n"
	if got != want {
		t.Fatalf("记录骨架不符：\n得到:\n%s\n期望:\n%s", got, want)
	}
}

// 失败时的骨架：失败记录带正确的段名与编码，且插在发生的位置（不是最后补记）。
func TestDiagRecordShapeFailure(t *testing.T) {
	d := diag.New(true, func() time.Time { return time.Unix(1000, 0) })
	res := runEx(testScene(), testCfg(), RunOpts{Diag: d}, failOne(verr.KindRasterSuper))
	if len(res.Failures) != 1 {
		t.Fatalf("应一条失败")
	}
	recs := d.Records()
	var sawDropped bool
	for i, r := range recs {
		if r.Kind == verr.KindRasterSuper {
			if r.Stage != verr.StageRaster {
				t.Fatalf("编码段名不符: %v", r.Stage)
			}
			if r.Event != diag.EvStageDropped && r.Event != diag.EvStageFail {
				t.Fatalf("事件不符: %v", r.Event)
			}
			// 必须在 finish 之前、在 start 之后。
			if i == 0 || i == len(recs)-1 {
				t.Fatalf("失败记录位置不对 @%d", i)
			}
			sawDropped = true
		}
	}
	if !sawDropped {
		t.Fatal("诊断里找不到光栅失败")
	}
}

// 缓冲溢出后仍保留连续的时间顺序（Seq 递增），便于还原“最后发生了什么”。
func TestDiagOverflowKeepsTimeline(t *testing.T) {
	d := diag.New(true, func() time.Time { return time.Unix(1000, 0) })
	for i := 0; i < diag.Capacity+50; i++ {
		d.Add(diag.EvStageOK, 0, int64(i), 0)
	}
	recs := d.Records()
	if len(recs) != diag.Capacity {
		t.Fatalf("条数: %d", len(recs))
	}
	if recs[0].Seq != uint64(51) {
		t.Fatalf("应保留最近的记录，首条 Seq=%d", recs[0].Seq)
	}
	for i := 1; i < len(recs); i++ {
		if recs[i].Seq != recs[i-1].Seq+1 {
			t.Fatalf("Seq 断裂 @%d", i)
		}
	}
	if d.Overflowed() != 50 {
		t.Fatalf("溢出计数: %d", d.Overflowed())
	}
}

// 注入器不能泄漏到生产路径：Run 的公开签名不暴露 inject。
func TestRunPublicAPIClean(t *testing.T) {
	scene := [][]geom.Point{{{X: 0, Y: 0}, {X: 4, Y: 0}, {X: 4, Y: 4}}}
	cfg := config.Default()
	cfg.Width, cfg.Height = 8, 8
	im, err := Run(scene, cfg)
	if err != nil {
		t.Fatalf("干净输入不应失败: %v", err)
	}
	if im.CountOn(1) == 0 {
		t.Fatal("应有像素")
	}
}
