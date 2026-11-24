package main

import (
	"fmt"
	"os"
	"strconv"

	"vecgo/config"
	"vecgo/geom"
	"vecgo/pipe"
)

func main() {
	cfg := config.Default()
	// 诊断/测量用的画幅覆盖，仅在显式设置环境变量时生效。
	if v := os.Getenv("VECGO_WIDTH"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Width = n
		}
	}
	if v := os.Getenv("VECGO_HEIGHT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Height = n
		}
	}
	scene := [][]geom.Point{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}}
	var d *pipe.Diag
	if os.Getenv("VECGO_DIAG") == "1" {
		d = &pipe.Diag{}
	}
	im, err := pipe.RunDiag(scene, cfg, d)
	if err != nil {
		if d != nil {
			d.Report(os.Stderr)
		}
		fmt.Fprintln(os.Stderr, "失败:", err)
		os.Exit(1)
	}
	if len(os.Args) > 1 {
		cfg.Output = os.Args[1]
	}
	if err := d.Time("export", func() error { return im.SavePGM(cfg.Output) }); err != nil {
		if d != nil {
			d.Fail("raster", err)
			d.Report(os.Stderr)
		}
		fmt.Fprintln(os.Stderr, "导出失败:", err)
		os.Exit(2)
	}
	if d != nil {
		d.Report(os.Stderr)
	}
	fmt.Printf("on=%d size=%dx%d\n", im.CountOn(128), im.W, im.H)
}
