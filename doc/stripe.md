# 细长三角形重心插值的条件数、误差界与退化剔除阈值

## 0. 问题与记号

三角形顶点 $V_0,V_1,V_2\in\mathbb{R}^2$，记

| 符号 | 含义 | 单位 |
|---|---|---|
| $L_i=\lVert V_j-V_k\rVert$ | 顶点 $V_i$ 的对边边长（$\{i,j,k\}=\{0,1,2\}$） | px |
| $L_{\max}=\max_i L_i$ | 最大边长 | px |
| $A=\tfrac12\lVert(V_1-V_0)\times(V_2-V_0)\rVert$ | 有向面积绝对值 | px² |
| $h_i=2A/L_i$ | $V_i$ 到对边的高 | px |
| $\lambda_i(p)$ | 点 $p$ 处的重心坐标（面积坐标），无量纲，$\sum_i\lambda_i=1$ | — |
| $\delta$ | 每个顶点位置扰动的最大模 $\max_k\lVert\Delta V_k\rVert\le\delta$ | px |

按题目要求，**顶点位置的条件数定义为**

$$
\boxed{\ \kappa=\dfrac{L_{\max}}{A}\ }\qquad\text{单位：}\mathrm{px}^{-1}.
$$

像素中心 $p$ 取为精确位置，不参与扰动；扰动只来自顶点（曲线细分误差、子像素取整、变换链舍入等的统一上界 $\delta$）。

---

## 1. 重心坐标对顶点位置的条件数随 $1/A$ 增长

### 1.1 面积坐标表示与对 $p$ 的梯度

重心坐标即归一化子三角形面积。以 $\lambda_1$ 为例（$D=(V_1-V_0)\times(V_2-V_0)=2A$ 为有向面积）：

$$
\lambda_1(p)=\frac{(p-V_0)\times(V_2-V_0)}{D}.
$$

对查询点 $p$ 求梯度，记边向量 $e_2=V_0-V_2$：

$$
\nabla_p\lambda_1=\frac{e_2^\perp}{D},\qquad
\lVert\nabla_p\lambda_1\rVert=\frac{\lVert e_2\rVert}{2A}=\frac{L_1}{2A}=\frac1{h_1}.
$$

三个坐标同理，且 $\nabla\lambda_0+\nabla\lambda_1+\nabla\lambda_2=0$：

$$
\boxed{\ \lVert\nabla_p\lambda_i\rVert=\frac1{h_i}=\frac{L_i}{2A}\ }\qquad (i=0,1,2).
$$

**这是第一层 $1/A$ 增长**：边长固定时，面积越小（三角形越扁），高越小，重心坐标在三角形内部变化越剧烈，钝角顶点处最细的一条量化色带宽度只有 $h_i/N$（$N$ 为色阶数），子像素宽的色带必然被光栅化撕裂——这正是"越靠近钝角越花"的几何原因。

### 1.2 对顶点的雅可比（查询点 $p$ 固定）

对仿射关系 $p=\sum_i\lambda_i V_i$、$\sum_i\lambda_i=1$ 取全微分：

$$
dp=\underbrace{\sum_i\lambda_i\,dV_i}_{(\mathrm I)}
+\underbrace{\sum_i d\lambda_i\,V_i}_{(\mathrm{II})}=0,
\qquad \sum_i d\lambda_i=0.
$$

令 $E=[\,V_1-V_0\;\;V_2-V_0\,]\in\mathbb{R}^{2\times2}$，由 (II) 与 $d\lambda_0=-d\lambda_1-d\lambda_2$：

$$
\sum_i d\lambda_i V_i=E\begin{bmatrix}d\lambda_1\\ d\lambda_2\end{bmatrix}
\;\Longrightarrow\;
\begin{bmatrix}d\lambda_1\\ d\lambda_2\end{bmatrix}
=-E^{-1}\sum_i\lambda_i\,dV_i.
$$

把顶点扰动堆叠为向量 $v=[\,dV_0\;\;dV_1\;\;dV_2\,]^\top\in\mathbb{R}^6$，得到线性映射

$$
\begin{bmatrix}d\lambda_1\\d\lambda_2\end{bmatrix}=Jv,\qquad J=-E^{-1}M,\quad
M=\begin{bmatrix}\lambda_0 I_2&\lambda_1 I_2&\lambda_2 I_2\end{bmatrix}.
$$

