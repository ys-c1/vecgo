"""Generate the two report figures as standalone SVG (light/dark aware via CSS)."""
import sys, os
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import numpy as np
import colorlab as cl
from experiment import (ArcMetric, lch_lab, hsl_lab, signed_hue_delta,
                        PAIRS, N_STEPS)

INK = "#0b0b0b"; INK2 = "#52514e"; MUTED = "#898781"
GRID = "#e1e0d9"; AXIS = "#c3c2b7"; SURF = "#fcfcfb"
BLUE = "#2a78d6"; ORANGE = "#eb6834"
INK_D = "#f0efec"; INK2_D = "#c3c2b7"; GRID_D = "#2c2c2a"; AXIS_D = "#383835"; SURF_D = "#1a1a19"
BLUE_D = "#3987e5"; ORANGE_D = "#d95926"

CSS = f"""
text {{ font-family: system-ui, -apple-system, 'Segoe UI', sans-serif; }}
.ink {{ fill: {INK}; }} .ink2 {{ fill: {INK2}; }} .muted {{ fill: {MUTED}; }}
.grid {{ stroke: {GRID}; stroke-width:1; }}
.axis {{ stroke: {AXIS}; stroke-width:1; }}
.s1 {{ stroke: {BLUE}; }} .s2 {{ stroke: {ORANGE}; }}
.sw {{ stroke: {SURF}; stroke-width:2; }}
.title {{ font-size:15px; font-weight:600; }}
.sub {{ font-size:12px; }}
.tick {{ font-size:11px; }}
.lbl {{ font-size:12px; font-weight:600; }}
@media (prefers-color-scheme: dark) {{
  .ink {{ fill: {INK_D}; }} .ink2 {{ fill: {INK2_D}; }} .muted {{ fill: {MUTED}; }}
  .grid {{ stroke: {GRID_D}; }} .axis {{ stroke: {AXIS_D}; }}
  .s1 {{ stroke: {BLUE_D}; }} .s2 {{ stroke: {ORANGE_D}; }}
  .sw {{ stroke: {SURF_D}; }}
}}
"""

def hex_rgb(rgb01):
    v = np.clip(np.round(np.asarray(rgb01) * 255), 0, 255).astype(int)
    return "#%02x%02x%02x" % tuple(v)


def line_path(x, y, x0, y0, w, h, ymax):
    pts = []
    for xi, yi in zip(x, y):
        px = x0 + xi / 360.0 * w
        py = y0 + h - yi / ymax * h
        pts.append(f"{px:.2f},{py:.2f}")
    return "M" + " L".join(pts)


def fig_density(path):
    mL = ArcMetric("LCh(L50 C50)", lch_lab)
    mH = ArcMetric("HSL(S1 L0.5)", hsl_lab)
    W, H = 980, 580
    x0, w = 78, 860
    panels = [(92, 205, mL, BLUE, "LCh circle  L*=50, C*=50", 0.75),
              (337, 205, mH, ORANGE, "HSL wheel  S=1, L=0.5", 1.75)]
    parts = [f'<svg xmlns="http://www.w3.org/2000/svg" width="{W}" height="{H}" viewBox="0 0 {W} {H}" role="img">',
             f"<style>{CSS}</style>", f'<rect width="{W}" height="{H}" fill="{SURF}" class="surf"/>']
    xs = mL.h1
    for (y0, h, m, color, name, ymax) in panels:
        # gridlines + y ticks
        for gy in np.linspace(0, ymax, 5):
            py = y0 + h - gy / ymax * h
            parts.append(f'<line x1="{x0}" y1="{py:.1f}" x2="{x0+w}" y2="{py:.1f}" class="grid"/>')
            parts.append(f'<text x="{x0-8}" y="{py+4:.1f}" text-anchor="end" class="muted tick">{gy:.2f}</text>')
        # x ticks every 60 deg
        for hx in range(0, 361, 60):
            px = x0 + hx / 360.0 * w
            parts.append(f'<line x1="{px:.1f}" y1="{y0+h}" x2="{px:.1f}" y2="{y0+h+5}" class="axis"/>')
            parts.append(f'<text x="{px:.1f}" y="{y0+h+19}" text-anchor="middle" class="muted tick">{hx}°</text>')
        parts.append(f'<line x1="{x0}" y1="{y0}" x2="{x0}" y2="{y0+h}" class="axis"/>')
        parts.append(f'<line x1="{x0}" y1="{y0+h}" x2="{x0+w}" y2="{y0+h}" class="axis"/>')
        parts.append(f'<path d="{line_path(xs, m.g1, x0, y0, w, h, ymax)}" fill="none" stroke-width="2" stroke-linejoin="round" class="{"s1" if "LCh" in name else "s2"}"/>')
        parts.append(f'<text x="{x0}" y="{y0-10}" class="ink lbl">{name}</text>')
        parts.append(f'<text x="{x0+w}" y="{y0-10}" text-anchor="end" class="muted sub">g min={m.g1.min():.3f}  max={m.g1.max():.3f}  mean={m.S_per_cycle/360:.3f} ΔE00/deg</text>')
    parts.append(f'<text x="{W/2}" y="30" text-anchor="middle" class="ink title">Perceptual density g(h) = dΔE00/dh along the hue ring</text>')
    parts.append(f'<text x="{W/2}" y="52" text-anchor="middle" class="ink2 sub">constant angular steps traverse unequal perceptual distance where g(h) varies</text>')
    parts.append(f'<text transform="translate(24,{92+205+108}) rotate(-90)" text-anchor="middle" class="muted sub">ΔE00 per degree</text>')
    parts.append(f'<text x="{x0+w/2}" y="{H-12}" text-anchor="middle" class="muted sub">hue angle h (degrees)</text>')
    parts.append("</svg>")
    open(path, "w").write("\n".join(parts))


