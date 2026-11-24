// imgdiff 逐像素比较两幅 PGM：报告差异数、前十处位置与归因类别。
// 用法: imgdiff <基准.pgm> <当前.pgm>；有差异时退出码为 1。
package main

import (
	"fmt"
	"os"

	"vecgo/raster"
	"vecgo/regress"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "用法: imgdiff <基准.pgm> <当前.pgm>")
		os.Exit(2)
	}
	base, err := raster.LoadPGM(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "读基准失败:", err)
		os.Exit(2)
	}
	cur, err := raster.LoadPGM(os.Args[2])
	if err != nil {
		fmt.Fprintln(os.Stderr, "读当前失败:", err)
		os.Exit(2)
	}
	rep, err := regress.Diff(base, cur)
	if err != nil {
		fmt.Fprintln(os.Stderr, "差分失败:", err)
		os.Exit(2)
	}
	fmt.Printf("差异像素: %d (丢失 %d / 新增 %d)\n", rep.Count, rep.Lost, rep.Gained)
	fmt.Printf("平均幅度: %.1f 实心度: %.2f\n", rep.MeanMag, rep.Solidity)
	fmt.Printf("归因: %s\n", regress.Label(rep.Category))
	for i, p := range rep.Top {
		fmt.Printf("  #%d (%d,%d) %d -> %d\n", i+1, p.X, p.Y, p.Base, p.Cur)
	}
	if rep.Count > 0 {
		os.Exit(1)
	}
}