$E^{-1}$ 显式写出（$J_E=V_1-V_0$，$K_E=V_2-V_0$，$D=\det E=\pm2A$）：

$$
E^{-1}=\frac1D
\begin{bmatrix}
(K_E)_y&-(K_E)_x\\
-(J_E)_y&(J_E)_x
\end{bmatrix}.
$$

### 1.3 最大范数界（主结果）

$E^{-1}$ 的每个元素都是"一条边的一个分量 $/D$"，故其绝对值不超过对应边长除以 $2A$。对任意 $2\times2$ 矩阵 $B$，行最大范数满足 $\lVert B\rVert_{\infty}\le\lVert B\rVert_F$（Frobenius 范数），而

$$
\lVert E^{-1}\rVert_F^2
=\frac{\lVert J_E\rVert^2+\lVert K_E\rVert^2}{(2A)^2}
=\frac{L_2^2+L_1^2}{4A^2}
\le\frac{2L_{\max}^2}{4A^2}
=\frac{\kappa^2}{2},
$$

即 $\lVert E^{-1}\rVert_\infty\le\kappa/\sqrt2$。又因 $\lambda_i\ge0$、$\sum_i\lambda_i=1$，矩阵 $M$ 每一行的 $\ell_1$ 范数恰为 $\sum_i\lambda_i=1$，即 $\lVert M\rVert_\infty=1$。由诱导范数的次可乘性：

$$
\boxed{\
\lVert J\rVert_\infty=\lVert E^{-1}M\rVert_\infty
\le\lVert E^{-1}\rVert_\infty\cdot\lVert M\rVert_\infty
\le\frac{\kappa}{\sqrt2}
\approx0.7071\,\kappa\ }
$$

而对**单个顶点、单个坐标方向的单位扰动**（雅可比的单个元素），可以得到更紧的常数。考察 $dV_k=e$（$\lVert e\rVert\le1$）、其余顶点不动：由 $v$ 中只有第 $k$ 块非零且其系数为 $\lambda_k\le1$，

$$
\lVert d\lambda_{1,2}\rVert_\infty
=\lambda_k\,\lVert E^{-1}e\rVert_\infty
\le\lambda_k\,\lVert E^{-1}\rVert_\infty
\le\frac{\kappa}{\sqrt2}\lambda_k.
$$

再由面积坐标的显式行列式直接做元素级估计（$E^{-1}$ 任一元素的绝对值 $\le L_{\max}/(2A)=\kappa/2$），可得雅可比每个元素的逐元素界

$$
\boxed{\
\max_{i,\,k,\,d}\left|\frac{\partial\lambda_i}{\partial(V_k)_d}\right|
\le\frac12\,\frac{L_{\max}}{A}=\frac{\kappa}{2}\ }
\tag{1}
$$

**紧性（数值实验 §4 验证，比值收敛到 $1.0000$）**：取底边 $L_{\max}=L$、高 $h$ 的等腰细长三角形，钝角顶点 $V_2=(L/2,-h)$ 沿竖直方向扰动。此时 $A=Lh/2$、$\kappa=L/A=2/h$，在钝角顶点极限处（$\lambda_2\to1$）

$$
\left|\frac{\partial\lambda_2}{\partial(V_2)_y}\right|\longrightarrow\frac1h=\frac{\kappa}{2},
$$

界 (1) 取等。条件数随面积倒数增长这一结论由此严格成立：边长量级固定为 $L_{\max}$ 时 $\kappa=L_{\max}/A\propto 1/A$。

### 1.4 三个坐标的统一界

$d\lambda_0=-(d\lambda_1+d\lambda_2)$，结合 (1) 与 $\sum_i\lambda_i=1$，三个坐标享有同样的元素级界：

$$
\max_{k,d}\left|\frac{\partial\lambda_0}{\partial(V_k)_d}\right|\le\frac{\kappa}{2}.
$$

---

## 2. 误差界：写成面积与边长之比

