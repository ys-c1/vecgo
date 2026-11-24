package main

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"

	"vecgo/config"
	"vecgo/geom"
	"vecgo/pipe"
)

// shape 是一类待验证的多边形及其名称。
type shape struct {
	Name string
	Poly []geom.Point
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "失败:", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) < 2 {
		return errors.New("用法: vecgo <输出目录> [覆盖|采样]")
	}
	outDir := os.Args[1]
	fill := "coverage"
	if len(os.Args) > 2 {
		switch os.Args[2] {
		case "覆盖", "coverage":
			fill = "coverage"
		case "采样", "supersample":
			fill = "supersample"
		default:
			return fmt.Errorf("未知填充方式: %s", os.Args[2])
		}
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}

	shapes := []shape{
		{"triangle", []geom.Point{{X: 0, Y: 0}, {X: 7, Y: 0}, {X: 2.5, Y: 5}}},
		{"pentagon", regular(5, 5, -math.Pi/2)},
		{"star", star(10, 6, 2.4)},
	}
	for _, s := range shapes {
		if !geom.IsSimple(s.Poly) {
			return fmt.Errorf("%s 不是简单多边形", s.Name)
		}
	}

	for _, s := range shapes {
		if err := renderShape(s, fill, outDir); err != nil {
			return err
		}
	}
	return nil
}

func renderShape(s shape, fill, outDir string) error {
	cfg := config.Default()
	cfg.Width, cfg.Height = 96, 96
	cfg.Scale = 8
	cfg.FillMode = fill
	cfg.FillThreshold = 0.5
	cfg.Output = filepath.Join(outDir, s.Name+"-weighted.pgm")
	cfg.BinaryOutput = filepath.Join(outDir, s.Name+"-binary.pgm")

	res, err := pipe.Run([][]geom.Point{s.Poly}, cfg)
	if err != nil {
		return err
	}
	// 变换后顶点的精确面积/周长/顶点数均取自同一条变换路径。
	moved := pipe.Transform(s.Poly, cfg.Scale, cfg.Rotate)
	n := len(moved)
	perim := geom.Perimeter(moved)
	exact := geom.Area(moved)

	if err := res.Weighted.SavePGM(cfg.Output); err != nil {
		return err
	}
	if res.Binary != nil {
		if err := res.Binary.SavePGM(cfg.BinaryOutput); err != nil {
			return err
		}
	}

	fmt.Printf("== %s (%s) ==\n", s.Name, fill)
	fmt.Printf("vertices   %d\n", n)
	fmt.Printf("perimeter  %.6f\n", perim)
	fmt.Printf("exact      %.6f\n", exact)
	if res.Cov != nil {
		fmt.Printf("weighted   %.6f\n", res.WeightedArea)
		fmt.Printf("relerr     %.3e\n", res.RelErr)
		fmt.Printf("binary     %.6f\n", res.BinArea)
		fmt.Printf("bin_diff   %.6f\n", res.BinDiff)
		fmt.Printf("bin_l1     %.6f\n", res.BinL1)
		fmt.Printf("bin_bound  %.6f (partial=%d)\n", res.BinBound, res.Partial)
	} else {
		fmt.Printf("sampled    %.6f\n", res.SupersampleArea)
		rel := 0.0
		if exact > 0 {
			rel = math.Abs(res.SupersampleArea-exact) / exact
		}
		fmt.Printf("relerr     %.3e\n", rel)
		fmt.Printf("(supersample=%d，无二值图)\n", cfg.Supersample)
	}
	fmt.Printf("weighted_pgm %s\n", cfg.Output)
	if res.Binary != nil {
		fmt.Printf("binary_pgm   %s\n", cfg.BinaryOutput)
	}
	fmt.Println()
	if res.Cov != nil && res.RelErr >= 1e-3 {
		return fmt.Errorf("%s 加权面积相对误差 %.3e 超过千分之一", s.Name, res.RelErr)
	}
	return nil
}

func regular(k int, r, start float64) []geom.Point {
	out := make([]geom.Point, k)
	for i := range out {
		a := start + 2*math.Pi*float64(i)/float64(k)
		out[i] = geom.Point{X: r * math.Cos(a), Y: r * math.Sin(a)}
	}
	return out
}

func star(points int, rOuter, rInner float64) []geom.Point {
	out := make([]geom.Point, points)
	for i := range out {
		r := rOuter
		if i%2 == 1 {
			r = rInner
		}
		a := -math.Pi/2 + math.Pi*float64(i)/float64(points/2)
		out[i] = geom.Point{X: r * math.Cos(a), Y: r * math.Sin(a)}
	}
	return out
}
