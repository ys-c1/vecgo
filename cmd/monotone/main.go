// 命令 monotone 用三组控制点对比未加限制器与加限制器的三次插值：
// 逐组并列输出过冲数、方向反转数、形状偏差，并把每条渐变曲线渲染为
// PGM 灰度条带（可直接观察暗部回勾是否消失）。
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"vecgo/interp"
	"vecgo/raster"
)

type controlSet struct {
	name string
	x, y []float64
}

func sets() []controlSet {
	return []controlSet{
		{"A",
			[]float64{0, 1, 2, 3, 4},
			[]float64{0, 0, 0.1, 1, 1}},
		{"B",
			[]float64{0, 1, 2, 3, 4},
			[]float64{0, 0.2, 1, 0.9, 0}},
		{"C",
			[]float64{0, 2, 3, 5, 6},
			[]float64{1, 0.95, 0.4, 0.05, 0}},
	}
}

const (
	samplesPerSpan = 400
	rampWidth      = 256
	bandHeight     = 24
)

func main() {
	outDir := "artifacts"
	if len(os.Args) > 1 {
		outDir = os.Args[1]
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "创建目录失败:", err)
		os.Exit(1)
	}

	fmt.Printf("每区间密集采样 %d 点；截断=量化到像素域 [0,1] 时被夹住的样本数\n\n", samplesPerSpan)
	fmt.Printf("%-4s %-8s %-8s %-8s %-8s %-8s %-8s %-11s %-11s %-10s\n",
		"集合", "过冲原", "过冲限", "反转原", "反转限", "截断原", "截断限",
		"形状maxAbs", "形状RMS", "控制点偏差")
	fmt.Println("-----------------------------------------------------------------------------------------------")

	for _, cs := range sets() {
		raw, err := interp.New(cs.x, cs.y, false)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		lim, err := interp.New(cs.x, cs.y, true)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		maxAbs, rms := interp.ShapeDeviation(raw, lim, rampWidth)
		lo1, hi1 := raw.ClippedCount(rampWidth)
		lo2, hi2 := lim.ClippedCount(rampWidth)

		fmt.Printf("%-4s %-8d %-8d %-8d %-8d %-8d %-8d %-11.5f %-11.5f %-10.2e\n",
			cs.name,
			raw.OvershootCount(samplesPerSpan),
			lim.OvershootCount(samplesPerSpan),
			interp.ReversalCount(raw.Sample(cs.x[0], cs.x[len(cs.x)-1], rampWidth)),
			interp.ReversalCount(lim.Sample(cs.x[0], cs.x[len(cs.x)-1], rampWidth)),
			lo1+hi1, lo2+hi2,
			maxAbs, rms, lim.KnotError())

		fmt.Printf("     割线 Δ  : %v\n", interp.Secants(cs.x, cs.y))
		fmt.Printf("     原始切线: %v\n", raw.Tangents)
		fmt.Printf("     限制切线: %v\n", lim.Tangents)

		if err := renderRamp(filepath.Join(outDir, "ramp_"+cs.name+"_raw.pgm"), raw); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if err := renderRamp(filepath.Join(outDir, "ramp_"+cs.name+"_limited.pgm"), lim); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	fmt.Printf("\nPGM 渐变条带已写入 %s/ramp_{A,B,C}_{raw,limited}.pgm\n", outDir)
}

func renderRamp(path string, s *interp.Spline) error {
	im, err := raster.New(rampWidth, bandHeight)
	if err != nil {
		return err
	}
	x0, x1 := s.X[0], s.X[len(s.X)-1]
	for col := 0; col < rampWidth; col++ {
		z := x0 + (x1-x0)*float64(col)/float64(rampWidth-1)
		v := s.Eval(z)
		if v < 0 {
			v = 0
		} else if v > 1 {
			v = 1
		}
		g := uint8(255 * v)
		for row := 0; row < bandHeight; row++ {
			im.Pix[row*rampWidth+col] = g
		}
	}
	return im.SavePGM(path)
}
