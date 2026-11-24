#!/usr/bin/env bash
# 确定性核验脚本：
#   档 A：-gcflags=all=-N -l  关闭优化与内联
#   档 B：默认编译            开启全部优化
# 两档渲染同一组标准场景，逐字节比对 PGM，并各重跑 10 次核对 sha256。
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="$ROOT/artifacts/determinism"
BIN_A="$OUT/detrender_noopt"
BIN_B="$OUT/detrender_opt"
DIR_A="$OUT/tier_noopt"
DIR_B="$OUT/tier_opt"
RUNS=10

cd "$ROOT"
rm -rf "$DIR_A" "$DIR_B" "$BIN_A" "$BIN_B"
mkdir -p "$DIR_A" "$DIR_B"

echo "== 编译两档 =="
go build -gcflags="all=-N -l" -o "$BIN_A" ./cmd/detrender
go build -o "$BIN_B" ./cmd/detrender
echo "档 A 标志: -gcflags=all=-N -l （优化与内联关闭）"
echo "档 B 标志: 默认（优化开启）"

echo
echo "== 基线渲染 + 逐字节比对 =="
"$BIN_A" -out "$DIR_A" | sort
"$BIN_B" -out "$DIR_B" | sort
printf "%-12s %-10s %-10s %s\n" 场景 字节数 cmp 整文件sha256
all_equal=1
for f in "$DIR_A"/*.pgm; do
	name="$(basename "$f")"
	sz="$(stat -c %s "$f")"
	if cmp -s "$DIR_A/$name" "$DIR_B/$name"; then
		status="相同"
	else
		status="不同!"
		all_equal=0
	fi
	hash="$(sha256sum "$DIR_A/$name" | cut -d' ' -f1)"
	printf "%-12s %-10s %-10s %s\n" "${name%.pgm}" "$sz" "$status" "$hash"
done
[ "$all_equal" -eq 1 ] || { echo "字节比对存在差异"; exit 1; }

echo
echo "== 十次重跑哈希一致性（每档各 $RUNS 次）=="
for tier in noopt opt; do
	bin="$BIN_A"; [ "$tier" = opt ] && bin="$BIN_B"
	echo "-- 档 $tier --"
	for r in $(seq 1 "$RUNS"); do
		rdir="$OUT/run_${tier}_$r"
		rm -rf "$rdir"
		"$bin" -out "$rdir" | sort | awk -v r="$r" '{printf "run%02d  %s  %s\n", r, $1, $2}'
	done
done

echo
echo "== 跨全部重跑按场景归并哈希 =="
tmp="$(mktemp)"
for tier in noopt opt; do
	for r in $(seq 1 "$RUNS"); do
		for f in "$OUT/run_${tier}_$r"/*.pgm; do
			name="$(basename "$f" .pgm)"
			h="$(sha256sum "$f" | cut -d' ' -f1)"
			echo "$name $h" >> "$tmp"
		done
	done
done
# 基线两档也纳入
for f in "$DIR_A"/*.pgm; do
	name="$(basename "$f" .pgm)"
	echo "$name $(sha256sum "$f" | cut -d' ' -f1)" >> "$tmp"
done
for f in "$DIR_B"/*.pgm; do
	name="$(basename "$f" .pgm)"
	echo "$name $(sha256sum "$f" | cut -d' ' -f1)" >> "$tmp"
done
rc=0
report="$OUT/determinism_report.txt"
{
echo "确定性核验报告  生成于 $(date -u '+%Y-%m-%dT%H:%M:%SZ')"
echo "档 A: go build -gcflags=all=-N -l （关闭优化与内联）"
echo "档 B: go build （默认优化）"
echo "重跑次数: 每档 $RUNS 次，另加两档基线，共 $((RUNS*2+2)) 份/场景"
echo
echo "【一】两档基线 PGM 整文件逐字节比对"
printf "%-12s %-12s %-8s %s\n" 场景 整文件字节 cmp 整文件sha256
for f in "$DIR_A"/*.pgm; do
	name="$(basename "$f")"
	sz="$(stat -c %s "$f")"
	if cmp -s "$DIR_A/$name" "$DIR_B/$name"; then st="相同"; else st="不同"; rc=1; fi
	printf "%-12s %-12s %-8s %s\n" "${name%.pgm}" "$sz" "$st" "$(sha256sum "$f" | cut -d' ' -f1)"
done
echo
echo "【二】跨两档、$RUNS 次重跑哈希归并（像素 sha256）"
printf "%-12s %-10s %s\n" 场景 哈希种数 像素sha256
for name in single overlap large sliver degenerate; do
	n="$(grep -c "^$name " "$tmp")"
	uniq_n="$(grep "^$name " "$tmp" | awk '{print $2}' | sort -u | wc -l)"
	h="$(grep "^$name " "$tmp" | head -1 | awk '{print $2}')"
	printf "%-12s %-10s %s\n" "$name" "$uniq_n/$n" "$h"
	[ "$uniq_n" -eq 1 ] || rc=1
done
} | tee "$report"
rm -f "$tmp"
# 重跑目录已完成归并，清理磁盘（2048² 单份约 4 MiB），只保留两档基线与二进制。
for r in $(seq 1 "$RUNS"); do
	rm -rf "$OUT/run_noopt_$r" "$OUT/run_opt_$r"
done
[ "$rc" -eq 0 ] && echo "全部场景跨两档、$RUNS 次重跑哈希唯一，确定性核验通过。" | tee -a "$report"
echo "报告已写入: $report"
exit "$rc"
