package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"sync"
	"time"

	"vecgo/config"
	"vecgo/geom"
	"vecgo/pipe"
	"vecgo/raster"
)

// 退出码约定：
//
//	0  渲染成功
//	1  参数、配置或渲染过程中的错误
//	2  输出文件写入失败
//	3  内存峰值超过输入原图两倍（仅在启用 -mem-curve 时检查）
const (
	exitOK          = 0
	exitFailure     = 1
	exitWriteFailed = 2
	exitMemoryOver  = 3
)

// renderBudget 是采样缓冲覆盖的最长渲染时长，只用于确定缓冲容量。
const renderBudget = 4 * time.Second

const usage = `用法: vecgo [选项]

用一条管线渲染一帧：光栅化 -> 着色 -> 后处理(gamma/模糊) -> 导出。
整条管线只分配一块整图缓冲，各阶段原地处理，入口之外不再拷贝整图。

选项:
  -w int        画幅宽度 (默认 64)
  -hh int       画幅高度 (默认 64)
  -scale float  缩放 (默认 1)
  -rotate float 旋转弧度 (默认 0)
  -ss int       超采样倍数 (默认 2)
  -shading str  着色模式 flat|gradient (默认 flat)
  -gamma float  伽马 (默认 1)
  -blur int     模糊半径 (默认 0)
  -o string     输出 PGM 路径 (默认 out.pgm)
  -mem-curve string
                若给出路径，则渲染期间按间隔采样堆内存并写入 CSV，
                同时检查峰值是否超过输入原图的两倍
  -mem-interval duration
                内存采样间隔 (默认 500µs)

输出:
  标准输出打印 尺寸、亮像素数、整图分配/整图拷贝/阶段取图计数。
  内存采样与拷贝计数用于对照“输入落地一次之后不再拷贝”的目标。

退出码:
  0  渲染成功
  1  参数、配置或渲染错误
  2  输出写入失败
  3  内存峰值超过输入原图的两倍（仅在 -mem-curve 时检查）

示例:
  vecgo -w 512 -h 512 -blur 3 -gamma 2.2 -shading gradient -o out.pgm
  vecgo -w 2048 -h 2048 -blur 3 -mem-curve /tmp/mem.csv -o out.pgm
`

type memSample struct {
	tMS        float64
	heapAlloc  uint64
	heapInuse  uint64
	aboveInput float64
}

// startSampler 从渲染开始那一刻起连续采样堆内存。基线在进程入口就已
// 捕获，因此曲线反映的是渲染与导出真正带来的增量，不含 Go 运行时的
// 固定占用。触发渲染的那一刻会在曲线里留下一个锚点，便于对齐阶段。
// startSampler 从渲染开始那一刻起连续采样堆内存。samples 必须由调用方
// 在测量基线之前分配好，且足够大以免采样期间扩容——采样工具本身的开销
// 因此落在基线里，不会混进渲染的增量。base 是渲染前的固定占用。
func startSampler(interval time.Duration, input uint64, samples []memSample) (stop func() []memSample, trigger func()) {
	var ms runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&ms)
	base := ms.HeapAlloc

	var (
		mu sync.Mutex
		n  int
	)
	started := make(chan struct{})
	done := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(done)
		<-started
		start := time.Now()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			var cur runtime.MemStats
			runtime.ReadMemStats(&cur)
			above := uint64(0)
			if cur.HeapAlloc > base {
				above = cur.HeapAlloc - base
			}
			mu.Lock()
			if n < len(samples) {
				samples[n] = memSample{
					tMS:        float64(time.Since(start).Microseconds()) / 1000,
					heapAlloc:  cur.HeapAlloc,
					heapInuse:  cur.HeapInuse,
					aboveInput: float64(above) / float64(input),
				}
				n++
			}
			mu.Unlock()
			select {
			case <-stopped:
				return
			case <-ticker.C:
			}
		}
	}()
	var once sync.Once
	trigger = func() { once.Do(func() { close(started) }) }
	stop = func() []memSample {
		trigger()
		// 先停采样，再读取样本：避免采样协程还在追加时读切片。
		close(stopped)
		<-done
		mu.Lock()
		defer mu.Unlock()
		out := make([]memSample, n)
		copy(out, samples[:n])
		return out
	}
	return stop, trigger
}

func main() {
	os.Exit(run())
}

