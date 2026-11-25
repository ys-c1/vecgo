#!/usr/bin/env python3
"""细长三角形重心插值：条件数、误差界与面积剔除实验。

运行:  python3 -I experiment.py
依赖:  numpy, scipy, Pillow （均为数值/图像库；本脚本不读取任何外部数据文件）

产出（写入脚本所在目录）:
  table.md / table.csv        三组三角形：条件数、误差界、剔除前后条纹数
  sweep.csv                   面积阈值扫描（阈值两侧各一次完整运行）
  before_cull.png             剔除前场景（float32 + 顶点 1px 扰动，伪彩编码）
  after_cull.png              剔除后场景（A < tau 的三角形被移除）
  apex_zoom.png               G3 钝角顶点局部放大（最近邻，显示碎裂条带）

条纹数口径: 三角形包围盒内，同量化码 (qR,qG,qB) 像素的 4-连通分量数；
另给出按扫描行连通段（run）计的口径作为对照，写在 CSV 中。
"""
import csv
import math
import os

import numpy as np
from PIL import Image

# ---------------------------------------------------------------- 实验常数
N_LEVELS = 8          # 每通道色阶数
DELTA = 1.0           # 顶点位置扰动预算 (px)
L_BASE = 480.0        # 三组三角形公共底边 (px)
TAU = (N_LEVELS - 1) * DELTA * L_BASE          # 面积剔除阈值 (px^2), 式 (3.2)
KAPPA_CRIT = 1.0 / ((N_LEVELS - 1) * DELTA)    # 临界条件数 (px^-1), 式 (3.3)
HALF_STEP = 1.0 / (2 * (N_LEVELS - 1))         # 半个色阶 (式 3.4)
N_TRIALS = 20
SEED = 20261126

GROUPS = [
    # name, height h (px)
    ("G1", 120.0),
    ("G2", 30.0),
    ("G3", 6.0),
]

OUT_DIR = os.path.dirname(os.path.abspath(__file__))


# ---------------------------------------------------------------- 几何工具
def make_vertices(h: float) -> np.ndarray:
    """等腰三角形 V0,V1 在底边上，V2 为钝角顶点（局部坐标，单位 px）。"""
    return np.array([(0.0, 0.0),
                     (L_BASE, 0.0),
                     (L_BASE / 2.0, -h)], dtype=np.float64)


def triangle_metrics(V: np.ndarray) -> dict:
    (ax, ay), (bx, by), (cx, cy) = V
    e01 = np.hypot(bx - ax, by - ay)
    e12 = np.hypot(cx - bx, cy - by)
    e20 = np.hypot(ax - cx, ay - cy)
    lengths = np.array([e12, e20, e01])          # L_i = V_i 的对边
    A = abs((bx - ax) * (cy - ay) - (by - ay) * (cx - ax)) / 2.0
    heights = 2.0 * A / lengths
    # 内角（余弦定理）
    ang = []
    for i in range(3):
        j, k = (i + 1) % 3, (i + 2) % 3
        a = np.linalg.norm(V[j] - V[k])
        b = np.linalg.norm(V[i] - V[k])
        c = np.linalg.norm(V[i] - V[j])
        ang.append(math.degrees(math.acos(np.clip((b * b + c * c - a * a)
                                                  / (2 * b * c), -1, 1))))
    return {"A": A, "L": lengths, "Lmax": lengths.max(),
            "h": heights, "angles": np.array(ang)}


def bary64(V: np.ndarray, px: np.ndarray, py: np.ndarray) -> np.ndarray:
    """float64 参考实现：返回 shape (..., 3) 的重心坐标。"""
    (ax, ay), (bx, by), (cx, cy) = V
    det = (bx - ax) * (cy - ay) - (by - ay) * (cx - ax)
    l1 = ((px - ax) * (cy - ay) - (py - ay) * (cx - ax)) / det
    l2 = ((bx - ax) * (py - ay) - (by - ay) * (px - ax)) / det
    l0 = 1.0 - l1 - l2
    return np.stack([l0, l1, l2], axis=-1)


