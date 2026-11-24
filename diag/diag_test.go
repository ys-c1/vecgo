package diag

import (
	"testing"
	"time"

	"vecgo/verr"
)

// fixedClock 返回恒定时间的 clock，避免计时器本身成为不确定因素。
func fixedClock(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

// 常开 / 关闭两种状态下，各入口的堆分配前后差值都必须为 0。
// 差值用 testing.AllocsPerRun 的 before/after 相减，
// 直接把“诊断本身没有多出任何分配”钉在测试里。
func TestRecorderAllocsZeroWhenOn(t *testing.T) {
	clock := fixedClock(time.Unix(1000, 0))
	r := New(true, clock)
	// 预热：先让缓冲内槽位都写过一遍，AllocsPerRun 期间不再有首次触碰。
	for i := 0; i < Capacity+8; i++ {
		r.Add(EvStageOK, verr.KindConfigWidth, int64(i), 0)
	}

	before := testing.AllocsPerRun(2000, func() {
		r.Add(EvStageOK, verr.KindGeomSegments, 3, 4)
	})
	after := testing.AllocsPerRun(2000, func() {
		r.Add(EvStageOK, verr.KindGeomSegments, 3, 4)
	})
	if before != 0 || after != 0 {
		t.Fatalf("常开诊断有分配: before=%v after=%v", before, after)
	}
	if d := after - before; d != 0 {
		t.Fatalf("前后差值非零: %v", d)
	}

	// 其余入口逐项同样测一遍。
	check := func(name string, fn func()) {
		t.Helper()
		b := testing.AllocsPerRun(1000, fn)
		a := testing.AllocsPerRun(1000, fn)
		if b != 0 || a != 0 || a-b != 0 {
			t.Fatalf("%s 分配非零: before=%v after=%v diff=%v", name, b, a, a-b)
		}
	}
	check("AddText", func() { r.AddText(EvStageFail, verr.KindPostGamma, 1, "gamma") })
	check("ObserveFail", func() { r.ObserveFail(verr.KindRasterSize) })
	check("ObserveDrop", func() { r.ObserveDrop(verr.StageRaster) })
	check("Offset", func() { _ = r.Offset() })
	check("On", func() { _ = r.On() })
}

func TestRecorderAllocsZeroWhenOff(t *testing.T) {
	r := New(false, fixedClock(time.Unix(1000, 0)))
	before := testing.AllocsPerRun(5000, func() {
		r.Add(EvStageOK, verr.KindConfigWidth, 1, 2)
		r.AddText(EvStageOK, verr.KindConfigWidth, 1, "x")
		r.ObserveFail(verr.KindPostGamma)
		r.ObserveDrop(verr.StagePost)
		_ = r.On()
		_ = r.Offset()
	})
	after := testing.AllocsPerRun(5000, func() {
		r.Add(EvStageOK, verr.KindConfigWidth, 1, 2)
		r.AddText(EvStageOK, verr.KindConfigWidth, 1, "x")
		r.ObserveFail(verr.KindPostGamma)
		r.ObserveDrop(verr.StagePost)
		_ = r.On()
		_ = r.Offset()
	})
	if before != 0 || after != 0 || after-before != 0 {
		t.Fatalf("关闭诊断有分配: before=%v after=%v diff=%v", before, after, after-before)
	}
}

// 顺序：Seq 严格递增，Offset 单调不减，与写入先后一致。
func TestRecorderOrder(t *testing.T) {
	ts := time.Unix(1000, 0)
	step := int64(0)
	clock := func() time.Time {
		step += 1000
		return ts.Add(time.Duration(step))
	}
	r := New(true, clock)
	r.Add(EvStart, 0, 0, 0)
	r.Add(EvStageOK, verr.KindConfigWidth, 1, 0)
	r.Add(EvStageFail, verr.KindRasterSize, 2, 0)
	r.Add(EvFinish, 0, 0, 0)

	recs := r.Records()
	if len(recs) != 4 {
		t.Fatalf("条数: %d", len(recs))
	}
	var seq uint64
	var off int64 = -1
	for _, rec := range recs {
		if rec.Seq <= seq {
			t.Fatalf("Seq 未递增: %v -> %v", seq, rec.Seq)
		}
		if rec.Offset < off {
			t.Fatalf("Offset 倒退: %v -> %v", off, rec.Offset)
		}
		seq, off = rec.Seq, rec.Offset
	}
	if recs[0].Event != EvStart || recs[3].Event != EvFinish {
		t.Fatalf("首尾事件不对: %v %v", recs[0].Event, recs[3].Event)
	}
}

// 环形缓冲：超过容量后保留最早与最近，且顺序仍连续。
func TestRecorderWrapKeepsOrder(t *testing.T) {
	r := New(true, fixedClock(time.Unix(1000, 0)))
	n := Capacity + 37
	for i := 0; i < n; i++ {
		r.Add(EvStageOK, verr.KindGeomSegments, int64(i), 0)
	}
	if r.Overflowed() != uint64(n-Capacity) {
		t.Fatalf("溢出计数: %d", r.Overflowed())
	}
	recs := r.Records()
	if len(recs) != Capacity {
		t.Fatalf("可取条数: %d", len(recs))
	}
	// 环形缓冲只保最近 Capacity 条；顺序必须从 n-Capacity+1 连续到 n。
	for i := 1; i < len(recs); i++ {
		if recs[i].Seq != recs[i-1].Seq+1 {
			t.Fatalf("顺序断裂: [%d] %d -> [%d] %d", i-1, recs[i-1].Seq, i, recs[i].Seq)
		}
	}
	if recs[len(recs)-1].A != int64(n-1) {
		t.Fatalf("最后一条不是最新记录")
	}
}
