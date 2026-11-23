package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"

	"vecgo/config"
	"vecgo/geom"
	"vecgo/layer"
	"vecgo/pipe"
)

// demoScene 生成 n 层演示场景：每层是一个正方形，
// 随层号旋转并放大，层层交叠，方便观察开关效果。
func demoScene(n int) [][]geom.Point {
	scene := make([][]geom.Point, n)
	for i := 0; i < n; i++ {
		base := []geom.Point{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}
		scene[i] = pipe.Transform(base, 1.0+0.15*float64(i), 0.35*float64(i))
	}
	return scene
}

func main() {
	layersN := flag.Int("layers", 4, "演示场景图层数")
	maskStr := flag.String("mask", "", "图层掩码（如 0b1011 或 0xb），空为全部可见")
	flag.Parse()

	cfg := config.Default()
	if *maskStr != "" {
		m, err := strconv.ParseUint(*maskStr, 0, 64)
		if err != nil {
			fmt.Fprintln(os.Stderr, "掩码解析失败:", *maskStr, err)
			os.Exit(1)
		}
		cfg.LayerMask = &m
	}

	scene := demoScene(*layersN)

	// 运行期诊断：位数超限、未知图层位、掩码全为零都给出具体位号。
	if cfg.LayerMask != nil {
		for _, is := range layer.Check(layer.Mask(*cfg.LayerMask), len(scene)) {
			fmt.Fprintln(os.Stderr, "诊断:", is)
		}
	}

	start := time.Now()
	im, err := pipe.Run(scene, cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "失败:", err)
		os.Exit(1)
	}
	ms := float64(time.Since(start).Microseconds()) / 1000.0

	if flag.NArg() > 0 {
		cfg.Output = flag.Arg(0)
	}
	if err := im.SavePGM(cfg.Output); err != nil {
		fmt.Fprintln(os.Stderr, "导出失败:", err)
		os.Exit(2)
	}
	maskBits := len(scene)
	if maskBits > layer.MaxLayers {
		maskBits = layer.MaxLayers
	}
	fmt.Printf("on=%d size=%dx%d layers=%d maskBits=%d renderMs=%.3f\n",
		im.CountOn(128), im.W, im.H, len(scene), maskBits, ms)
}
