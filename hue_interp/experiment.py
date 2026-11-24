"""
Hue-ring interpolation with perceptual arc-length parameterization.

Metric: CIEDE2000 (kL=kC=kH=1).
Two hue circles are tested:
  (A) constant-LCh  L*=50, C*=50 (clean circle in CIELAB)
  (B) HSL saturated S=1, L=0.5 (the classic "hue wheel")

For every endpoint pair we compare:
  - equal-angle steps:                h_i = h0 + i * dh / N
  - equal-perceptual-step (arc-length) parameterization: S(h_i) = i * S_total / N
and report variance of adjacent-step DeltaE00, plus the path-length/angle ratio.
"""
import sys, os, json
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import numpy as np
import colorlab as cl

N_STEPS = 12                 # intermediate steps per interpolation
GRID = 360_001                # 0.001 deg resolution over 3 cycles
N_CYCLES = 3
STEP_DEG = 360.0 * N_CYCLES / (GRID - 1)

# ---------------------------------------------------------------- hue circles
def lch_lab(h_deg, L=50.0, C=50.0):
    h = np.asarray(h_deg, dtype=float)
    return cl.lch_to_lab(np.stack([np.full_like(h, L), np.full_like(h, C), h], axis=-1))


def hsl_lab(h_deg):
    rgb = cl.hsl_to_srgb(h_deg, 1.0, 0.5)
    return cl.srgb_to_lab(rgb)


CIRCLES = {
    "LCh(L50 C50)": lch_lab,
    "HSL(S1 L0.5)": hsl_lab,
}

# --------------------------------------------------------- shortest-arc delta
def signed_hue_delta(h1, h0):
    """Signed shortest-arc delta h1-h0 in degrees, in (-180, 180]."""
    return (h1 - h0 + 180.0) % 360.0 - 180.0


# --------------------------------------------------------------- build metric
class ArcMetric:
    """Perceptual density g(h) = dE00/dh and cumulative arc length S(h)."""
    def __init__(self, name, lab_fn):
        self.name = name
        self.lab_fn = lab_fn
        h = np.linspace(0, 360 * N_CYCLES, GRID)
        lab = lab_fn(h)
        dE = cl.ciede2000(lab[:-1], lab[1:])
        self.h = h
        self.g = dE / STEP_DEG                       # DeltaE00 per degree
        S = np.concatenate([[0.0], np.cumsum(dE)])   # over 3 cycles
        self.S = S
        self.S_per_cycle = S[np.argmin(np.abs(h - 360.0))]
        # density over the canonical [0,360) cycle
        one = h < 360.0
        self.g1 = self.g[one[:-1]]
        self.h1 = h[:-1][one[:-1]]

    def S_at(self, hdeg):
        """Cumulative arc length at absolute angle hdeg (valid on [0,1080])."""
        return np.interp(hdeg, self.h, self.S)

    def equal_angle_hues(self, h0, dh, n):
        return h0 + np.linspace(0, dh, n + 1)

    def equal_metric_hues(self, h0, dh, n):
        """Invert S: hues with equal cumulative DeltaE00 increments."""
        base = h0 + 360.0   # shift to cycle 2 so h0+dh-180 stays on the grid
        S0 = self.S_at(base)
        S1 = self.S_at(base + dh)
        targets = np.linspace(S0, S1, n + 1)
        return np.interp(targets, self.S, self.h) - 360.0

    def equal_chord_hues(self, h0, dh, n, iters=60, tol=1e-12):
        """Exact finite-step equalization: redistribute nodes so the measured
        chordal DeltaE00 of every step is identical (fixed point on step budget)."""
        h = self.equal_metric_hues(h0, dh, n)
        for _ in range(iters):
            d = self.step_deltaE(h)
            Smeas = np.concatenate([[0.0], np.cumsum(d)])
            h_new = np.interp(np.linspace(0, Smeas[-1], n + 1), Smeas, h)
            h_new[0], h_new[-1] = h0, h0 + dh
            if np.max(np.abs(h_new - h)) < tol:
                h = h_new
                break
            h = h_new
        return h

    def path_length(self, h0, dh):
        return abs(self.S_at(h0 + 360.0 + dh) - self.S_at(h0 + 360.0))

    def step_deltaE(self, hues_abs):
        """DeltaE00 between consecutive points (hues given as absolute deg)."""
        lab = self.lab_fn(hues_abs % 360.0)
        return cl.ciede2000(lab[:-1], lab[1:])


# ------------------------------------------------------------------- run pairs
PAIRS = [
    (350.0,  10.0),   # crosses 0/360 seam, short arc 20 deg
    (  0.0, 180.0),   # exact antipode
    ( 20.0, 260.0),   # naive 240 -> wrapped -120 (long way avoided)
]

