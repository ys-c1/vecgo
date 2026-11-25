# Brute-force search (pure integer 8-bit arithmetic) for associativity failures.
# Two pipelines:
#   premult (integer Porter-Duff):
#       ao = (aa*255 + (255-aa)*ab) / 255
#       po = (pa*255 + (255-aa)*pb) / 255
#   straight (naive, per-step divide back to unassociated color):
#       ao as above; co = round(255 * (aa*cA + (255-aa)*ab*cB/255) / ao)
N = 255

def rdiv(x, d):  # round-to-nearest integer division
    if x >= 0:
        return (x + d // 2) // d
    return -((-x + d // 2) // d)

def over_pm(P, B):
    pa, aa = P
    pb, ab = B
    ao = (aa * 255 + (255 - aa) * ab) // 255
    po = rdiv(pa * 255 + (255 - aa) * pb, 255)
    return (min(255, po), min(255, ao))

def over_st(P, B):
    cA, aa = P
    cB, ab = B
    ao = (aa * 255 + (255 - aa) * ab) // 255
    if ao == 0:
        return (0, 0)
    # energy aa*cA + (255-aa)*ab*cB/255, in 1/255 units
    energy = rdiv(aa * cA * 255 + (255 - aa) * ab * cB, 255)
    co = rdiv(energy * 255, ao)
    return (min(255, co), ao)

def both(over, A, B, C):
    L = over(over(A, B), C)
    R = over(A, over(B, C))
    return L, R

def to_pm(p):
    c, a = p
    return (rdiv(c * a, 255), a)

alpha_sets = [(80, 160, 120), (40, 200, 100), (120, 90, 170)]

# brute force every color triple
def search(aa, ab, ac):
    hits_st, hits_pm = [], []
    for cA in range(256):
        for cB in range(256):
            for cC in range(256):
                A, B, C = (cA, aa), (cB, ab), (cC, ac)
                Ls, Rs = both(over_st, A, B, C)
                if Ls != Rs:
                    hits_st.append((cA, cB, cC, Ls, Rs))
                Lp, Rp = both(over_pm, to_pm(A), to_pm(B), to_pm(C))
                if Lp != Rp:
                    hits_pm.append((cA, cB, cC, Lp, Rp))
    return hits_st, hits_pm

for ai, (aa, ab, ac) in enumerate(alpha_sets):
    hs, hp = search(aa, ab, ac)
    print(f"set {ai} a=({aa},{ab},{ac}): straight failures={len(hs)} premult failures={len(hp)}")
    for h in hs[:3]:
        print("   st counterexample colors=", h[:3], "L=", h[3], "R=", h[4],
              "dC=", abs(h[3][0]-h[4][0]))
