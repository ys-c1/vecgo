# 分块裁剪的面积误差：证明、上界与三档实测

本文回答一个具体问题：一个多边形被裁剪窗切成若干块后，把各块面积加回来，
为什么与原面积只差一点点？误差从哪里来，上界是多少？

记号约定：

- 点 $p=(x_p,y_p)$，向量叉积 $a\times b=a_xb_y-a_yb_x$；
- 多边形 $P$ 的面积一律用鞋带公式 $A(P)=\tfrac12\left|\sum_i v_i\times v_{i+1}\right|$；
- float64（binary64）的单位舍入 $u=2^{-53}\approx1.11\times10^{-16}$，每次 +、−、×、÷ 的
  相对误差满足 $\operatorname{fl}(a\circ b)=(a\circ b)(1+\varepsilon)$，$|\varepsilon|\le u$；
- 三档裁剪指把方形域 $[0,10]^2$ 分成 $1\times1$、$2\times2$、$4\times4$ 个轴对齐窗口，
  对同一输入多边形分别做 Sutherland–Hodgman（以下简称 SH）裁剪。

---

## 1. 裁剪只保面积，不保顶点

### 1.1 单条半平面裁剪保持面积

SH 裁剪沿窗口的每条边 $a\to b$ 施加一次半平面裁剪，半平面内侧由叉积
$c(p)=e\times(p-a)\ge 0$ 判定，$e=b-a$。

考察输入多边形的一条有向边 $p\to q$，它与裁剪线只有三种关系：

1. **两端都在内侧**：输出顶点 $q$，对应原边整段保留；
2. **两端都在外侧**：无输出，对应原边整段舍弃；
3. **跨越裁剪线**：输出交点 $I$（进入时再跟一个 $q$，离开时只输出 $I$）。

把多边形看成由其有向边围成的区域。一条跨线边 $p\to q$ 在输出里被改写成
$p\to I$（舍弃段）与 $I\to q$（保留段）两条子边，而裁剪线本身被新增的
$I$ 之间的连线覆盖。由于相邻跨线边产生的交点共线，这些新增连线在鞋带求和中
两两配对、方向相反，贡献相互抵消。舍弃段与新增连线围成的部分恰好在半平面外侧，
保留段围成的部分恰好在内侧，二者沿裁剪线严丝合缝。因此

$$
A(\operatorname{Clip}_{H}(P)) = A(P\cap H).
$$

这是一个**恒等式**，与顶点数、交点位置、窗口边数都无关。

### 1.2 分块裁剪的面积守恒

矩形窗口是四条半平面的交，SH 依次施加四条半平面裁剪，由 1.1 逐层有
$A(\operatorname{Clip}_W(P))=A(P\cap W)$。

$k\times k$ 的窗口集合 $\{W_g\}$ 是方形域的一个剖分：内部互不重叠，并集为整个域。
输入多边形完全位于域内，故 $\{P\cap W_g\}$ 是 $P$ 的一个剖分。于是在精确算术下

$$
\boxed{\;\sum_g A(\operatorname{Clip}_{W_g}(P))=\sum_g A(P\cap W_g)=A(P).\;}
$$

**代码佐证（不是数值近似，而是严格相等）**：`cliparea/rational.go` 用 `math/big.Rat`
逐行镜像 `geom.Clip` 的代数过程做了一条零舍入的有理裁剪路径。测试
`TestExactClippingPreservesArea` 断言三档下“精确分块面积和 − 精确原面积”恒为 0，
程序输出的核对行三档全部为 `0.000e+00`。这就把“裁剪保面积”从几何论证落到了可执行事实。

### 1.3 裁剪不保顶点

面积是区域的度量，顶点只是边界的一种采样表示；同一个区域可以有不同的顶点序列。
SH 裁剪在跨线时用新算出的交点 $I$ 替换原边的一部分，原顶点可能整个消失，交点则成为
新顶点。本实验的输入六边形（6 个顶点）在三档下的顶点总数分别为

| 档 | 非空块数 | 顶点总数 $V$ |
|---|---|---|
| 1×1 | 1 | 6 |
| 2×2 | 4 | 18 |
| 4×4 | 16 | 66 |

1×1 档窗口就是整个域，不产生交点，顶点数保持 6；分块越细，交点越多，顶点总数
随之变化且不等于原顶点数。测试 `TestClipLosesVertices` 对此做了断言。
**结论：裁剪保持的是面积这一标量，从不承诺保持顶点集合。**

---

## 2. 误差来源被唯一限定到 float64 取整

面积差的候选来源只有三处，逐一排除：

1. **裁剪的几何过程**：第 1 节已证其在精确算术中严格保面积，有理路径三档差值恒为 0，
   排除；
2. **拓扑/内外判定**：判定只决定某条边走哪条分支，不参与任何数值加减；其唯一的数值风险
   是“点在线上”时的二义（第 5 节单独处理），且有理路径证明无论怎样判定，精确面积都不变；
3. **float64 浮点取整**：交点计算 `intersect` 与鞋带求和 `Shoelace` 中的每次四则运算都
   被舍入到最近的 binary64，这是仅剩的、且确实存在的误差源。

