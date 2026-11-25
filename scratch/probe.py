# Probe: associativity of Porter-Duff "over" in premultiplied vs straight alpha.
# Runs both exact rational arithmetic and an 8-bit per-step quantized simulation.
from fractions import Fraction

N = 255

def q8(x):  # round-to-nearest, 0..255
    v = int(round(float(x)))
    return max(0, min(255, v))

# ---------- exact rational (math level), values as Fraction in [0,1] ----------
def over_pm_exact(P, B):
    # premultiplied: pixel = (p, a) where p = a*C
    pp, ap = P
    pb, ab = B
    return (pp + (1 - ap) * pb, ap + (1 - ap) * ab)

def over_st_exact(P, B):
    # straight: pixel = (C, a); energy = a*C
    Ca, aa = P
    Cb, ab = B
    ao = aa + (1 - aa) * ab
    if ao == 0:
        return (Fraction(0), Fraction(0))
    co = (aa * Ca + (1 - aa) * ab * Cb) / ao
    return (co, ao)

# ---------- 8-bit integer simulation, per-step round to 0..255 ----------
def over_pm_u8(P, B):
    pp, ap = P
    pb, ab = B
    ao = q8(255 * (ap / 255 + (1 - ap / 255) * (ab / 255)))
    po = q8(pp + (255 - ap) * pb / 255)
    return (po, ao)

def over_st_u8(P, B):
    cP, aP = P
    cB, aB = B
    af, bf, cf, df = aP / 255, aB / 255, cP / 255, cB / 255
    ao = af + (1 - af) * bf
    if ao == 0:
        return (0, 0)
    co = (af * cf + (1 - af) * bf * df) / ao
    return (q8(co * 255), q8(ao * 255))

def assoc_check(over, A, B, C):
    L = over(over(A, B), C)
    R = over(A, over(B, C))
    return L, R

# --- exact math sanity on one triple ---
A = (Fraction(200, 255), Fraction(80, 255))
B = (Fraction(40, 255), Fraction(160, 255))
C = (Fraction(230, 255), Fraction(120, 255))
L, R = assoc_check(over_st_exact, A, B, C)
print("exact straight diff:", abs(L[0] - R[0]), abs(L[1] - R[1]))
L, R = assoc_check(over_pm_exact, (A[0]*A[1], A[1]), (B[0]*B[1], B[1]), (C[0]*C[1], C[1]))
print("exact premult  diff:", abs(L[0] - R[0]), abs(L[1] - R[1]))

# --- 8-bit sweep over alpha triples, fixed colors ---
# layers colors chosen so the three counterexample points are easy to recheck
color_triples = [
    ((220, 80),  (30, 160),  (10, 200)),   # point 0
    ((255, 60),  (0,  180),  (128, 140)),  # point 1
    ((10, 100),  (200, 120), (60, 220)),   # point 2
]

alpha_sets = [
    (80, 160, 120),
    (40, 200, 100),
    (120, 90, 170),
]

for ai, (aa, ab, ac) in enumerate(alpha_sets):
    print(f"\n=== alpha set {ai}: aA={aa} aB={ab} aC={ac} ===")
    for pi, ((cA, _), (cB, _), (cC, _)) in enumerate(color_triples):
        A8 = (cA, aa); B8 = (cB, ab); C8 = (cC, ac)
        Ls, Rs = assoc_check(over_st_u8, A8, B8, C8)
        Ap = (q8(cA * aa / 255), aa); Bp = (q8(cB * ab / 255), ab); Cp = (q8(cC * ac / 255), ac)
        Lp, Rp = assoc_check(over_pm_u8, Ap, Bp, Cp)
        print(f" point {pi}: straight L={Ls} R={Rs} dC={abs(Ls[0]-Rs[0])} da={abs(Ls[1]-Rs[1])} | "
              f"premult L={Lp} R={Rp} dC={abs(Lp[0]-Rp[0])} da={abs(Lp[1]-Rp[1])}")

# --- whole-image diff counts for the 3 alpha sets (3x3 grid of the above points) ---
import itertools
for ai, (aa, ab, ac) in enumerate(alpha_sets):
    img = [((cA, aa), (cB, ab), (cC, ac))
           for ((cA, _), (cB, _), (cC, _)) in color_triples]
    d_st = d_pm = 0
    for A8, B8, C8 in img:
        Ls, Rs = assoc_check(over_st_u8, A8, B8, C8)
        if Ls != Rs: d_st += 1
        Ap = (q8(A8[0]*aa/255), aa); Bp = (q8(B8[0]*ab/255), ab); Cp = (q8(C8[0]*ac/255), ac)
        Lp, Rp = assoc_check(over_pm_u8, Ap, Bp, Cp)
        if Lp != Rp: d_pm += 1
    print(f"set {ai}: differing pixels straight={d_st}/{len(img)} premult={d_pm}/{len(img)}")
