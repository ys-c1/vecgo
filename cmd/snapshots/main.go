// Command snapshots 是基准图的唯一生成入口。
//
// 用法:
//
//	go run ./cmd/snapshots <outdir>            # 生成当前树五段的基准图
//
// 生成后与 baseline 树的输出逐段比对（见 snap_test.go 与任务说明里的
// 复现脚本）；这里不写任何“自认正确”的基准，只如实出图。
package main

import (
	"fmt"
	"os"

	"vecgo/snap"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "用法: go run ./cmd/snapshots <outdir>")
		os.Exit(2)
	}
	if err := snap.RenderAll(os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, "生成失败:", err)
		os.Exit(1)
	}
	fmt.Println("已生成:", os.Args[1])
}
