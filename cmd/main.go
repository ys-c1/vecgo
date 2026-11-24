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
	rule := flag.String("rule", "half-pixel", "贴边像素归属规则: half-pixel(默认, 中心采样+左上角规则) | legacy(改动前射线法)")
	flag.Parse()
	cfg := config.Default()
	cfg.EdgeRule = *rule
	scene := [][]geom.Point{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}}
	im, err := pipe.Run(scene, cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "失败:", err)
		os.Exit(1)
	}
	if flag.NArg() > 0 {
		cfg.Output = flag.Arg(0)
	}
	if err := im.SavePGM(cfg.Output); err != nil {
		fmt.Fprintln(os.Stderr, "导出失败:", err)
		os.Exit(2)
	}
	fmt.Printf("on=%d size=%dx%d rule=%s\n", im.CountOn(128), im.W, im.H, cfg.EdgeRule)
}
