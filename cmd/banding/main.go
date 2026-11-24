// banding 实验：分段线性逼近 sRGB 曲线能否消除 8-bit 渐变色的色带。
// 流程：反解断点 -> 出误差数据 -> 两档画幅渲染 -> 预测/实测色带数对比。
// 默认渲染路径（cmd/main.go、config.Default）不受影响。
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"vecgo/geom"
	"vecgo/post"
	"vecgo/raster"
	"vecgo/shading"
)

const eps = 0.5 / 255 // 半 LSB：保证 8-bit 量化后每项最多差 1

// gradientImage 走渲染器的 raster.Fill + shading.Apply 两级（pipe 默认路径不变），
// 不经 post.Gamma：sRGB 查找表在本实验中顶替它的位置。
func gradientImage(size int) (*raster.Image, error) {
	im, err := raster.New(size, size)
	if err != nil {
		return nil, err
	}
	scene := [][]geom.Point{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}}
	if err := im.Fill(scene, 1); err != nil {
		return nil, err
	}
	if err := shading.Apply(im, "gradient", 1.0, 0.0); err != nil {
		return nil, err
	}
	return im, nil
}

// bands 数图中不同灰度值的个数（即色带数）。
func bands(im *raster.Image) int {
	var seen [256]bool
	n := 0
	for _, v := range im.Pix {
		if !seen[v] {
			seen[v] = true
			n++
		}
	}
	return n
}

// predictBands 按 shading.Apply 的公式推出 H 行渐变经 table 后的色带数。
func predictBands(h int, table [256]uint8) int {
	var seen [256]bool
	n := 0
	for row := 0; row < h; row++ {
		t := 1.0
		if h > 1 {
			t = 1.0 - float64(row)/float64(h-1)
		}
		v := table[uint8(255*t)]
		if !seen[v] {
			seen[v] = true
			n++
		}
	}
	return n
}

func identityTable() [256]uint8 {
	var t [256]uint8
	for i := 0; i < 256; i++ {
		t[i] = uint8(i)
	}
	return t
}

func main() {
	outDir := "artifacts"
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "建目录失败:", err)
		os.Exit(1)
	}

	// 1. 反解分段数与断点
	pw, err := post.NewSRGBPiecewise(eps)
	if err != nil {
		fmt.Fprintln(os.Stderr, "断点求解失败:", err)
		os.Exit(1)
	}
	fmt.Printf("eps = 0.5/255 = %.6f (半 LSB)\n", eps)
	fmt.Printf("分段数 N = %d\n", len(pw.Xs)-1)
	fmt.Println("断点表:")
	fmt.Println(" i |      x_i      |    f(x_i)")
	for i, x := range pw.Xs {
		fmt.Printf("%2d | %13.8f | %11.8f\n", i, x, pw.Ys[i])
	}

	// 2. 误差曲线抽样（最大值 + 两个代表点）
	xMax, dMax := pw.MaxDev(200000)
	fmt.Printf("\n误差曲线抽样: max|dev| = %.6f @ x=%.5f (%.3f LSB)\n", dMax, xMax, dMax*255)
	for _, x := range []float64{0.01, 0.5} {
		d := pw.At(x) - post.SRGBEncode(x)
		fmt.Printf("  x=%.2f  dev=%+.6f (%.3f LSB)\n", x, d, d*255)
	}

	// 3. 两张 8-bit 表逐位对比
	exact := post.SRGBTable()
	approx := pw.Table()
	mismatch, maxDiff := 0, 0
	for i := 0; i < 256; i++ {
		d := int(exact[i]) - int(approx[i])
		if d < 0 {
			d = -d
		}
		if d > 0 {
			mismatch++
		}
		if d > maxDiff {
			maxDiff = d
		}
	}
	fmt.Printf("\n8-bit 表逐位对比: 精确 vs 分段逼近, 不同表项 = %d / 256, 最大差 = %d 级\n", mismatch, maxDiff)

	// 4. 两档画幅：预测 vs 实测
	ident := identityTable()
	tables := []struct {
		name  string
		table [256]uint8
	}{
		{"linear(恒等)", ident},
		{"sRGB精确", exact},
		{"sRGB分段逼近", approx},
	}
	for _, size := range []int{64, 512} {
		base, err := gradientImage(size)
		if err != nil {
			fmt.Fprintln(os.Stderr, "渲染失败:", err)
			os.Exit(1)
		}
		fmt.Printf("\n画幅 %dx%d:\n", size, size)
		fmt.Println("  曲线           | 预测色带 | 实测色带")
		for _, tb := range tables {
			im, _ := raster.New(size, size)
			copy(im.Pix, base.Pix)
			post.ApplyTable(im, tb.table)
			path := filepath.Join(outDir, fmt.Sprintf("gradient_%d_%s.pgm", size, sanitize(tb.name)))
			if err := im.SavePGM(path); err != nil {
				fmt.Fprintln(os.Stderr, "导出失败:", err)
				os.Exit(1)
			}
			fmt.Printf("  %-14s | %8d | %8d\n", tb.name, predictBands(size, tb.table), bands(im))
		}
	}
}

func sanitize(s string) string {
	out := []byte(s)
	for i, c := range out {
		if c == '(' || c == ')' {
			out[i] = '_'
		}
	}
	return string(out)
}

