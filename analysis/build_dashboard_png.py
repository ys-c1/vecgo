#!/usr/bin/env python3
"""静态 PNG 仪表盘：与 report.html 同数据，用于无浏览器环境的目检与存档。"""
import csv
import json
import os

import matplotlib
matplotlib.use("Agg")
import matplotlib.pyplot as plt
from matplotlib.ticker import LogLocator, FuncFormatter

BASE = os.path.join(os.path.dirname(__file__), "..", "artifacts", "exp")
BLUE, ORANGE = "#2a78d6", "#eb6834"
GRID = "#e1e0d9"
plt.rcParams.update({
    "font.family": "DejaVu Sans", "font.size": 9,
    "axes.edgecolor": "#898781", "axes.grid": True,
    "grid.color": GRID, "grid.linewidth": 0.8,
    "axes.titlesize": 10.5, "axes.titleweight": "bold",
    "axes.labelsize": 9.5,
})


def read_csv(name):
    with open(os.path.join(BASE, name), newline="") as f:
        return list(csv.DictReader(f))


def main():
    variance = read_csv("variance.csv")
    timing = read_csv("timing.csv")
    drift = read_csv("drift.csv")
    summary = json.load(open(os.path.join(BASE, "summary.json")))
    n_star = summary["n_star_theory"]

    def col(size, name, key):
        return [(int(r["n"]), float(r[key])) for r in name if int(r["size"]) == size]

    fig, axes = plt.subplots(2, 2, figsize=(13, 8.6))
    fig.suptitle("vecgo coverage-sampling stopping-point analysis", fontsize=13, fontweight="bold")

    # ① variance
    ax = axes[0][0]
    for size, c in [(64, BLUE), (256, ORANGE)]:
        emp = col(size, variance, "emp_variance")
        theo = col(size, variance, "theo_variance")
        ax.plot([x for x, _ in emp], [y for _, y in emp], "o-", color=c, ms=4.5, lw=1.6,
                label=f"{size}x{size} empirical")
        ax.plot([x for x, _ in theo], [y for _, y in theo], "--", color=c, alpha=0.55, lw=1.4,
                label=f"{size}x{size} theory C/n")
    ax.axvline(n_star, color="#898781", ls=":", lw=1.3)
    ax.text(n_star + 2, ax.get_ylim()[1] * 0.6, "n*=10", color="#52514e", fontsize=8.5)
    ax.set_yscale("log"); ax.set_xlabel("samples per pixel n"); ax.set_ylabel("edge-pixel MSE (log)")
    ax.set_title("(1) Coverage variance vs sample count")
    ax.legend(fontsize=8, framealpha=0.9)

    # ② marginal gain
    ax = axes[0][1]
    first = {64: float(variance[0]["marginal_gain"])}
    first[256] = [float(r["marginal_gain"]) for r in variance if int(r["size"]) == 256][0]
    for size, c in [(64, BLUE), (256, ORANGE)]:
        m = col(size, variance, "marginal_gain")
        ax.plot([x for x, _ in m], [y for _, y in m], "o-", color=c, ms=4.5, lw=1.6,
                label=f"{size}x{size} marginal gain")
    ax.axhline(0.01 * first[64], color="#52514e", ls=":", lw=1.3, label="1% of first-sample gain")
    ax.axvline(n_star, color="#898781", ls=":", lw=1.3)
    ax.text(n_star + 2, ax.get_ylim()[1] * 0.5, "n*=10", color="#52514e", fontsize=8.5)
    ax.set_yscale("log"); ax.set_xlabel("samples per pixel n"); ax.set_ylabel("per-sample marginal gain (log)")
    ax.set_title("(2) Marginal gain and the stopping point")
    ax.legend(fontsize=8, framealpha=0.9)

    # ③ timing
    ax = axes[1][0]
    for size, c in [(64, BLUE), (256, ORANGE)]:
        t = col(size, timing, "median_ms")
        ax.plot([x for x, _ in t], [y for _, y in t], "o-", color=c, ms=4.5, lw=1.6,
                label=f"{size}x{size} (edge px {summary['edge_pixels'][str(size)]})")
    ax.axvline(n_star, color="#898781", ls=":", lw=1.3)
    ax.text(n_star + 2, ax.get_ylim()[1] * 0.92, "n*=10", color="#52514e", fontsize=8.5)
    ax.set_xlabel("samples per pixel n"); ax.set_ylabel("median render time (ms)")
    ax.set_title("(3) Render time: why a wrong stop costs more on larger canvas")
    ax.legend(fontsize=8, framealpha=0.9)

    # ④ drift
    ax = axes[1][1]
    calls = [int(r["call"]) for r in drift]
    ax.plot(calls, [float(r["fixed_ms"]) for r in drift], "o-", color=BLUE, ms=5, lw=1.6,
            label="fixed supersample s=2")
    ax.plot(calls, [float(r["ramped_ms"]) for r in drift], "s-", color=ORANGE, ms=5, lw=1.6,
            label="ramped s=2..8")
    for i, r in enumerate(drift):
        s = int(r["ramped_supersample"])
        if s >= 3:
            ax.annotate(f"s={s}", (calls[i], float(r["ramped_ms"])),
                        textcoords="offset points", xytext=(6, -2), fontsize=7.5, color="#52514e")
    ax.set_xlabel("render call #"); ax.set_ylabel("time (ms)")
    ax.set_title("(4) Production Fill drift check (256x256)")
    ax.legend(fontsize=8, framealpha=0.9)

    fig.tight_layout(rect=[0, 0, 1, 0.96])
    out = os.path.join(BASE, "dashboard.png")
    fig.savefig(out, dpi=140, facecolor="white")
    print("wrote", out)


if __name__ == "__main__":
    main()
