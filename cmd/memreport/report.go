//go:build ignore

// Command memreport 采集内存峰值曲线、分配次数和整帧拷贝次数。
//
// 它自己不实现渲染逻辑，只调用 vecgo 管线；同一份源码分别放进“改造前”“改造后”
// 两棵树里运行，即可得到可比的对照数据。
//
// 用法（在某一棵树的根目录）：
//
//	go run cmd/memreport/report.go <label> <输出目录>
//
// 会对 1024² 与 2048² 两档画幅各跑一次，打印 JSON 摘要，并写出逐次采样的 CSV 曲线。
package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"vecgo/config"
	"vecgo/geom"
	"vecgo/pipe"
	"vecgo/post"
	"vecgo/raster"
	"vecgo/shading"
)

type sample struct {
	ms    float64
	alloc uint64
	inuse uint64
}

func scene(n int) [][]geom.Point {
	s := make([][]geom.Point, 0, n)
	for i := 0; i < n; i++ {
		x := float64(i%4) * 12
		y := float64(i/4) * 12
		s = append(s, []geom.Point{{X: x, Y: y}, {X: x + 10, Y: y}, {X: x + 10, Y: y + 10}, {X: x, Y: y + 10}})
	}
	return s
}

func writeStage(outDir, label string, size int, name string, pix []uint8) {
	path := filepath.Join(outDir, fmt.Sprintf("%s_%d_%s.raw", label, size, name))
	if err := os.WriteFile(path, pix, 0o644); err != nil {
		panic(err)
	}
}

// allocCount 连跑多次、取众数，得到稳定的分配次数（单次易受后台 GC/栈增长干扰）。
func allocCount(c config.Config, sc [][]geom.Point) (mallocs, bytes uint64) {
	for i := 0; i < 5; i++ {
		pipe.Run(sc, c) // 预热，排除一次性运行时初始化
	}
	const reps = 20
	type key struct{ m, b uint64 }
	freq := map[key]int{}
	for i := 0; i < reps; i++ {
		runtime.GC()
		runtime.GC()
		var a, b runtime.MemStats
		runtime.ReadMemStats(&a)
		pipe.Run(sc, c)
		runtime.ReadMemStats(&b)
		freq[key{b.Mallocs - a.Mallocs, b.TotalAlloc - a.TotalAlloc}]++
	}
	best, bestN := key{}, -1
	for k, n := range freq {
		if n > bestN || (n == bestN && k.m < best.m) {
			best, bestN = k, n
		}
	}
	return best.m, best.b
}