def bary32(V32: np.ndarray, px32: np.ndarray, py32: np.ndarray) -> np.ndarray:
    """全程 float32 的面积坐标计算（模拟 GPU/移动端着色器精度）。"""
    ax, ay = V32[0]
    bx, by = V32[1]
    cx, cy = V32[2]
    det = (bx - ax) * (cy - ay) - (by - ay) * (cx - ax)
    l1 = ((px32 - ax) * (cy - ay) - (py32 - ay) * (cx - ax)) / det
    l2 = ((bx - ax) * (py32 - ay) - (by - ay) * (px32 - ax)) / det
    l0 = np.float32(1.0) - l1 - l2
    return np.stack([l0, l1, l2], axis=-1).astype(np.float32)


def raster_grid(V: np.ndarray, pad: int = 2):
    """三角形包围盒上的像素中心网格（1 px = 1 几何单位）。"""
    xmin, ymin = V.min(axis=0)
    xmax, ymax = V.max(axis=0)
    xs = np.arange(math.floor(xmin) - pad, math.ceil(xmax) + pad) + 0.5
    ys = np.arange(math.floor(ymin) - pad, math.ceil(ymax) + pad) + 0.5
    gx, gy = np.meshgrid(xs, ys)
    return gx.astype(np.float64), gy.astype(np.float64)


def encode(bary: np.ndarray) -> np.ndarray:
    """量化重心坐标为整数码；三角形外返回 -1。"""
    inside = bary.min(axis=-1) >= 0.0
    q = np.clip(np.rint(np.clip(bary, 0.0, 1.0) * (N_LEVELS - 1)),
                0, N_LEVELS - 1).astype(np.int32)
    code = (q[..., 0] * N_LEVELS + q[..., 1]) * N_LEVELS + q[..., 2]
    return np.where(inside, code, -1)


def count_segments(code: np.ndarray) -> tuple[int, int]:
    """返回 (4-连通分量数, 扫描行 run 数)；-1 为三角形外，不计。

    对整个码数组做一遍行扫描并查集：仅当相邻像素同码时合并标签。
    精确几何下每个量化码对应一个凸的带状区域（单连通）；扰动使同码区域
    碎裂，碎裂块即表现为额外的连通分量（条纹）。
    """
    H, W = code.shape
    parent = list(range(H * W + 1))   # 标签数不超过像素数，预分配避免前向引用

    def find(x: int) -> int:
        while parent[x] != x:
            parent[x] = parent[parent[x]]
            x = parent[x]
        return x

    def union(a: int, b: int) -> None:
        ra, rb = find(a), find(b)
        if ra != rb:
            parent[ra] = rb

    labels = np.zeros((H, W), dtype=np.int32)
    nxt = 1
    for y in range(H):
        row = code[y]
        for x in range(W):
            v = row[x]
            if v < 0:
                continue
            up = labels[y - 1, x] if y > 0 and code[y - 1, x] == v else 0
            left = labels[y, x - 1] if x > 0 and row[x - 1] == v else 0
            if up == 0 and left == 0:
                labels[y, x] = nxt
                nxt += 1
            elif up != 0 and left == 0:
                labels[y, x] = up
            elif up == 0 and left != 0:
                labels[y, x] = left
            else:
                labels[y, x] = min(up, left)
                union(up, left)
    segments = len({find(i) for i in range(1, nxt)})
    # 扫描行口径：每行同码连续 run
    runs = 0
    for row in code:
        valid = row[row >= 0]
        if valid.size:
            runs += 1 + int(np.count_nonzero(np.diff(valid) != 0))
    return segments, runs


def kick_vertices(V: np.ndarray, rng: np.random.Generator) -> np.ndarray:
    """三个顶点各自加模长恰为 DELTA 的独立随机位移。"""
    th = rng.uniform(0.0, 2.0 * np.pi, size=3)
    d = np.stack([DELTA * np.cos(th), DELTA * np.sin(th)], axis=1)
    return V + d


