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

	// 可选参数；全部缺省取 config.Default()，因此无参/仅位置参数时行为与历史完全一致。
	flag.IntVar(&cfg.Width, "w", cfg.Width, "画幅宽（像素）")
	flag.IntVar(&cfg.Height, "h", cfg.Height, "画幅高（像素）")
	flag.StringVar(&cfg.Shading, "shading", cfg.Shading, "着色模式 flat|gradient")
	flag.Float64Var(&cfg.Gamma, "gamma", cfg.Gamma, "gamma")
	flag.IntVar(&cfg.Blur, "blur", cfg.Blur, "模糊半径")
	// -dist 与 -space 是同一开关的两个名字；默认 linear，产物逐字节不变。
	flag.StringVar(&cfg.LevelDist, "dist", cfg.LevelDist, "色阶参数轴分布 linear|perceptual|lab")
	flag.StringVar(&cfg.LevelDist, "space", cfg.LevelDist, "同 -dist（别名）")
	flag.Float64Var(&cfg.LevelAlpha, "alpha", cfg.LevelAlpha, "幂律感知指数，0=缺省 2")
	flag.StringVar(&cfg.Output, "output", cfg.Output, "输出 PGM 路径（位置参数优先级更高）")
	flag.Parse()

	scene := [][]geom.Point{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}}
	im, err := pipe.Run(scene, cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "失败:", err)
		os.Exit(1)
	}
	// 保留历史用法：第一个位置参数若是路径，则覆盖 -output。
	if args := flag.Args(); len(args) > 0 {
		cfg.Output = args[0]
	}
	if err := im.SavePGM(cfg.Output); err != nil {
		fmt.Fprintln(os.Stderr, "导出失败:", err)
		os.Exit(2)
	}
	fmt.Printf("on=%d size=%dx%d\n", im.CountOn(128), im.W, im.H)
}
