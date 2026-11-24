"""
Color conversions (sRGB D65 <-> XYZ <-> CIELAB <-> LCh) and CIEDE2000.

Pure numpy implementation, written to follow Sharma, Wu & Dalal (2005),
"The CIEDE2000 Color-Difference Formula: Implementation Notes, Supplementary
Test Data, and Mathematical Observations".
"""
import numpy as np

# D65 reference white (2 degree standard observer; matches colour-science default)
XN, YN, ZN = 0.9504559270516716, 1.0, 1.0890577507598784


# ---------------------------------------------------------------- sRGB / XYZ
def srgb_to_linear(c):
    c = np.asarray(c, dtype=float)
    return np.where(c <= 0.04045, c / 12.92, ((c + 0.055) / 1.055) ** 2.4)


def linear_to_srgb(c):
    c = np.asarray(c, dtype=float)
    return np.where(c <= 0.0031308, 12.92 * c, 1.055 * np.power(np.maximum(c, 0), 1 / 2.4) - 0.055)


def srgb_to_xyz(rgb):
    rgb = np.asarray(rgb, dtype=float)
    lin = srgb_to_linear(rgb)
    # Rounded sRGB matrix used by the colour-science defaults / common textbooks
    m = np.array([
        [0.4124, 0.3576, 0.1805],
        [0.2126, 0.7152, 0.0722],
        [0.0193, 0.1192, 0.9505],
    ])
    return lin @ m.T


def xyz_to_srgb(xyz):
    m = np.array([
        [ 3.2406, -1.5372, -0.4986],
        [-0.9689,  1.8758,  0.0415],
        [ 0.0557, -0.2040,  1.0570],
    ])
    lin = np.asarray(xyz, dtype=float) @ m.T
    return linear_to_srgb(lin)


# ---------------------------------------------------------------------- Lab
def _f(t):
    t = np.asarray(t, dtype=float)
    d = 6 / 29
    return np.where(t > d ** 3, np.cbrt(t), t / (3 * d * d) + 4 / 29)


def _finv(f):
    f = np.asarray(f, dtype=float)
    d = 6 / 29
    return np.where(f > d, f ** 3, 3 * d * d * (f - 4 / 29))


def xyz_to_lab(xyz):
    xyz = np.asarray(xyz, dtype=float)
    fx = _f(xyz[..., 0] / XN)
    fy = _f(xyz[..., 1] / YN)
    fz = _f(xyz[..., 2] / ZN)
    L = 116 * fy - 16
    a = 500 * (fx - fy)
    b = 200 * (fy - fz)
    return np.stack([L, a, b], axis=-1)


def lab_to_xyz(lab):
    lab = np.asarray(lab, dtype=float)
    L, a, b = lab[..., 0], lab[..., 1], lab[..., 2]
    fy = (L + 16) / 116
    fx = fy + a / 500
    fz = fy - b / 200
    return np.stack([XN * _finv(fx), YN * _finv(fy), ZN * _finv(fz)], axis=-1)


def srgb_to_lab(rgb):
    return xyz_to_lab(srgb_to_xyz(rgb))


def lab_to_srgb(lab):
    return xyz_to_srgb(lab_to_xyz(lab))


# ---------------------------------------------------------------------- LCh
def lab_to_lch(lab):
    lab = np.asarray(lab, dtype=float)
    L, a, b = lab[..., 0], lab[..., 1], lab[..., 2]
    C = np.hypot(a, b)
    h = np.degrees(np.arctan2(b, a)) % 360
    return np.stack([L, C, h], axis=-1)


def lch_to_lab(lch):
    lch = np.asarray(lch, dtype=float)
    L, C, h = lch[..., 0], lch[..., 1], lch[..., 2]
    hr = np.radians(h)
    return np.stack([L, C * np.cos(hr), C * np.sin(hr)], axis=-1)


