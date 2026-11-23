// bench 覆盖率量化两档对比测量台：
// 直边/斜边/细缝/尖角/纯色五个场景 × 64、256 两档图幅，
// 输出 8 位基准、4 位(关扩散)、4 位(开扩散)的平均误差、台阶数、体积、耗时。
package main

import (
	"bytes"
	"compress/gzip"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"time"

	"vecgo/geom"
	"vecgo/raster"
)

type scene struct {
	name  string
	polys [][]geom.Point
}

type variant struct {
	name    string
	bits    int
	diffuse bool
}

func rot(poly []geom.Point, deg float64) []geom.Point {
	c, s := math.Cos(deg*math.Pi/180), math.Sin(deg*math.Pi/180)
	out := make([]geom.Point, len(poly))
	for i, p := range poly {
		out[i] = geom.Point{X: p.X*c - p.Y*s, Y: p.X*s + p.Y*c}
	}
	return out
}

func buildScenes() []scene {
	// L 形：所有边均为轴对齐（直边），内部边 x=27/y=-27 不与子点网格对齐，
	// 因而能产生部分覆盖像素；外轮廓撑满 bbox。
	lShape := []geom.Point{
		{X: -40, Y: -40}, {X: 40, Y: -40}, {X: 40, Y: 40},
		{X: 27, Y: 40}, {X: 27, Y: -27}, {X: -40, Y: -27},
	}
	return []scene{
		// 直边：轴对齐 L 形
		{"直边", [][]geom.Point{lShape}},
		// 斜边：旋转 45° 的方块（边缘沿对角线，台阶最密集）
		{"斜边", [][]geom.Point{rot([]geom.Point{{X: -40, Y: -40}, {X: 40, Y: -40}, {X: 40, Y: 40}, {X: -40, Y: 40}}, 45)}},
		// 细缝：两堵墙夹一条窄缝，小图幅下为亚像素缝
		{"细缝", [][]geom.Point{
			{{X: -50, Y: -40}, {X: -1, Y: -40}, {X: -1, Y: 40}, {X: -50, Y: 40}},
			{{X: 1, Y: -40}, {X: 50, Y: -40}, {X: 50, Y: 40}, {X: 1, Y: 40}},
		}},
		// 尖角：12° 锐角三角形
		{"尖角", [][]geom.Point{{{X: -45, Y: -8}, {X: 45, Y: 0}, {X: -45, Y: 8}}}},
		// 纯色：铺满
		{"纯色", [][]geom.Point{{{X: -50, Y: -50}, {X: 50, Y: -50}, {X: 50, Y: 50}, {X: -50, Y: 50}}}},
	}
}

func gzipSize(b []byte) int {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, _ = zw.Write(b)
	_ = zw.Close()
	return buf.Len()
}

// timeCoverage 重复跑到累计 >=100ms，返回每次平均耗时与最后一次覆盖率。
func timeCoverage(size int, polys [][]geom.Point) ([]float64, time.Duration) {
	start := time.Now()
	var cov []float64
	n := 0
	for {
		var err error
		cov, err = raster.Coverage(size, size, polys, 8)
		if err != nil {
			panic(err)
		}
		n++
		if time.Since(start) >= 100*time.Millisecond {
			break
		}
	}
	return cov, time.Since(start) / time.Duration(n)
}

type row struct {
	scene  string
	size   int
	v      variant
	mae    float64
	steps  int
	pgm    int
	native int
	gz     int
	ns     time.Duration
}

