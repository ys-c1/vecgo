package main

import (
	"fmt"
	"os"

	"vecgo/config"
	"vecgo/geom"
	"vecgo/pipe"
)

func main() {
	cfg := config.Default()
	scene := [][]geom.Point{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}}
	im, err := pipe.Run(scene, cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "失败:", err)
		os.Exit(1)
	}
	if len(os.Args) > 1 {
		cfg.Output = os.Args[1]
	}
	if err := im.SavePGM(cfg.Output); err != nil {
		fmt.Fprintln(os.Stderr, "导出失败:", err)
		os.Exit(2)
	}
	on, err := im.CountOn(128)
	if err != nil {
		fmt.Fprintln(os.Stderr, "统计失败:", err)
		os.Exit(3)
	}
	fmt.Printf("on=%d size=%dx%d\n", on, im.W, im.H)
}
