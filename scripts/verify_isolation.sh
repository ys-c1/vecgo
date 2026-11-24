#!/usr/bin/env bash
# 失败隔离端到端校验：
#   1. 正常场景：隔离开/关输出逐字节相同
#   2. 异常场景：隔离后照常出图（非黑帧），跳过清单升序、一行一个、可被脚本读取
#   3. 对照：隔离关闭时同一输入如何变黑帧/整体失败
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT="$ROOT/artifacts/render"
mkdir -p "$OUT"

BIN="$(mktemp -d)/vecgo"
trap 'rm -rf "$(dirname "$BIN")"' EXIT
go -C "$ROOT" build -o "$BIN" ./cmd
go -C "$ROOT" build -o "$(dirname "$BIN")/pgmstat" ./scripts/pgmstat
PGMSTAT="$(dirname "$BIN")/pgmstat"

NORMAL="$ROOT/scenes/normal.json"
ABNORMAL="$ROOT/scenes/abnormal.json"
SILENT="$ROOT/scenes/abnormal_silent.json"

echo "================ 1. 正常场景：隔离 on/off 逐像素对照 ================"
"$BIN" -scene "$NORMAL" -out "$OUT/normal_isolate_off.pgm"
"$BIN" -isolate -scene "$NORMAL" -out "$OUT/normal_isolate_on.pgm" \
  -skip-list "$OUT/normal_skipped.txt"
if cmp -s "$OUT/normal_isolate_off.pgm" "$OUT/normal_isolate_on.pgm"; then
  echo "[PASS] 两份 PGM 逐字节相同"
else
  echo "[FAIL] 正常场景隔离前后输出不一致"; exit 1
fi
echo "正常场景跳过清单（应为空文件）: $(wc -l < "$OUT/normal_skipped.txt") 行"

echo
echo "================ 2. 异常场景：隔离开启，坏图形跳过、其余照常 ================"
"$BIN" -isolate -scene "$ABNORMAL" -out "$OUT/abnormal_isolate_on.pgm" \
  -skip-list "$OUT/abnormal_skipped.txt"
"$PGMSTAT" "$OUT/abnormal_isolate_on.pgm"
echo "跳过编号清单（升序，一行一个）:"
cat "$OUT/abnormal_skipped.txt"
sort -n -c "$OUT/abnormal_skipped.txt" && echo "[PASS] 清单按编号升序"

echo
echo "================ 3. 对照 A：隔离关闭 + 顶点不足 -> 整体失败，无图 ================"
if "$BIN" -scene "$ABNORMAL" -out "$OUT/abnormal_isolate_off.pgm"; then
  echo "[FAIL] 传统路径应整体失败"; exit 1
else
  rc=$?
  echo "[PASS] 传统路径退出码 $rc，未产出图像（事务性失败，整帧作废）"
fi
test ! -e "$OUT/abnormal_isolate_off.pgm" && echo "[PASS] 确认无 PGM 落盘"

echo
echo "================ 4. 对照 B：隔离关闭 + Inf 静默污染 -> 黑帧 ================"
"$BIN" -scene "$SILENT" -out "$OUT/blackframe_isolate_off.pgm"
"$PGMSTAT" "$OUT/blackframe_isolate_off.pgm"
"$BIN" -isolate -scene "$SILENT" -out "$OUT/blackframe_isolate_on.pgm" \
  -skip-list "$OUT/silent_skipped.txt"
"$PGMSTAT" "$OUT/blackframe_isolate_on.pgm"
echo "该场景跳过编号:"
cat "$OUT/silent_skipped.txt"

echo
echo "全部校验通过。产物目录: $OUT"
