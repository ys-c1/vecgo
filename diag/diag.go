// Package diag 是诊断出口。默认关闭，关闭时所有入口都是空操作；
// 常开时每个入口都不做堆分配（所有缓冲都在创建时一次性申请，
// 记录只做覆盖写，满了就保留最早的一段与最近的，见 Overflowed）。
//
// 记录带时间顺序：每条 Record 都带单调递增的 Seq 与自 Clock 起算的
// 纳秒偏移。Seq 用来还原“谁先谁后”，Offset 用来还原“隔了多久”；
// 两者单调，不依赖墙钟，也不受系统时间调整影响。Clock 本身也是调用方
// 注入的（New 时传 time.Now），所以 diag 包自己没有计时源。
//
// 分配约束的验证方式见 diag_test.go：用 testing.AllocsPerRun 取
// 前后差值，要求诊断关闭与常开两种状态下差值都为 0。
package diag

import (
	"time"

	"vecgo/verr"
)

// Recorder 是顺序记录器。零值不可用，必须用 New 或装进 Sink。
type Recorder struct {
	on    bool
	recs  []Record
	head  int    // 环形缓冲的下一条写入位置
	count uint64 // 累计写入条数（可能大于缓冲容量）
	start time.Time
	now   func() time.Time

	// 计数器：常开时要能回答“记了多少、丢了多少”，且不额外分配。
	written  uint64
	overflow uint64
	stageErr [7]uint64 // 按 verr.Stage 计失败次数
	dispatch [7]uint64 // 按 verr.Stage 计被摘掉的次数
}

// Record 是一条诊断记录。字段全部是值类型，写入时零分配。
//
// 数值槽 A/B 的语义由 Event 与调用点约定（见 pipe 的记录点）：
//
//	EvStart        A=场景多边形数, B=画幅像素数
//	EvStageOK      A/B=该段的指纹（config:宽高；raster-new:宽高；
//	               geom:有效/总数；raster-fill:亮像素数/有效多边形数；
//	               shading:0/0；gamma:0/1；blur:0/2）
//	EvStageDropped A=int(Stage), B=本次渲染里的第几条失败
//	EvFinish       A=失败总数, B=最终图亮像素数
type Record struct {
	Seq    uint64
	Offset int64 // 距 Clock 起点的纳秒
	Kind   verr.Kind
	Stage  verr.Stage
	Event  Event
	A, B   int64  // 见上面的槽位说明
	Text   string // 只允许指向常量或调用方预置字符串；为空即空
}

// Event 是记录类型。
type Event uint8

const (
	EvStart Event = iota + 1
	EvStageOK
	EvStageFail
	EvStageDropped
	EvStageSkip
	EvFinish
	EvSaveOK
	EvSaveFail
	EvConfigNote
)

func (e Event) String() string {
	switch e {
	case EvStart:
		return "start"
	case EvStageOK:
		return "stage-ok"
	case EvStageFail:
		return "stage-fail"
	case EvStageDropped:
		return "stage-dropped"
	case EvStageSkip:
		return "stage-skip"
	case EvFinish:
		return "finish"
	case EvSaveOK:
		return "save-ok"
	case EvSaveFail:
		return "save-fail"
	case EvConfigNote:
		return "config-note"
	}
	return "unknown"
}

// Capacity 是环形缓冲的容量。五段各最多 3 条记录（start/finish/计数），
// 一次完整渲染远低于 128 条；留 256 的余量够放异常路径与重试。
const Capacity = 256

// New 创建记录器。on=false 时返回的记录器所有入口都短路，不申请缓冲。
func New(on bool, now func() time.Time) *Recorder {
	if now == nil {
		now = time.Now
	}
	r := &Recorder{on: on}
	if on {
		r.start = now()
		r.recs = make([]Record, Capacity)
		r.now = now
	}
	return r
}

// On 报告诊断是否常开。
func (r *Recorder) On() bool { return r != nil && r.on }

// Offset 是相对起点的纳秒偏移；关闭时返回 0。
func (r *Recorder) Offset() int64 {
	if !r.On() {
		return 0
	}
	return int64(r.now().Sub(r.start))
}

