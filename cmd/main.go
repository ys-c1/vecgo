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
	cfgPath := flag.String("config", "", "配置文件路径（JSON，支持第 1/2 代弃用键自动迁移）")
	flag.Parse()

	cfg := config.Default()
	if *cfgPath != "" {
		loaded, warnings, err := config.LoadFile(*cfgPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "配置加载失败:", err)
			os.Exit(1)
		}
		for _, w := range warnings {
			fmt.Fprintln(os.Stderr, "警告:", w.Message)
		}
		cfg = loaded
	}

	scene := [][]geom.Point{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}}
	im, err := pipe.Run(scene, cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "失败:", err)
		os.Exit(1)
	}
	// 位置参数仍可覆盖配置里的输出路径，保持旧行为。
	if flag.NArg() > 0 {
		cfg.Output = flag.Arg(0)
	}
	if err := im.SavePGM(cfg.Output); err != nil {
		fmt.Fprintln(os.Stderr, "导出失败:", err)
		os.Exit(2)
	}
	fmt.Printf("on=%d size=%dx%d\n", im.CountOn(128), im.W, im.H)
}
