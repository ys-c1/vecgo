// Command sweepbench 运行扫描线求交基准：两档常规段数加一档极端退化档，
// 每档连跑十次验证逐字节确定性，并报告交点表 SHA-256 全串、总耗时与
// 插入/删除的实测复杂度（treap 路径长度与每操作纳秒）。
//
// 用法：
//
//	go run ./cmd/sweepbench [smallN] [largeN] [extremeN]
//
// 不带参数时使用内置默认档位 300 / 3000 / 20000。
package main

import (
	"encoding/hex"
	"fmt"
	"os"
	"time"

	"vecgo/sweep"
)

func main() {
	smallN, largeN, extremeN := 300, 3000, 20000
	if len(os.Args) > 1 {
		if _, err := fmt.Sscanf(os.Args[1], "%d", &smallN); err != nil {
			fatal("smallN 解析失败: %v", err)
		}
	}
	if len(os.Args) > 2 {
		if _, err := fmt.Sscanf(os.Args[2], "%d", &largeN); err != nil {
			fatal("largeN 解析失败: %v", err)
		}
	}
	if len(os.Args) > 3 {
		if _, err := fmt.Sscanf(os.Args[3], "%d", &extremeN); err != nil {
			fatal("extremeN 解析失败: %v", err)
		}
	}

	const seed = 20261124
	tiers := []struct {
		name    string
		n       int
		extreme bool
	}{
		{name: "小档", n: smallN},
		{name: "大档", n: largeN},
		{name: "极端退化档", n: extremeN, extreme: true},
	}

	fmt.Println("# vecgo 扫描线求交基准报告")
	fmt.Printf("# 生成时间: %s\n# 种子: %d\n\n", time.Now().Format(time.RFC3339), seed)

	for _, tier := range tiers {
		var in []sweep.Segment
		if tier.extreme {
			in = sweep.MakeExtremeFixture(tier.n, seed)
		} else {
			in = sweep.MakeFixture(tier.n, seed)
		}
		runTier(tier.name, in)
	}
}

func runTier(name string, in []sweep.Segment) {
	fmt.Printf("## %s（段数 n=%d）\n\n", name, len(in))

	// 连跑十次：哈希全串必须逐次相同，并记录每次总耗时。
	var firstHash string
	var totalTime time.Duration
	var lastStats *sweep.Stats
	var lastRows []sweep.Intersection
	for k := 0; k < 10; k++ {
		start := time.Now()
		rows, st, err := sweep.FindIntersections(in)
		elapsed := time.Since(start)
		if err != nil {
			fatal("%s 第 %d 次运行出错: %v", name, k+1, err)
		}
		digest := sweep.HashTable(rows)
		h := hex.EncodeToString(digest[:])
		if k == 0 {
			firstHash = h
		} else if h != firstHash {
			fatal("%s 第 %d 次哈希不一致:\n  首次 %s\n  本次 %s", name, k+1, firstHash, h)
		}
		totalTime += elapsed
		lastStats, lastRows = st, rows
	}
	avg := totalTime / 10

	fmt.Printf("- 十次运行哈希（SHA-256 全串，逐次相同）:\n  `%s`\n", firstHash)
	fmt.Printf("- 交点表行数: %d\n", len(lastRows))
	fmt.Printf("- 十次总耗时: %v；平均每次: %v\n", totalTime, avg)
	fmt.Println("- 实测插入/删除复杂度（计数器，非大 O 估计）:")
	fmt.Printf("  - 插入事件 %d 次：treap 路径深度累计 %d、最大 %d、平均 %.3f；插入计时合计 %d ns、每次 %.1f ns\n",
		lastStats.InsertEvents,
		lastStats.InsertDepthSum, lastStats.InsertDepthMax,
		avgDepth(lastStats.InsertDepthSum, int64(lastStats.InsertEvents)),
		lastStats.InsertNanos, avgNanos(lastStats.InsertNanos, int64(lastStats.InsertEvents)))
	fmt.Printf("  - 删除事件 %d 次：treap 下沉旋转累计 %d、最大 %d、平均 %.3f；删除计时合计 %d ns、每次 %.1f ns\n",
		lastStats.DeleteEvents,
		lastStats.DeleteDepthSum, lastStats.DeleteDepthMax,
		avgDepth(lastStats.DeleteDepthSum, int64(lastStats.DeleteEvents)),
		lastStats.DeleteNanos, avgNanos(lastStats.DeleteNanos, int64(lastStats.DeleteEvents)))
	fmt.Printf("  - 活动段状态树峰值: %d；交点批处理 %d 批、批内重建路径累计 %d；竖直扫描 %d 次\n",
		lastStats.MaxStatus, lastStats.SwapBatches, lastStats.SwapRebuildDepth, lastStats.VerticalScans)
	fmt.Printf("  - 调度交点事件 %d 个（去重后）\n\n", lastStats.ScheduledEvents)
}

func avgDepth(sum, count int64) float64 {
	if count == 0 {
		return 0
	}
	return float64(sum) / float64(count)
}

func avgNanos(sum, count int64) float64 {
	if count == 0 {
		return 0
	}
	return float64(sum) / float64(count)
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "错误: "+format+"\n", args...)
	os.Exit(1)
}