def fig_strips(path):
    mL = ArcMetric("LCh(L50 C50)", lch_lab)
    W, H = 980, 470
    x0, w = 150, 700
    rowh, gap = 30, 14
    ytop = 95
    parts = [f'<svg xmlns="http://www.w3.org/2000/svg" width="{W}" height="{H}" viewBox="0 0 {W} {H}" role="img">',
             f"<style>{CSS}</style>", f'<rect width="{W}" height="{H}" fill="{SURF}" class="surf"/>',
             f'<text x="{W/2}" y="32" text-anchor="middle" class="ink title">Hue interpolation steps: equal-angle vs perceptually equalized (LCh L*=50 C*=50)</text>',
             f'<text x="{W/2}" y="54" text-anchor="middle" class="ink2 sub">swatch width = angular step; each row spans the same endpoint pair, 12 steps, swatches show the rendered sRGB color</text>']
    y = ytop
    for k, (h0, h1) in enumerate(PAIRS):
        dh = signed_hue_delta(h1, h0)
        h_ang = mL.equal_angle_hues(h0, dh, N_STEPS)
        h_ex = mL.equal_chord_hues(h0, dh, N_STEPS)
        d_ang = mL.step_deltaE(h_ang)
        d_ex = mL.step_deltaE(h_ex)
        for row_i, (hh, dd, tag) in enumerate([(h_ang, d_ang, "equal-angle"), (h_ex, d_ex, "equalized")]):
            yy = y + row_i * (rowh + gap)
            parts.append(f'<text x="{x0-12}" y="{yy+rowh/2+4:.1f}" text-anchor="end" class="ink2 sub">{h0:.0f}°→{h1:.0f} {tag}</text>')
            # equal-angle: equal widths; equalized: widths proportional to angular step
            if tag == "equal-angle":
                widths = np.full(N_STEPS, w / N_STEPS)
            else:
                adh = np.abs(np.diff(hh))
                widths = adh / adh.sum() * w
            cx = x0
            lab = mL.lab_fn(hh)
            rgb = np.clip(cl.lab_to_srgb(lab), 0, 1)
            for i in range(N_STEPS):
                ww = widths[i]
                col = hex_rgb(rgb[i])
                parts.append(f'<rect x="{cx:.2f}" y="{yy:.1f}" width="{max(ww-2,1):.2f}" height="{rowh}" rx="3" fill="{col}" class="sw"><title>step {i+1}: h={hh[i]:.1f}°, ΔE00={dd[i]:.3f}</title></rect>')
                cx += ww
            cv = 100 * np.std(dd) / np.mean(dd)
            parts.append(f'<text x="{x0+w+12}" y="{yy+rowh/2+4:.1f}" class="ink2 sub">CV {cv:.2f}%</text>')
        y += 2 * (rowh + gap) + 18
    # legend
    parts.append(f'<rect x="{x0}" y="{H-34}" width="14" height="14" rx="3" fill="#888" class="sw"/>')
    parts.append(f'<text x="{x0+20}" y="{H-22}" class="muted sub">swatch color = CIELAB LCh point rendered to sRGB; hover a swatch for its hue and step ΔE00</text>')
    parts.append("</svg>")
    open(path, "w").write("\n".join(parts))


if __name__ == "__main__":
    out = os.path.dirname(os.path.abspath(__file__))
    fig_density(os.path.join(out, "fig_density.svg"))
    fig_strips(os.path.join(out, "fig_strips.svg"))
    print("wrote fig_density.svg, fig_strips.svg")
