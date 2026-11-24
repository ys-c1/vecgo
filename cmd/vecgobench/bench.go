//go:build vecgotiming

// 仅在 -tags vecgotiming 下编译的实测驱动：
//
//	go run -tags vecgotiming ./cmd/vecgobench [iters]
//
// 对两档画幅各跑 iters 次（默认 200），输出每阶段累计耗时与占比（CSV）。
// 场景与 cmd/main.go 保持一致：单个四边形。
package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"runtime"
	"strconv"

	"vecgo/config"
	"vecgo/geom"
	"vecgo/pipe"
	"vecgo/pipe/timing"
)

type sizeSpec struct {
	name string
	w, h int
}

func main() {
	iters := 200
	if len(os.Args) > 1 {
		if n, err := strconv.Atoi(os.Args[1]); err == nil && n > 0 {
			iters = n
		}
	}

	sizes := []sizeSpec{
		{name: "64x64", w: 64, h: 64},
		{name: "256x256", w: 256, h: 256},
	}
	scene := [][]geom.Point{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}}

	w := csv.NewWriter(os.Stdout)
	_ = w.Write([]string{"size", "iters", "stage", "total_ns", "share_pct"})

	for _, sz := range sizes {
		cfg := config.Default()
		cfg.Width, cfg.Height = sz.w, sz.h
		cfg.Output = os.DevNull

		// 预热：触发懒加载、稳定分配路径。
		for k := 0; k < 10; k++ {
			if _, err := pipe.Run(scene, cfg); err != nil {
				fmt.Fprintln(os.Stderr, "预热失败:", err)
				os.Exit(1)
			}
		}
		runtime.GC()

		timing.Reset()
		for k := 0; k < iters; k++ {
			if _, err := pipe.Run(scene, cfg); err != nil {
				fmt.Fprintln(os.Stderr, "实测失败:", err)
				os.Exit(1)
			}
		}
		report := timing.Report()
		var sum int64
		for _, r := range report {
			sum += r.TotalNs
		}
		for _, r := range report {
			share := 0.0
			if sum > 0 {
				share = float64(r.TotalNs) / float64(sum) * 100
			}
			_ = w.Write([]string{
				sz.name,
				strconv.Itoa(iters),
				r.Stage,
				strconv.FormatInt(r.TotalNs, 10),
				strconv.FormatFloat(share, 'f', 2, 64),
			})
		}
	}
	w.Flush()
}
