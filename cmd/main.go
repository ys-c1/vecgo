package main

import (
	"flag"
	"fmt"
	"os"

	"vecgo/cache"
	"vecgo/config"
	"vecgo/geom"
	"vecgo/pipe"
)

func main() {
	capacity := flag.Int("cache-capacity", config.DefaultCacheCapacity, "LRU entry capacity (used with -cache)")
	enable := flag.Bool("cache", false, "enable the per-polygon mask cache")
	statsPath := flag.String("cache-stats", "", "write per-entry hit statistics to this file (implies -cache)")
	flag.Parse()

	cfg := config.Default()
	if *enable || *statsPath != "" {
		cfg.CacheEnabled = true
		cfg.CacheCapacity = *capacity
	}
	if args := flag.Args(); len(args) > 0 {
		cfg.Output = args[0]
	}
	scene := [][]geom.Point{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}}

	var c *cache.Cache
	if cfg.CacheEnabled {
		var err error
		c, err = cache.New(cfg.CacheCapacity)
		if err != nil {
			fmt.Fprintln(os.Stderr, "缓存初始化失败:", err)
			os.Exit(1)
		}
	}
	im, err := pipe.RunCached(scene, cfg, c)
	if err != nil {
		fmt.Fprintln(os.Stderr, "失败:", err)
		os.Exit(1)
	}
	if err := im.SavePGM(cfg.Output); err != nil {
		fmt.Fprintln(os.Stderr, "导出失败:", err)
		os.Exit(2)
	}
	fmt.Printf("on=%d size=%dx%d\n", im.CountOn(128), im.W, im.H)

	if c != nil && *statsPath != "" {
		f, err := os.Create(*statsPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "创建统计文件失败:", err)
			os.Exit(2)
		}
		if err := c.WriteTable(f); err != nil {
			f.Close()
			fmt.Fprintln(os.Stderr, "写统计失败:", err)
			os.Exit(2)
		}
		if err := f.Close(); err != nil {
			fmt.Fprintln(os.Stderr, "关闭统计文件失败:", err)
			os.Exit(2)
		}
	}
}
