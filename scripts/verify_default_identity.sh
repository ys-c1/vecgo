#!/usr/bin/env bash
# 硬约束校验：默认采样配置下，cmd 产物必须与改动前捕获的黄金字节逐字节相同。
# 黄金文件 pipe/testdata/default_square.golden 的 sha256 为
# fbda3e5665174433272beab4f25172bc03466e3f8700bcf6007b32c3636f2dc3。
set -euo pipefail
cd "$(dirname "$0")/.."

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

go build -o "$tmp/vecgo" ./cmd
"$tmp/vecgo" "$tmp/out.pgm"

want_sha="fbda3e5665174433272beab4f25172bc03466e3f8700bcf6007b32c3636f2dc3"
got_sha="$(sha256sum "$tmp/out.pgm" | awk '{print $1}')"
golden_sha="$(sha256sum pipe/testdata/default_square.golden | awk '{print $1}')"

echo "实测 sha256 : $got_sha"
echo "黄金 sha256 : $golden_sha"
echo "基线 sha256 : $want_sha"

cmp "$tmp/out.pgm" pipe/testdata/default_square.golden
[ "$got_sha" = "$want_sha" ]
echo "PASS: 默认采样数下产物与现在逐字节相同"
