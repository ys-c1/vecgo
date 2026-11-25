"""Figures for the alias/sharpness report.

Outputs into results/:
  fig1_transfer.png   |H(nu)| of box vs width-matched Gaussian (linear + dB)
  fig2_scales.png     alias energy and gradient sharpness side by side, s=2/4/8
  fig3_tradeoff.png   width sweep: alias vs sharpness, the Pareto trade-off
  fig4_zoneplate.png  2-D visual: moire from box vs Gaussian vs Lanczos
"""
import csv
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

import numpy as np
import matplotlib

matplotlib.use("Agg")
import matplotlib.pyplot as plt

from kernels import (
    box_kernel, gaussian_kernel, lanczos3_kernel, H_box, H_gauss,
)

RES = os.path.join(os.path.dirname(__file__), "results")
BLUE, ORANGE, AQUA = "#2a78d6", "#eb6834", "#1baf7a"
INK, MUTED, GRID = "#0b0b0b", "#52514e", "#d9d8d3"
plt.rcParams.update({
    "font.size": 11,
    "axes.edgecolor": "#b9b8b2",
    "axes.labelcolor": INK,
    "axes.titlecolor": INK,
    "xtick.color": MUTED,
    "ytick.color": MUTED,
    "axes.grid": True,
    "grid.color": GRID,
    "grid.linewidth": 0.8,
    "figure.dpi": 130,
    "savefig.dpi": 150,
})


def load(name):
    with open(os.path.join(RES, name)) as fh:
        return list(csv.DictReader(fh))


# ------------------------------------------------------------- Fig 1: H(nu)
def fig_transfer():
    s = 4
    fc = 0.5 / s
    nu = np.linspace(0.0, 0.5, 2000)
    hg, sigma = gaussian_kernel(s)
    Hb = H_box(nu, s)
    Hg = H_gauss(nu, sigma)
    # discrete DTFT of the actual sampled kernels (dotted)
    nb = np.arange(s) - (s - 1) / 2.0
    ng = np.arange(len(hg)) - (len(hg) - 1) / 2.0
    Hb_d = np.array([np.abs(np.sum(box_kernel(s) *
                       np.exp(-1j * 2 * np.pi * f * nb))) for f in nu])
    Hg_d = np.array([np.abs(np.sum(hg *
                       np.exp(-1j * 2 * np.pi * f * ng))) for f in nu])

    fig, (ax1, ax2) = plt.subplots(1, 2, figsize=(11, 4.2))
    ax1.plot(nu, np.abs(Hb), color=BLUE, lw=2,
             label=f"box  H=sinc(πsν), width s={s}")
    ax1.plot(nu, np.abs(Hg), color=ORANGE, lw=2,
             label=f"Gaussian  H=exp(−2π²σ²ν²), σ={sigma:.3f}")
    ax1.plot(nu, Hb_d, color=BLUE, lw=1, ls=":", alpha=0.7,
             label="box discrete DTFT")
    ax1.plot(nu, Hg_d, color=ORANGE, lw=1, ls=":", alpha=0.7,
             label="Gaussian discrete DTFT")
    ax1.axvspan(fc, 0.5, color=MUTED, alpha=0.10)
    ax1.axvline(fc, color=MUTED, ls="--", lw=1.2)
    ax1.text(fc + 0.006, 0.92, "output Nyquist f_c", color=MUTED, fontsize=9)
    ax1.set(xlabel="frequency ν (cycles/input sample)", ylabel="|H(ν)|",
            title="Equivalent transfer functions (linear)", ylim=(-0.12, 1.08))
    ax1.legend(frameon=False, fontsize=8.5, loc="upper right")

    eps = 1e-6
    ax2.plot(nu, 20 * np.log10(np.abs(Hb) + eps), color=BLUE, lw=2,
             label="box (continuous FT)")
    ax2.plot(nu, 20 * np.log10(np.abs(Hg) + eps), color=ORANGE, lw=2,
             label="Gaussian (continuous FT)")
    ax2.axvspan(fc, 0.5, color=MUTED, alpha=0.10)
    ax2.axvline(fc, color=MUTED, ls="--", lw=1.2)
    ax2.set(xlabel="frequency ν (cycles/input sample)",
            ylabel="20 log₁₀ |H(ν)|  (dB)",
            title="Stop-band behaviour (log magnitude)", ylim=(-60, 6))
    ax2.legend(frameon=False, fontsize=9, loc="lower left")
    fig.tight_layout()
    fig.savefig(os.path.join(RES, "fig1_transfer.png"), bbox_inches="tight")
    plt.close(fig)


