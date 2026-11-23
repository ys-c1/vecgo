// depthdemo 渲染一个含遮挡、交叉、全透明的多图层场景，
// 用 z-buffer 消隐与逐像素排序参照各画一遍，打印深度表和
// 两者的差异像素数，并把消隐结果存为 PGM。
package main

import (
	"fmt"
	"os"

	"vecgo/depth"
	"vecgo/raster"
)

func quad(z [4]float64, w float64, shade uint8, alpha float64) depth.Layer {
	return depth.Layer{
		Poly: []depth.Vertex{
			{X: 0, Y: 0, Z: z[0], W: w},
			{X: 16, Y: 0, Z: z[1], W: w},
			{X: 16, Y: 16, Z: z[2], W: w},
			{X: 0, Y: 16, Z: z[3], W: w},
		},
		Shade: shade,
		Alpha: alpha,
	}
}

func main() {
	layers := []depth.Layer{
		quad([4]float64{5, 5.4, 5.4, 5}, 1, 60, 1),      // 远景斜面
		quad([4]float64{1, 4, 4, 1}, 1, 120, 1),         // 交叉面 A：左近右远
		quad([4]float64{4, 1, 1, 4}, 1, 180, 1),         // 交叉面 B：左远右近
		quad([4]float64{6, 6.4, 6.4, 6}, 2, 240, 1),     // w=2，d=z/w=3..3.2
		quad([4]float64{0.5, 0.6, 0.6, 0.5}, 1, 255, 0), // 最近但全透明，不遮挡
	}

	fmt.Println("深度表 (d = z/w 透视除法归一):")
	fmt.Println("图层  顶点   z      w      d")
	for i, l := range layers {
		for j, v := range l.Poly {
			fmt.Printf("%4d  %4d  %5.2f  %5.2f  %5.2f\n", i, j, v.Z, v.W, v.Z/v.W)
		}
	}

	zbuf, err := raster.New(64, 64)
	if err != nil {
		fmt.Fprintln(os.Stderr, "失败:", err)
		os.Exit(1)
	}
	if err := depth.Render(zbuf, layers, 2); err != nil {
		fmt.Fprintln(os.Stderr, "消隐失败:", err)
		os.Exit(1)
	}
	ref, err := raster.New(64, 64)
	if err != nil {
		fmt.Fprintln(os.Stderr, "失败:", err)
		os.Exit(1)
	}
	if err := depth.RenderReference(ref, layers, 2); err != nil {
		fmt.Fprintln(os.Stderr, "参照失败:", err)
		os.Exit(1)
	}
	n, err := depth.Diff(zbuf, ref)
	if err != nil {
		fmt.Fprintln(os.Stderr, "比较失败:", err)
		os.Exit(1)
	}
	fmt.Printf("差异像素数: %d\n", n)

	out := "artifacts/depth.pgm"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}
	if err := zbuf.SavePGM(out); err != nil {
		fmt.Fprintln(os.Stderr, "导出失败:", err)
		os.Exit(2)
	}
	fmt.Println("已导出:", out)
}
