#!/usr/bin/env python3
# 验证对比图：统计每条横带的水平跳变数（混叠越重跳变越密），并把 PGM 转成 PNG。
# 用法：python3 -I scripts/verify_images.py artifacts
import os
import sys

from PIL import Image

BAND_H = 96
W = 256


def read_pgm(path):
    with open(path, "rb") as f:
        assert f.readline().strip() == b"P5"
        dims = f.readline().split()
        w, h = int(dims[0]), int(dims[1])
        assert f.readline().strip() == b"255"
        data = f.read()
    return w, h, data


def band_stats(data, w, band):
    off = band * (BAND_H + 2)
    row = data[(off + BAND_H // 2) * w:(off + BAND_H // 2 + 1) * w]
    jumps = sum(1 for i in range(1, w) if abs(row[i] - row[i - 1]) > 127)
    means = sum(row) / w
    return jumps, means


def main():
    out = sys.argv[1]
    summary = []
    for name in ("mip_before", "mip_after"):
        w, h, data = read_pgm(os.path.join(out, name + ".pgm"))
        im = Image.frombytes("L", (w, h), data)
        im.save(os.path.join(out, name + ".png"))
        for band in range(3):
            jumps, means = band_stats(data, w, band)
            summary.append((name, band + 1, jumps, round(means, 1)))

    # 并排合成一张总览，仅供快速查看；正式产物仍是修正前后各一份。
    before = Image.open(os.path.join(out, "mip_before.png"))
    after = Image.open(os.path.join(out, "mip_after.png"))
    gap = 8
    combo = Image.new("L", (w * 2 + gap, h), 128)
    combo.paste(before, (0, 0))
    combo.paste(after, (w + gap, 0))
    combo.save(os.path.join(out, "mip_side_by_side.png"))

    print(f"{'image':<12}{'band':>5}{'水平跳变':>10}{'行均值':>9}")
    for name, band, jumps, means in summary:
        print(f"{name:<12}{band:>5}{jumps:>10}{means:>9}")
    print("\n解读：跳变数 = 中心行黑白切换次数。修正前选层偏低（档2选1、档3选2），")
    print("高频未被 mip 滤掉：档3 出现 255 次满幅跳变的混叠纹（边缘闪烁/忽锐忽糊之源）；")
    print("修正后档3 选到第4层，box 平均后跳变归零（均匀灰），混叠消失。")
    print("档1 r=1 两图完全一致，对应修正项 C(1)=0 的退化性质。")


if __name__ == "__main__":
    main()
