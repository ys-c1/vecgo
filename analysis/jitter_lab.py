#!/usr/bin/env python3
"""Jitter/noise/aliasing lab for vecgo.

The experiment is a deterministic straight edge.  Noise power is the exact
Bernoulli variance of the N-sample coverage estimator; no Monte-Carlo seed is
needed for metrics.  Seeded Bernoulli realisations are used only for the grain
comparison bitmaps.
"""
from __future__ import annotations

import csv
import json
import math
from pathlib import Path
from typing import Dict, List, Tuple

import numpy as np
from PIL import Image, ImageDraw, ImageFont

ROOT = Path(__file__).resolve().parents[1]
OUT = ROOT / "artifacts"
OUT.mkdir(exist_ok=True)

W = H = 128
THETA = math.radians(22.5)
M = math.tan(THETA)
NORM = math.hypot(M, 1.0)
NX, NY = -M / NORM, 1.0 / NORM
OX, OY = W / 2.0, H / 2.0 + 0.137
NS = [1, 4, 16]
TYPES = ["uniform", "gaussian"]
P_MAX = 0.0036
SIGMA_MAX = math.sqrt(P_MAX)  # 0.06 coverage units, about 15.3/255
J_MAX = 0.28
J_HIGH = 0.24
SEED = 0x5EED1234

YY, XX = np.mgrid[0:H, 0:W]
DC = NX * (XX + 0.5 - OX) + NY * (YY + 0.5 - OY)
EDGE = np.abs(DC) <= (abs(NX) + abs(NY)) / 2.0 + 1e-9

ERF_VEC = np.vectorize(math.erf, otypes=[float])

# Light theme tokens from the validated dataviz palette.
SURFACE = "#fcfcfb"
INK = "#0b0b0b"
INK2 = "#52514e"
MUTED = "#898781"
GRID = "#e1e0d9"
AXIS = "#c3c2b7"
BLUE = "#2a78d6"
ORANGE = "#eb6834"
GREEN = "#1baf7a"
RED = "#d03b3b"
N_COLORS = {1: BLUE, 4: ORANGE, 16: GREEN}

FONT_PATH = "/home/ys/.local/share/fonts/NotoSansCJKsc-Regular.ttf"
FONT_BOLD_PATH = FONT_PATH


def font(size: int, bold: bool = False) -> ImageFont.FreeTypeFont:
    path = FONT_BOLD_PATH if bold and Path(FONT_BOLD_PATH).exists() else FONT_PATH
    return ImageFont.truetype(path, size)


def signed_sample_distances(n_samples_side: int) -> np.ndarray:
    n = n_samples_side
    si, sj = np.meshgrid(np.arange(n), np.arange(n), indexing="ij")
    bx = (si + 0.5) / n
    by = (sj + 0.5) / n
    x = XX[..., None] + bx.reshape(1, 1, n * n)
    y = YY[..., None] + by.reshape(1, 1, n * n)
    return NX * (x - OX) + NY * (y - OY)


def cdf_probability(d: np.ndarray, j: float, kind: str) -> np.ndarray:
    """P(sample is inside | nominal signed distance d, RMS jitter j)."""
    if j <= 0.0:
        return (d >= 0.0).astype(float)
    if kind == "uniform":
        # Normal projection is deliberately 1-D here; tangent jitter does not
        # change coverage of an infinite straight edge.
        a = math.sqrt(3.0) * j
        return np.clip(0.5 + d / (2.0 * a), 0.0, 1.0)
    if kind == "gaussian":
        return 0.5 * (1.0 + ERF_VEC(d / (math.sqrt(2.0) * j)))
    raise ValueError(kind)


def bilinear(im: np.ndarray, x: np.ndarray, y: np.ndarray) -> np.ndarray:
    x0 = np.floor(x).astype(int)
    y0 = np.floor(y).astype(int)
    x1, y1 = x0 + 1, y0 + 1
    fx, fy = x - x0, y - y0
    x0c = np.clip(x0, 0, W - 1)
    x1c = np.clip(x1, 0, W - 1)
    y0c = np.clip(y0, 0, H - 1)
    y1c = np.clip(y1, 0, H - 1)
    return (
        im[y0c, x0c] * (1 - fx) * (1 - fy)
        + im[y0c, x1c] * fx * (1 - fy)
        + im[y1c, x0c] * (1 - fx) * fy
        + im[y1c, x1c] * fx * fy
    )