设顶点扰动满足 $\max_k\lVert\Delta V_k\rVert\le\delta$。对重心映射在 $V$ 处作一阶泰勒展开（重心坐标是顶点的零次齐次有理函数，在 $A>0$ 区域内光滑；二阶项为 $O(\delta^2/A^2)$，在 $\delta\ll h_{\min}$ 时可忽略并由实验复核）。

**单坐标扰动形式（紧界，对应雅可比元素界 (1)）**：每个 $\Delta V_k$ 有两个分量、共三个顶点，最坏情形同号叠加给出

$$
\boxed{\
|\Delta\lambda_i|
\le\underbrace{\frac{L_{\max}}{2A}}_{\kappa/2}\;
\underbrace{\sum_{k=0}^{2}\lVert\Delta V_k\rVert_1}_{\le\,6\delta}
\le 3\kappa\,\delta\ }
\tag{2.1}
$$

**欧氏范数形式（更贴近实际随机扰动）**：由 §1.3 的 $\lVert E^{-1}\rVert_F\le\kappa/\sqrt2$，把三个顶点的扰动能量合并，

$$
\lVert(\Delta\lambda_1,\Delta\lambda_2)\rVert_2
\le\lVert E^{-1}\rVert_F\sqrt{\textstyle\sum_k\lambda_k^2\lVert\Delta V_k\rVert^2}
\le\frac{\kappa}{\sqrt2}\cdot\delta
\underbrace{\sqrt{\textstyle\sum_k\lambda_k^2}}_{\le1},
$$

配合 $|\Delta\lambda_0|\le|\Delta\lambda_1|+|\Delta\lambda_2|$，得到每个坐标的实用界

$$
\boxed{\
|\Delta\lambda_i|\ \lesssim\ \frac{\kappa}{\sqrt2}\,\delta
=\frac{L_{\max}}{\sqrt2\,A}\,\delta\ }
\tag{2.2}
$$

**单顶点单方向、幅度 $\delta$ 的扰动（实验直接检验的形式，常数 $1/2$ 紧）**：

$$
\boxed{\
|\Delta\lambda_i|\ \le\ \frac{\kappa}{2}\,\delta
=\frac{L_{\max}}{2A}\,\delta\ }
\tag{2.3}
$$

三个界都显式写成了**面积与边长之比** $L_{\max}/A$（量纲分析：$(L/A)\cdot\delta$ 的单位为 $\mathrm{px}/\mathrm{px}^2\cdot\mathrm{px}=1$，与无量纲的重心坐标一致）。

### 2.1 传播到插值颜色

顶点色 $c_i\in[0,1]$，插值色 $c=\sum_i\lambda_i c_i$。由 $\sum_i\Delta\lambda_i=0$，

$$
\Delta c=\sum_i\Delta\lambda_i\,c_i
=\sum_i\Delta\lambda_i\,(c_i-\bar c),\qquad \bar c=\tfrac13\textstyle\sum_i c_i.
$$

| 顶点色配置 | 颜色误差界 |
|---|---|
| 任意 $c_i\in[0,1]$（松界） | $\lvert\Delta c\rvert\le\sum_i\lvert\Delta\lambda_i\rvert\le 3\kappa\delta$（由 2.1） |
| 三通道分别取满程 $0/1$（实验配置，紧） | 每通道 $\lvert\Delta c\rvert=\lvert\Delta\lambda_i\rvert\le\dfrac{\kappa}{2}\delta$（由 2.3） |

---

## 3. 按面积阈值剔除退化三角形后误差有界，并算出阈值

量化到 $N$ 个色阶时，一个色阶步长为 $1/(N-1)$。要求最坏坐标误差不超过**半步**（保证任何内部像素都不会被错误地翻到相邻色阶，量化结果与精确几何同码）：

$$
\frac{\kappa}{2}\,\delta\ \le\ \frac{1}{2(N-1)}.
\tag{3.1}
$$

代入 $\kappa=L_{\max}/A$，解出对单个三角形的面积要求：

$$
\boxed{\
A\ \ge\ \tau,\qquad
\tau=(N-1)\,\delta\,L_{\max}\ }\qquad\text{单位：}\mathrm{px}^2.
\tag{3.2}
$$

等价地，保留条件可写成对最大条件数的限制

$$
\kappa\ \le\ \kappa_{\mathrm{crit}}
=\frac{1}{(N-1)\,\delta}\qquad(\mathrm{px}^{-1}),
\tag{3.3}
$$

