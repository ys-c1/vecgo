# Compare three implementations under both alpha conventions.
#   PM-acc  : premultiplied float64 accumulator, quantized ONCE at the end
#   PM-step : premultiplied, quantized (rounded) to 8 bits after every over()
#   ST-step : straight (unassociated), per-step divide-back + 8-bit quantization
N = 255

def rdiv(x, d):
    if x >= 0:
        return (x + d // 2) // d
    return -((-x + d // 2) // d)

def q8(x):
    return max(0, min(255, int(round(x))))

# ---- PM accumulator: exact Porter-Duff in float 0..1, no intermediate rounding ----
def over_pm_acc(P, B):
    pa, aa = P
    pb, ab = B
    return (pa + (1 - aa) * pb, aa + (1 - aa) * ab)

def finalize_pm(pa, aa):
    if aa <= 0:
        return (0, 0)
    return (q8(pa / aa), q8(aa))

# ---- PM with 8-bit rounding after each step ----
def over_pm_step(P, B):
    pa, aa = P
    pb, ab = B
    ao = (aa * 255 + (255 - aa) * ab) // 255
    po = rdiv(pa * 255 + (255 - aa) * pb, 255)
    return (min(255, po), min(255, ao))

# ---- straight, per-step unassociated quantization ----
def over_st_step(P, B):
    cA, aa = P
    cB, ab = B
    ao = (aa * 255 + (255 - aa) * ab) // 255
    if ao == 0:
        return (0, 0)
    # energy (color in premult units, 1/255 scale):
    # e = (aa*cA + (255-aa)*ab*cB/255) / 255 ; color = e/ao*255
    e_num = rdiv(aa * cA * 255 + (255 - aa) * ab * cB, 255)
    co = rdiv(e_num * 255, ao * 255)
    return (min(255, co), min(255, ao))

def pair(over, A, B, C):
    L = over(over(A, B), C)
    R = over(A, over(B, C))
    return L, R

def to_pm(p):
    c, a = p
    return (rdiv(c * a, 255), a)

alpha_sets = [(80, 160, 120), (40, 200, 100), (120, 90, 170)]

def run_pm_acc(A, B, C):
    # pixels carry (premult color float 0..1, alpha float 0..1)
    conv = lambda p: (p[0]/255 * p[1]/255, p[1]/255)
    Ap, Bp, Cp = conv(A), conv(B), conv(C)
    L = over_pm_acc(over_pm_acc(Ap, Bp), Cp)
    R = over_pm_acc(Ap, over_pm_acc(Bp, Cp))
    def fin(p):
        pc, aa = p
        if aa <= 0:
            return (0, 0)
        return (q8(pc / aa * 255), q8(aa * 255))
    return fin(L), fin(R)

def search(aa, ab, ac):
    n_st = n_pmstep = n_pmacc = 0
    ex_st, ex_pmstep, ex_pmacc = [], [], []
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
                La, Ra = run_pm_acc(A, B, C)
                if La != Ra:
                    n_pmacc += 1
                    if len(ex_pmacc) < 3: ex_pmacc.append((cA, cB, cC, La, Ra))
    return (n_st, ex_st), (n_pmstep, ex_pmstep), (n_pmacc, ex_pmacc)

def main():
 for ai, (aa, ab, ac) in enumerate(alpha_sets):
    (ns, es), (np_, ep), (na, ea) = search(aa, ab, ac)
    print(f"set {ai} a=({aa},{ab},{ac}): ST-step fails={ns} PM-step fails={np_} PM-acc fails={na} / 16777216")
    for e in es[:3]:
        print("   ST  colors=", e[:3], "L=", e[3], "R=", e[4], "dC=", abs(e[3][0]-e[4][0]))
    for e in ep[:2]:
        print("   PMs colors=", e[:3], "L=", e[3], "R=", e[4], "dC=", abs(e[3][0]-e[4][0]))
    for e in ea[:2]:
        print("   PMa colors=", e[:3], "L=", e[3], "R=", e[4])

if __name__ == "__main__":
    main()