因此浮点面积差 $\Delta A=\big|\sum_g A_{fl}(P_g)-A(P)\big|$ 完全由取整造成。
下文给出它的严格一阶上界。

---

## 3. 取整误差的上界推导（系数算出）

设所有顶点坐标满足 $|x|,|y|\le M$（即 $\|v\|\le\sqrt2\,M$，本实验直接取
$M=\max_i\|v_i\|$），多边形一条边的方向向量 $d=q-p$，裁剪线方向 $e=b-a$，
$r=a-p$。交点公式（与 `geom.intersect` 代数等价的写法）为

$$
\operatorname{den}=d\times e,\qquad
\operatorname{num}=r\times e,\qquad
t=\frac{\operatorname{num}}{\operatorname{den}},\qquad
I=p+t\,d.
$$

### 3.1 单个交点的坐标误差半径

用标准一阶浮点误差模型（略去 $O(u^2)$ 项，$nu\ll1$ 时成立）：

- 叉积由两次乘法与一次减法构成：$|\delta(\operatorname{den})|\le 4u\,|d||e|$，
  $|\delta(\operatorname{num})|\le 8u\,|r||e|$（$r=a-p$ 含一次减法，取 $\|r\|\le\sqrt2 M$）；
- 除法：$|\delta t|\le u\,|t|+u\,\dfrac{|\operatorname{num}|}{|\operatorname{den}|}$，
  代入 $|t|\le1$（交点落在边段内）与 $|d||e|/|\operatorname{den}|=1/\sin\theta$，整理得

$$
\frac{|\delta t|}{|t|}\ \text{的主导项}\le
u\left(\frac{8|r|}{|d|}+\frac{4|\operatorname{num}|}{|\operatorname{den}|}\right);
$$

- 合成 $I=p+td$，再计入 $td$ 与加法的三次舍入（系数 3）：

$$
\boxed{\;\delta_I=\|I_{fl}-I_{ex}\|
\ \le\ |d|\,u\left(\frac{8\|r\|}{|d|}
+\frac{4|\operatorname{num}|}{|\operatorname{den}|}+3\right).\;}
$$

这正是 `localIntersect` 逐交点算出并累加的半径。**近线交点**（边与裁剪线夹角 $\theta$
很小）使 $|\operatorname{den}|=|d||e|\sin\theta$ 变小，$\delta_I$ 通过
$1/\sin\theta$ 项如实增大——误差界不需要假设“不接近平行”，只要求确有跨线
（此时精确 $\operatorname{den}\ne0$）。

### 3.2 误差在裁剪轮次间的传播

矩形窗口共 $r=4$ 轮半平面裁剪。被保留的原顶点误差半径不变；新交点继承上游顶点半径，
再加本轮的 $\delta_I$（每次坐标运算的 $(1+O(u))$ 放大在一阶模型下并入 $\delta_I$）。
故任一输出顶点的误差半径不超过 $r\,\delta_{\max}$，其中

$$
\delta_{\max}=\max_I\delta_I
\le L_{\max}u\left(\frac{8\sqrt2\,M}{L_{\min}}+\frac{4}{s_0}+3\right),
$$

$L_{\max},L_{\min}$ 为多边形最长、最短边，$s_0=\min\sin\theta$ 只在**实际跨线交点**上取
（平行边不产生交点，不参与，故不存在除零）。

### 3.3 从顶点误差到面积误差

对一块含 $n_g$ 个顶点的裁剪块 $P_g$，鞋带公式的被加项是 $f_i=v_i\times v_{i+1}$。
设 $\|v_i-v_i^{ex}\|\le\varepsilon_i$，则

$$
|\delta f_i|
\le \|v_i\|\,\varepsilon_{i+1}+\|v_{i+1}\|\,\varepsilon_i+4uM_g^2,
$$

末项是 $f_i$ 自身两次乘法、一次减法的求值舍入。对 $i$ 求和，每个 $\varepsilon_i$
恰好出现两次且 $\|v_i\|\le M\le\sqrt2 M$，配合求和本身的 $(n_g-1)u$ 项，得到

$$
|\delta A(P_g)|\ \le\ M\sum_{i=1}^{n_g}\varepsilon_i+2u\,n_g M_g^2 .
$$

对所有块求和。记 $V=\sum_g n_g$ 为顶点总数，每顶点半径不超过 $r\delta_{\max}$，
$M_g\le\sqrt2 M$，则

$$
\boxed{\;
|\Delta A|\ \le\
\underbrace{\bigl(M\,r\,\delta_{\max}+2uM^2\bigr)}_{C}\cdot V\;}
$$

**这就是与顶点数成正比的误差上界，比例系数**

$$
\boxed{\;C=M\,r\,\delta_{\max}+2uM^2,\qquad
\delta_{\max}=L_{\max}u\left(\frac{8\sqrt2 M}{L_{\min}}+\frac4{s_0}+3\right),\quad r=4.\;}
$$

$C$ 的量纲是面积/顶点，只依赖输入几何（$M,L_{\max},L_{\min},s_0$）与机器常数 $u$，
与分块方式无关；分块越细、$V$ 越大，上界线性放大。