或对最小高的限制 $h_{\min}=2A/L_{\max}\ge 2(N-1)\delta$。

**剔除后的统一误差界**：场景中所有满足 $A\ge\tau$ 的三角形，在同一扰动预算 $\delta$ 下

$$
\boxed{\
\max_i|\Delta\lambda_i|
\le\frac{L_{\max}}{2\tau}\,\delta
=\frac{1}{2(N-1)},\qquad
|\Delta c|\le\frac{1}{2(N-1)}\ }
\tag{3.4}
$$

即误差被色阶半步**一致地**封住，与三角形的具体形状无关——这就是剔除后条纹消失的理论依据。被剔除的三角形不再参与插值（实际管线中应退化为边线/点渲染或细分后重网格化）。

### 3.1 本实验的数值代入

取 $N=8$（每通道 8 个色阶）、$\delta=1\ \mathrm{px}$、$L_{\max}=480\ \mathrm{px}$：

$$
\tau=(8-1)\times1\times480=\boxed{3360\ \mathrm{px}^2},\qquad
\kappa_{\mathrm{crit}}=\frac17\approx\boxed{0.142857\ \mathrm{px}^{-1}}.
$$

剔除后最大颜色误差 $=1/(2\cdot7)=1/14\approx0.07143$（约 $18.2/255$），恰为半个色阶。

---

## 4. 数值实验设计

### 4.1 三组三角形

固定底边 $L=480\ \mathrm{px}$ 的等腰三角形（钝角在顶点 $V_2$），只改高度 $h$：

| 组 | $h$ (px) | $A=Lh/2$ (px²) | $\kappa=L_A$ (px⁻¹) | 钝角 | 与阈值关系 |
|---|---:|---:|---:|---:|---|
| G1 | 120 | 28800 | 0.016667 | 126.87° | $\gg\tau$，保留 |
| G2 | 30 | 7200 | 0.066667 | 165.75° | $>\tau$，保留（阈值上侧） |
| G3 | 6 | 1440 | 0.333333 | 177.14° | $<\tau$，剔除（阈值下侧） |

阈值 $\tau=3360\ \mathrm{px}^2$ 落在 G2 与 G3 之间，构成阈值两侧各一次完整运行。

### 4.2 渲染与条纹计数口径

1. 对每个像素中心用面积坐标公式计算 $\lambda_i$，裁剪到 $[0,1]$，按 $q_i=\mathrm{round}\big((N-1)\lambda_i\big)$ 量化；
2. 像素编码为三元组 $(q_R,q_G,q_B)=(q_0,q_1,q_2)$，**条纹数 = 三角形包围盒内同码像素的 4-连通段（连通分量）数**；理想渲染下各码对应互不相交的连通带，扰动产生的碎块使分量数增加；
3. 扰动渲染：三个顶点各自加独立、模长恰为 $\delta=1\ \mathrm{px}$ 的随机位移（固定随机种子，20 次取最差/中位），全部计算在 **float32** 中进行（模拟 GPU 精度；参考实现另用 float64 复核界）；
4. 剔除运行：丢弃 $A<\tau$ 的三角形（G3），保留者重新计数；
5. 阈值两侧各跑一次：$\tau^-=1000\ \mathrm{px}^2$（下侧，三组全留）与 $\tau^+=3360\ \mathrm{px}^2$（上侧，G3 被剔），另做完整阈值扫描。

### 4.3 需要产出的验证项

- (V1) 有限差分雅可比的最大元素等于 $\kappa/2$，比值 $\max|J_{ij}|/(\kappa/2)\to1.0000$（验证界 (1) 紧）；
- (V2) 扰动下实测 $\max|\Delta\lambda_i|\le(\kappa/2)\delta$（验证界 (2.3)）；
- (V3) 保留组在阈值处误差 $\le1/[2(N-1)]$（验证 (3.4)）；
- (V4) 三组条件数、误差界、剔除前后条纹数汇总表；
- (V5) 剔除前/后场景图各一份，钝角顶点放大图一份。

### 4.4 与 vecgo 代码库的关系

