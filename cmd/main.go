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
	frame := flag.String("frame", "", "映射/缩放路径：bbox=原包围盒归一化（默认），world=固定世界视口")
	scale := flag.Float64("scale", 1, "均匀缩放系数")
	rotate := flag.Float64("rotate", 0, "旋转角（弧度）")
	ppu := flag.Float64("ppu", 4, "world 路径的像素/世界单位")
	minX := flag.Float64("minx", -3, "world 视口左边界世界坐标")
	minY := flag.Float64("miny", -3, "world 视口上边界世界坐标")
	flag.Parse()

	cfg := config.Default()
	scene := [][]geom.Point{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}}

	// 兼容旧用法：第一个位置参数仍是输出路径。
	if args := flag.Args(); len(args) > 0 {
		cfg.Output = args[0]
	}
	cfg.Scale = *scale
	cfg.Rotate = *rotate
	if *frame != "" {
		cfg.Frame = *frame
	}
	if cfg.Frame == "world" {
		cfg.PPU = *ppu
		cfg.MinX = *minX
		cfg.MinY = *minY
	}

	im, err := pipe.Run(scene, cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "失败:", err)
		os.Exit(1)
	}
	if err := im.SavePGM(cfg.Output); err != nil {
		fmt.Fprintln(os.Stderr, "导出失败:", err)
		os.Exit(2)
	}
	fmt.Printf("on=%d size=%dx%d\n", im.CountOn(128), im.W, im.H)
}
