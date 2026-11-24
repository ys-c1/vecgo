package main

import (
	"flag"
	"fmt"
	"os"

	"vecgo/config"
	"vecgo/diag"
	"vecgo/geom"
	"vecgo/pipe"
)

func main() {
	def := config.Default()
	cfg := def
	width := flag.Int("width", def.Width, "画面宽度（1..4096）")
	height := flag.Int("height", def.Height, "画面高度（1..4096）")
	scale := flag.Float64("scale", def.Scale, "缩放倍数（0 < scale <= 100）")
	rotate := flag.Float64("rotate", def.Rotate, "旋转弧度（有限值即可）")
	supersample := flag.Int("supersample", def.Supersample, "每像素采样数（1..8）")
	shading := flag.String("shading", def.Shading, "着色模式（flat 或 gradient）")
	gamma := flag.Float64("gamma", def.Gamma, "gamma（0.1..10）")
	blur := flag.Int("blur", def.Blur, "模糊半径（0..16）")
	diagOn := flag.Bool("diag", false, "参数非法时输出诊断图")
	diagOut := flag.String("diag-out", "", "诊断图路径（默认 <输出>.diag.png）")
	flag.Parse()

	cfg.Width, cfg.Height = *width, *height
	cfg.Scale, cfg.Rotate = *scale, *rotate
	cfg.Supersample = *supersample
	cfg.Shading, cfg.Gamma, cfg.Blur = *shading, *gamma, *blur
	if flag.NArg() > 0 {
		cfg.Output = flag.Arg(0)
	}

	scene := [][]geom.Point{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}}

	// 诊断开启时先校验：非法参数不进入渲染，只产出诊断图，避免写出
	// 一张看不出问题的黑图覆盖正常产物。
	if findings := diag.Analyze(cfg); len(findings) > 0 && *diagOn {
		im, findings, err := diag.Render(scene, cfg)
		if err != nil {
			fmt.Fprintln(os.Stderr, "诊断失败:", err)
			os.Exit(1)
		}
		path := *diagOut
		if path == "" {
			base := cfg.Output
			if base == "" { // output 本身是非法参数时也别生成隐藏文件
				base = "out.pgm"
			}
			path = base + ".diag.png"
		}
		if err := im.SavePNG(path); err != nil {
			fmt.Fprintln(os.Stderr, "诊断图导出失败:", err)
			os.Exit(2)
		}
		diag.Report(os.Stderr, findings)
		fmt.Fprintf(os.Stderr, "[diag] 诊断图: %s\n", path)
		fmt.Fprintln(os.Stderr, "失败: 参数非法，已拒绝渲染")
		os.Exit(3)
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