func run(label string, size, blur, npoly int, outDir string) (map[string]any, error) {
	c := config.Default()
	c.Width, c.Height = size, size
	c.Blur = blur
	c.Supersample = 2
	c.Shading = "gradient"
	c.Gamma = 2.2
	c.Rotate = 0.3
	c.Scale = 1
	c.Output = filepath.Join(outDir, "out.pgm")
	sc := scene(npoly)

	mallocs, allocBytes := allocCount(c, sc)

	runtime.GC()
	runtime.GC()
	var base runtime.MemStats
	runtime.ReadMemStats(&base)

	var samples []sample
	var stop atomic.Bool
	var wg sync.WaitGroup
	start := time.Now()
	wg.Add(1)
	go func() {
		defer wg.Done()
		var ms runtime.MemStats
		for !stop.Load() {
			runtime.ReadMemStats(&ms)
			samples = append(samples, sample{
				ms:    float64(time.Since(start).Microseconds()) / 1000,
				alloc: ms.HeapAlloc,
				inuse: ms.HeapInuse,
			})
			time.Sleep(100 * time.Microsecond)
		}
	}()

	startCopies := post.FullFrameCopies.Load()
	t0 := time.Now()
	im, err := pipe.Run(sc, c)
	elapsed := time.Since(t0)
	stop.Store(true)
	wg.Wait()
	if err != nil {
		return nil, err
	}
	copies := post.FullFrameCopies.Load() - startCopies

	var peakAlloc, peakInuse uint64
	for _, s := range samples {
		if s.alloc > peakAlloc {
			peakAlloc = s.alloc
		}
		if s.inuse > peakInuse {
			peakInuse = s.inuse
		}
	}
	var ru syscall.Rusage
	syscall.Getrusage(syscall.RUSAGE_SELF, &ru)

	// 逐阶段留底，和内存曲线同一次配置，供“四处改完逐像素对齐”比对。
	// 依次是：几何变换+填充、着色、gamma、模糊、最终 Run 输出。
	moved := make([][]geom.Point, len(sc))
	for i, p := range sc {
		moved[i] = pipe.Transform(p, c.Scale, c.Rotate)
	}
	stage, err := raster.New(c.Width, c.Height)
	if err != nil {
		return nil, err
	}
	if err := stage.Fill(moved, c.Supersample); err != nil {
		return nil, err
	}
	writeStage(outDir, label, size, "1_fill", stage.Pix)
	if err := shading.Apply(stage, c.Shading, 1.0, 0.0); err != nil {
		return nil, err
	}
	writeStage(outDir, label, size, "2_shading", stage.Pix)
	if err := post.Gamma(stage, c.Gamma); err != nil {
		return nil, err
	}
	writeStage(outDir, label, size, "3_gamma", stage.Pix)
	blurred, err := post.Blur(stage, c.Blur)
	if err != nil {
		return nil, err
	}
	writeStage(outDir, label, size, "4_blur", blurred.Pix)
	writeStage(outDir, label, size, "5_run", im.Pix)

	f, err := os.Create(filepath.Join(outDir, fmt.Sprintf("%s_%d.csv", label, size)))
	if err != nil {
		return nil, err
	}
	w := csv.NewWriter(f)
	w.Write([]string{"t_ms", "heap_alloc_bytes", "heap_inuse_bytes", "alloc_per_input"})
	for _, s := range samples {
		w.Write([]string{
			strconv.FormatFloat(s.ms, 'f', 3, 64),
			strconv.FormatUint(s.alloc, 10),
			strconv.FormatUint(s.inuse, 10),
			strconv.FormatFloat(float64(s.alloc)/float64(size*size), 'f', 4, 64),
		})
	}
	w.Flush()
	f.Close()

	in := uint64(size * size)
	peakMB := float64(peakAlloc) / 1048576
	fmt.Printf("[%s] %d×%d  峰值 %.2f× 输入 (%.2f MB)  分配 %d 次  整帧拷贝 %d 次  用时 %.1f ms\n",
		label, size, size, float64(peakAlloc)/float64(in), peakMB, mallocs, copies, float64(elapsed.Microseconds())/1000)
	return map[string]any{
		"label":             label,
		"size":              size,
		"blur":              blur,
		"npoly":             npoly,
		"input_bytes":       in,
		"peak_heap_alloc":   peakAlloc,
		"peak_heap_inuse":   peakInuse,
		"peak_above_base":   peakAlloc - base.HeapAlloc,
		"alloc_per_input":   float64(peakAlloc) / float64(in),
		"above_per_input":   float64(peakAlloc-base.HeapAlloc) / float64(in),
		"maxrss_kb":         ru.Maxrss,
		"mallocs":           mallocs,
		"total_alloc_bytes": allocBytes,
		"full_frame_copies": copies,
		"samples":           len(samples),
		"elapsed_ms":        float64(elapsed.Microseconds()) / 1000,
		"count_on_128":      im.CountOn(128),
	}, nil
}

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "用法: memreport <label> <输出目录> [画幅...]")
		os.Exit(2)
	}
	label, outDir := os.Args[1], os.Args[2]
	sizes := []int{1024, 2048}
	if len(os.Args) > 3 {
		sizes = sizes[:0]
		for _, a := range os.Args[3:] {
			n, err := strconv.Atoi(a)
			if err != nil || n <= 0 {
				fmt.Fprintf(os.Stderr, "非法画幅: %q\n", a)
				os.Exit(2)
			}
			sizes = append(sizes, n)
		}
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		panic(err)
	}
	var all []map[string]any
	for _, size := range sizes {
		s, err := run(label, size, 3, 1, outDir)
		if err != nil {
			fmt.Fprintln(os.Stderr, "ERR:", err)
			os.Exit(1)
		}
		all = append(all, s)
		b, _ := json.MarshalIndent(s, "", "  ")
		fmt.Println(string(b))
	}
	agg, _ := json.MarshalIndent(all, "", "  ")
	os.WriteFile(filepath.Join(outDir, "summary_"+label+".json"), append(agg, '\n'), 0o644)
}
