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
	configPath := flag.String("config", "", "配置 JSON 路径；缺省时使用 config.Default()，与改动前一致")
	flag.Parse()

	cfg := config.Default()
	if *configPath != "" {
		var err error
		cfg, err = config.LoadFile(*configPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "配置读取失败:", err)
			os.Exit(1)
		}
	}

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
