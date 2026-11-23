// pngdemo 用演示场景渲染两档画幅, 各导出 PNG 与 PGM,
// 并报告 PNG 产物体积与编码耗时。
package main

import (
	"fmt"
	"os"
	"time"

	"vecgo/config"
	"vecgo/geom"
	"vecgo/pipe"
	"vecgo/pngx"
)

func main() {
	scene := [][]geom.Point{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}}
	outDir := "artifacts"
	if len(os.Args) > 1 {
		outDir = os.Args[1]
	}
	for _, size := range []int{64, 1024} {
		cfg := config.Default()
		cfg.Width, cfg.Height = size, size
		im, err := pipe.Run(scene, cfg)
		if err != nil {
			fmt.Fprintln(os.Stderr, "渲染失败:", err)
			os.Exit(1)
		}
		start := time.Now()
		data, e := pngx.Encode(im.W, im.H, im.Pix)
		if e != nil {
			fmt.Fprintln(os.Stderr, "PNG 编码失败:", e)
			os.Exit(int(e.Code))
		}
		ms := float64(time.Since(start).Microseconds()) / 1000
		pngPath := fmt.Sprintf("%s/render-%d.png", outDir, size)
		pgmPath := fmt.Sprintf("%s/render-%d.pgm", outDir, size)
		if err := os.WriteFile(pngPath, data, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "写文件失败:", err)
			os.Exit(1)
		}
		if err := im.SavePGM(pgmPath); err != nil {
			fmt.Fprintln(os.Stderr, "PGM 导出失败:", err)
			os.Exit(1)
		}
		fmt.Printf("%dx%d: png=%d 字节, 编码=%.3f ms -> %s\n", size, size, len(data), ms, pngPath)
	}
}
