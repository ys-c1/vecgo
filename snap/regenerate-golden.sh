#!/usr/bin/env bash
# 从原始基线（改动前的 HEAD）重新生成 snap/testdata/golden。
#
# 前提：当前仓库里 snap/render.go 只依赖 config/geom/raster/shading/post
# 五个包——这五个包在基线树上的“成功路径”必须未被改动，否则本脚本会
# 用改动后的契约生成基准，就失去“逐像素对得上”的意义。
#
# 用法:
#   snap/regenerate-golden.sh [<baseline-ref>]
#   <baseline-ref> 省略时取 $BASELINE_REF，再取 git merge-base HEAD origin/main；
#   本仓库的真实基线是改动前的提交 fad87b8。
#
# 脚本做的事：
#   1) 把 <baseline-ref> 的树导出到临时目录（不含当前工作区改动）；
#   2) 把当前树的 snap/render.go 复制成独立包 vecgo/snapgen（只含输入矩阵
#      与 PGM 写出，不 import verr，故可在基线树上编译）；
#   3) 在基线树上编译运行它，输出到 snap/testdata/golden。
#
# 为什么用“当前树的 render.go + 基线树的数据包”：render.go 只是输入矩阵
# 和 PGM 写出，不含被测行为；被测的是五个包在基线上的输出。若 render.go
# 自身改了输入矩阵，基准与当前树就都变——这正是我们希望被察觉的。
set -euo pipefail

root=$(git rev-parse --show-toplevel)
ref=${1:-${BASELINE_REF:-$(git merge-base HEAD origin/main 2>/dev/null || true)}}
if [[ -z "$ref" ]]; then
  echo "需要基线 ref：snap/regenerate-golden.sh <baseline-ref>" >&2
  exit 2
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

git archive "$ref" | tar -x -C "$tmp"

mkdir -p "$tmp/snapgen" "$tmp/snapgencmd"
cp "$root/snap/render.go" "$tmp/snapgen/render.go"
# render.go 是 package snap；在基线树里把它作为独立包 vecgo/snapgen。
sed -i 's/^package snap$/package snapgen/' "$tmp/snapgen/render.go"

cat > "$tmp/snapgencmd/main.go" <<'EOF'
package main

import (
	"fmt"
	"os"

	"vecgo/snapgen"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "用法: snapgen <outdir>")
		os.Exit(2)
	}
	if err := snapgen.RenderAll(os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, "生成失败:", err)
		os.Exit(1)
	}
	fmt.Println("已生成:", os.Args[1])
}
EOF

echo "基线: $ref"
echo "基线树: $tmp"
(cd "$tmp" && go run ./snapgencmd "$root/snap/testdata/golden")
echo "已写入 $root/snap/testdata/golden"