def stats(d):
    return dict(mean=float(np.mean(d)), var=float(np.var(d)),
                std=float(np.std(d)), cv=float(np.std(d) / np.mean(d)),
                dmin=float(np.min(d)), dmax=float(np.max(d)))

report = {}
for cname, lab_fn in CIRCLES.items():
    if __name__ != "__main__":
        break
    m = ArcMetric(cname, lab_fn)
    print("=" * 78)
    print(f"circle: {cname}   full-cycle perceptual length P = {m.S_per_cycle:.4f} DeltaE00")
    g_mean = m.S_per_cycle / 360.0
    print(f"g(h) range = [{m.g1.min():.5f}, {m.g1.max():.5f}]  "
          f"mean = {g_mean:.5f} DeltaE00/deg   gmax/gmin = {m.g1.max()/m.g1.min():.4f}")
    # analytic check under E76 for LCh: g should be C*pi/180 constant
    if cname.startswith("LCh"):
        g76 = 50.0 * np.pi / 180.0
        print(f"E76 analytic constant density C*pi/180 = {g76:.5f} DeltaE76/deg")

    rows = []
    for (h0, h1) in PAIRS:
        raw = h1 - h0
        dh = signed_hue_delta(h1, h0)
        wrap = raw - dh                      # multiple of 360
        span = abs(dh)

        h_eq = m.equal_angle_hues(h0, dh, N_STEPS)
        h_ar = m.equal_metric_hues(h0, dh, N_STEPS)
        h_ex = m.equal_chord_hues(h0, dh, N_STEPS)
        d_eq = m.step_deltaE(h_eq)
        d_ar = m.step_deltaE(h_ar)
        d_ex = m.step_deltaE(h_ex)
        s_eq, s_ar, s_ex = stats(d_eq), stats(d_ar), stats(d_ex)

        P = m.path_length(h0, dh)
        ratio = P / span

        # midpoint (t=0.5) hues, naive vs shortest-arc
        mid_naive = (h0 + raw / 2.0) % 360.0
        mid_arc = (h0 + dh / 2.0) % 360.0

        rows.append(dict(pair=(h0, h1), raw=raw, dh=dh, wrap=wrap, span=span,
                         eq=s_eq, ar=s_ar, ex=s_ex, P=P, ratio=ratio,
                         mid_naive=mid_naive, mid_arc=mid_arc,
                         h_eq=h_eq.tolist(), h_ar=h_ar.tolist(), h_ex=h_ex.tolist(),
                         d_eq=d_eq.tolist(), d_ar=d_ar.tolist(), d_ex=d_ex.tolist()))

        print("-" * 78)
        print(f"pair {h0:6.1f} -> {h1:6.1f} | raw dH={raw:7.1f}  signed dH={dh:7.1f}  "
              f"wrap jump={wrap:+6.1f} deg | midpoint naive={mid_naive:6.1f} short-arc={mid_arc:6.1f}")
        print(f"  equal-angle  : mean={s_eq['mean']:.4f} var={s_eq['var']:.3e} "
              f"cv={s_eq['cv']*100:.2f}%  range [{s_eq['dmin']:.4f},{s_eq['dmax']:.4f}]")
        print(f"  arc-length   : mean={s_ar['mean']:.4f} var={s_ar['var']:.3e} "
              f"cv={s_ar['cv']*100:.2f}%  range [{s_ar['dmin']:.4f},{s_ar['dmax']:.4f}]")
        print(f"  chord-equal  : mean={s_ex['mean']:.4f} var={s_ex['var']:.3e} "
              f"cv={s_ex['cv']*100:.2f}%  range [{s_ex['dmin']:.4f},{s_ex['dmax']:.4f}]")
        print(f"  variance reduction factor = {s_eq['var']/max(s_ar['var'],1e-30):.1f}x")
        print(f"  path length P={P:.4f}  angular span={span:.1f} deg  P/|dH|={ratio:.5f} "
              f"DeltaE00/deg")
    report[cname] = dict(P_cycle=float(m.S_per_cycle),
                         g_min=float(m.g1.min()), g_max=float(m.g1.max()),
                         g_mean=float(g_mean), rows=rows)

# ------------------------------------------------- 180 deg discontinuity sweep
if __name__ == "__main__":
    print("=" * 78)
    print("antipodal discontinuity: fixed h0=0, sweep h1 (LCh circle)")
    m0 = ArcMetric("LCh(L50 C50)", lch_lab)
    for h1 in [178.0, 179.0, 179.9, 180.0, 180.1, 181.0, 182.0]:
        dh = signed_hue_delta(h1, 0.0)
        mid = (0 + dh / 2.0) % 360.0
        print(f"  h1={h1:6.1f}  signed dH={dh:7.1f}  midpoint(t=.5)={mid:6.1f}")

    out = os.path.join(os.path.dirname(os.path.abspath(__file__)), "results.json")
    with open(out, "w") as f:
        json.dump(report, f, indent=1)
    print(f"\nwrote {out}")