def edge_first_difference(im: np.ndarray) -> float:
    """Mean |first difference| along edge-parallel 1-px probe lines."""
    tx, ty = NY, -NX
    ss = np.arange(-W * 0.40, W * 0.40, 1.0)
    offsets = (-0.8, -0.4, 0.0, 0.4, 0.8)
    vals: List[float] = []
    for d in offsets:
        x = OX + d * NX + ss * tx
        y = OY + d * NY + ss * ty
        v = bilinear(im, x, y)
        vals.append(float(np.mean(np.abs(np.diff(v)))))
    return float(np.mean(vals))


def compute(N: int, j: float, kind: str) -> Tuple[float, float, np.ndarray]:
    n = int(round(math.sqrt(N)))
    d = signed_sample_distances(n)
    p = cdf_probability(d, j, kind)
    mean_image = p.mean(axis=2)
    # Independent samples: Var(mean)=sum Var(Y_s)/N^2.
    power_map = (p * (1.0 - p)).sum(axis=2) / float(N * N)
    power = float(power_map[EDGE].mean())
    jag = edge_first_difference(mean_image)
    return power, jag, mean_image


def sample_image(N: int, j: float, kind: str, rng: np.random.Generator) -> np.ndarray:
    n = int(round(math.sqrt(N)))
    d = signed_sample_distances(n)
    p = cdf_probability(d, j, kind)
    hits = (rng.random(p.shape) < p).astype(float)
    return hits.mean(axis=2)


def optimal_j(js: np.ndarray, powers: np.ndarray) -> float:
    if powers[-1] <= P_MAX:
        return float(js[-1])
    return float(np.interp(P_MAX, powers, js))


def fit_gamma(rows: List[dict], kind: str) -> float:
    # Hard-edge local law: P = gamma * j / N, 0.02 <= j <= 0.16 px.
    num = den = 0.0
    for r in rows:
        if r["kind"] != kind:
            continue
        j = float(r["jitter_px"])
        if 0.02 <= j <= 0.16:
            z = r["N"] * float(r["noise_power"])
            num += j * z
            den += j * j
    return num / den