func run() int {
	def := config.Default()
	fs := flag.NewFlagSet("vecgo", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() { fmt.Fprint(os.Stderr, usage) }

	var (
		width   = fs.Int("w", def.Width, "画幅宽度")
		height  = fs.Int("hh", def.Height, "画幅高度")
		scale   = fs.Float64("scale", def.Scale, "缩放")
		rotate  = fs.Float64("rotate", def.Rotate, "旋转弧度")
		ss      = fs.Int("ss", def.Supersample, "超采样倍数")
		shading = fs.String("shading", def.Shading, "着色模式 flat|gradient")
		gamma   = fs.Float64("gamma", def.Gamma, "伽马")
		blur    = fs.Int("blur", def.Blur, "模糊半径")
		output  = fs.String("o", def.Output, "输出 PGM 路径")
		memCSV  = fs.String("mem-curve", "", "内存采样 CSV 输出路径")
		memTick = fs.Duration("mem-interval", 500*time.Microsecond, "内存采样间隔")
	)
	if err := fs.Parse(os.Args[1:]); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitFailure
	}
	if fs.NArg() != 0 {
		fmt.Fprintf(os.Stderr, "多余的参数: %v\n", fs.Args())
		fs.Usage()
		return exitFailure
	}

	cfg := config.Config{
		Width:       *width,
		Height:      *height,
		Scale:       *scale,
		Rotate:      *rotate,
		Supersample: *ss,
		Shading:     *shading,
		Gamma:       *gamma,
		Blur:        *blur,
		Output:      *output,
	}
	if err := cfg.Validate(); err != nil {
		fmt.Fprintln(os.Stderr, "配置错误:", err)
		fs.Usage()
		return exitFailure
	}
	if *memTick <= 0 {
		fmt.Fprintln(os.Stderr, "mem-interval 必须为正")
		return exitFailure
	}

	scene := [][]geom.Point{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 4}, {X: 6, Y: 4}, {X: 6, Y: 10}, {X: 0, Y: 10}}}

	raster.ResetStats()

	input := uint64(cfg.Width * cfg.Height)
	idealBuf := int(renderBudget / *memTick)
	var samples []memSample
	var stopSampler func() []memSample
	var triggerRender func()
	if *memCSV != "" {
		// 采样缓冲在取基线之前分配，算作固定开销而非渲染增量。
		samples = make([]memSample, idealBuf)
		stopSampler, triggerRender = startSampler(*memTick, input, samples)
	}

	if triggerRender != nil {
		triggerRender()
	}
	im, err := pipe.Run(scene, cfg)
	if err != nil {
		if stopSampler != nil {
			stopSampler()
		}
		fmt.Fprintln(os.Stderr, "渲染失败:", err)
		return exitFailure
	}
	saveErr := im.SavePGM(cfg.Output)
	if stopSampler != nil {
		samples = stopSampler()
	}
	if saveErr != nil {
		fmt.Fprintln(os.Stderr, "导出失败:", saveErr)
		return exitWriteFailed
	}

	st := raster.Snapshot()
	fmt.Printf("on=%d size=%dx%d\n", im.CountOn(128), im.W, im.H)
	fmt.Printf("frame_allocs=%d full_copies=%d stage_leases=%d\n",
		st.FrameAllocs, st.FullCopies, st.StageLeases)

	if *memCSV != "" {
		if err := writeMemCSV(*memCSV, samples); err != nil {
			fmt.Fprintln(os.Stderr, "写内存曲线失败:", err)
			return exitWriteFailed
		}
		var peakAbove uint64
		var peakRatio float64
		for _, s := range samples {
			if s.aboveInput > peakRatio {
				peakRatio = s.aboveInput
				peakAbove = s.heapAlloc
			}
		}
		fmt.Printf("mem_samples=%d peak_heap=%d input_bytes=%d peak_ratio=%.4f\n",
			len(samples), peakAbove, input, peakRatio)
		if peakRatio > 2 {
			fmt.Fprintf(os.Stderr, "扣除基线后的内存峰值超过输入原图两倍: ratio=%.4f\n", peakRatio)
			return exitMemoryOver
		}
	}
	return exitOK
}

func writeMemCSV(path string, samples []memSample) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	if err := w.Write([]string{"t_ms", "heap_alloc_bytes", "heap_inuse_bytes", "above_base_over_input"}); err != nil {
		return err
	}
	for _, s := range samples {
		if err := w.Write([]string{
			strconv.FormatFloat(s.tMS, 'f', 3, 64),
			strconv.FormatUint(s.heapAlloc, 10),
			strconv.FormatUint(s.heapInuse, 10),
			strconv.FormatFloat(s.aboveInput, 'f', 4, 64),
		}); err != nil {
			return err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return err
	}
	return f.Close()
}
