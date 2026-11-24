#!/usr/bin/env bash
# 验证共享图像缓冲改动：
#   1. 五处输出（光栅化/着色/gamma/模糊/命令行导出）与改动前逐像素比对
#   2. 打印两侧的整图分配、整图拷贝、阶段取图计数
#   3. 采样内存曲线，核对峰值不超过输入原图两倍
#
# 用法: scripts/verify.sh [画幅 ...]
#   BASE_REF 指定对照的 git 版本，默认 HEAD（即改动前）。
# 对照实现从 git 导出到临时目录，仅在隔离副本里加计数，不污染工作树。
set -euo pipefail

cd "$(dirname "$0")/.."
BASE_REF=${BASE_REF:-HEAD}
SIZES=("$@")
if [ ${#SIZES[@]} -eq 0 ]; then
  SIZES=(256 1024 2048 4096)
fi

WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT
mkdir -p "$WORK/base" "$WORK/new"
git archive "$BASE_REF" | tar -x -C "$WORK/base"
tar --exclude=.git --exclude=artifacts -cf - . | tar -x -C "$WORK/new"

# ---- 在基线副本里为整图分配/拷贝加计数（仅度量，不改行为）----
python3 - "$WORK/base" <<'PY'
import pathlib, sys
root = pathlib.Path(sys.argv[1])
p = root/"raster/raster.go"; s = p.read_text()
s = s.replace('import (\n\t"errors"', 'import (\n\t"errors"\n\t"sync/atomic"')
s = s.replace('type Image struct {', 'var FrameAllocs atomic.Int64\n\ntype Image struct {')
s = s.replace('\treturn &Image{W: w, H: h, Pix: make([]uint8, w*h)}, nil',
              '\tFrameAllocs.Add(1)\n\treturn &Image{W: w, H: h, Pix: make([]uint8, w*h)}, nil')
p.write_text(s)
p = root/"post/post.go"; s = p.read_text()
s = s.replace('import (\n\t"errors"', 'import (\n\t"errors"\n\t"sync/atomic"')
s = s.replace('func Gamma(', 'var FullCopies atomic.Int64\n\nfunc Gamma(')
s = s.replace('\t\tout, _ := raster.New(im.W, im.H)\n\t\tcopy(out.Pix, im.Pix)',
              '\t\tout, _ := raster.New(im.W, im.H)\n\t\tcopy(out.Pix, im.Pix)\n\t\tFullCopies.Add(1)')
s = s.replace('\tout, _ := raster.New(im.W, im.H)\n\tfor y := 0;',
              '\tout, _ := raster.New(im.W, im.H)\n\tFullCopies.Add(1)\n\tfor y := 0;')
p.write_text(s)
PY

# ---- 五阶段快照 harness：两侧各一份，仅计数来源不同 ----
snap_common=$(cat <<'GO'
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"vecgo/config"
	"vecgo/geom"
	"vecgo/pipe"
	"vecgo/post"
	"vecgo/raster"
	"vecgo/shading"
)

func scene() [][]geom.Point {
	return [][]geom.Point{{
		{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 4}, {X: 6, Y: 4},
		{X: 6, Y: 10}, {X: 0, Y: 10},
	}}
}

func cfg() config.Config {
	c := config.Default()
	c.Width, c.Height = 48, 40
	c.Supersample = 2
	c.Shading = "gradient"
	c.Gamma = 2.2
	c.Blur = 3
	c.Rotate = 0.3
	c.Scale = 1
	return c
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func main() {
	out := os.Args[1]
	must(os.MkdirAll(out, 0o755))
	c := cfg()
	im, err := raster.New(c.Width, c.Height)
	must(err)
	moved := make([][]geom.Point, len(scene()))
	for i, poly := range scene() {
		moved[i] = pipe.Transform(poly, c.Scale, c.Rotate)
	}
	must(im.Fill(moved, c.Supersample))
	must(os.WriteFile(filepath.Join(out, "01_raster.raw"), im.Pix, 0o644))
	must(shading.Apply(im, c.Shading, 1.0, 0.0))
	must(os.WriteFile(filepath.Join(out, "02_shading.raw"), im.Pix, 0o644))
	must(post.Gamma(im, c.Gamma))
	must(os.WriteFile(filepath.Join(out, "03_post_gamma.raw"), im.Pix, 0o644))
	bl, err := post.Blur(im, c.Blur)
	must(err)
	must(os.WriteFile(filepath.Join(out, "04_post_blur.raw"), bl.Pix, 0o644))
	path := filepath.Join(out, "05_cmd_output.pgm")
	must(bl.SavePGM(path))
	pgm, err := os.ReadFile(path)
	must(err)
	must(os.WriteFile(filepath.Join(out, "05_cmd_output.raw"), pgm, 0o644))
GO
)

mkdir -p "$WORK/base/cmd/snap" "$WORK/new/cmd/snap"
{
  echo "$snap_common"
  echo '	fmt.Printf("allocs=%d copies=%d\n", raster.FrameAllocs.Load(), post.FullCopies.Load())'
  echo '}'
} > "$WORK/base/cmd/snap/main.go"
{
  echo "$snap_common"
  echo '	st := raster.Snapshot()'
  echo '	fmt.Printf("allocs=%d copies=%d leases=%d\n", st.FrameAllocs, st.FullCopies, st.StageLeases)'
  echo '}'
} > "$WORK/new/cmd/snap/main.go"

BASE_BIN="$WORK/snap-base"
NEW_BIN="$WORK/snap-new"
(cd "$WORK/base" && go build -o "$BASE_BIN" ./cmd/snap)
(cd "$WORK/new" && go build -o "$NEW_BIN" ./cmd/snap)

echo
echo "== 五处输出逐像素比对（含计数）=="
echo "| 输出 | 比对 |"
echo "|---|---|"
"$BASE_BIN" "$WORK/out/base" | sed 's/^/  改前: /'
"$NEW_BIN" "$WORK/out/new" | sed 's/^/  改后: /'
status=0
for f in 01_raster.raw 02_shading.raw 03_post_gamma.raw 04_post_blur.raw 05_cmd_output.raw; do
  if cmp -s "$WORK/out/base/$f" "$WORK/out/new/$f"; then
    echo "| $f | 一致 |"
  else
    echo "| $f | 不一致 |"
    status=1
  fi
done

# ---- 内存曲线 ----
echo
echo "== 内存曲线（峰值 / 输入原图）=="
echo "| 画幅 | 改后 峰值比 | 采样点 |"
echo "|---|---|---|"
CLI_BIN="$WORK/vecgo"
(cd "$WORK/new" && go build -o "$CLI_BIN" ./cmd)
for s in "${SIZES[@]}"; do
  line=$("$CLI_BIN" -w "$s" -hh "$s" -blur 3 -gamma 2.2 \
    -shading gradient -rotate 0.3 -ss 2 \
    -o "$WORK/out/$s.pgm" -mem-curve "$WORK/out/$s.csv" 2>/dev/null || true)
  ratio=$(printf '%s\n' "$line" | sed -n 's/.*peak_ratio=\([0-9.]*\).*/\1/p')
  ns=$(printf '%s\n' "$line" | sed -n 's/.*mem_samples=\([0-9]*\).*/\1/p')
  echo "| ${s}x${s} | $ratio | $ns |"
done

echo
echo "== 命令行计数 =="
"$CLI_BIN" -w 256 -hh 256 -blur 3 -o "$WORK/out/cli.pgm" | grep frame_allocs

if [ $status -ne 0 ]; then
  echo "五处输出存在不一致" >&2
  exit 1
fi
echo "全部一致。"
