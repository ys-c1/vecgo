// Command dedupreport prints the deduplication table for a set of sample scenes.
// It is a diagnostic; it lives outside the library packages.
package main

import (
	"bytes"
	"fmt"
	"os"

	"vecgo/config"
	"vecgo/dedup"
	"vecgo/geom"
	"vecgo/pipe"
)

func sq(x, y, s float64) []geom.Point {
	return []geom.Point{{X: x, Y: y}, {X: x + s, Y: y}, {X: x + s, Y: y + s}, {X: x, Y: y + s}}
}

func tri(x, y, s float64) []geom.Point {
	return []geom.Point{{X: x, Y: y}, {X: x + s, Y: y}, {X: x, Y: y + s}}
}

func main() {
	collinear := []geom.Point{{X: 0, Y: 0}, {X: 5, Y: 0}, {X: 10, Y: 0}}
	spike := []geom.Point{{X: 10, Y: 50}, {X: 10, Y: 50}, {X: 14, Y: 50}, {X: 30, Y: 52}}
	cases := []struct {
		name  string
		scene [][]geom.Point
	}{
		{"全同", [][]geom.Point{sq(0, 0, 10), sq(0, 0, 10), sq(0, 0, 10), sq(0, 0, 10)}},
		{"部分同", [][]geom.Point{sq(0, 0, 10), sq(20, 0, 5), sq(0, 0, 10), sq(20, 0, 5), tri(40, 40, 6)}},
		{"全不同", [][]geom.Point{sq(0, 0, 10), sq(20, 0, 10), sq(40, 0, 10), sq(60, 0, 10)}},
		{"单图形", [][]geom.Point{tri(3, 3, 7)}},
		{"退化图形", [][]geom.Point{collinear, spike, collinear, spike, sq(0, 0, 10)}},
	}
	cfg := config.Default()
	cfg.Width, cfg.Height = 24, 24

	fmt.Printf("| 用例 | 提交 | 去重后 | 重复 | 重复率 | 反解位数 | 实际位数 | 绘制次数 | 逐字节不变 |\n")
	fmt.Printf("|---|---|---|---|---|---|---|---|---|\n")
	fail := false
	for _, c := range cases {
		merged, stats, err := pipe.RunWithStats(c.scene, cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", c.name, err)
			os.Exit(1)
		}
		base, err := pipe.RenderUndeduplicated(c.scene, cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", c.name, err)
			os.Exit(1)
		}
		same := bytes.Equal(merged.Pix, base.Pix)
		drawnOK := stats.Drawn == stats.Unique
		bitsOK := stats.FingerprintBits <= stats.RequiredBits
		if !(same && drawnOK && bitsOK) {
			fail = true
		}
		fmt.Printf("| %s | %d | %d | %d | %.2f | %d | %d | %d | %v |\n",
			c.name, stats.Submitted, stats.Unique, stats.Duplicates, float64(stats.Duplicates)/float64(stats.Submitted),
			stats.RequiredBits, stats.FingerprintBits, stats.Drawn, same)
	}
	// Error cases.
	type bad struct {
		name  string
		scene [][]geom.Point
	}
	for _, b := range []bad{
		{"空图形（1 号）", [][]geom.Point{sq(0, 0, 10), {}, sq(1, 1, 2)}},
		{"顶点不足三（1 号）", [][]geom.Point{sq(0, 0, 10), {{X: 0, Y: 0}, {X: 1, Y: 1}}}},
	} {
		res, err := dedup.MergeAuto(b.scene)
		_ = res
		fmt.Printf("| %s | 拒绝: %v | | | | | | | |\n", b.name, err)
	}
	if fail {
		os.Exit(1)
	}
}