def make_chart_panel(draw: ImageDraw.ImageDraw, box: Tuple[int, int, int, int],
                     kind: str, curves: Dict[Tuple[str, int], dict],
                     fonts: Dict[str, ImageFont.FreeTypeFont]) -> None:
    x0, y0, x1, y1 = box
    draw.rectangle(box, fill=SURFACE, outline=AXIS, width=1)
    ml, mr, mt, mb = 72, 28, 72, 54
    px0, py0, px1, py1 = x0 + ml, y0 + mt, x1 - mr, y1 - mb
    xmax, ymax = 0.40, 0.18

    def X(v: float) -> float:
        return px0 + (v / xmax) * (px1 - px0)

    def Y(v: float) -> float:
        return py1 - (v / ymax) * (py1 - py0)

    draw.text((x0 + 18, y0 + 12),
              "均匀 RMS 抖动" if kind == "uniform" else "高斯 RMS 抖动",
              font=fonts["h2"], fill=INK)
    draw.text((x0 + 18, y0 + 46), "横轴 σn，纵轴 D",
              font=fonts["small"], fill=MUTED)

    for gx in np.arange(0, xmax + 1e-9, 0.05):
        x = X(gx)
        draw.line((x, py0, x, py1), fill=GRID, width=1)
        draw.text((x - 16, py1 + 10), f"{gx:.2f}".rstrip("0").rstrip("."),
                  font=fonts["small"], fill=MUTED)
    for gy in np.arange(0, ymax + 1e-9, 0.03):
        y = Y(gy)
        draw.line((px0, y, px1, y), fill=GRID, width=1)
        draw.text((px0 - 52, y - 10), f"{gy:.2f}", font=fonts["small"], fill=MUTED)

    draw.line((px0, py1, px1, py1), fill=AXIS, width=1)
    draw.line((px0, py0, px0, py1), fill=AXIS, width=1)
    draw.text(((px0 + px1) / 2 - 110, y1 - 24),
              "边缘噪声 RMS σn（覆盖率单位）", font=fonts["body"], fill=INK2)

    xcap = X(SIGMA_MAX)
    draw.line((xcap, py0, xcap, py1), fill=RED, width=2)
    draw.text((xcap + 8, py0 + 8), f"噪声上限 σ={SIGMA_MAX:.3f}",
              font=fonts["body"], fill=RED)

    for N in NS:
        c = N_COLORS[N]
        cu = curves[(kind, N)]
        sig = np.sqrt(cu["powers"])
        jag = np.asarray(cu["jags"])
        pts = [(X(s), Y(j)) for s, j in zip(sig, jag) if s <= xmax]
        draw.line(pts, fill=c, width=3, joint="curve")
        for jstar, pstar, dstar in [(cu["jstar"], cu["pstar"], cu["dstar"])]:
            sx, sy = X(math.sqrt(pstar)), Y(dstar)
            r = 7
            draw.ellipse((sx - r - 2, sy - r - 2, sx + r + 2, sy + r + 2),
                         fill=SURFACE)
            draw.ellipse((sx - r, sy - r, sx + r, sy + r), fill=c, outline=INK, width=1)
        # Direct endpoint label.
        ex, ey = pts[-1]
        draw.text((ex + 8, ey - 12), f"N={N}", font=fonts["body_bold"], fill=INK2)

    # Legend.
    lx, ly = px1 - 150, py0 + 12
    for i, N in enumerate(NS):
        yy = ly + i * 28
        draw.line((lx, yy + 8, lx + 30, yy + 8), fill=N_COLORS[N], width=4)
        draw.ellipse((lx + 11, yy + 3, lx + 21, yy + 13), fill=N_COLORS[N])
        draw.text((lx + 40, yy - 2), f"N={N}", font=fonts["body"], fill=INK2)


def crop_image(im: np.ndarray, crop: int = 44) -> Image.Image:
    x0 = int(round(OX - crop / 2))
    y0 = int(round(OY - crop / 2))
    part = im[y0:y0 + crop, x0:x0 + crop]
    rgb = np.repeat((np.clip(part, 0, 1) * 255).astype(np.uint8)[..., None], 3, axis=2)
    return Image.fromarray(rgb, "RGB").resize((crop * 4, crop * 4), Image.Resampling.NEAREST)