// Add 追加一条记录，阶段由 k 的段位推导。常开时零分配；关闭时直接返回。
// 缓冲满后覆盖最旧的一条，Overflow 计数加一。
func (r *Recorder) Add(ev Event, k verr.Kind, a, b int64) {
	if !r.On() {
		return
	}
	r.push(Record{
		Seq:    r.count + 1,
		Offset: int64(r.now().Sub(r.start)),
		Kind:   k,
		Stage:  k.Stage(),
		Event:  ev,
		A:      a,
		B:      b,
	})
}

// AddStage 追加一条记录并显式指定阶段。用于 stage-ok 这类“没有失败编码、
// 但必须知道是哪一段”的记录：k 可以为 0，Stage 仍正确写明。
// 常开时零分配。
func (r *Recorder) AddStage(ev Event, s verr.Stage, k verr.Kind, a, b int64) {
	if !r.On() {
		return
	}
	r.push(Record{
		Seq:    r.count + 1,
		Offset: int64(r.now().Sub(r.start)),
		Kind:   k,
		Stage:  s,
		Event:  ev,
		A:      a,
		B:      b,
	})
}

// push 是两条追加路径共用的入队逻辑：分离出来避免重复，且仍零分配。
func (r *Recorder) push(rec Record) {
	r.count++
	r.written++
	if r.count > Capacity {
		r.overflow++
	}
	r.recs[r.head] = rec
	r.head = (r.head + 1) % Capacity
}

// AddText 追加一条带文案的记录。text 必须是常量或调用方长期持有的字符串，
// 记录器不会复制它。这是为了零分配。
func (r *Recorder) AddText(ev Event, k verr.Kind, a int64, text string) {
	if !r.On() {
		return
	}
	r.push(Record{
		Seq:    r.count + 1,
		Offset: int64(r.now().Sub(r.start)),
		Kind:   k,
		Stage:  k.Stage(),
		Event:  ev,
		A:      a,
		Text:   text,
	})
}

// ObserveFail 按阶段记一次失败（不影响记录顺序，只做计数）。
// 计数是诊断的一部分，也要求零分配。
func (r *Recorder) ObserveFail(k verr.Kind) {
	if !r.On() {
		return
	}
	if s := k.Stage(); s != verr.StageNone && int(s) < len(r.stageErr) {
		r.stageErr[s]++
	}
}

// ObserveDrop 按阶段记一次“该段被摘掉”。
func (r *Recorder) ObserveDrop(s verr.Stage) {
	if !r.On() {
		return
	}
	if s != verr.StageNone && int(s) < len(r.dispatch) {
		r.dispatch[s]++
	}
}

// Len 是当前缓冲里能取到的记录条数（≤ Capacity）。
func (r *Recorder) Len() int {
	if !r.On() {
		return 0
	}
	if r.count > Capacity {
		return Capacity
	}
	return int(r.count)
}

// Overflowed 是被覆盖掉的记录条数。
func (r *Recorder) Overflowed() uint64 {
	if !r.On() {
		return 0
	}
	return r.overflow
}

// Written 是累计写入条数。
func (r *Recorder) Written() uint64 {
	if !r.On() {
		return 0
	}
	return r.written
}

// Records 按 Seq 递增返回全部可取的记录。返回的是缓冲内的底层数组切片，
// 调用方不得修改；只建议在渲染结束后调用一次。
func (r *Recorder) Records() []Record {
	if !r.On() {
		return nil
	}
	n := r.Len()
	out := make([]Record, 0, n)
	if r.count <= Capacity {
		for i := 0; i < n; i++ {
			out = append(out, r.recs[i])
		}
		return out
	}
	for i := 0; i < Capacity; i++ {
		out = append(out, r.recs[(r.head+i)%Capacity])
	}
	return out
}

// StageErrors 返回某阶段的失败次数。
func (r *Recorder) StageErrors(s verr.Stage) uint64 {
	if !r.On() || s == verr.StageNone || int(s) >= len(r.stageErr) {
		return 0
	}
	return r.stageErr[s]
}

// StageDrops 返回某阶段被摘掉的次数。
func (r *Recorder) StageDrops(s verr.Stage) uint64 {
	if !r.On() || s == verr.StageNone || int(s) >= len(r.dispatch) {
		return 0
	}
	return r.dispatch[s]
}
