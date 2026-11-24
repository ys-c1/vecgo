# vecgo

纯 Go 矢量渲染器（几何 → 光栅化 → 着色 → gamma/模糊 → PGM），无第三方依赖。

## 金字塔层数—重建误差实验

“金字塔层数给多了浪费，给少了糊”。`pyramid/` 实现 2×2 均值图像金字塔与
逐层残差，`lab/` 在真实 vecgo 渲染上标定误差衰减、推导总误差—层数关系、
解析求最小层数并与全层数穷举对照。完整推导、三档目标代入、实测/预测对照表
与衰减曲线见：

- **[`pyramid/REPORT.md`](pyramid/REPORT.md)**（含 [`pyramid/decay.png`](pyramid/decay.png) 曲线图）
- [`artifacts/REPORT.md`](artifacts/REPORT.md)、[`artifacts/pyramid_decay.png`](artifacts/pyramid_decay.png)、[`artifacts/pyramid_lab.json`](artifacts/pyramid_lab.json)

一键复现：

```bash
go run ./cmd/pyrlab -out artifacts    # 重算并生成 JSON/SVG/报告
go test ./...                         # 含三档“解析解==穷举解”与固定路径测试
```

结论速览（256×256 gradient 场景，L=9，RMSE 为 0–255 灰度级）：每层细节误差
δⱼ=d1·q^(j−1)，实测 q≈0.502；由 Haar 正交平方尾和推出 E(n)≈A·q^n
（A≈147.6），故最小层数 n\*=ceil(ln(ε/A)/ln q)。三档目标 ε=20、5、1.5
分别解出 n=3、5、7，与穷举首个达标层完全一致，实测 18.52、4.77、1.12 对预测
18.68、4.71、1.19，偏差 ≤6%。

## 固定层数与自适应两条路径

```bash
go run ./cmd -levels 4 out.pgm                     # 固定层数路径（保留）
go run ./cmd -shading gradient -target-error 5 out.pgm  # 自适应最小层数
```

默认不启用金字塔，渲染结果与原有管线一致；`Config.PyramidLevels` /
`Config.TargetError` 与上述标志对应。