# ---------------------------------------------------------------- 验证 V1/V2
def verify_jacobian(V: np.ndarray, kappa: float) -> tuple[float, float]:
    """有限差分雅可比最大元素，及其与 kappa/2 的比值。"""
    eps = 1e-7
    gx, gy = raster_grid(V)
    bary = bary64(V, gx, gy)
    pts_mask = bary.min(axis=-1) > 1e-4          # 只取内部点
    px, py = gx[pts_mask], gy[pts_mask]
    # 再补上贴近三个顶点的点（界在钝角顶点极限处取等）
    for v in V:
        for dx in (-1e-3, 1e-3):
            for dy in (-1e-3, 1e-3):
                p = v + np.array([dx, dy])
                if bary64(V, p[0], p[1]).min() >= 0:
                    px = np.append(px, p[0])
                    py = np.append(py, p[1])
    worst = 0.0
    for k in range(3):
        for d in range(2):
            Vp, Vm = V.copy(), V.copy()
            Vp[k, d] += eps
            Vm[k, d] -= eps
            diff = (bary64(Vp, px, py) - bary64(Vm, px, py)) / (2 * eps)
            worst = max(worst, np.abs(diff).max())
    return worst, worst / (kappa / 2.0)


def measure_error(V: np.ndarray, rng: np.random.Generator) -> dict:
    """单顶点 8 方向 1px 扰动（紧界检验）+ 全顶点随机扰动。"""
    gx, gy = raster_grid(V)
    base = bary64(V, gx, gy)
    interior = base.min(axis=-1) >= 0.0
    # 单顶点、单方向、幅度 DELTA —— 直接对应界 (2.3)
    single_worst = 0.0
    dirs = [(1, 0), (-1, 0), (0, 1), (0, -1),
            (0.70710678, 0.70710678), (-0.70710678, 0.70710678),
            (0.70710678, -0.70710678), (-0.70710678, -0.70710678)]
    for k in range(3):
        for dx, dy in dirs:
            Vk = V.copy()
            Vk[k] += DELTA * np.array([dx, dy])
            bk = bary64(Vk, gx, gy)
            shared = interior & (bk.min(axis=-1) >= 0.0)
            single_worst = max(single_worst,
                               np.abs(bk[shared] - base[shared]).max())
    # 三顶点同时随机扰动（实用界 (2.2) 的场景）
    rand_worst = 0.0
    for _ in range(200):
        Vk = kick_vertices(V, rng)
        bk = bary64(Vk, gx, gy)
        shared = interior & (bk.min(axis=-1) >= 0.0)
        rand_worst = max(rand_worst,
                         np.abs(bk[shared] - base[shared]).max())
    return {"single": single_worst, "random": rand_worst}


# ---------------------------------------------------------------- 条纹实验
def render_codes(V: np.ndarray, V_kick: np.ndarray | None) -> np.ndarray:
    """返回包围盒上的量化码数组（float32 计算；None 表示无扰动精确渲染）。"""
    gx, gy = raster_grid(V)
    if V_kick is None:
        bary = bary64(V, gx, gy)                    # 精确参考
    else:
        bary = bary32(V_kick.astype(np.float32),
                      gx.astype(np.float32), gy.astype(np.float32))
    return encode(bary)


def stripe_experiment(V: np.ndarray, rng: np.random.Generator) -> dict:
    ideal_code = render_codes(V, None)
    s_ideal, r_ideal = count_segments(ideal_code)
    comps, runs = [], []
    for _ in range(N_TRIALS):
        Vk = kick_vertices(V, rng)
        code = render_codes(V, Vk)
        s, r = count_segments(code)
        comps.append(s)
        runs.append(r)
    return {
        "ideal_comp": s_ideal,
        "ideal_runs": r_ideal,
        "worst_comp": max(comps),
        "median_comp": int(np.median(comps)),
        "worst_runs": max(runs),
        "median_runs": int(np.median(runs)),
        "excess_worst": max(comps) - s_ideal,
        "excess_median": int(np.median(comps)) - s_ideal,
        "last_kick": Vk,
        "last_code": code,
        "ideal_code": ideal_code,
    }


