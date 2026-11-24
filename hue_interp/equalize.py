"""
Exact finite-step equalization: iteratively adjust hues so that the *chordal*
CIEDE2000 distance between consecutive samples (not the infinitesimal arc) is
constant. This separates parameterization error from discretization error:
  - arc-length inversion equalizes the differential metric g(h)=dE/dh exactly,
  - this routine equalizes the actual N-step measured DeltaE00 exactly.
"""
import sys, os
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import numpy as np
from experiment import ArcMetric, lch_lab, hsl_lab, stats, signed_hue_delta, PAIRS


def equalize_chords(metric: ArcMetric, h0, dh, n, iters=40, tol=1e-12):
    """Newton-style fixed point: h_{i+1} = S^{-1}(S(h_i) + target),
    with target re-estimated from measured chord lengths each iteration."""
    h = metric.equal_metric_hues(h0, dh, n)          # start from arc-length guess
    base = h0 + 360.0
    for _ in range(iters):
        d = metric.step_deltaE(h)
        target = d.mean()
        # rebuild cumulative S at current node positions using *measured* chords,
        # then place every interior node at equal measured budget
        Smeas = np.concatenate([[0.0], np.cumsum(d)])
        Snodes = np.linspace(0, Smeas[-1], n + 1)
        h_new = np.interp(Snodes, Smeas, h)          # re-grid on measured budget
        h_new[0], h_new[-1] = h0, h0 + dh
        # one correction pass using the differential metric for interpolation
        if np.max(np.abs(h_new - h)) < tol:
            h = h_new
            break
        h = h_new
    return h


if __name__ == "__main__":
    for cname, fn in [("LCh(L50 C50)", lch_lab), ("HSL(S1 L0.5)", hsl_lab)]:
        m = ArcMetric(cname, fn)
        print("=" * 74)
        print(cname)
        for (h0, h1) in PAIRS:
            dh = signed_hue_delta(h1, h0)
            h_arc = m.equal_metric_hues(h0, dh, 12)
            h_eqd = equalize_chords(m, h0, dh, 12)
            s_arc = stats(m.step_deltaE(h_arc))
            s_eqd = stats(m.step_deltaE(h_eqd))
            s_ang = stats(m.step_deltaE(m.equal_angle_hues(h0, dh, 12)))
            print(f" {h0:5.0f}->{h1:5.0f}  equal-angle CV={s_ang['cv']*100:6.2f}%   "
                  f"arc-length CV={s_arc['cv']*100:6.3f}%   "
                  f"chord-equalized CV={s_eqd['cv']*100:8.5f}%   "
                  f"var factor eq-angle/arc={s_ang['var']/s_arc['var']:8.1f}x")
