//go:build vecgotiming

// 计时开启时的实现：仅在 -tags vecgotiming 构建下参与编译，
// 累积每个阶段的挂钟耗时（time.Since，单调时钟读数，不触发系统调用）。
package timing

import (
	"sort"
	"sync"
	"time"
)

const (
	StageTransform = "transform"
	StageRasterize = "rasterize"
	StageShading   = "shading"
	StageGamma     = "gamma"
	StageBlur      = "blur"
)

var (
	mu      sync.Mutex
	started = map[string]time.Time{}
	totals  = map[string]time.Duration{}
)

func Start(stage string) {
	mu.Lock()
	started[stage] = time.Now()
	mu.Unlock()
}

func Done(stage string) {
	end := time.Now()
	mu.Lock()
	if t, ok := started[stage]; ok {
		totals[stage] += end.Sub(t)
		delete(started, stage)
	}
	mu.Unlock()
}

func Reset() {
	mu.Lock()
	started = map[string]time.Time{}
	totals = map[string]time.Duration{}
	mu.Unlock()
}

// StageTime 是单个阶段的累计耗时。
type StageTime struct {
	Stage   string
	TotalNs int64
}

// Report 按执行顺序（首次 Start 的顺序不可靠，这里用固定阶段序）返回累计耗时。
func Report() []StageTime {
	order := []string{StageTransform, StageRasterize, StageShading, StageGamma, StageBlur}
	mu.Lock()
	defer mu.Unlock()
	out := make([]StageTime, 0, len(order))
	for _, s := range order {
		if d, ok := totals[s]; ok {
			out = append(out, StageTime{Stage: s, TotalNs: d.Nanoseconds()})
		}
	}
	return out
}

// SortedByNs 按耗时降序返回，供调试使用。
func SortedByNs() []StageTime {
	r := Report()
	sort.Slice(r, func(i, j int) bool { return r[i].TotalNs > r[j].TotalNs })
	return r
}
