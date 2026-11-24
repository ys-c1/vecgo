package main

import (
	"flag"
	"fmt"
	"os"

	"vecgo/config"
	"vecgo/geom"
	"vecgo/pipe"
	"vecgo/raster"
)

const usage = `用法: vecgo [选项] [输出路径]

渲染一帧：光栅化 -> 着色 -> 后处理(gamma/模糊) -> 导出 PGM。

选项:
  -scene string   场景 square|seam|overlap|stack (默认 square)
                  square  单个正方形，覆盖次数全为 1
                  seam    正方形沿对角线拆成两个三角形，接缝像素被两个图形覆盖
                  overlap 两个部分重叠的正方形，重叠区被两个图形覆盖
                  stack   四个错位正方形，中心区被三个以上图形覆盖
  -coverage       开启填充覆盖率统计：按覆盖次数分 1/2/3+ 三档统计像素数，
                  核对覆盖总次数与图形面积和，并列出覆盖次数最高的 10 个像素
  -coverage-csv string
                  若给出路径，把分桶表导出为 CSV（两列：次数区间、像素数）
  -w int          画幅宽度 (默认 64)
  -hh int         画幅高度 (默认 64)
  -scale float    缩放 (默认 1)
  -rotate float   旋转弧度 (默认 0)
  -ss int         超采样倍数 (默认 2)
  -shading string 着色模式 flat|gradient (默认 flat)
  -gamma float    伽马 (默认 1)
  -blur int       模糊半径 (默认 0)
  -o string       输出 PGM 路径 (默认 out.pgm)

说明:
  覆盖率统计只观察填充过程，不改变任何像素；关闭统计（默认）时导出
  的 PGM 与开启统计时逐字节相同，也与未加统计的旧版本相同。

场景与输出见 docs/coverage.md。
`

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "失败:", err)
		os.Exit(1)
	}
}

func run() error {
	def := config.Default()
	fs := flag.NewFlagSet("vecgo", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() { fmt.Fprint(os.Stderr, usage) }

	var (
		sceneName   = fs.String("scene", "square", "场景 square|seam|overlap|stack")
		coverage    = fs.Bool("coverage", false, "开启填充覆盖率统计")
		coverageCSV = fs.String("coverage-csv", "", "分桶表 CSV 导出路径（隐含开启统计）")
		width       = fs.Int("w", def.Width, "画幅宽度")
		height      = fs.Int("hh", def.Height, "画幅高度")
		scale       = fs.Float64("scale", def.Scale, "缩放")
		rotate      = fs.Float64("rotate", def.Rotate, "旋转弧度")
		ss          = fs.Int("ss", def.Supersample, "超采样倍数")
		shading     = fs.String("shading", def.Shading, "着色模式 flat|gradient")
		gamma       = fs.Float64("gamma", def.Gamma, "伽马")
		blur        = fs.Int("blur", def.Blur, "模糊半径")
		output      = fs.String("o", def.Output, "输出 PGM 路径")
	)
	if err := fs.Parse(os.Args[1:]); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}
	// 兼容旧用法：第一个位置参数是输出路径。
	if fs.NArg() > 1 {
		return fmt.Errorf("多余的参数: %v", fs.Args()[1:])
	}
	if fs.NArg() == 1 {
		*output = fs.Arg(0)
	}

	cfg := config.Config{
		Width:       *width,
		Height:      *height,
		Scale:       *scale,
		Rotate:      *rotate,
		Supersample: *ss,
		Shading:     *shading,
		Gamma:       *gamma,
		Blur:        *blur,
		Output:      *output,
	}
	if err := cfg.Validate(); err != nil {
		return err
	}

	scene, err := buildScene(*sceneName)
	if err != nil {
		return err
	}

	withStats := *coverage || *coverageCSV != ""
	var stats *raster.CoverageStats
	var im *raster.Image
	if withStats {
		im, stats, err = pipe.RunWithStats(scene, cfg)
	} else {
		im, err = pipe.Run(scene, cfg)
	}
	if err != nil {
		return err
	}
	if err := im.SavePGM(cfg.Output); err != nil {
		return fmt.Errorf("导出 %s: %w", cfg.Output, err)
	}
	fmt.Printf("on=%d size=%dx%d\n", im.CountOn(128), im.W, im.H)

	if *coverageCSV != "" {
		f, err := os.Create(*coverageCSV)
		if err != nil {
			return fmt.Errorf("导出分桶表 %s: %w", *coverageCSV, err)
		}
		werr := stats.WriteBucketCSV(f)
		cerr := f.Close()
		if werr != nil {
			return werr
		}
		if cerr != nil {
			return cerr
		}
	}
	if withStats {
		if err := stats.Report(os.Stdout); err != nil {
			return err
		}
	}
	return nil
}

// buildScene 返回命名场景。square 与旧版本的默认场景完全一致，
// 保证默认调用导出的产物不变。
func buildScene(name string) ([][]geom.Point, error) {
	switch name {
	case "square":
		return [][]geom.Point{{
			{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10},
		}}, nil
	case "seam":
		// 沿对角线 (0,0)-(10,10) 拆开：两个三角形严格共边，
		// 接缝两侧的像素会被两个图形各覆盖一次。
		return [][]geom.Point{
			{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}},
			{{X: 0, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}},
		}, nil
	case "overlap":
		// 两个 8×8 正方形错开 4：重叠区 4×4 被两个图形覆盖。
		return [][]geom.Point{
			{{X: 0, Y: 0}, {X: 8, Y: 0}, {X: 8, Y: 8}, {X: 0, Y: 8}},
			{{X: 4, Y: 4}, {X: 12, Y: 4}, {X: 12, Y: 12}, {X: 4, Y: 12}},
		}, nil
	case "stack":
		// 四个 8×8 正方形，四角分别向内错位 3。中央 2×2 区域被四个
		// 图形覆盖，用于展示 3+ 档。
		return [][]geom.Point{
			{{X: 0, Y: 0}, {X: 8, Y: 0}, {X: 8, Y: 8}, {X: 0, Y: 8}},
			{{X: 3, Y: 0}, {X: 11, Y: 0}, {X: 11, Y: 8}, {X: 3, Y: 8}},
			{{X: 0, Y: 3}, {X: 8, Y: 3}, {X: 8, Y: 11}, {X: 0, Y: 11}},
			{{X: 3, Y: 3}, {X: 11, Y: 3}, {X: 11, Y: 11}, {X: 3, Y: 11}},
		}, nil
	default:
		return nil, fmt.Errorf("未知场景: %s", name)
	}
}
