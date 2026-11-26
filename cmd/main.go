package main

import (
	"flag"
	"fmt"
	"os"

	"vecgo/config"
	"vecgo/geom"
	"vecgo/pipe"
)

func main() {
	rule := flag.String("rule", "non-zero", "填充规则: non-zero 或 even-odd")
	out := flag.String("out", "out.pgm", "输出 PGM 路径")
	flag.Parse()

	cfg := config.Default()
	cfg.FillRule = *rule
	cfg.Output = *out

	scene := [][]geom.Point{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}}
	im, err := pipe.Run(scene, cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "失败:", err)
		os.Exit(1)
	}
	if err := im.SavePGM(cfg.Output); err != nil {
		fmt.Fprintln(os.Stderr, "导出失败:", err)
		os.Exit(2)
	}
	fmt.Printf("on=%d size=%dx%d rule=%s\n", im.CountOn(128), im.W, im.H, cfg.FillRule)
}
