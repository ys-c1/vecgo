// gengolden 渲染各份老配置对应的当前键 twin，生成逐像素比对的 golden 基线。
// 这些图代表"迁移正确时老配置应当出的图"。生成器只使用 config 与 pipe 的
// 公开 API；渲染管线（geom/raster/shading/post）在迁移改动中未被触碰。
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"vecgo/config"
	"vecgo/geom"
	"vecgo/pipe"
)

var square = [][]geom.Point{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}}

var cases = []string{"v1_1", "v1_2", "v2_1", "v2_2"}

func main() {
	root := "."
	if len(os.Args) > 1 {
		root = os.Args[1]
	}
	legacyDir := filepath.Join(root, "config", "testdata", "legacy")
	goldenDir := filepath.Join(root, "pipe", "testdata", "golden")
	if err := os.MkdirAll(goldenDir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for _, name := range cases {
		cfg, _, err := config.LoadFile(filepath.Join(legacyDir, name+".twin.json"))
		if err != nil {
			fmt.Fprintln(os.Stderr, name, err)
			os.Exit(1)
		}
		im, err := pipe.Run(square, cfg)
		if err != nil {
			fmt.Fprintln(os.Stderr, name, err)
			os.Exit(1)
		}
		out := filepath.Join(goldenDir, name+".pgm")
		if err := im.SavePGM(out); err != nil {
			fmt.Fprintln(os.Stderr, name, err)
			os.Exit(1)
		}
		fmt.Printf("golden: %s (%dx%d, %d bytes pix)\n", out, im.W, im.H, len(im.Pix))
	}
}
