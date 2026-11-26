#!/usr/bin/env python3
"""Plot measured vs first-order area-error accumulation for each chain."""

from __future__ import annotations

import argparse
import json
from pathlib import Path
import subprocess

import matplotlib

matplotlib.use("Agg")
import matplotlib.font_manager as fm
import matplotlib.pyplot as plt
from matplotlib.ticker import ScalarFormatter


def configure_cjk_font() -> None:
    """Register a CJK font when matplotlib's cached font list misses it."""
    candidates: list[Path] = []
    try:
        matched = subprocess.check_output(
            ["fc-match", "-f", "%{file}", "Noto Sans CJK SC"], text=True
        ).strip()
        if matched:
            candidates.append(Path(matched))
    except (OSError, subprocess.CalledProcessError):
        pass
    candidates.extend(
        [
            Path.home() / ".local/share/fonts/NotoSansCJKsc-Regular.ttf",
            Path("/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc"),
            Path("/usr/share/fonts/truetype/noto/NotoSansCJK-Regular.ttc"),
        ]
    )
    for path in candidates:
        if path.exists():
            try:
                fm.fontManager.addfont(str(path))
            except Exception:
                pass


configure_cjk_font()

BLUE = "#2a78d6"
ORANGE = "#eb6834"
GRAY = "#898781"
BASELINE = "#c3c2b7"
GRID = "#e1e0d9"
INK = "#0b0b0b"
SECONDARY = "#52514e"

plt.rcParams.update(
    {
        "font.family": ["Noto Sans CJK SC", "DejaVu Sans"],
        "axes.unicode_minus": False,
        "font.size": 11,
        "axes.edgecolor": GRAY,
        "axes.labelcolor": INK,
        "text.color": INK,
        "xtick.color": SECONDARY,
        "ytick.color": SECONDARY,
        "figure.facecolor": "#fcfcfb",
        "axes.facecolor": "#fcfcfb",
        "savefig.facecolor": "#fcfcfb",
    }
)


def format_signed(x: float, digits: int = 2) -> str:
    return f"{x:+.{digits}e}"


def plot_chain(data: dict, kind: str, output_dir: Path) -> None:
    series = next(s for s in data["series"] if s["kind"] == kind)
    chain_name = series["chain"]
    rows = [r for r in data["rows"] if r["kind"] == kind]
    lengths = data["lengths"]
    p = data["pNominal"]
    u = data["unitRoundoff"]

    fig, axes = plt.subplots(3, 1, figsize=(9.5, 10.5), dpi=160)
    fig.suptitle(
        f"链 {kind} {chain_name}：面积误差的线性累积"
        f"（p = {p} 位尾数，u = {u:.3e}）",
        fontsize=16,
        fontweight="semibold",
        y=0.985,
    )

    for ax, n in zip(axes, lengths):
        row = next(r for r in rows if r["n"] == n)
        x = list(range(1, n + 1))
        measured = series["measured"][:n]
        predicted = series["predicted"][:n]
        d_max = row["maxAbsD"]

        env_x = list(range(0, n + 1))
        ax.plot(
            env_x,
            [v * d_max for v in env_x],
            color=GRAY,
            linestyle=":",
            linewidth=1.7,
            label=r"线性包络 $\pm k\,\max|d_i|$",
            zorder=1,
        )
        ax.plot(
            env_x,
            [-v * d_max for v in env_x],
            color=GRAY,
            linestyle=":",
            linewidth=1.7,
            zorder=1,
        )
        ax.axhline(0.0, color=BASELINE, linewidth=1.1, zorder=0)

        if n == 16:
            mark_every = 2
            ticks = list(range(2, n + 1, 2))
        elif n == 64:
            mark_every = 8
            ticks = list(range(0, n + 1, 10))
        else:
            mark_every = 32
            ticks = list(range(0, n + 1, 50))

        ax.plot(
            x,
            measured,
            color=BLUE,
            linewidth=2.2,
            marker="o",
            markersize=6.0,
            markevery=mark_every,
            markerfacecolor=BLUE,
            markeredgecolor="#fcfcfb",
            markeredgewidth=0.8,
            label=r"实测累积面积误差 $e_k$",
            zorder=3,
        )
        ax.plot(
            x,
            predicted,
            color=ORANGE,
            linewidth=2.0,
            linestyle="--",
            marker="s",
            markersize=6.2,
            markevery=mark_every,
            markerfacecolor="none",
            markeredgecolor=ORANGE,
            markeredgewidth=1.5,
            label=r"一阶预测 $\sum_{i\leq k}d_i$（与实测重合，残差为二阶项）",
            zorder=4,
        )

        header = (
            f"链长 n = {n}    末值 e_n = {format_signed(row['areaErr'], 3)}    "
            f"预测残差 = {format_signed(row['predError'], 1)}    "
            f"线性度 = {row['linearity']:.3f}"
        )
        ax.set_title(header, loc="left", fontsize=12.5, pad=30, color=INK)
        ax.legend(
            loc="lower left",
            bbox_to_anchor=(0.0, 1.005),
            ncol=3,
            frameon=False,
            fontsize=10.2,
            handlelength=2.6,
            columnspacing=1.4,
        )

        ax.set_xlabel("复合步数 k", fontsize=12)
        ax.set_ylabel("面积误差", fontsize=12)
        ax.set_xticks(ticks)
        ax.set_xlim(-n * 0.035, n * 1.035)
        bound = n * d_max * 1.20
        ax.set_ylim(-bound, bound)
        ax.grid(True, color=GRID, linewidth=0.8)
        ax.set_axisbelow(True)
        ax.spines["top"].set_visible(False)
        ax.spines["right"].set_visible(False)
        ax.spines["left"].set_color(GRAY)
        ax.spines["bottom"].set_color(GRAY)

        formatter = ScalarFormatter(useMathText=True)
        formatter.set_scientific(True)
        formatter.set_powerlimits((-2, 2))
        ax.yaxis.set_major_formatter(formatter)

    fig.align_ylabels(axes)
    fig.subplots_adjust(top=0.91, hspace=0.62, left=0.085, right=0.985, bottom=0.055)

    stem = output_dir / f"error-stack-{kind}"
    fig.savefig(stem.with_suffix(".png"), format="png")
    fig.savefig(stem.with_suffix(".svg"), format="svg")
    plt.close(fig)
    print(f"wrote {stem.with_suffix('.png')} and .svg")


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("input", type=Path, help="area-drift.json path")
    parser.add_argument("output_dir", type=Path, help="directory for charts")
    args = parser.parse_args()

    data = json.loads(args.input.read_text(encoding="utf-8"))
    args.output_dir.mkdir(parents=True, exist_ok=True)
    for kind in ("A", "B", "C"):
        plot_chain(data, kind, args.output_dir)


if __name__ == "__main__":
    main()