def make_compare(path: Path, kind: str, summary: dict,
                 curves: Dict[Tuple[str, int], dict]) -> None:
    W_IMG, H_IMG = 1800, 1480
    canvas = Image.new("RGB", (W_IMG, H_IMG), SURFACE)
    draw = ImageDraw.Draw(canvas)
    fonts = {
        "title": font(34, True),
        "h2": font(24, True),
        "body": font(20),
        "body_bold": font(20, True),
        "small": font(17),
        "mono": font(18),
    }
    draw.text((40, 28),
              ("均匀有界抖动对照" if kind == "uniform" else "高斯抖动对照")
              + "：三档样本数 × 无抖动 / 最优 / 强抖动",
              font=fonts["title"], fill=INK)
    draw.text((42, 72),
              "直边 θ=22.5°，128×128；指标只在边缘带统计；最优点点在噪声上限边界。",
              font=fonts["body"], fill=INK2)

    make_chart_panel(draw, (40, 110, 900, 650), kind, curves, fonts)

    # Summary table panel.
    tx0, ty0, tx1, ty1 = 940, 110, 1760, 650
    draw.rectangle((tx0, ty0, tx1, ty1), fill="#ffffff", outline=AXIS, width=1)
    draw.text((tx0 + 24, ty0 + 20), "最优抖动量（P_max=0.0036，σ_max=0.060）",
              font=fonts["h2"], fill=INK)
    headers = ["N", "j* (px RMS)", "σn", "D*", "D/D0", "锯齿抑制"]
    xs = [tx0 + 28, tx0 + 110, tx0 + 285, tx0 + 420, tx0 + 555, tx0 + 690]
    y = ty0 + 78
    for x, h in zip(xs, headers):
        draw.text((x, y), h, font=fonts["body_bold"], fill=INK2)
    draw.line((tx0 + 20, y + 34, tx1 - 20, y + 34), fill=GRID, width=2)
    for i, row in enumerate([r for r in summary["optimal"] if r["kind"] == kind]):
        y += 52
        vals = [
            str(row["N"]), f"{row['jitter_star_px']:.4f}",
            f"{row['sigma']:.4f}", f"{row['jag']:.4f}",
            f"{row['jag_ratio']:.4f}", f"{100*(1-row['jag_ratio']):.1f}%",
        ]
        for x, v in zip(xs, vals):
            draw.text((x, y), v, font=fonts["body"], fill=INK)

    fy = ty0 + 330
    formulas = [
        "硬边覆盖：P(j,N)=Λ(j)/N，Λ(j)=∫F_j(d)[1−F_j(d)]dd",
        "小抖动局部律：P≈γ j/N；平滑灰度场才是 P≈K j²/N。",
        f"本场景拟合 γ={summary['gamma'][kind]:.4f}；相位均匀极限 "
        + ("γ=1/√3≈0.5774" if kind == "uniform" else "γ=1/√π≈0.5642"),
        "约束最优：D 随 j 单调下降，故 j* 取 P(j,N)=P_max 的根。",
    ]
    for line in formulas:
        draw.text((tx0 + 24, fy), line, font=fonts["mono"], fill=INK2)
        fy += 38

    # Image strips.
    top = 700
    draw.text((40, top), "边缘放大 4×（每一行是一个样本数；固定随机种子）",
              font=fonts["h2"], fill=INK)
    col_x = [300, 760, 1220]
    captions = ["无抖动 j=0", "最优 j*", f"强抖动 j={J_HIGH:.2f} px RMS"]
    for x, cap in zip(col_x, captions):
        draw.text((x, top + 42), cap, font=fonts["body_bold"], fill=INK)

    rng = np.random.default_rng(SEED + (0 if kind == "uniform" else 1000))
    for ri, N in enumerate(NS):
        yimg = top + 92 + ri * 220
        opt = next(r for r in summary["optimal"] if r["kind"] == kind and r["N"] == N)
        levels = [0.0, opt["jitter_star_px"], J_HIGH]
        draw.text((42, yimg + 70), f"N={N}\n(n={int(math.sqrt(N))}²)",
                  font=fonts["h2"], fill=N_COLORS[N])
        for ci, j in enumerate(levels):
            im = crop_image(sample_image(N, j, kind, rng))
            canvas.paste(im, (col_x[ci], yimg))
            draw.rectangle((col_x[ci], yimg, col_x[ci] + im.width, yimg + im.height),
                           outline=AXIS, width=1)
            p, d, _ = compute(N, j, kind)
            draw.text((col_x[ci], yimg + im.height + 8),
                      f"σ={math.sqrt(p):.3f}, D={d:.4f}",
                      font=fonts["small"], fill=INK2)

    canvas.save(path)


def make_tradeoff_png(path: Path, curves: Dict[Tuple[str, int], dict]) -> None:
    # A compact two-panel chart is embedded conceptually in reports; create a
    # standalone copy as well.
    im = Image.new("RGB", (1800, 760), SURFACE)
    d = ImageDraw.Draw(im)
    fonts = {
        "title": font(32, True), "h2": font(23, True), "body": font(20),
        "body_bold": font(20, True), "small": font(17),
    }
    d.text((40, 24), "抖动—噪声—锯齿权衡曲线（同量纲：覆盖率单位）",
           font=fonts["title"], fill=INK)
    make_chart_panel(d, (40, 90, 880, 720), "uniform", curves, fonts)
    make_chart_panel(d, (920, 90, 1760, 720), "gaussian", curves, fonts)
    im.save(path)


