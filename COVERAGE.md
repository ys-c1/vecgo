# 解析覆盖率（面积裁切）核验结论

边缘灰度不再只靠数子点估计：新增 `raster.ClipCoveragePx`（`raster/coverage.go`），
把每个边界像素的单位正方形与多边形求交，按交叠面积直接得到覆盖率。
默认的 2×2 子点路径 `Image.Fill` 一行未改，默认产物逐字节不变。

## 面积裁切算法（三步）

1. 取像素单位正方形为裁剪窗，用 Sutherland–Hodgman（`geom.Clip`）按四条边半平面逐边裁多边形；
2. 裁后顶点走鞋带公式 `geom.Area`：`A = ½|Σ(xᵢyᵢ₊₁ − xᵢ₊₁yᵢ)|`；
3. 单位像素面积为 1，覆盖率 `c = clamp(A, 0, 1)`（通用映射下除以像素面积 `SX·SY`）。

## 交叠判断

裁剪窗顶点逆时针 `(c,r)→(c+1,r)→(c+1,r+1)→(c,r+1)`。对有向裁剪边 `A→B`，令 `E=B−A`：

```
cross(E, P−A) = (Bx−Ax)(Py−Ay) − (By−Ay)(Px−Ax)
cross ≥ 0  →  P 在左侧半平面（内侧，含边界）
cross < 0  →  P 在外侧
```

当前点与前一点一内一外时，求线段与裁剪直线交点并插入（见 `geom.Clip`）。

## 两档角度误差（24×24，边缘像素覆盖率 RMSE；基准为独立竖直板条精确积分 `internal/exactcov`）

| 角度 | 边缘像素 | 子点法 2×2 RMSE | 解析面积裁切 RMSE | 解析/子点 | 判定 (<1/2) |
|---|---:|---:|---:|---:|:---:|
| 15°（缓斜） | 88 | **0.09409** | **6.94×10⁻¹⁵** | 7.4×10⁻¹⁴ | ✅ |
| 45°（对角） | 48 | **0.25000** | **5.71×10⁻¹⁶** | 2.3×10⁻¹⁵ | ✅ |

解析误差约为子点法的千亿分之一，远小于“一半”的要求；残余仅为浮点误差。
45° 时斜边与规则子点网格对齐，2×2 子点在每个边缘像素上误差恒为 0.25，
这就是斜边上台阶的量化来源。

## 默认子点数产物逐字节相同

HEAD `fad87b8` 基线与当前代码（`config.Default()`，Supersample=2）产物 sha256 均为

```
fbda3e5665174433272beab4f25172bc03466e3f8700bcf6007b32c3636f2dc3
```

`pipe.TestDefaultArtifactByteIdentical` 锁定该哈希；全新构建的 `cmd` 输出已与
改动前二进制 `cmp` 复核一致。

## 每像素交叠比例与可核面积

运行：

```sh
go test ./...
go run ./cmd/coverageaudit     # 重新生成 artifacts/ 下全部证据
```

- `artifacts/coverage-report.md` — 完整报告（含每像素比例矩阵）
- `artifacts/coverage-15deg.csv` / `coverage-45deg.csv` — 全部 576 像素的交叠比例、裁切面积、子点命中、精确基准
- `artifacts/clipped-15deg.jsonl` / `clipped-45deg.jsonl` — 每个边缘像素裁切后的真实顶点，可直接重算鞋带面积
- `artifacts/compare-15deg.png` / `compare-45deg.png` — 左：2×2 子点台阶；右：解析面积平滑边缘
- 面积守恒：15° 全部像素覆盖率之和 384.0、45° 为 288.0，分别等于多边形像素坐标面积，差额 0
