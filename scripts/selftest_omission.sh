#!/bin/bash
# 缺省实验的自检：往临时副本里注入已知故障，确认实验会变红。
# 一个永远都是 0 的比对没有判别力；这里证明失败能被抓到。
#
# 故障清单：
#   1. Default() 的 Gamma 改成 1.2（代码读错默认值，注释不动）
#   2. Default() 的 Shading 改成 gradient（代码读错默认值，且像素可观测）
#   3. 注释把 Gamma 写成 2（注释写错，代码不动）
set -euo pipefail
cd "$(dirname "$0")/.."
ROOT=$(pwd)
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

fails=0
# expect_fail <名称> <目录> <命令>
expect_fail() {
  local name=$1 dir=$2; shift 2
  if ( cd "$dir" && "$@" ) >/dev/null 2>&1; then
    echo "FAIL  $name：故障未被检出"
    fails=$((fails + 1))
  else
    echo "ok    $name"
  fi
}
# expect_output <名称> <期望片段> <目录> <命令>
expect_output() {
  local name=$1 want=$2 dir=$3; shift 3
  local out
  out=$( cd "$dir" && "$@" 2>&1 || true )
  if printf '%s' "$out" | grep -q -- "$want"; then
    echo "ok    $name"
  else
    echo "FAIL  $name：输出中没有 $want"
    fails=$((fails + 1))
  fi
}

# 故障 1：Default().Gamma 改错。
cp -a "$ROOT" "$TMP/gamma"
sed -i 's|Gamma:       1,         // 默认值: 1|Gamma:       1.2,       // 默认值: 1|' "$TMP/gamma/config/config.go"
expect_fail "Gamma 改错 → 注释核对失败" "$TMP/gamma" go test ./config -run TestCommentsMatchDefault
expect_fail "Gamma 改错 → Default 冻结值失败" "$TMP/gamma" go test ./config -run TestDefaultUnchanged
expect_fail "Gamma 改错 → 缺省实验判为不一致" \
  bash -c "cd '$TMP/gamma' && go run ./cmd/omission -out '$TMP/out-gamma' >/dev/null && grep -qE '^gamma .*不一致' '$TMP/out-gamma/summary.txt'"

# 故障 2：Default().Shading 改错（像素可观测）。
cp -a "$ROOT" "$TMP/shading"
sed -i 's|Shading:     "flat",    // 默认值: "flat"|Shading:     "gradient", // 默认值: "flat"|' "$TMP/shading/config/config.go"
expect_fail "Shading 改错 → 缺省实验判为不一致" \
  bash -c "cd '$TMP/shading' && go run ./cmd/omission -out '$TMP/out-shading' >/dev/null && grep -qE '^shading .*不一致' '$TMP/out-shading/summary.txt'"
expect_fail "Shading 改错 → 全缺省偏离改动前基线" \
  bash -c "cd '$TMP/shading' && go run ./cmd/omission -out '$TMP/out-shading2' >/dev/null && grep -q '不一致' '$TMP/out-shading2/summary.txt' && grep -q '全缺省基线比对: 与改动前产物不一致' '$TMP/out-shading2/summary.txt'"

# 故障 3：注释写错。
cp -a "$ROOT" "$TMP/comment"
sed -i 's|Gamma:       1,         // 默认值: 1|Gamma:       1,         // 默认值: 2|' "$TMP/comment/config/config.go"
expect_output "注释写错 → 报出“字段 Gamma 注释不一致”" "字段 Gamma 注释不一致" "$TMP/comment" go test ./config -run TestCommentsMatchDefault

echo
if [ "$fails" -ne 0 ]; then
  echo "自检失败：$fails 项"
  exit 1
fi
echo "自检通过：注入的故障全部被检出。"
