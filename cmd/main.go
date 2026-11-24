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
	levels := flag.Int("levels", 0, "固定金字塔层数（>0 启用固定层数重建路径）")
	targetErr := flag.Float64("target-error", 0, "目标 RMSE 误差（灰度级，>0 且未设 -levels 时自适应选最小层数）")
	shading := flag.String("shading", "", "覆盖着色模式 flat|gradient（留空用默认 flat）")
	outPath := flag.String("o", "", "输出 PGM 路径（也可用第一个位置参数给出）")
	flag.Parse()

	cfg := config.Default()
	cfg.PyramidLevels = *levels
	cfg.TargetError = *targetErr
	if *shading != "" {
		cfg.Shading = *shading
	}

	scene := [][]geom.Point{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}}
	res, err := pipe.RunDetailed(scene, cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "失败:", err)
		os.Exit(1)
	}
	im := res.Image

	out := *outPath
	if out == "" {
		if args := flag.Args(); len(args) > 0 {
			out = args[0]
		} else {
			out = cfg.Output
		}
	}
	if err := im.SavePGM(out); err != nil {
		fmt.Fprintln(os.Stderr, "导出失败:", err)
		os.Exit(2)
	}
	if res.PyramidEnabled {
		switch {
		case res.Adaptive && res.Model != nil && res.Model.Usable():
			fmt.Printf("pyramid=adaptive target=%.3f n=%d q=%.4f A=%.2f\n",
				cfg.TargetError, res.SelectedLevels, res.Model.Q, res.Model.A)
		case res.Adaptive:
			fmt.Printf("pyramid=adaptive target=%.3f n=%d (常量/无衰减图像，最粗层已无误差)\n",
				cfg.TargetError, res.SelectedLevels)
		default:
			fmt.Printf("pyramid=fixed n=%d\n", res.SelectedLevels)
		}
	}
	fmt.Printf("on=%d size=%dx%d\n", im.CountOn(128), im.W, im.H)
}
