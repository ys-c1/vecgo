# 降采样核频域分析 — 复现说明

配套报告见 `REPORT.md`。本目录是独立的 Python 分析，与 vecgo 的 Go 代码无运行时依赖
（仅概念上对应 `raster/raster.go` 的超采样盒式平均）。

## 环境

需要 numpy + matplotlib（系统 Python 受 PEP 668 保护，用独立 venv）：

```bash
python3 -m venv .venv            # 在仓库根目录创建
.venv/bin/pip install numpy matplotlib
```

## 运行

```bash
# 数值引擎：打印三档缩放比指标，写 results/{metrics,sweep,tail}.csv 与 bounds.txt
.venv/bin/python -I analysis/run_analysis.py

# 绘图：读 results/ 下的 CSV，写四张 PNG
.venv/bin/python -I analysis/make_figures.py
```

`-I` 为隔离模式（忽略环境变量与用户 site-packages）；脚本已把自身目录加入
`sys.path`，可从任意工作目录运行。

## 文件

| 文件 | 作用 |
|---|---|
| `kernels.py` | 盒/高斯/Lanczos 核构造、等效半径、解析傅里叶变换 H_box / H_gauss |
| `run_analysis.py` | 数值引擎：DFT 掩码法、连续谱积分、梯度锐度、MTF50、Q 闭式、不确定性下界 |
| `make_figures.py` | 四张图（传递函数 / 三档并排 / 核宽权衡 / zone plate 视觉） |
| `results/` | CSV 指标、bounds.txt 闭式对账、PNG 图 |

## 两条独立核算路径

- **DFT 掩码法**：循环卷积后按输出 Nyquist 掩码分基带/阻带，Parseval 与时域
  线性卷积对账（相对误差 ≤ 5.4e−3）；
- **连续积分法**：用核的解析 FT 代入 `E_a = ∫_{|ν|≥f_c} |H|² S_x dν`。
  白噪声 Q 另有 erfc（高斯）与 Si（盒式）闭式，与数值积分互相校验。