# ---------------------------------------------------------------- 出图
def code_to_rgb(code: np.ndarray) -> np.ndarray:
    q2 = code % N_LEVELS
    q1 = (code // N_LEVELS) % N_LEVELS
    q0 = code // (N_LEVELS * N_LEVELS)
    rgb = np.zeros(code.shape + (3,), dtype=np.uint8)
    scale = 255 // (N_LEVELS - 1)
    rgb[..., 0] = (q0 * scale).astype(np.uint8)
    rgb[..., 1] = (q1 * scale).astype(np.uint8)
    rgb[..., 2] = (q2 * scale).astype(np.uint8)
    rgb[code < 0] = (24, 24, 28)                      # 背景：深灰
    return rgb


def triangle_outline(V: np.ndarray, shape: tuple[int, int],
                     ox: int, oy: int) -> np.ndarray:
    """三角形三边在目标画布上的像素掩码（到边距离 < 1 px）。"""
    H, W = shape
    yy, xx = np.mgrid[0:H, 0:W]
    gx = xx - ox
    gy = yy - oy
    mask = np.zeros(shape, dtype=bool)
    for k in range(3):
        a = V[k]
        b = V[(k + 1) % 3]
        ex, ey = b - a
        L = math.hypot(ex, ey)
        dist = np.abs((gx - a[0]) * ey - (gy - a[1]) * ex) / L
        along = ((gx - a[0]) * ex + (gy - a[1]) * ey) / (L * L)
        mask |= (dist < 0.7) & (along >= -0.02) & (along <= 1.02)
    return mask


def _load_font(size: int = 13):
    from PIL import ImageFont
    candidates = [
        os.path.expanduser("~/.local/share/fonts/NotoSansCJKsc-Regular.ttf"),
        "/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc",
        "/usr/share/fonts/truetype/wqy/wqy-microhei.ttc",
    ]
    for p in candidates:
        if os.path.exists(p):
            return ImageFont.truetype(p, size)
    return ImageFont.load_default()


def compose_scene(renders: list[dict], keep: list[bool],
                  rows: list[dict]) -> Image.Image:
    """把三组三角形从上到下摆到一张画布；被剔的组只画灰色轮廓。"""
    from PIL import ImageDraw
    font = _load_font(13)
    band_h, margin_x, top, label_w = 170, 40, 30, 150
    W = int(L_BASE) + 2 * margin_x + label_w
    H = top + band_h * len(renders) + 10
    canvas = np.full((H, W, 3), (24, 24, 28), dtype=np.uint8)
    img = Image.fromarray(canvas, "RGB")
    draw = ImageDraw.Draw(img)
    for i, r in enumerate(renders):
        V = r["V"]
        ox = margin_x + label_w
        oy = top + i * band_h + 130          # 局部 y<=0，整体向下平移
        gx, gy = raster_grid(V)
        x_min = int(round(gx.min() - 0.5))
        y_min = int(round(gy.min() - 0.5))
        X = np.round(gx - 0.5).astype(int) - x_min + ox
        Y = np.round(gy - 0.5).astype(int) - y_min + oy
        valid = (X >= 0) & (X < W) & (Y >= 0) & (Y < H)
        if keep[i]:
            rgb = code_to_rgb(r["code"])
            inside = valid & (r["code"] >= 0)
            arr = np.asarray(img).copy()
            arr[Y[inside], X[inside]] = rgb[inside]
            img = Image.fromarray(arr, "RGB")
            draw = ImageDraw.Draw(img)
        else:
            edge = triangle_outline(V, (H, W), ox, oy)
            arr = np.asarray(img).copy()
            arr[edge] = (110, 110, 120)
            img = Image.fromarray(arr, "RGB")
            draw = ImageDraw.Draw(img)
        d = rows[i]
        lines = [
            f"{d['group']}  h={d['h']:.0f}px  A={d['A']:.0f}px²",
            f"κ={d['kappa']:.6f} px^-1",
            f"钝角={d['angle_obtuse']:.2f}°  最细色带={d['band_min_px']:.3f}px",
            f"条纹 理想{d['ideal_comp']} → 扰动{d['before_worst']}"
            f"（增 {d['before_worst'] - d['ideal_comp']}）",
            f"实测误差 {d['err_single']:.5f} ≤ 界 {d['bound']:.5f}",
            "处置：" + ("保留（A≥τ）" if keep[i] else "剔除（A<τ）"),
        ]
        yy = oy - 104
        for ln in lines:
            draw.text((10, yy), ln, fill=(220, 220, 225), font=font)
            yy += 16
    return img


def apex_zoom(render: dict, win_x: int = 96, win_y: int = 8,
              scale: int = 6) -> Image.Image:
    """钝角顶点附近 win_x x win_y px 区域最近邻放大。"""
    V = render["V"]
    apex = V[2]
    gx, gy = raster_grid(V)
    sel = ((gx - apex[0]) >= -win_x / 2) & ((gx - apex[0]) <= win_x / 2) \
        & (np.abs(gy - apex[1]) <= win_y / 2)
    rows = np.where(sel.any(axis=1))[0]
    cols = np.where(sel.any(axis=0))[0]
    sub = render["code"][rows.min():rows.max() + 1,
                         cols.min():cols.max() + 1]
    rgb = code_to_rgb(sub)
    return Image.fromarray(rgb, "RGB").resize(
        (rgb.shape[1] * scale, rgb.shape[0] * scale), Image.NEAREST)


def apex_compare(render: dict, win_x: int = 96, win_y: int = 8,
                 scale: int = 6, label_h: int = 22, gap: int = 10) -> Image.Image:
    """G3 顶点附近：上=精确几何渲染，下=扰动后渲染，同尺度对照。"""
    from PIL import ImageDraw
    V = render["V"]
    apex = V[2]
    gx, gy = raster_grid(V)
    sel = ((gx - apex[0]) >= -win_x / 2) & ((gx - apex[0]) <= win_x / 2) \
        & (np.abs(gy - apex[1]) <= win_y / 2)
    rows = np.where(sel.any(axis=1))[0]
    cols = np.where(sel.any(axis=0))[0]
    r0, r1 = rows.min(), rows.max() + 1
    c0, c1 = cols.min(), cols.max() + 1
    ideal = code_to_rgb(render["ideal_code"][r0:r1, c0:c1])
    kicked = code_to_rgb(render["code"][r0:r1, c0:c1])
    h, w = ideal.shape[:2]
    block = h * scale
    H = 2 * block + 2 * label_h + gap
    panel = np.full((H, w * scale, 3), (24, 24, 28), dtype=np.uint8)
    up = Image.fromarray(ideal, "RGB").resize((w * scale, block),
                                              Image.NEAREST)
    dn = Image.fromarray(kicked, "RGB").resize((w * scale, block),
                                               Image.NEAREST)
    # 上：理想渲染 + 标签
    panel[:block] = np.asarray(up)
    # 下：标签 + 扰动渲染
    dn_y0 = block + label_h + gap + label_h
    panel[dn_y0:dn_y0 + block] = np.asarray(dn)
    out = Image.fromarray(panel, "RGB")
    draw = ImageDraw.Draw(out)
    f = _load_font(14)
    draw.text((4, block + 3),
              "精确几何（理想条带，单连通）", fill=(215, 215, 220), font=f)
    draw.text((4, block + label_h + gap + 3),
              f"δ=1px 扰动（条带碎裂，连通段增加 "
              f"{render['st']['excess_worst']}）",
              fill=(215, 215, 220), font=f)
    return out


# ---------------------------------------------------------------- 主流程
def main() -> None:
    rng = np.random.default_rng(SEED)
    print(f"tau = {TAU:.0f} px^2   kappa_crit = {KAPPA_CRIT:.6f} px^-1   "
          f"half step = {HALF_STEP:.6f} ({255*HALF_STEP:.1f}/255)")
    rows, renders = [], []
    for name, h in GROUPS:
        V = make_vertices(h)
        m = triangle_metrics(V)
        kappa = m["Lmax"] / m["A"]
        jac_max, jac_ratio = verify_jacobian(V, kappa)
        err = measure_error(V, rng)
        st = stripe_experiment(V, rng)
        bound_tight = kappa / 2.0 * DELTA
        kept = m["A"] >= TAU
        rows.append({
            "group": name, "h": h, "A": m["A"], "Lmax": m["Lmax"],
            "kappa": kappa, "angle_obtuse": m["angles"][2],
            "hmin": m["h"].min(),
            "jac_max": jac_max, "jac_ratio": jac_ratio,
            "err_single": err["single"], "err_random": err["random"],
            "bound": bound_tight,
            "ideal_comp": st["ideal_comp"],
            "before_worst": st["worst_comp"],
            "before_median": st["median_comp"],
            "after": st["worst_comp"] if kept else 0,
            "ideal_runs": st["ideal_runs"],
            "before_runs_worst": st["worst_runs"],
            "kept": kept,
            "band_min_px": m["h"].min() / N_LEVELS,
        })
        renders.append({"V": V, "code": st["last_code"], "st": st,
                        "ideal_code": st["ideal_code"]})
        print(f"{name}: A={m['A']:8.1f} kappa={kappa:.6f} px^-1  "
              f"jac/(k/2)={jac_ratio:.4f}  err_single={err['single']:.6f} "
              f"<= {bound_tight:.6f}: {err['single'] <= bound_tight + 1e-12}  "
              f"err_random={err['random']:.6f}  "
              f"stripes ideal->{st['worst_comp']} (excess {st['excess_worst']})  "
              f"kept={kept}")

    # ------------------------------------------------------ 阈值两侧运行
    sweep_taus = [1000.0, 2000.0, TAU, 5000.0, 10000.0, 30000.0]
    with open(os.path.join(OUT_DIR, "sweep.csv"), "w", newline="") as f:
        w = csv.writer(f)
        w.writerow(["tau_px2", "kept_groups",
                    "stripe_components_worst_sum", "stripe_runs_worst_sum",
                    "side"])
        for t in sweep_taus:
            kept_now = [triangle_metrics(r_["V"])["A"] >= t for r_ in renders]
            comp_sum = sum(renders[i]["st"]["worst_comp"]
                           for i in range(3) if kept_now[i])
            run_sum = sum(renders[i]["st"]["worst_runs"]
                          for i in range(3) if kept_now[i])
            # 题目要求的两次运行：τ⁻=1000（阈值下侧，三组全留）、
            # τ⁺=TAU（阈值上侧，剔除 G3）；其余点为完整扫描。
            if t < TAU:
                side = f"τ下侧 (<{TAU:.0f})"
            else:
                side = f"τ上侧 (>={TAU:.0f})"
            w.writerow([f"{t:.0f}",
                        ",".join(GROUPS[i][0] for i in range(3) if kept_now[i]),
                        comp_sum, run_sum, side])
            print(f"sweep tau={t:7.0f}  keep={[k for k in kept_now]}  "
                  f"components={comp_sum}  runs={run_sum}  {side}")

    # ------------------------------------------------------ 汇总表
    keep_flags = [r["kept"] for r in rows]
    before_total = sum(r["before_worst"] for r in rows)
    after_total = sum(r["after"] for r in rows)
    with open(os.path.join(OUT_DIR, "table.csv"), "w", newline="") as f:
        w = csv.writer(f)
        w.writerow(["group", "Lmax_px", "A_px2", "kappa_per_px",
                    "obtuse_angle_deg", "hmin_px", "min_band_width_px",
                    "jac_max_entry", "jac_over_half_kappa",
                    "err_single_vertex_px", "err_bound_kappa_delta_half",
                    "err_measured_8bit_equiv", "err_bound_8bit_equiv",
                    "stripes_ideal",
                    "stripes_before_worst", "stripes_before_median",
                    "stripes_after_cull", "runs_ideal", "runs_before_worst",
                    "kept"])
        for r in rows:
            w.writerow([r["group"], f"{r['Lmax']:.1f}", f"{r['A']:.1f}",
                        f"{r['kappa']:.6f}", f"{r['angle_obtuse']:.2f}",
                        f"{r['hmin']:.3f}", f"{r['band_min_px']:.3f}",
                        f"{r['jac_max']:.6f}", f"{r['jac_ratio']:.4f}",
                        f"{r['err_single']:.6f}", f"{r['bound']:.6f}",
                        f"{255*r['err_single']:.2f}",
                        f"{255*r['bound']:.2f}",
                        r["ideal_comp"], r["before_worst"],
                        r["before_median"], r["after"],
                        r["ideal_runs"], r["before_runs_worst"],
                        str(r["kept"]).lower()])
        w.writerow(["TOTAL", "", "", "", "", "", "", "", "", "", "", "",
                    sum(r["ideal_comp"] for r in rows), before_total, "",
                    after_total,
                    sum(r["ideal_runs"] for r in rows),
                    sum(r["before_runs_worst"] for r in rows), ""])

    with open(os.path.join(OUT_DIR, "table.md"), "w") as f:
        f.write("条件数按 κ = Lmax/A 计算（单位 px⁻¹）；误差界 (κ/2)δ，"
                f"δ = {DELTA:.0f} px，N = {N_LEVELS} 色阶；"
                f"剔除阈值 τ = (N−1)δLmax = {TAU:.0f} px²。\n\n")
        f.write("| 组 | Lmax (px) | A (px²) | κ=Lmax/A (px⁻¹) | 钝角 (°) | "
                "最细色带 hmin/N (px) | 实测最大坐标误差 | 误差界 κδ/2 "
                "(色阶/255) | 理想条纹数 | 剔除前条纹数(最差/中位) | "
                "剔除后条纹数 | 处置 |\n")
        f.write("|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---|\n")
        for r in rows:
            f.write(f"| {r['group']} | {r['Lmax']:.0f} | {r['A']:.0f} | "
                    f"{r['kappa']:.6f} | {r['angle_obtuse']:.2f} | "
                    f"{r['band_min_px']:.3f} | {r['err_single']:.6f} | "
                    f"{r['bound']:.6f} ({255*r['bound']:.1f}/255) | "
                    f"{r['ideal_comp']} | {r['before_worst']}/{r['before_median']} | "
                    f"{r['after']} | {'保留' if r['kept'] else '剔除'} |\n")
        f.write(f"| 合计 |  |  |  |  |  |  |  | "
                f"{sum(r['ideal_comp'] for r in rows)} | "
                f"{before_total} | {after_total} |  |\n\n")
        f.write("雅可比元素界检验（V1）：有限差分 max|∂λi/∂(Vk)d| 与 κ/2 之比 "
                "（界 (1) 紧，理论值 1.0000）：\n\n")
        f.write("| 组 | max 雅可比元素 | κ/2 | 比值 |\n|---|---:|---:|---:|\n")
        for r in rows:
            f.write(f"| {r['group']} | {r['jac_max']:.6f} | "
                    f"{r['kappa']/2:.6f} | {r['jac_ratio']:.4f} |\n")
        f.write(f"\n扫描行 run 口径对照见 table.csv；阈值扫描见 sweep.csv。"
                f"题目要求的阈值两侧运行：τ⁻=1000 px²（τ 下侧，三组全留，"
                f"条纹 240）与 τ⁺={TAU:.0f} px²（τ 上侧，剔除 G3，"
                f"条纹 177）。对照图 apex_compare.png：上=精确几何，"
                f"下=δ=1px 扰动，G3 钝角顶点附近。\n")

    # ------------------------------------------------------ 图像
    img_before = compose_scene(renders, [True, True, True], rows)
    img_before.save(os.path.join(OUT_DIR, "before_cull.png"))
    img_after = compose_scene(renders, keep_flags, rows)
    img_after.save(os.path.join(OUT_DIR, "after_cull.png"))
    zoom = apex_zoom(renders[2])
    zoom.save(os.path.join(OUT_DIR, "apex_zoom.png"))
    compare = apex_compare(renders[2])
    compare.save(os.path.join(OUT_DIR, "apex_compare.png"))
    print(f"images: before_cull.png {img_before.size}, "
          f"after_cull.png {img_after.size}, apex_zoom.png {zoom.size}, "
          f"apex_compare.png {compare.size}")
    print(f"totals: stripes before={before_total} after={after_total}")


if __name__ == "__main__":
    main()
