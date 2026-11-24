package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"vecgo/batch"
	"vecgo/config"
	"vecgo/geom"
)

func main() {
	dir := flag.String("dir", "out", "批次产物目录")
	isolate := flag.Bool("isolate", true, "逐场景隔离：失败跳过并记账，批次继续；关闭后首个失败即中断（原行为）")
	flag.Parse()

	cfg := config.Default()
	scenes := []batch.Scene{
		{Name: "square", Polys: [][]geom.Point{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}}},
		{Name: "triangle", Polys: [][]geom.Point{{{X: 0, Y: 0}, {X: 20, Y: 0}, {X: 10, Y: 18}}}},
		{Name: "bad-degenerate", Polys: [][]geom.Point{{{X: 0, Y: 0}, {X: 1, Y: 1}}}}, // 顶点不足，渲染报错
		{Name: "bad-empty", Polys: nil},                                              // 空场景，渲染时 panic
		{Name: "rect", Polys: [][]geom.Point{{{X: 0, Y: 0}, {X: 30, Y: 0}, {X: 30, Y: 12}, {X: 0, Y: 12}}}},
	}

	start := time.Now()
	results, err := batch.RunBatch(scenes, cfg, *dir, *isolate)
	total := time.Since(start)
	if err != nil {
		fmt.Fprintln(os.Stderr, "失败:", err)
		os.Exit(1)
	}
	batch.WriteReport(os.Stdout, results, total)
}
