# Final clean version. Three pipelines compared over the full 256^3 color space:
#   ST-step : straight (unassociated) over, quantize after every pair
#   PM-step : premultiplied over, quantize after every pair
#   ACC     : premultiplied over on exact integer fractions (gcd reduced),
#             quantized exactly once at output — the associative rewrite
import math

def rdiv(x, d):
    if x >= 0:
        return (x + d // 2) // d
    return -((-x + d // 2) // d)

def over_st_step(P, B):
    cA, aa = P
    cB, ab = B
    ao = (aa * 255 + (255 - aa) * ab) // 255
    if ao == 0:
        return (0, 0)
    e_num = rdiv(aa * cA * 255 + (255 - aa) * ab * cB, 255)
    co = rdiv(e_num * 255, ao * 255)
    return (min(255, co), min(255, ao))

def over_pm_step(P, B):
    pa, aa = P
    pb, ab = B
    ao = (aa * 255 + (255 - aa) * ab) // 255
    po = rdiv(pa * 255 + (255 - aa) * pb, 255)
    return (min(255, po), min(255, ao))

def to_pm(p):
    c, a = p
    return (rdiv(c * a, 255), a)

def pair(over, A, B, C):
    return over(over(A, B), C), over(A, over(B, C))

# ---- exact integer-fraction premultiplied over ----
# pixel = (energy numerator/denominator, alpha numerator/denominator)
def mk(c, a):
    return (c * a, 255 * 255, a, 255)          # e = c*a/255^2, a/255

def add(n1, d1, n2, d2):
    n, d = n1 * d2 + n2 * d1, d1 * d2
    g = math.gcd(n, d)
    return n // g, d // g

def over_acc(top, bot):
    e1n, e1d, a1n, a1d = top
    e2n, e2d, a2n, a2d = bot
    # (1-a1)
    m1n, m1d = a1d - a1n, a1d
    g = math.gcd(m1n, m1d); m1n //= g; m1d //= g
    an, ad = add(a1n, a1d, m1n * a2n, m1d * a2d)
    en, ed = add(e1n, e1d, m1n * e2n, m1d * e2d)
    return (en, ed, an, ad)

def out_acc(p):
    en, ed, an, ad = p
    if an == 0:
        return (0, 0)
    color = rdiv(en * ad * 255, ed * an)
    alpha = rdiv(an * 255, ad)
    return (min(255, color), min(255, alpha))

def acc_pair(A, B, C):
    p = [mk(*A), mk(*B), mk(*C)]
    L = over_acc(over_acc(p[0], p[1]), p[2])
    R = over_acc(p[0], over_acc(p[1], p[2]))
    return out_acc(L), out_acc(R)

alpha_sets = [(80, 160, 120), (40, 200, 100), (120, 90, 170)]

def search(aa, ab, ac):
    n_st = n_pm = n_acc = 0
    ex_st, ex_pm = [], []
    for cA in range(256):
        for cB in range(256):
            for cC in range(256):
                A, B, C = (cA, aa), (cB, ab), (cC, ac)
                Ls, Rs = pair(over_st_step, A, B, C)
                if Ls != Rs:
                    n_st += 1
                    if len(ex_st) < 3: ex_st.append((cA, cB, cC, Ls, Rs))
                Lp, Rp = pair(over_pm_step, to_pm(A), to_pm(B), to_pm(C))
                if Lp != Rp:
                    n_pm += 1
                    if len(ex_pm) < 3: ex_pm.append((cA, cB, cC, Lp, Rp))
                La, Ra = acc_pair(A, B, C)
                if La != Ra:
                    n_acc += 1
    return (n_st, ex_st), (n_pm, ex_pm), n_acc

def main():
    for ai, al in enumerate(alpha_sets):
        (ns, es), (np_, ep), na = search(*al)
        print(f"set {ai} a={al}: ST-step={ns} PM-step={np_} ACC={na} / 16777216")
        print("  ST examples:", es)
        print("  PM examples:", ep[:2])

if __name__ == "__main__":
    main()
