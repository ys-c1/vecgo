"""Numerical engine for the alias/sharpness analysis.

Notation (1-D; cycles per input sample)
---------------------------------------
Prefilter h, downsampling factor s, output Nyquist f_c = 1/(2s).
Downsampler output mean square (per output sample):

    E[y^2] = integral_{-1/2}^{1/2} |H(nu)|^2 S_x(nu) dnu,

split by spectral masking into the baseband E_b (|nu| < f_c) and the alias
energy E_a (|nu| >= f_c, folded onto the output baseband). Two independent
evaluations are reported:

  * DFT: exact discrete accounting on the actual finite kernel (circular conv,
    mask-out-of-baseband, decimate), Parseval-checked against time domain;
  * integral: the continuous-domain formula with H the kernel's analytic FT,
        E_a = integral_{|nu| >= f_c} |H(nu)|^2 S_x(nu) dnu.

Sharpness is gradient energy mean((y[m]-y[m-1])^2), both full and baseband-only.
"""
import csv
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

import numpy as np

from kernels import (
    box_kernel,
    gaussian_kernel,
    gaussian_kernel_sigma,
    lanczos3_kernel,
    empirical_variance,
    energy_moment,
    H_box,
    H_gauss,
)

OUT = os.path.join(os.path.dirname(__file__), "results")
os.makedirs(OUT, exist_ok=True)
SCALES = (2, 4, 8)


# --------------------------------------------------------------------- input
def build_input(n: int = 2 ** 18) -> np.ndarray:
    """Periodic, broadband input with real edges: every component is
    n-periodic so circular convolution has no wrap discontinuity, while the
    square/trapezoid waves supply genuine gradients for the sharpness metric.
    """
    t = np.arange(n, dtype=float)
    x = np.zeros(n)

    def square(period, duty, amp):
        return amp * (np.mod(t, period) < duty * period).astype(float)

    def trapezoid(period, rise, amp):
        ph = np.mod(t, period) / period
        z = np.zeros(n)
        z[ph < rise] = ph[ph < rise] / rise
        z[(ph >= rise) & (ph < 0.5)] = 1.0
        z[(ph >= 0.5) & (ph < 0.5 + rise)] = (0.5 + rise - ph[
            (ph >= 0.5) & (ph < 0.5 + rise)]) / rise
        return amp * z

    x += 0.8 * square(256, 0.5, 1.0)
    x += 0.5 * square(137, 0.3, 1.0)
    x += 0.6 * trapezoid(401, 0.08, 1.0)
    # tones spread across the spectrum; integer-cycle => exactly periodic
    for f0, amp in ((0.03, 0.35), (0.11, 0.25), (0.19, 0.20),
                    (0.31, 0.18), (0.43, 0.12)):
        x += amp * np.sin(2 * np.pi * f0 * t)
    # stationary colored-noise component (fixed realization)
    rng = np.random.default_rng(20261126)
    w = rng.standard_normal(n)
    a = 0.45
    u = np.empty(n)
    u[0] = w[0]
    for i in range(1, n):
        u[i] = a * u[i - 1] + np.sqrt(1 - a ** 2) * w[i]
    x += 0.3 * u
    return x