def main() -> None:
    OUT.mkdir(exist_ok=True)
    js = np.round(np.arange(0.0, J_MAX + 1e-12, 0.002), 6)
    csv_rows: List[dict] = []
    curves: Dict[Tuple[str, int], dict] = {}
    all_opt: List[dict] = []

    for kind in TYPES:
        for N in NS:
            powers, jags = [], []
            for j in js:
                p, jag, _ = compute(N, float(j), kind)
                powers.append(p)
                jags.append(jag)
                csv_rows.append({
                    "kind": kind, "N": N, "jitter_px": f"{float(j):.3f}",
                    "noise_power": f"{p:.12e}", "noise_rms": f"{math.sqrt(p):.12e}",
                    "jag_D": f"{jag:.12e}",
                })
            powers_a = np.asarray(powers)
            jags_a = np.asarray(jags)
            jstar = optimal_j(js, powers_a)
            pstar, dstar, _ = compute(N, jstar, kind)
            _, d0, _ = compute(N, 0.0, kind)
            p_end, d_end, _ = compute(N, J_MAX, kind)
            curves[(kind, N)] = {
                "js": js.tolist(), "powers": powers, "jags": jags,
                "jstar": jstar, "pstar": pstar, "dstar": dstar,
            }
            all_opt.append({
                "kind": kind, "N": N,
                "jitter_star_px": jstar,
                "noise_power_cap": P_MAX,
                "noise_power": pstar,
                "sigma": math.sqrt(pstar),
                "jag": dstar,
                "D0": d0,
                "jag_ratio": dstar / d0,
                "suppression_pct": 100.0 * (1.0 - dstar / d0),
                "endpoint_zero": {"jitter_px": 0.0, "noise_power": 0.0,
                                  "sigma": 0.0, "jag": d0, "jag_ratio": 1.0},
                "endpoint_max": {"jitter_px": J_MAX, "noise_power": p_end,
                                 "sigma": math.sqrt(p_end), "jag": d_end,
                                 "jag_ratio": d_end / d0},
            })

    gamma = {kind: fit_gamma(csv_rows, kind) for kind in TYPES}
    summary = {
        "scene": {
            "width": W, "height": H, "angle_deg": math.degrees(THETA),
            "edge_pixels": int(EDGE.sum()), "jitter_coordinate": "edge-normal RMS",
        },
        "noise_cap": {"P_max": P_MAX, "sigma_max": SIGMA_MAX,
                      "sigma_max_8bit_levels": 255 * SIGMA_MAX},
        "hard_edge_formula": "P(j,N)=Lambda(j)/N; Lambda=integral F_j(d)(1-F_j(d)) dd ~= gamma*j/N",
        "smooth_field_formula": "P ~= K*j^2/N",
        "gamma": gamma,
        "j_max_curve": J_MAX,
        "optimal": all_opt,
        "angle_scalar_file": "angle_result.json",
    }
    (OUT / "jitter_summary.json").write_text(
        json.dumps(summary, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")

    with (OUT / "jitter_metrics.csv").open("w", newline="", encoding="utf-8") as f:
        w = csv.DictWriter(f, fieldnames=list(csv_rows[0].keys()))
        w.writeheader()
        w.writerows(csv_rows)

    # Generated markdown table.
    lines = [
        "| 抖动 | N | j* (px RMS) | P_n* | σ_n* | D* | D/D0 | 锯齿抑制 |",
        "|---|---:|---:|---:|---:|---:|---:|---:|",
    ]
    for r in all_opt:
        lines.append(
            f"| {r['kind']} | {r['N']} | {r['jitter_star_px']:.4f} | "
            f"{r['noise_power']:.3e} | {r['sigma']:.4f} | {r['jag']:.4f} | "
            f"{r['jag_ratio']:.4f} | {r['suppression_pct']:.2f}% |"
        )
    (OUT / "jitter_table.generated.md").write_text("\n".join(lines) + "\n", encoding="utf-8")

    make_compare(OUT / "jitter_uniform_compare.png", "uniform", summary, curves)
    make_compare(OUT / "jitter_gaussian_compare.png", "gaussian", summary, curves)
    make_tradeoff_png(OUT / "tradeoff_curves.png", curves)
    print(json.dumps({"out": str(OUT), "rows": len(csv_rows),
                      "optimal": [(r["kind"], r["N"], round(r["jitter_star_px"], 5),
                                   round(r["suppression_pct"], 3)) for r in all_opt]},
                     ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