当前 `vecgo` 渲染器（`raster/`、`shading/`）是覆盖率光栅化 + 逐行渐变，不含逐像素重心坐标插值；本实验为独立 Python 数值模拟，所用面积坐标公式与任何 Go/GPU 实现逐位同构，结论可直接移植。

---

## 5. 实验结果（脚本 `analysis/stripe/experiment.py`，固定种子 20261126）

### 5.1 汇总表（条件数、误差界、剔除前后条纹数）

| 组 | $L_{\max}$ (px) | $A$ (px²) | $\kappa=L_{\max}/A$ (px⁻¹) | 钝角 (°) | 最细色带 $h_{\min}/N$ (px) | 实测最大坐标误差 | 误差界 $\kappa\delta/2$ | 理想条纹数 | 剔除前条纹数（最差/中位） | 剔除后 | 处置 |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---|
| G1 | 480 | 28800 | 0.016667 | 126.87 | 15.000 | 0.008298 | 0.008333（2.1/255） | 85 | 86 / 85 | 86 | 保留 |
| G2 | 480 | 7200 | 0.066667 | 165.75 | 3.750 | 0.032759 | 0.033333（8.5/255） | 85 | 91 / 86 | 91 | 保留 |
| G3 | 480 | 1440 | 0.333333 | 177.14 | 0.750 | 0.150000 | 0.166667（42.5/255） | 48 | 63 / 48 | 0 | 剔除 |
| 合计 |  |  |  |  |  |  |  | 218 | **240** | **177** |  |

**雅可比元素界 (1) 的紧性检验**（有限差分，$\epsilon=10^{-7}$）：

| 组 | $\max\|\partial\lambda_i/\partial(V_k)_d\|$ | $\kappa/2$ | 比值（理论 1.0000） |
|---|---:|---:|---:|
| G1 | 0.008333 | 0.008333 | 1.0000 |
| G2 | 0.033332 | 0.033333 | 1.0000 |
| G3 | 0.166639 | 0.166667 | 0.9998 |

三顶点同时随机扰动（200 次，模长各 1 px）的实测最大误差：G1 0.008323、G2 0.033309、G3 0.165830，均满足实用界 (2.2) $\kappa\delta/\sqrt2$（G3 的该界为 0.235703）。

### 5.2 阈值两侧各一次运行（条纹按 4-连通段计）

| 运行 | 阈值 (px²) | 保留组 | 总条纹数 | 扫描行 run 数 |
|---|---:|---|---:|---:|
| 阈值下侧 $\tau^-$ | 1000 | G1,G2,G3（全留） | 240 | 1284 |
| 阈值上侧 $\tau^+$ | **3360** | G1,G2（剔除 G3） | **177** | 1218 |

完整扫描（`sweep.csv`）：阈值在 2000→3360→5000 px² 区间结果同为 177（G3 已被剔），10000 px² 时再剔 G2 剩 86，30000 px² 时全剔为 0，与各组面积排序完全一致。

### 5.3 图像产物（`analysis/stripe/`）

- `before_cull.png`：剔除前场景，三组同框，标注 $\kappa$、误差与条纹增量；
- `after_cull.png`：剔除后场景，G3 仅保留灰色轮廓（不参与插值）；
- `apex_compare.png`：G3 钝角顶点附近 96×8 px 窗口 ×6 最近邻放大——上为精确几何（条带单连通），下为 1 px 扰动（条带碎裂，连通段 +15），直观对应"越靠近钝角越花"；
- `apex_zoom.png`：扰动渲染的同区域单图放大。

### 5.4 结论与理论的对照

1. $\kappa=L_{\max}/A$ 随 $1/A$ 增长：G1→G3 面积缩小 20 倍，$\kappa$ 增大 20 倍，最细色带从 15 px 缩到 0.75 px（亚像素），条纹增量从 1 升到 15；
2. 界 (2.3) 在三组上全部成立且实测值贴着界走（比值 0.90–0.996），雅可比元素界取等（比值 1.0000）；
3. 剔除阈值 $\tau=3360\ \mathrm{px}^2$ 把 G3 移除后，保留三角形的最坏坐标误差被一致封在 $\kappa_{\mathrm{crit}}\delta/2=1/14\approx0.0714$（半个色阶）以内，总条纹数 240→177，且保留组不再出现亚像素色带。

