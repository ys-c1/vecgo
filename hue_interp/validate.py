"""Validate colorlab against colour-science and Sharma et al. (2005) test data."""
import sys, os
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import numpy as np
import colorlab as cl

import colour
from colour.models import sRGB_to_XYZ, XYZ_to_Lab, RGB_to_HSL, HSL_to_RGB

rng = np.random.default_rng(0)

# ---- 1. sRGB -> Lab round trip and cross check
rgb = rng.random((2000, 3))
mine = cl.srgb_to_lab(rgb)
ref = XYZ_to_Lab(sRGB_to_XYZ(rgb))
err_lab = np.abs(mine - ref).max()
print(f"sRGB->Lab max abs error vs colour-science: {err_lab:.3e}")

# round trip
back = cl.lab_to_srgb(mine)
print(f"Lab->sRGB roundtrip max error:            {np.abs(back-rgb).max():.3e}")

# ---- 2. CIEDE2000 cross check on random Lab pairs
lab1 = np.column_stack([rng.uniform(0,100,5000), rng.uniform(-128,128,(5000,2))])
lab2 = np.column_stack([rng.uniform(0,100,5000), rng.uniform(-128,128,(5000,2))])
d00_mine = cl.ciede2000(lab1, lab2)
d00_ref = colour.delta_E(lab1, lab2, method="CIE 2000")
print(f"CIEDE2000 max abs error vs colour:        {np.abs(d00_mine-d00_ref).max():.3e}")
print(f"CIEDE2000 max rel error vs colour:        {(np.abs(d00_mine-d00_ref)/np.maximum(d00_ref,1e-9)).max():.3e}")

# ---- 3. Sharma et al. 2005, Table 1 (first 7 rows, official expected DeltaE00)
sharma = [
    ((50,  2.6772,-79.7751),(50, 0.0000,-82.7485), 2.0425),
    ((50,  3.1571,-77.2803),(50, 0.0000,-82.7485), 2.8615),
    ((50,  2.8361,-74.0200),(50, 0.0000,-82.7485), 3.4412),
    ((50, -1.3802,-84.2814),(50, 0.0000,-82.7485), 1.0000),
    ((50, -1.1848,-84.8006),(50, 0.0000,-82.7485), 1.0000),
    ((50, -0.9009,-85.5211),(50, 0.0000,-82.7485), 1.0000),
    ((50,  0.0000,  0.0000),(50,-1.0000,  2.0000), 2.3669),
]
print("\nSharma 2005 spot check (ours / colour / paper):")
ok = True
for p1, p2, expect in sharma:
    a = cl.ciede2000(np.array(p1), np.array(p2))
    b = colour.delta_E(np.array(p1), np.array(p2), method="CIE 2000")
    good = abs(a-expect) < 0.0015 and abs(a-b) < 1e-9
    ok &= good
    print(f"  {a:8.4f} {b:8.4f} {expect:8.3f}  {'OK' if good else 'FAIL'}")

# ---- 4. HSL cross check
h = rng.uniform(0,360,500)
rgb_hsl_mine = cl.hsl_to_srgb(h, 1.0, 0.5)
hsl_ref = np.column_stack([h/360, np.ones(500), np.full(500,0.5)])
rgb_hsl_ref = HSL_to_RGB(hsl_ref)
print(f"\nHSL->sRGB max error vs colour:            {np.abs(rgb_hsl_mine-rgb_hsl_ref).max():.3e}")

print("\nALL CHECKS PASSED" if ok and err_lab < 1e-9 else "\nVALIDATION FAILED")