func measure(outDir string) ([]row, error) {
	scenes := buildScenes()
	sizes := []int{64, 256}
	variants := []variant{
		{name: "8位基准", bits: 8, diffuse: false},
		{name: "4位关扩散", bits: 4, diffuse: false},
		{name: "4位开扩散", bits: 4, diffuse: true},
	}
	var rows []row
	for _, sc := range scenes {
		for _, size := range sizes {
			cov, covNs := timeCoverage(size, sc.polys)
			ref, err := raster.New(size, size)
			if err != nil {
				return nil, err
			}
			if err := raster.Quantize(cov, ref, 8, false); err != nil {
				return nil, err
			}
			for _, v := range variants {
				im, err := raster.New(size, size)
				if err != nil {
					return nil, err
				}
				// 量化计时：同样跑到累计 >=100ms。
				start := time.Now()
				qn := 0
				for {
					if err := raster.Quantize(cov, im, v.bits, v.diffuse); err != nil {
						return nil, err
					}
					qn++
					if time.Since(start) >= 100*time.Millisecond {
						break
					}
				}
				qNs := time.Since(start) / time.Duration(qn)
				mae, err := raster.MeanAbsError(ref, im)
				if err != nil {
					return nil, err
				}
				// 体积：内存中构造与落盘一致的字节，避免磁盘抖动。
				var pgm, native, gz int
				if v.bits == 8 {
					var buf bytes.Buffer
					fmt.Fprintf(&buf, "P5\n%d %d\n255\n", size, size)
					buf.Write(im.Pix)
					pgm, native, gz = buf.Len(), buf.Len(), gzipSize(buf.Bytes())
				} else {
					var buf bytes.Buffer
					fmt.Fprintf(&buf, "P5\n%d %d\n15\n", size, size)
					for _, b := range im.Pix {
						buf.WriteByte(b / 17)
					}
					pgm = buf.Len()
					gz = gzipSize(buf.Bytes())
					packed, err := im.Pack4()
					if err != nil {
						return nil, err
					}
					native = len(packed)
				}
				rows = append(rows, row{
					scene: sc.name, size: size, v: v,
					mae: mae, steps: raster.StaircaseCount(im),
					pgm: pgm, native: native, gz: gz,
					ns: covNs + qNs,
				})
			}
			if outDir != "" {
				base := fmt.Sprintf("%s_%d", sc.name, size)
				if err := ref.SavePGM(filepath.Join(outDir, base+"_8bit.pgm")); err != nil {
					return nil, err
				}
				for _, v := range []variant{{bits: 4, diffuse: false}, {bits: 4, diffuse: true}} {
					im, _ := raster.New(size, size)
					if err := raster.Quantize(cov, im, v.bits, v.diffuse); err != nil {
						return nil, err
					}
					suffix := "4flat"
					if v.diffuse {
						suffix = "4fs"
					}
					if err := im.SavePGM4(filepath.Join(outDir, base+"_"+suffix+".pgm")); err != nil {
						return nil, err
					}
				}
			}
		}
	}
	return rows, nil
}

func main() {
	outDir := flag.String("out", "artifacts/bench", "PGM 输出目录，空串则不落盘")
	flag.Parse()
	if *outDir != "" {
		if err := os.MkdirAll(*outDir, 0o755); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	rows, err := measure(*outDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "测量失败:", err)
		os.Exit(1)
	}
	fmt.Println("子点网格: 8×8=64 | 误差/台阶以 8 位逐像素输出为基准 | 耗时=覆盖率+量化，每段重复至累计≥100ms")
	fmt.Println()
	header := fmt.Sprintf("%-6s %-5s %-12s %8s %8s %10s %10s %10s %10s",
		"场景", "图幅", "档位", "MAE", "台阶数", "PGM字节", "4位原生", "gzip字节", "总耗时")
	for i, r := range rows {
		if i%3 == 0 {
			fmt.Println(header)
		}
		native := "-"
		if r.native > 0 {
			native = fmt.Sprintf("%d", r.native)
		}
		fmt.Printf("%-6s %-5d %-12s %8.3f %8d %10d %10s %10d %9v\n",
			r.scene, r.size, r.v.name, r.mae, r.steps, r.pgm, native, r.gz, r.ns)
		if i%3 == 2 {
			fmt.Println()
		}
	}
	// 斜边两图幅：台阶降幅核对（需求要求少四成）。
	fmt.Println("台阶降幅核对（斜边，相对 8 位基准）:")
	for _, size := range []int{64, 256} {
		var base, flat, fs int
		for _, r := range rows {
			if r.scene != "斜边" || r.size != size {
				continue
			}
			switch r.v.name {
			case "8位基准":
				base = r.steps
			case "4位关扩散":
				flat = r.steps
			case "4位开扩散":
				fs = r.steps
			}
		}
		pct := func(n int) float64 {
			if base == 0 {
				return 0
			}
			return (1 - float64(n)/float64(base)) * 100
		}
		fmt.Printf("  %d×%d: 8位=%d  4位关扩散=%d(%+.1f%%)  4位开扩散=%d(%.1f%%, 门槛-40%%: %v)\n",
			size, size, base, flat, pct(flat), fs, pct(fs), map[bool]string{true: "达标", false: "未达标"}[pct(fs) >= 40])
	}
}