# ----------------------------------------------------------- Fig 2: 3 scales
def fig_scales():
    rows = load("metrics.csv")
    scales = [2, 4, 8]
    kernels = ["box", "gauss", "lanczos3"]
    colors = {"box": BLUE, "gauss": ORANGE, "lanczos3": AQUA}
    labels = {"box": "box", "gauss": "Gaussian", "lanczos3": "Lanczos-3"}
    x = np.arange(len(scales))
    w = 0.25

    fig, axes = plt.subplots(1, 3, figsize=(13.5, 4.6))
    for j, (field, title, ylab) in enumerate((
            ("E_alias_integral", "Alias energy (spectral integral)",
             "E_a  (per output sample)"),
            ("E_alias_dft", "Alias energy (discrete DFT check)",
             "E_a  (per output sample)"),
            ("sharp_baseband", "Sharpness (baseband gradient energy)",
             "mean |∇y_bb|²"))):
        ax = axes[j]
        for i, k in enumerate(kernels):
            vals = [float(r[field]) for s_ in scales
                    for r in rows if r["kernel"] == k and int(r["scale"]) == s_]
            ax.bar(x + (i - 1) * w, vals, w, color=colors[k],
                   label=labels[k], edgecolor="white", linewidth=1.0)
        ax.set_xticks(x)
        ax.set_xticklabels([f"s={s_}" for s_ in scales])
        ax.set_title(title, fontsize=10.5, pad=8)
        ax.set_ylabel(ylab, fontsize=9.5)
        if j == 0:
            ax.set_yscale("log")
        ax.legend(frameon=False, fontsize=9, loc=(
            "upper right" if j != 2 else "lower right"))
    fig.suptitle("s = 2 / 4 / 8 — box and Gaussian share the same "
                 "equivalent radius; values in the table below",
                 fontsize=11.5, y=1.0)
    fig.tight_layout(rect=(0, 0.06, 1, 0.96))
    fig.savefig(os.path.join(RES, "fig2_scales.png"), bbox_inches="tight")
    plt.close(fig)


# ----------------------------------------------------------- Fig 3: tradeoff
def fig_tradeoff():
    rows = load("sweep.csv")
    fig, (ax1, ax2, ax3) = plt.subplots(1, 3, figsize=(14, 4.3))
    for k, c, lab in (("box", BLUE, "box"), ("gauss", ORANGE, "Gaussian")):
        d = [r for r in rows if r["kernel"] == k]
        w_ = [float(r["width_taps"]) for r in d]
        ax1.plot(w_, [float(r["E_alias"]) for r in d], "o-", color=c,
                 lw=2, ms=7, label=lab)
        ax2.plot(w_, [float(r["sharp_bb"]) for r in d], "s-", color=c,
                 lw=2, ms=7, label=lab)
        ax3.plot([float(r["E_alias"]) for r in d],
                 [float(r["sharp_bb"]) for r in d], "o-", color=c,
                 lw=2, ms=7, label=lab)
        for r in d[::2]:
            ax3.annotate(r["width_taps"],
                         (float(r["E_alias"]), float(r["sharp_bb"])),
                         fontsize=7.5, color=MUTED,
                         xytext=(4, 3), textcoords="offset points")
    ax1.set(xlabel="kernel width (matched equivalent radius)",
            ylabel="alias energy E_a", title="Alias vs width", yscale="log")
    ax2.set(xlabel="kernel width (matched equivalent radius)",
            ylabel="baseband gradient energy", title="Sharpness vs width")
    ax3.set(xlabel="alias energy E_a (log)", ylabel="sharpness",
            title="The trade-off frontier", xscale="log")
    ax3.annotate("wider kernel:\nless alias,\nless sharp",
                 xy=(0.05, 0.55), xycoords="axes fraction",
                 fontsize=9, color=MUTED, va="top")
    for ax in (ax1, ax2, ax3):
        ax.legend(frameon=False, fontsize=9.5)
    fig.tight_layout()
    fig.savefig(os.path.join(RES, "fig3_tradeoff.png"), bbox_inches="tight")
    plt.close(fig)


# ----------------------------------------------------------- Fig 4: 2-D demo
def _downscale2d(img, h, s):
    pad = len(h)
    p = np.pad(img, pad, mode="reflect")
    out = np.zeros_like(img)
    out = np.convolve(p.ravel(), h, mode="same").reshape(p.shape)
    out = np.apply_along_axis(lambda r: np.convolve(r, h, mode="same"),
                              0, out)
    return out[pad::s, pad::s][:img.shape[0] // s, :img.shape[1] // s]


def fig_zoneplate():
    n = 512
    t = np.arange(n) - n / 2
    xx, yy = np.meshgrid(t, t)
    r = np.hypot(xx, yy)
    # circular zone plate: instantaneous radial frequency grows with radius
    img = 0.5 + 0.5 * np.sin(2 * np.pi * (r ** 2) / (2 * n))
    s = 4
    hb = box_kernel(s)
    hg, _ = gaussian_kernel(s)
    hl = lanczos3_kernel(s)
    outs = [("box", _downscale2d(img, hb, s)),
            ("Gaussian", _downscale2d(img, hg, s)),
            ("Lanczos-3", _downscale2d(img, hl, s))]
    fig, axes = plt.subplots(1, 3, figsize=(12.5, 4.4))
    for ax, (name, o) in zip(axes, outs):
        ax.imshow(o, cmap="gray", vmin=0, vmax=1)
        ax.set_title(f"{name}, s={4}", fontsize=11)
        ax.set_xticks([])
        ax.set_yticks([])
    fig.suptitle("Zone plate downsampled ×4: moiré rings (box) vs smoothed "
                 "rings (Gaussian) vs suppressed rings (Lanczos-3)",
                 fontsize=11, y=1.02)
    fig.tight_layout()
    fig.savefig(os.path.join(RES, "fig4_zoneplate.png"), bbox_inches="tight")
    plt.close(fig)


if __name__ == "__main__":
    fig_transfer()
    fig_scales()
    fig_tradeoff()
    fig_zoneplate()
    print("figures written to", RES)
    for fn in sorted(os.listdir(RES)):
        if fn.endswith(".png"):
            print(" ", fn)
