#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""读取 shadowlab 产物，绘制：
  1) edge_compare_<point|small|large>.png  每档一张影边对照图
  2) edge_compare_all.png                  三档归一化剖面对照 + 预测/实测柱状
  3) similar_triangles.png/.svg            两次相似三角形推导示意图

用法: python3 hack/penumbra_plots.py [artifacts_dir]
只依赖 matplotlib / numpy(csv 标准库也可，这里用 numpy)。
"""
import csv
import json
import os
import sys

import matplotlib

matplotlib.use("Agg")
import matplotlib.pyplot as plt
from matplotlib import font_manager as fm
from matplotlib.patches import Polygon as MplPolygon
import numpy as np

# 显式注册 CJK 字体（某些环境的 matplotlib 字体缓存扫不到用户字体目录）。
for _p in (
    os.path.expanduser("~/.local/share/fonts/NotoSansCJKsc-Regular.ttf"),
    "/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc",
    "/usr/share/fonts/truetype/noto/NotoSansCJK-Regular.ttc",
):
    if os.path.exists(_p):
        try:
            fm.fontManager.addfont(_p)
        except Exception:
            pass

plt.rcParams["font.sans-serif"] = ["Noto Sans CJK SC", "Noto Sans CJK JP", "DejaVu Sans"]
plt.rcParams["axes.unicode_minus"] = False

BLUE = "#2a78d6"
ORANGE = "#eb6834"
AQUA = "#1baf7a"
INK = "#0b0b0b"
SUB = "#52514e"
GRID = "#e6e3da"
SURF = "#fcfcfb"
GRAY = "#8a8982"

KEYS = ["point", "small", "large"]
COLORS = {"point": SUB, "small": BLUE, "large": ORANGE}
TITLES = {"point": "点光源（硬阴影）", "small": "小面光源", "large": "大面光源"}


def load_profiles(path):
    with open(path, newline="", encoding="utf-8") as f:
        rows = list(csv.DictReader(f))
    x = np.array([float(r["x_px"]) for r in rows])
    g = {k: np.array([float(r[f"gray_{k}"]) for r in rows]) for k in KEYS}
    return x, g


def load_records(path):
    with open(path, encoding="utf-8") as f:
        recs = json.load(f)
    return {r["key"]: r for r in recs}


def style_ax(ax):
    ax.set_facecolor(SURF)
    for s in ("top", "right"):
        ax.spines[s].set_visible(False)
    for s in ("left", "bottom"):
        ax.spines[s].set_color("#c9c6bc")
    ax.grid(True, color=GRID, lw=0.8, zorder=0)
    ax.tick_params(colors=SUB, labelsize=9)
    ax.xaxis.label.set_color(INK)
    ax.yaxis.label.set_color(INK)


def per_size_chart(d, x, g, rec):
    key = rec["key"]
    r = rec["right_edge"]
    x50 = r["x_half_gray_px"]
    pred = rec["pred_width_px"]
    meas = rec["penumbra_width_meas_px"]
    dark, lit = rec["gray_dark"], rec["gray_lit"]
    g50 = (dark + lit) / 2
    slope = r["slope_gray_per_px"]

    half_span = max(pred, meas, 10) + 34
    xlo, xhi = x50 - half_span, x50 + half_span

    fig = plt.figure(figsize=(10.2, 6.6), dpi=160, facecolor="white")
    gs = fig.add_gridspec(2, 1, height_ratios=[1.05, 2.3], hspace=0.12)
    ax_img = fig.add_subplot(gs[0])
    ax = fig.add_subplot(gs[1], sharex=ax_img)

    # --- 上：影边位图局部（extent 使像素中心 = 列号 + 0.5，与剖面一致）---
    img = plt.imread(os.path.join(d, rec["image_png"]))
    if img.ndim == 3:
        img = img[..., :3].mean(axis=2)
    ax_img.imshow(img, cmap="gray", vmin=0, vmax=1, aspect="auto",
                  extent=[0, img.shape[1], img.shape[0], 0])
    if pred > 0.01:
        ax_img.axvspan(x50 - pred / 2, x50 + pred / 2, color=AQUA, alpha=0.18, zorder=2)
        for xx in (x50 - pred / 2, x50 + pred / 2):
            ax_img.axvline(xx, color=AQUA, ls="--", lw=1.2, zorder=3)
    ax_img.axvline(x50, color=ORANGE, ls="-", lw=1.4, zorder=4)
    ax_img.set_ylabel("影边图像", fontsize=10)
    ax_img.set_yticks([])
    ax_img.set_xlim(xlo, xhi)
    ax_img.tick_params(labelbottom=False)
    ax_img.set_facecolor(SURF)

    # --- 下：灰度剖面与测量 ---
    style_ax(ax)
    if key != "point":
        ax.plot(x, g["point"], color=GRAY, lw=1.4, ls="--", zorder=2,
                label="点光源硬影边（对照）")
    ax.plot(x, g[key], color=BLUE, lw=2.2, zorder=3, label="实测列灰度剖面（行带平均）")

    ax.axhline(g50, color=SUB, ls=":", lw=1.2, zorder=1)
    ax.text(xlo + 2, g50 + 7, f"灰度过半 g50={g50:.0f}", color=SUB, fontsize=9)
    ax.scatter([x50], [g50], s=90, color=ORANGE, edgecolor="white", lw=1.4, zorder=6,
               label=f"半灰位置 x50={x50:.2f} px")

    # 半灰位置切线，外推整个明暗差 -> 实测宽度
    xt = np.array([x50 - meas / 2, x50 + meas / 2])
    yt = g50 + slope * (xt - x50)
    ax.plot(xt, yt, color=ORANGE, lw=2.0, alpha=0.95, zorder=5,
            label=f"x50 中心切线外推：实测 w={meas:.2f} px")
    ax.scatter(xt, yt, s=28, color=ORANGE, zorder=6)

    # 预测半影带 [x50-w_pred/2, x50+w_pred/2]
    if pred > 0.01:
        ax.axvspan(x50 - pred / 2, x50 + pred / 2, color=AQUA, alpha=0.13, zorder=1,
                   label=f"相似三角形预测带 w={pred:.2f} px")
        for xx in (x50 - pred / 2, x50 + pred / 2):
            ax.axvline(xx, color=AQUA, ls="--", lw=1.3, zorder=2)
    else:
        ax.axvline(x50, color=AQUA, ls="--", lw=1.6, zorder=2,
                   label="预测 w=0（点光源极限）")

    ax.set_xlim(xlo, xhi)
    ax.set_ylim(-14, 262)
    ax.set_xlabel("x 坐标（输出像素，像素中心 = 列号 + 0.5）", fontsize=10)
    ax.set_ylabel("灰度（0 暗 – 255 亮）", fontsize=10)

    rel = rec["rel_error_pct"]
    rel_txt = "相对误差 —（预测基准为 0）" if rel is None else f"相对误差 {rel:+.2f}%"
    info = (f"L={rec['light_size_world']:g}　h={rec['gap_h_world']:g}　"
            f"H={rec['dist_H_world']:g}　r=h/(H−h)={rec['ratio_h_over_HminusH']:g}\n"
            f"预测 w = s·L·r = {pred:.2f} px\n"
            f"实测 w = {meas:.2f} px　绝对误差 {rec['abs_error_px']:+.2f} px　{rel_txt}")
    ax.text(0.014, 0.97, info, transform=ax.transAxes, va="top", ha="left", fontsize=9.2,
            color=INK, bbox=dict(boxstyle="round,pad=0.5", fc="white", ec="#c9c6bc", lw=1))

    ax.legend(loc="lower right", fontsize=8.8, frameon=True, framealpha=0.94,
              edgecolor="#c9c6bc")
    fig.suptitle(f"{TITLES[key]} · 影边对照（{key}）", fontsize=14, color=INK, y=0.975)
    out = os.path.join(d, f"edge_compare_{key}.png")
    fig.savefig(out, bbox_inches="tight", facecolor="white")
    plt.close(fig)
    print("wrote", out)


def combined_chart(d, x, g, recs):
    fig = plt.figure(figsize=(12.4, 5.4), dpi=160, facecolor="white")
    gs = fig.add_gridspec(1, 2, width_ratios=[1.55, 1.0], wspace=0.22)
    ax = fig.add_subplot(gs[0])
    axb = fig.add_subplot(gs[1])
    style_ax(ax)
    style_ax(axb)

    # 左：以半灰位置归一化的三条剖面 + 各自预测宽度刻度
    for k in KEYS:
        rec = recs[k]
        x50 = rec["right_edge"]["x_half_gray_px"]
        pred = rec["pred_width_px"]
        m = (x > x50 - 55) & (x < x50 + 55)
        ax.plot(x[m] - x50, g[k][m], color=COLORS[k], lw=2.2 if k != "point" else 1.8,
                ls="-" if k != "point" else "--", label=TITLES[k], zorder=3)
        if pred > 0.01:
            ax.plot([-pred / 2, pred / 2], [127, 127], color=COLORS[k], lw=6,
                    alpha=0.25, solid_capstyle="butt", zorder=2)
            ylab = 143 if k == "small" else 108
            ax.text(pred / 2 + 1.4, ylab, f"w_pred={pred:.0f} px", color=COLORS[k],
                    fontsize=9.5, va="center", fontweight="bold")
    ax.axhline(127, color=SUB, ls=":", lw=1.1, zorder=1)
    ax.scatter([0], [127], s=70, color=ORANGE, edgecolor="white", lw=1.3, zorder=6)
    ax.set_xlim(-46, 46)
    ax.set_ylim(-14, 262)
    ax.set_xlabel("相对灰度过半位置 x − x50（px）", fontsize=10)
    ax.set_ylabel("灰度", fontsize=10)
    ax.set_title("三档影边剖面（粗透明段为预测半影宽）", fontsize=12)
    ax.legend(loc="upper left", fontsize=9, framealpha=0.94, edgecolor="#c9c6bc")

    # 右：实测 vs 预测 分组柱
    labels = ["点", "小面", "大面"]
    pred = [recs[k]["pred_width_px"] for k in KEYS]
    meas = [recs[k]["penumbra_width_meas_px"] for k in KEYS]
    xx = np.arange(3)
    bw = 0.36
    b1 = axb.bar(xx - bw / 2, meas, bw, color=BLUE, label="实测半影宽", zorder=3)
    b2 = axb.bar(xx + bw / 2, pred, bw, color="white", edgecolor=AQUA, lw=2.0,
                 label="预测 w=Lh/(H−h)s", zorder=3)
    for bars, vals in ((b1, meas), (b2, pred)):
        for rect, v in zip(bars, vals):
            axb.text(rect.get_x() + rect.get_width() / 2, rect.get_height() + 0.7,
                     f"{v:.2f}", ha="center", va="bottom", fontsize=9, color=INK)
    # 点光源实测 1px 基底标注
    axb.annotate("1 px 离散基底\n(几何预测 0)", xy=(0 - bw / 2, 1.0), xytext=(0.25, 9),
                 fontsize=8.6, color=SUB,
                 arrowprops=dict(arrowstyle="->", color=GRAY, lw=1))
    axb.set_xticks(xx)
    axb.set_xticklabels(labels)
    axb.set_ylabel("宽度（px）", fontsize=10)
    axb.set_ylim(0, 47)
    axb.set_title("实测宽度 vs 相似三角形预测", fontsize=12)
    axb.legend(loc="upper left", fontsize=8.8, framealpha=0.94, edgecolor="#c9c6bc")

    fig.suptitle("面光源半影：点 → 小面 → 大面（硬阴影到明显半影）", fontsize=14.5, y=0.99)
    out = os.path.join(d, "edge_compare_all.png")
    fig.savefig(out, bbox_inches="tight", facecolor="white")
    plt.close(fig)
    print("wrote", out)


def geometry_diagram(d):
    H, h, L = 5.0, 2.0, 3.0
    r = h / (H - h)
    run = L / 2 * r  # =1

    fig, ax = plt.subplots(figsize=(10.4, 6.2), dpi=170, facecolor="white")
    ax.set_facecolor(SURF)

    # 两个相似三角形填色
    ax.add_patch(MplPolygon([(-L / 2, H), (-L / 2, h), (0, h)], closed=True,
                            color=BLUE, alpha=0.13, zorder=1))
    ax.add_patch(MplPolygon([(0, h), (0, 0), (run, 0)], closed=True,
                            color=BLUE, alpha=0.13, zorder=1))
    ax.add_patch(MplPolygon([(L / 2, H), (L / 2, h), (0, h)], closed=True,
                            color=ORANGE, alpha=0.12, zorder=1))
    ax.add_patch(MplPolygon([(0, h), (0, 0), (-run, 0)], closed=True,
                            color=ORANGE, alpha=0.12, zorder=1))

    # 接收面
    ax.axhline(0, color=INK, lw=2.2, zorder=3)
    ax.text(-4.6, -0.35, "接收面 R (y=0)", fontsize=10, color=INK)
    # 遮挡半平面（向左延伸的不透明墙，边在 B）
    ax.plot([-4.6, 0], [h, h], color=INK, lw=5, solid_capstyle="butt", zorder=4)
    for xx in np.arange(-4.45, 0.0, 0.35):
        ax.plot([xx, xx - 0.22], [h, h + 0.28], color=INK, lw=1.0)
    ax.scatter([0], [h], s=46, color=INK, zorder=6)
    ax.text(0.12, h + 0.12, "遮挡边 B=(0,h)", fontsize=10.5)

    # 光源
    ax.plot([-L / 2, L / 2], [H, H], color="#eda100", lw=7, solid_capstyle="butt", zorder=4)
    ax.text(0, H + 0.95, r"面光源  $S_-\ldots S_+$，尺寸 L（L=0 为点光源）",
            ha="center", fontsize=11.5)
    ax.plot([0], [H], marker="o", ms=4, color=SUB)

    # 两条边界光线
    ray_kw = dict(lw=1.8, zorder=5)
    ax.plot([-L / 2, 0, run], [H, h, 0], color=BLUE, **ray_kw)
    ax.plot([L / 2, 0, -run], [H, h, 0], color=ORANGE, **ray_kw)
    ax.scatter([-L / 2, L / 2], [H, H], s=40, color="#eda100", edgecolor=INK, lw=0.8, zorder=6)
    ax.scatter([run, -run], [0, 0], s=36, color=AQUA, edgecolor=INK, lw=0.7, zorder=6)
    ax.text(run + 0.08, 0.12, "$P_+$", fontsize=11)
    ax.text(-run - 0.35, 0.12, "$P_-$", fontsize=11)

    # 尺寸标注
    def dim(x1, y1, x2, y2, text, tx, ty, color=SUB):
        ax.annotate("", xy=(x2, y2), xytext=(x1, y1),
                    arrowprops=dict(arrowstyle="<->", color=color, lw=1.3))
        ax.text(tx, ty, text, fontsize=10.5, color=color, ha="center")

    dim(-L / 2, H + 0.45, 0, H + 0.45, "L/2", -L / 4, H + 0.64)
    dim(-L / 2 - 0.28, h, -L / 2 - 0.28, H, "H−h", -L / 2 - 0.55, (H + h) / 2)
    dim(0.22, 0, 0.22, h, "h", 0.42, h / 2)
    ax.annotate("", xy=(run, -0.42), xytext=(0, -0.42),
                arrowprops=dict(arrowstyle="<->", color=AQUA, lw=1.5))
    ax.text(run / 2, -0.72, "$x_+$", fontsize=11, color=AQUA, ha="center")
    ax.annotate("", xy=(-run, -0.42), xytext=(0, -0.42),
                arrowprops=dict(arrowstyle="<->", color=AQUA, lw=1.5))
    ax.text(-run / 2, -0.72, "$x_-$", fontsize=11, color=AQUA, ha="center")

    box = (
        "两次相似三角形（两条边界光线各一对）：\n"
        r"  右：  $x_+/h=(L/2)/(H-h)$" + "\n"
        r"  左：  $|x_-|/h=(L/2)/(H-h)$" + "\n"
        r"故  $w=x_++|x_-|=L\,h/(H-h)$" + "\n"
        r"极限 $L\to0$：两式右端的 $L/2\to0$，$x_+,|x_-|\to0$，$w\to0$（点光源硬边）"
    )
    ax.text(1.7, 4.7, box, fontsize=11.2, va="top", ha="left",
            bbox=dict(boxstyle="round,pad=0.6", fc="white", ec="#c9c6bc", lw=1.1))

    ax.set_xlim(-4.8, 6.4)
    ax.set_ylim(-1.15, 6.55)
    ax.set_aspect("equal")
    ax.axis("off")
    ax.set_title("半影宽度公式：两次相似三角形推导", fontsize=14, color=INK, pad=10)
    for ext in ("png", "svg"):
        out = os.path.join(d, "similar_triangles." + ext)
        fig.savefig(out, bbox_inches="tight", facecolor="white")
        print("wrote", out)
    plt.close(fig)


def main():
    d = sys.argv[1] if len(sys.argv) > 1 else "artifacts"
    x, g = load_profiles(os.path.join(d, "profiles.csv"))
    recs = load_records(os.path.join(d, "penumbra_measurements.json"))
    for k in KEYS:
        per_size_chart(d, x, g, recs[k])
    combined_chart(d, x, g, recs)
    geometry_diagram(d)


if __name__ == "__main__":
    main()
