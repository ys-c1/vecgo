# Integer fixed-point premultiplied accumulator:
# every layer contributes once; intermediate results stay in exact integer
# accumulators; the only division happens at final output.
import math
N = 255
SCALE = 255 * 255 * 255          # enough headroom for any number of layers
SC = 255 * 255                   # scale of one term: color * alphas product

def rdiv(x, d):
    if x >= 0:
        return (x + d // 2) // d
    return -((-x + d // 2) // d)

def q8(x):
    return max(0, min(255, int(round(x))))

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

# ---- fixed-point accumulator composites ----
def composite_acc(layers):
    # layers: list of (straight color 0..255, alpha 0..255), bottom -> top
    # returns straight (color, alpha) in 0..255, exactly the same regardless
    # of how the caller conceptually parenthesizes, because folding uses a
    # single closed-form sum:
    #   a = sum_k alpha_k * prod_{j>k}(1-alpha_j)
    #   e = sum_k color_k*alpha_k * prod_{j>k}(1-alpha_j)   (premult energy)
    acc_a, acc_e = 0, 0          # in SCALE units
    tail = 1                     # prod (255-alpha_j) for layers already placed
    # fold from top layer downward
    for c, a in reversed(layers):
        acc_a = acc_a * 255 + a * tail
        acc_e = acc_e * 255 + c * a * tail
        tail *= (255 - a)
    if acc_a == 0:
        return (0, 0)
    color = rdiv(acc_e, acc_a)
    alpha = rdiv(acc_a, SC)
    return (min(255, color), min(255, alpha))

def parenthesized_acc(A, B, C):
    # Exact integer premultiplied fold, single division at final output.
    # Model (cross-checked against fractions.Fraction):
    #   layer k represented by a_k in [0,255], c_k in [0,255]
    #   over(top=(et,at), bottom=(eb,ab)):
    #     e = et + (1-at)*eb ,  a = at + (1-at)*ab
    # Carry everything as integer numerators over one shared denominator D:
    #   alpha field A/D with D a power of 255, energy field E/D^2 scale.
    # To keep both in one integer system we store the pair of *polynomial
    # coefficients* directly (closed form), which any parenthesization reaches.
    layers = [A, B, C]
    n = len(layers)
    # Exact closed form (cross-checked against fractions.Fraction):
    #   a = sum_k (a_k/255) * prod_{j>k}((255-a_j)/255)
    #   e = sum_k (c_k a_k/255^2) * prod_{j>k}((255-a_j)/255)
    # Common denominator for alpha terms and energy terms alike: 255^(n+1)
    # is not needed; each term's own denominator is exact, sum via scaling.
    def closed():
        a_num = e_num = 0
        d_a = d_e = 1                       # running common denominators
        for k, (c, a) in enumerate(layers):
            na, ne = a, c * a
            ta, te = 255, 255 * 255         # term denominators so far
            for _, aj in layers[k + 1:]:
                na *= (255 - aj)
                ne *= (255 - aj)
                ta *= 255
                te *= 255
            # extend global denominators
            if ta > d_a:
                a_num *= ta // d_a
                d_a = ta
            if te > d_e:
                e_num *= te // d_e
                d_e = te
            a_num += na * (d_a // ta)
            e_num += ne * (d_e // te)
        return a_num, d_a, e_num, d_e
    def out(t):
        a_num, d_a, e_num, d_e = t
        if a_num <= 0:
            return (0, 0)
        # straight color = (e_num/d_e) / (a_num/d_a) * 255
        color = rdiv(e_num * d_a * 255, d_e * a_num)
        alpha = rdiv(a_num * 255, d_a)
        return (min(255, color), min(255, alpha))
    t1 = closed()
    # second shape: right-recursive accumulation of the same fractions
    def rec(ks):
        if len(ks) == 1:
            c, a = layers[ks[0]]
            return a, 255, c * a, 255 * 255
        k = ks[0]
        c, a = layers[k]
        a2n, a2d, e2n, e2d = rec(ks[1:])
        # a = a_k/255 + (255-a_k)/255 * a2
        an = a * a2d + (255 - a) * a2n
        ad = 255 * a2d
        en = c * a * e2d + (255 - a) * e2n
        ed = 255 * 255 * e2d
        g = math.gcd(an, ad); an //= g; ad //= g
        g = math.gcd(en, ed); en //= g; ed //= g
        return an, ad, en, ed
    t2 = rec(list(range(n)))
    return out(t1), out(t2)

alpha_sets = [(80, 160, 120), (40, 200, 100), (120, 90, 170)]

def search(aa, ab, ac):
    n_st = n_pmstep = n_acc = 0
    ex_st, ex_acc, ex_pmstep = [], [], []
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
                    n_pmstep += 1
                    if len(ex_pmstep) < 3: ex_pmstep.append((cA, cB, cC, Lp, Rp))
                La, Ra = parenthesized_acc(A, B, C)
                if La != Ra:
                    n_acc += 1
                    if len(ex_acc) < 3: ex_acc.append((cA, cB, cC, La, Ra))
    return (n_st, ex_st), (n_pmstep, ex_pmstep), (n_acc, ex_acc)

def main():
    for ai, al in enumerate(alpha_sets):
        (ns, es), (np_, ep), (na, ea) = search(*al)
        print(f"set {ai} a={al}: ST-step fails={ns} PM-step fails={np_} int-acc fails={na} / 16777216")
        print("  ST examples:", es)
        print("  PMs examples:", ep[:2])
        print("  acc examples:", ea)

if __name__ == "__main__":
    main()
