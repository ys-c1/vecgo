"""Downsampling kernels for the alias/sharpness analysis.

Two candidate kernels (box, Gaussian) are width-matched by the SECOND-MOMENT
equivalent radius, plus a Lanczos-3 kernel kept only as an external benchmark.

Frequency convention used everywhere:
    xi in cycles per input sample; FT H(xi) = integral h(t) e^{-j 2 pi xi t} dt.
"""
import numpy as np


def box_kernel(s: int) -> np.ndarray:
    """Area-average box of width s (the kernel implicit in raster.Fill's
    supersample accumulation): h[k] = 1/s, k = 0..s-1."""
    return np.ones(s, dtype=float) / s


def gaussian_kernel(s: int, n_sigma: float = 3.0):
    """Gaussian matched to the s-tap box's DISCRETE probability variance:
    the s-tap box sits on integer taps, hence variance (s^2-1)/12 (not the
    continuous-width value s^2/12). sigma^2 is set equal to that."""
    sigma = np.sqrt((s * s - 1) / 12.0)
    k = int(np.ceil(n_sigma * sigma))
    n = np.arange(-k, k + 1, dtype=float)
    h = np.exp(-(n ** 2) / (2.0 * sigma ** 2))
    h /= h.sum()
    return h, sigma


def gaussian_kernel_sigma(sigma: float, n_sigma: float = 3.0) -> np.ndarray:
    """Gaussian with an explicit sigma (input-grid units), for the width sweep."""
    k = max(1, int(np.ceil(n_sigma * sigma)))
    n = np.arange(-k, k + 1, dtype=float)
    h = np.exp(-(n ** 2) / (2.0 * sigma ** 2))
    h /= h.sum()
    return h


def lanczos3_kernel(s: int) -> np.ndarray:
    """Lanczos-3 prefilter at downsampling factor s. Benchmark only."""
    k = 3 * s
    n = np.arange(-k, k + 1, dtype=float)
    t = n / s
    h = np.sinc(t) * np.sinc(t / 3.0) / s
    h /= h.sum()
    return h


def empirical_variance(h: np.ndarray) -> float:
    """Probability second moment about the centre (h sums to 1)."""
    n = np.arange(len(h), dtype=float) - (len(h) - 1) / 2.0
    return float(np.sum(h * n ** 2))


def energy_moment(h: np.ndarray) -> float:
    """Energy second moment r_E^2 = sum n^2 h[n]^2 / sum h[n]^2."""
    n = np.arange(len(h), dtype=float) - (len(h) - 1) / 2.0
    e = np.sum(h ** 2)
    return float(np.sum((n ** 2) * (h ** 2)) / e)


# ---- continuous-domain transfer functions (analytic FT of the kernels) ----

def H_box(xi: np.ndarray, s: int) -> np.ndarray:
    """FT of the unit-area box of width s: sinc(pi s xi)."""
    return np.sinc(s * np.asarray(xi, dtype=float))


def H_gauss(xi: np.ndarray, sigma: float) -> np.ndarray:
    """FT of N(0, sigma^2): exp(-2 pi^2 sigma^2 xi^2)."""
    xi = np.asarray(xi, dtype=float)
    return np.exp(-2.0 * (np.pi ** 2) * (sigma ** 2) * (xi ** 2))