# ---------------------------------------------------------------- CIEDE2000
def ciede2000(lab1, lab2, kL=1.0, kC=1.0, kH=1.0):
    """Delta E00. Inputs broadcast against each other; last axis is (L,a,b)."""
    lab1 = np.asarray(lab1, dtype=float)
    lab2 = np.asarray(lab2, dtype=float)
    L1, a1, b1 = lab1[..., 0], lab1[..., 1], lab1[..., 2]
    L2, a2, b2 = lab2[..., 0], lab2[..., 1], lab2[..., 2]

    C1 = np.hypot(a1, b1)
    C2 = np.hypot(a2, b2)
    Cbar = 0.5 * (C1 + C2)
    Cbar7 = Cbar ** 7
    G = 0.5 * (1 - np.sqrt(Cbar7 / (Cbar7 + 25.0 ** 7)))

    ap1 = (1 + G) * a1
    ap2 = (1 + G) * a2
    Cp1 = np.hypot(ap1, b1)
    Cp2 = np.hypot(ap2, b2)

    hp1 = np.degrees(np.arctan2(b1, ap1)) % 360
    hp2 = np.degrees(np.arctan2(b2, ap2)) % 360

    dLp = L2 - L1
    dCp = Cp2 - Cp1

    dhp = np.zeros_like(dCp)
    both = (Cp1 * Cp2) != 0
    diff = (hp2 - hp1 + 180) % 360 - 180  # wrapped into (-180, 180]
    dhp = np.where(both, diff, 0.0)

    dHp = 2 * np.sqrt(Cp1 * Cp2) * np.sin(np.radians(dhp) / 2)

    Lbarp = 0.5 * (L1 + L2)
    Cbarp = 0.5 * (Cp1 + Cp2)

    # mean hue
    hsum = hp1 + hp2
    hbarp = np.where(~both, hsum,
             np.where(np.abs(hp1 - hp2) <= 180, hsum / 2,
             np.where(hsum < 360, (hsum + 360) / 2, (hsum - 360) / 2)))

    T = (1
         - 0.17 * np.cos(np.radians(hbarp - 30))
         + 0.24 * np.cos(np.radians(2 * hbarp))
         + 0.32 * np.cos(np.radians(3 * hbarp + 6))
         - 0.20 * np.cos(np.radians(4 * hbarp - 63)))

    dtheta = 30 * np.exp(-(((hbarp - 275) / 25) ** 2))
    Cbarp7 = Cbarp ** 7
    RC = 2 * np.sqrt(Cbarp7 / (Cbarp7 + 25.0 ** 7))
    SL = 1 + (0.015 * (Lbarp - 50) ** 2) / np.sqrt(20 + (Lbarp - 50) ** 2)
    SC = 1 + 0.045 * Cbarp
    SH = 1 + 0.015 * Cbarp * T
    RT = -np.sin(np.radians(2 * dtheta)) * RC

    lterm = dLp / (kL * SL)
    cterm = dCp / (kC * SC)
    hterm = dHp / (kH * SH)
    return np.sqrt(lterm ** 2 + cterm ** 2 + hterm ** 2 + RT * cterm * hterm)


def ciede76(lab1, lab2):
    """Euclidean Lab distance, Delta E*ab."""
    lab1 = np.asarray(lab1, dtype=float)
    lab2 = np.asarray(lab2, dtype=float)
    return np.linalg.norm(lab2 - lab1, axis=-1)


# ---------------------------------------------------------------------- HSL
def hsl_to_srgb(h, s=1.0, l=0.5):
    """h in degrees, s,l in [0,1]. Vectorized over h."""
    h = np.asarray(h, dtype=float)
    s = np.broadcast_to(np.asarray(s, dtype=float), h.shape)
    l = np.broadcast_to(np.asarray(l, dtype=float), h.shape)
    hp = (h % 360) / 360.0
    c = (1 - np.abs(2 * l - 1)) * s
    x = c * (1 - np.abs((hp * 6) % 2 - 1))
    m = l - c / 2
    sext = np.floor(hp * 6).astype(int) % 6
    out = np.zeros(h.shape + (3,))
    z = np.zeros_like(c)
    # (R, G, B) triplet per hue sector
    for k, (rp, gp, bp) in enumerate([
        (c, x, z), (x, c, z), (z, c, x),
        (z, x, c), (x, z, c), (c, z, x),
    ]):
        msk = (sext == k)[..., None]
        out += np.where(msk, np.stack([rp, gp, bp], axis=-1), 0.0)
    out += m[..., None]
    return np.clip(out, 0, 1)