# ------------------------------------------------------------------ spectrum
def periodogram_onesided(x: np.ndarray, pad: int = 2 ** 20):
    """One-sided periodogram I(nu) = |X_pad|^2 / N on the padded DFT grid.
    Two-sided integrals use 2 * trapezoid(I, nu); the endpoints contribute a
    single bin each, negligible at pad = 2^20."""
    n = len(x)
    X = np.fft.fft(x, pad)
    f = np.arange(pad // 2 + 1, dtype=float) / pad
    I = (np.abs(X[:pad // 2 + 1]) ** 2) / n
    return f, I


# ------------------------------------------------------- exact discrete model
def mask_split(x: np.ndarray, h: np.ndarray, s: int):
    """Base/alias energy via spectral masking (exact for circular conv).

    V = X * DFT(h). The baseband mask keeps |nu| < f_c. Rather than taking an
    energy difference (which can pick up mask-edge leakage), the alias signal
    is built directly: V_a = V - V_b, inverse-transformed and decimated, so
    E_alias = mean(y_a^2) >= 0 by construction. Also returns the
    linear-convolution/reflect-pad output for an independent Parseval check."""
    n = len(x)
    nu = np.fft.fftfreq(n)
    V = np.fft.fft(x) * np.fft.fft(h, n)
    base_mask = np.abs(nu) < 0.5 / s
    Vb = np.where(base_mask, V, 0.0)
    Va = np.where(base_mask, 0.0, V)
    y_tot = np.fft.ifft(V).real[::s]
    y_base = np.fft.ifft(Vb).real[::s]
    y_alias = np.fft.ifft(Va).real[::s]
    e_tot = float(np.mean(y_tot ** 2))
    e_base = float(np.mean(y_base ** 2))
    e_alias = float(np.mean(y_alias ** 2))
    # independent time-domain path: linear convolution, reflect padding
    xp = np.pad(x, (len(h), len(h)), mode="reflect")
    y_lin = np.convolve(xp, h, mode="same")[len(h)::s][:n // s]
    e_lin = float(np.mean(y_lin ** 2))
    return e_base, e_alias, e_tot, e_lin, y_tot, y_base


# -------------------------------------------------- continuous spectral model
def integral_split(f, I, s: int, kind: str, sigma=None, h=None):
    """E_b, E_a from E = integral |H(nu)|^2 S_x(nu) dnu (two-sided, no 1/s:
    this is variance per output sample). H is the analytic kernel FT for
    box/Gaussian; the Lanczos DTFT is sampled directly (no closed form)."""
    if kind == "box":
        Hm2 = H_box(f, s) ** 2
    elif kind == "gauss":
        Hm2 = H_gauss(f, sigma) ** 2
    else:
        kk = np.arange(len(h)) - (len(h) - 1) / 2.0
        Hd = np.sum(h * np.exp(-1j * 2 * np.pi * f[:, None] * kk), axis=1)
        Hm2 = np.abs(Hd) ** 2
    fc = 0.5 / s
    inside = 2.0 * np.trapezoid(Hm2[f <= fc] * I[f <= fc], f[f <= fc])
    tail = 2.0 * np.trapezoid(Hm2[f > fc] * I[f > fc], f[f > fc])
    return inside, tail


# ----------------------------------------------------------------- sharpness
def gradient_energy(y: np.ndarray) -> float:
    return float(np.mean(np.diff(y) ** 2))


def mtf50(kind: str, s: int, sigma=None) -> float:
    """Frequency where |H| = 0.5, as a fraction of output Nyquist f_c."""
    if kind == "box":
        u0 = 0.60335384139657           # root of |sinc(pi u)| = 0.5
        return 2.0 * u0
    xi50 = np.sqrt(np.log(2.0) / (2.0 * np.pi ** 2 * sigma ** 2))
    return xi50 / (0.5 / s)


# ---------------------------------------------------------------------- main
def main():
    x = build_input()
    f, I = periodogram_onesided(x)

    rows = []
    for s in SCALES:
        hb = box_kernel(s)
        hg, sg = gaussian_kernel(s)
        hl = lanczos3_kernel(s)
        built = [("box", hb, {}), ("gauss", hg, {"sigma": sg}),
                 ("lanczos3", hl, {})]
        for name, h, extra in built:
            e_b_dft, e_a_dft, e_tot, e_lin, y_tot, y_base = mask_split(x, h, s)
            kind = {"box": "box", "gauss": "gauss",
                    "lanczos3": "lanczos"}[name]
            e_b_int, e_a_int = integral_split(
                f, I, s, kind, extra.get("sigma"), h)
            rows.append({
                "scale": s,
                "kernel": name,
                "taps": len(h),
                "r_prob": (np.sqrt(empirical_variance(h))
                           if np.all(h >= 0) else float("nan")),
                "r_energy": np.sqrt(energy_moment(h)),
                "E_alias_dft": e_a_dft,
                "E_alias_integral": e_a_int,
                "E_base_dft": e_b_dft,
                "E_base_integral": e_b_int,
                "alias_frac_dft": e_a_dft / e_tot,
                "sharp_gradient": gradient_energy(y_tot),
                "sharp_baseband": gradient_energy(y_base),
                "mtf50_frac_nyq": (mtf50(kind, s, extra.get("sigma"))
                                   if name != "lanczos3" else float("nan")),
                "parseval_check": abs(e_tot - e_lin) / e_tot,
            })

    # ----------------------------------------------- width sweep at fixed s=4
    sweep = []
    s_ref = 4
    for w in range(2, 10):
        hb = np.ones(w) / w
        hg = gaussian_kernel_sigma(np.sqrt((w * w - 1) / 12.0))
        for name, h in (("box", hb), ("gauss", hg)):
            e_b, e_a, e_tot, _, y_tot, y_base = mask_split(x, h, s_ref)
            sweep.append({
                "width_taps": w, "kernel": name,
                "r_prob": np.sqrt(empirical_variance(h)),
                "E_alias": e_a, "alias_frac": e_a / e_tot,
                "sharp": gradient_energy(y_tot),
                "sharp_bb": gradient_energy(y_base),
            })

    # ---------------- white-input out-of-band energy Q(fc), analytic kernels
    from math import erfc, sqrt, pi, sin
    s = 4
    fc = 0.5 / s
    # sinc^2 decays only as 1/nu^2: the integration range must be wide or the
    # truncated tail is badly biased. Dense grid to 100 captures >99.9%.
    grid = np.linspace(0.0, 100.0, 4_000_001)
    sigma_g = sqrt((s * s - 1) / 12.0)
    hb, hg = box_kernel(s), gaussian_kernel(s)[0]
    rE2 = {"box": energy_moment(hb), "gauss": energy_moment(hg)}
    Hm2 = {"box": H_box(grid, s) ** 2,
           "gauss": H_gauss(grid, sigma_g) ** 2}
    # Exact Parseval totals remove truncation bias.
    #   box:    integral_R sinc^2(pi s nu) dnu = 1/s.
    #   gauss:  H = exp(-2 pi^2 sig^2 nu^2), |H|^2 = exp(-4 pi^2 sig^2 nu^2),
    #           whose integral over R is 1/(2 sig sqrt(pi)).
    exact_total = {"box": 1.0 / s,
                   "gauss": 1.0 / (2.0 * sigma_g * sqrt(pi))}
    tail = []
    for name, z in Hm2.items():
        one_sided = np.trapezoid(z[grid > fc], grid[grid > fc])
        tail.append({"kernel": name,
                     "Q_white_numeric": 2.0 * one_sided / exact_total[name],
                     "r_energy_sq": rE2[name]})
    # Gaussian closed form: normalized tail of exp(-4 pi^2 sig^2 nu^2) is
    # erfc(2 pi sig fc).
    q_gauss_exact = erfc(2 * pi * sigma_g * fc)
    # Box closed form. With v = pi s nu,
    #   integral_0^U sinc^2(v) dv = Si(2U) - sin^2(U)/U,
    # and integral_0^inf sinc^2(v) dv = pi/2. Total two-sided |H|^2 energy is
    # 1/s, so the inside-band fraction is (2/(pi)) * that antiderivative at
    # U = pi s fc. Si implemented as a fine-grid integral (no scipy available).
    u = pi * s * fc
    si_grid = np.linspace(1e-12, 2.0 * u, 200001)
    Si_2u = float(np.trapezoid(np.sin(si_grid) / si_grid, si_grid))
    inside_one_uv = Si_2u - sin(u) ** 2 / u
    q_box_exact = 1.0 - 2.0 * inside_one_uv / pi
    box_capture = s * 2.0 * np.trapezoid(Hm2["box"], grid)

    # ---------------- Heisenberg, probability form, discrete DTFT one period
    dgrid = np.linspace(0.0, 0.5, 200001)
    n_b = np.arange(s) - (s - 1) / 2.0
    n_g = np.arange(len(hg)) - (len(hg) - 1) / 2.0
    Hdtft = {
        "box": np.abs(np.sum(hb * np.exp(
            -1j * 2 * np.pi * dgrid[:, None] * n_b), axis=1)) ** 2,
        "gauss": np.abs(np.sum(hg * np.exp(
            -1j * 2 * np.pi * dgrid[:, None] * n_g), axis=1)) ** 2,
    }
    rp2 = {"box": empirical_variance(hb), "gauss": empirical_variance(hg)}
    sig_xi2 = {}
    for name, z in Hdtft.items():
        full = 2.0 * np.trapezoid(z, dgrid)
        second = 2.0 * np.trapezoid((dgrid ** 2) * z, dgrid)
        sig_xi2[name] = second / full
    heis_lhs = {k: rp2[k] * sig_xi2[k] for k in rp2}
    heis_bound = 1.0 / (16.0 * pi ** 2)
    gauss_cont = 1.0 / (16.0 * pi ** 2)   # sigma^2 cancels: exact saturation

    # ---- sharpness-constrained Gaussian alias floor (passband response >= A)
    A = 0.9
    fp = fc / 2.0
    sig_max = sqrt(-np.log(A) / (2.0 * pi ** 2 * fp ** 2))
    q_floor = erfc(2 * pi * sig_max * fc)

    # ------------------------------------------------------------------ save
    def dump(name, data):
        with open(os.path.join(OUT, name), "w", newline="") as fh:
            w = csv.DictWriter(fh, fieldnames=list(data[0].keys()))
            w.writeheader()
            w.writerows(data)

    dump("metrics.csv", rows)
    dump("sweep.csv", sweep)
    dump("tail.csv", tail)
    with open(os.path.join(OUT, "bounds.txt"), "w") as fh:
        fh.write(f"s={s}, fc={fc:.6f} cycles/input-sample, "
                 f"sigma_gauss={sigma_g:.6f}\n")
        fh.write("white-input out-of-band energy fraction Q(fc):\n")
        fh.write(f"  gauss exact (erfc) = {q_gauss_exact:.6e} | numeric = "
                 f"{[t['Q_white_numeric'] for t in tail if t['kernel']=='gauss'][0]:.6e}\n")
        fh.write(f"  box   exact (Si)   = {q_box_exact:.6e} | numeric = "
                 f"{[t['Q_white_numeric'] for t in tail if t['kernel']=='box'][0]:.6e}\n")
        fh.write(f"  box numeric energy capture over grid = {box_capture:.4f} "
                 f"(sinc^2 tail truncation)\n")
        fh.write(f"Heisenberg  r_p^2 * r_nu^2 >= 1/(16 pi^2) = {heis_bound:.6e}\n")
        for k in ("gauss", "box"):
            fh.write(f"  {k:<5} discrete lhs = {heis_lhs[k]:.6e} "
                     f"(r_p^2={rp2[k]:.6f}, r_nu^2={sig_xi2[k]:.6e})\n")
        fh.write(f"  continuous Gaussian exact lhs = {gauss_cont:.6e}\n")
        fh.write("sharpness-constrained Gaussian alias floor:\n")
        fh.write(f"  |H(fp)| >= {A} at fp={fp:.4f} => sigma <= {sig_max:.4f}\n")
        fh.write(f"  => Q(fc) >= {q_floor:.6e}\n")

    hdr = (f"{'s':>2} {'kernel':<8} {'r_prob':>6} {'r_E':>6} "
           f"{'Ealias_DFT':>11} {'Ealias_INT':>11} {'frac%':>6} "
           f"{'sharp':>9} {'sharpBB':>9} {'MTF50':>6} {'chk':>7}")
    print(hdr)
    for r in rows:
        print(f"{r['scale']:>2} {r['kernel']:<8} {r['r_prob']:6.3f} "
              f"{r['r_energy']:6.3f} {r['E_alias_dft']:11.4e} "
              f"{r['E_alias_integral']:11.4e} {100*r['alias_frac_dft']:6.2f} "
              f"{r['sharp_gradient']:9.4e} {r['sharp_baseband']:9.4e} "
              f"{r['mtf50_frac_nyq']:6.3f} {r['parseval_check']:7.1e}")
    print()
    print(open(os.path.join(OUT, "bounds.txt")).read())


if __name__ == "__main__":
    main()
