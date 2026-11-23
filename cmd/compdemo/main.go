// compdemo 演示图层混合：四种模式合成三张确定性图层，输出
//   - 每种模式的 PGM 成品（两轮运行，供外部 sha256 对照幂等性）；
//   - 三种折叠顺序的字节哈希（结合律对照）；
//   - 256 档反解权重表（artifacts/weight_table.txt）。
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"os"
	"path/filepath"

	"vecgo/comp"
	"vecgo/raster"
)

const size = 8

func main() {
	outDir := "artifacts"
	if len(os.Args) > 1 {
		outDir = os.Args[1]
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fail(err)
	}

	layers := buildLayers()
	modes := []string{comp.ModeNormal, comp.ModeMultiply, comp.ModeAdd, comp.ModeScreen}
	runTag := os.Getenv("COMP_RUN")
	if runTag == "" {
		runTag = "run1"
	}

	fmt.Println("== 1) 四模式合成 + 两轮运行字节哈希（幂等性）==")
	for _, mode := range modes {
		stack := makeStack(mode, layers)
		out, err := comp.Composite(stack)
		if err != nil {
			fail(err)
		}
		path := filepath.Join(outDir, fmt.Sprintf("comp_%s_%s.pgm", mode, runTag))
		if err := out.SavePGM(path); err != nil {
			fail(err)
		}
		fmt.Printf("%-8s %s sha256=%s file=%s\n", mode, runTag, hashBytes(out.Pix), path)
	}

	fmt.Println()
	fmt.Println("== 2) 结合律：三种折叠顺序哈希对照（同序图层）==")
	for _, mode := range modes {
		stack := makeStack(mode, layers)
		names := []string{"左结合", "右结合", "幺元起步"}
		kinds := []comp.FoldKind{comp.FoldLeft, comp.FoldRight, comp.FoldIdentity}
		hashes := make([]string, 3)
		for i, kind := range kinds {
			out, err := comp.CompositeFold(stack, kind)
			if err != nil {
				fail(err)
			}
			hashes[i] = hashBytes(out.Pix)
			fmt.Printf("%-8s %s sha256=%s\n", mode, names[i], hashes[i])
		}
		if hashes[0] != hashes[1] || hashes[1] != hashes[2] {
			fmt.Printf("%-8s 结合律失败！\n", mode)
			os.Exit(3)
		}
	}

	fmt.Println()
	fmt.Println("== 3) 反解权重表（规范源 s̄=192/255，256 档，NaN=不可达）==")
	tablePath := filepath.Join(outDir, "weight_table.txt")
	if err := writeWeightTables(tablePath, modes); err != nil {
		fail(err)
	}
	fmt.Println("全表已写入:", tablePath)
	for _, mode := range modes {
		table, err := comp.WeightTable(mode)
		if err != nil {
			fail(err)
		}
		samples := []int{0, 32, 63, 64, 128, 192, 193, 255}
		line := ""
		for _, t := range samples {
			line += fmt.Sprintf(" t=%d:%s", t, fmtWeight(table[t]))
		}
		fmt.Printf("%-8s%s\n", mode, line)
	}

	fmt.Println()
	fmt.Println("== 4) 报错带图层号演示 ==")
	showError(comp.Layer{Image: layers[0], Mode: "overlay", Target: 0.3})       // 模式非法
	showError(comp.Layer{Image: layers[0], Mode: comp.ModeNormal, Target: 1.2}) // 目标越界
	showError(comp.Layer{Image: layers[0], Mode: comp.ModeNormal, Target: 0.9}) // 透明度越界（s̄=0.5，w=1.8）
	showError(comp.Layer{Image: nil, Mode: comp.ModeNormal, Target: 0.3})       // 图层为空
}

func buildLayers() []*raster.Image {
	mk := func(fill func(x, y int) uint8) *raster.Image {
		im, err := raster.New(size, size)
		if err != nil {
			fail(err)
		}
		for y := 0; y < size; y++ {
			for x := 0; x < size; x++ {
				im.Pix[y*size+x] = fill(x, y)
			}
		}
		return im
	}
	return []*raster.Image{
		mk(func(x, y int) uint8 { return uint8(x * 255 / (size - 1)) }), // 水平梯度
		mk(func(x, y int) uint8 { return uint8(y * 255 / (size - 1)) }), // 垂直梯度
		mk(func(x, y int) uint8 {
			if (x/2+y/2)%2 == 0 {
				return 255
			}
			return 0
		}), // 粗棋盘
	}
}

// makeStack 用每图均值把固定权重反算成目标亮度，保证目标合法可达。
func makeStack(mode string, imgs []*raster.Image) []comp.Layer {
	weights := []float64{0.25, 0.5, 0.75}
	out := make([]comp.Layer, len(imgs))
	for i, im := range imgs {
		mean := comp.MeanLuminance(im)
		var target float64
		if mode == comp.ModeMultiply {
			target = 1 - weights[i]*(1-mean)
		} else {
			target = weights[i] * mean
		}
		out[i] = comp.Layer{Image: im, Mode: mode, Target: target}
	}
	return out
}

func writeWeightTables(path string, modes []string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	fmt.Fprintln(f, "# 反解权重表 w(t)，t 为目标亮度档位 0..255，NaN 表示该目标不可达")
	fmt.Fprintln(f, "# 规范源亮度 s̄ = 192/255")
	fmt.Fprintln(f, "# normal/add/screen: w = T/s̄ （黑中性底）；multiply: w = (1-T)/(1-s̄)（白中性底）")
	fmt.Fprintln(f, "# 量化间隙：multiply 连续上界 T=63.75/255，故 t<=63 为 NaN，t>=64 可达")
	header := "t"
	for _, m := range modes {
		header += fmt.Sprintf("\t%s", m)
	}
	fmt.Fprintln(f, header)
	tables := make(map[string][256]float64)
	for _, m := range modes {
		tables[m], err = comp.WeightTable(m)
		if err != nil {
			return err
		}
	}
	for t := 0; t < 256; t++ {
		line := fmt.Sprintf("%d", t)
		for _, m := range modes {
			line += "\t" + fmtWeight(tables[m][t])
		}
		fmt.Fprintln(f, line)
	}
	return nil
}

func showError(l comp.Layer) {
	if _, err := comp.Composite([]comp.Layer{l}); err != nil {
		fmt.Println("  ", err)
	} else {
		fmt.Println("   （未报错，异常）")
	}
}

func fmtWeight(w float64) string {
	if math.IsNaN(w) {
		return "NaN"
	}
	return fmt.Sprintf("%.6f", w)
}

func hashBytes(b []uint8) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "失败:", err)
	os.Exit(1)
}
