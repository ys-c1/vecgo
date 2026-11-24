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
	cfg := config.Default()

	// 这些 flag 全部默认到 config.Default()，因此不带参数时行为与历史完全一致。
	w := flag.Int("w", cfg.Width, "画幅宽")
	h := flag.Int("h", cfg.Height, "画幅高")
	shading := flag.String("shading", cfg.Shading, "着色模式: flat|gradient")
	space := flag.String("space", cfg.InterpSpace, "渐变插值空间: encoded(默认)|linear|perceptual")
	gamma := flag.Float64("gamma", cfg.Gamma, "gamma 旋钮")
	ss := flag.Int("ss", cfg.Supersample, "超采样倍数")
	out := flag.String("out", "", "输出 PGM 路径（也可用第一个位置参数给出）")
	flag.Parse()

	cfg.Width = *w
	cfg.Height = *h
	cfg.Shading = *shading
	cfg.InterpSpace = *space
	cfg.Gamma = *gamma
	cfg.Supersample = *ss

	scene := [][]geom.Point{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}}
	im, err := pipe.Run(scene, cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "失败:", err)
		os.Exit(1)
	}

	output := *out
	if output == "" && flag.NArg() > 0 {
		output = flag.Arg(0) // 兼容历史用法: vecgo <output.pgm>
	}
	if output == "" {
		output = cfg.Output
	}
	if err := im.SavePGM(output); err != nil {
		fmt.Fprintln(os.Stderr, "导出失败:", err)
		os.Exit(2)
	}
	fmt.Printf("on=%d size=%dx%d\n", im.CountOn(128), im.W, im.H)
}