代码同时给两个粒度的界：

- **实测紧界** $B=\sum_g\bigl(M\sum_i\varepsilon_i+2u\,n_gM_g^2\bigr)$，
  用每个顶点实际跟踪到的半径计算（`LevelReport.Bound`）；
- **闭式线性界** $C\cdot V$（`ComputeCoefficient` 算出 $C$），是 $B$ 的统一上限。
  测试 `TestBoundScalesWithVertices` 验证 $B\le C V$ 且 $B$ 随 $V$ 单调增大。

---

## 4. 三档裁剪：面积差与上界并列（逐档判定）

输入六边形 $(0,0),(7,0),(10,4),(6,10),(3,10),(0,6)$，精确面积 $A(P)=76$。
`go run ./cmd/cliparea` 的实测输出：

```
档(k)  块数  顶点V  二义次数  面积差|ΔA|        实测上界B         C·V(闭式上界)     逐档判定
1      1     6      7         0.000e+00        1.812e-13        1.232e-11        在界内
2      4     18     13        0.000e+00        1.752e-12        3.697e-11        在界内
4      16    66     22        2.842e-14        1.028e-11        1.356e-10        在界内
```

逐档判定（程序中的 `Within` 字段，测试 `TestAreaDiffWithinBound` 强制断言）：

- **1×1 档**：$0\le1.812\times10^{-13}$，在界内；
- **2×2 档**：$0\le1.752\times10^{-12}$，在界内；
- **4×4 档**：$2.842\times10^{-14}\le1.028\times10^{-11}$，在界内。

三档全部在界内。注意面积差远小于上界（界是最坏情形保证，不是误差估计），且上界随
$V=6,18,66$ 线性增长。本输入算出的系数数值为

$$
u=1.110\times10^{-16},\ M=11.6619,\ L_{\max}=7.2111,\ L_{\min}=3,\
s_0=0.5547,\ \delta_{\max}=4.338\times10^{-14},\ \boxed{C=2.054\times10^{-12}}.
$$

另有三个含小数坐标的不规则多边形在 `TestGenericPolygonsAlsoBounded` 中复核，
全部满足 $\Delta A\le B\le CV$，说明界不是为单个例子凑出的。

---

## 5. 顶点落在裁剪线上的二义情形

半平面判定用的是叉积 $c(p)=e\times(p-a)\ge0$。点恰在线上时精确值 $c=0$，
但浮点求出的 $\hat c$ 可能是一个绝对值极小的正数或负数，于是“算内还是算外”
取决于最后一位取整，产生二义。

一次叉积求值的误差为 $|\hat c-c|\le8u\,|e|\,\|p-a\|$。因此二义判据（一句）：

> **当 $|e\times(p-a)|\le 8u\cdot|e|\cdot\|p-a\|$ 时判定该点在线上、内外分类不可信，
> 统一按“在内”处理并单独计入二义次数；精确有理路径以叉积符号 $c=0$ 为准，不受二义影响。**

本实验的输入故意让 $(3,10)$、$(0,6)$、$(7,0)$ 等顶点压在网格线上，三档二义次数分别为
7、13、22（口径：每个顶点在每条裁剪线上至多统计一次）。二义只改变浮点路径走哪条
拓扑分支，不产生数值误差；`TestTieConventionDoesNotMoveExactArea` 证明无论采用何种
线上约定，精确面积和仍严格等于原面积。若实际工程中选择“按在外”，只需翻转分支约定，
上界公式与结论不变。

---

## 6. 代码索引

| 文件 | 职责 |
|---|---|
| `geom/geom.go` `Clip` / `intersect` | 生产用 SH 裁剪与浮点交点 |
| `geom/geom.go` `Shoelace` | 面积的唯一定义（浮点鞋带公式） |
| `cliparea/rational.go` | `math/big.Rat` 零舍入镜像，证明精确保面积 |
| `cliparea/cliparea.go` `localIntersect` | 逐交点舍入半径 $\delta_I$ |
| `cliparea/cliparea.go` `clipTracked` | 误差半径传播 + 二义统计 |
| `cliparea/cliparea.go` `Run` / `LevelReport` | 三档面积差、实测紧界 $B$、逐档判定 |
| `cliparea/cliparea.go` `ComputeCoefficient` | 闭式系数 $C$（$s_0$ 只取实际跨线交点） |
| `cmd/cliparea/main.go` | 打印并列表格与二义判据 |
| `cliparea/cliparea_test.go` | 全部结论的可执行断言 |

适用前提：输入为**简单（不自交）多边形**——SH 裁剪与鞋带面积只对简单多边形有定义，
自交多边形的“面积”本身就有歧义（缠绕面积 ≠ 区域面积），测试 `isSimple` 会先拦截；
一阶误差模型 $Vu\ll1$（本实验 $V\le66$，裕量极大）；输入为 binary64
可精确表示的有限有理数；跨线交点的分母在精确算术中非零（真跨线必满足）。
500 个随机简单多边形（`TestStressSimplePolygons`，最小夹角正弦低至约 0.018）
全部满足精确守恒且两个界同时成立。
