#!/bin/bash
# 一键复现逐字段缺省实验。
#
# 前置：需要一个改动前 HEAD 的 git worktree，路径由 BEFORE_WORKTREE 指定
# （默认 /tmp/vecgo-head）。它用于导出冻结默认值和改动前基线图，
# 避免拿改动后的代码给自己作证。创建方式见 artifacts/omission/README.md。
set -euo pipefail
cd "$(dirname "$0")/.."
ROOT=$(pwd)
OUT=${1:-artifacts/omission}
BEFORE_WORKTREE=${BEFORE_WORKTREE:-/tmp/vecgo-head}

if [ ! -d "$BEFORE_WORKTREE" ]; then
  echo "缺少改动前 worktree: $BEFORE_WORKTREE" >&2
  echo "先执行: git worktree add --detach $BEFORE_WORKTREE \$(git rev-parse HEAD)" >&2
  exit 1
fi

echo "== 1/4 导出改动前冻结默认值 =="
mkdir -p "$BEFORE_WORKTREE/tools/freeze"
cat > "$BEFORE_WORKTREE/tools/freeze/main.go" <<'GO'
package main

import (
	"fmt"
	"strconv"

	"vecgo/config"
)

func main() {
	d := config.Default()
	fmt.Printf("width\t%d\n", d.Width)
	fmt.Printf("height\t%d\n", d.Height)
	fmt.Printf("scale\t%s\n", strconv.FormatFloat(d.Scale, 'g', -1, 64))
	fmt.Printf("rotate\t%s\n", strconv.FormatFloat(d.Rotate, 'g', -1, 64))
	fmt.Printf("supersample\t%d\n", d.Supersample)
	fmt.Printf("shading\t%s\n", strconv.Quote(d.Shading))
	fmt.Printf("gamma\t%s\n", strconv.FormatFloat(d.Gamma, 'g', -1, 64))
	fmt.Printf("blur\t%d\n", d.Blur)
	fmt.Printf("output\t%s\n", strconv.Quote(d.Output))
}
GO
(cd "$BEFORE_WORKTREE" && go run ./tools/freeze) > artifacts/defaults_before.txt

echo "== 2/4 渲染改动前基线（实验场景 + cmd 默认场景） =="
mkdir -p "$BEFORE_WORKTREE/tools/probe"
cat > "$BEFORE_WORKTREE/tools/probe/main.go" <<'GO'
package main

import (
	"fmt"
	"os"

	"vecgo/config"
	"vecgo/geom"
	"vecgo/pipe"
)

func save(scene [][]geom.Point, path string) {
	im, err := pipe.Run(scene, config.Default())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := im.SavePGM(path); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}

func main() {
	out := os.Args[1]
	save([][]geom.Point{{
		{X: 2.256, Y: -0.898},
		{X: 10.897, Y: 2.256},
		{X: 7.744, Y: 10.897},
		{X: -0.897, Y: 7.744},
	}}, out+"/00_before_HEAD.pgm")
	save([][]geom.Point{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}}, out+"/00_before_HEAD_cmdscene.pgm")
}
GO
mkdir -p "$OUT"
(cd "$BEFORE_WORKTREE" && go run ./tools/probe "$ROOT/$OUT")

echo "== 3/4 运行缺省实验 =="
go run ./cmd/omission -out "$OUT"

echo "== 4/4 单元测试 =="
go test ./config ./pipe

echo "完成。报告: $OUT/report.md"
