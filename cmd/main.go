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
	samples := flag.Int("samples", 0, "每像素随机采样数；0=默认确定性 2x2 超采样网格（产物与旧版逐字节相同）")
	seed := flag.Int64("seed", 1, "随机采样种子（仅 -samples>0 时生效）")
	flag.Parse()

	cfg := config.Default()
	cfg.Samples = *samples
	cfg.Seed = *seed
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
	fmt.Printf("on=%d size=%dx%d\n", im.CountOn(128), im.W, im.H)
}
