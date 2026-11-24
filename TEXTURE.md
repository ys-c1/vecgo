# 贴图越界采样：衰减带权外插

问题：采样坐标越过纹素边界时，截断到边界（clamp-to-edge）会把同一行/列边界纹素
拉伸到整片越界区域，边缘颜色形成明显色带。

## 衰减函数与两档系数

连续纹素坐标 `(x,y)`（纹素 `i` 的中心在 `i`）：

```
ox = max(-x, 0, x-(W-1))
oy = max(-y, 0, y-(H-1))
d  = hypot(ox, oy)                 // 越界距离，单位：源纹素
w(d; λ) = exp(-λ d)                // dw/dd = -λw, w(0)=1；越远权重越低
out = B + w·(E - B)                // E=外插色（零阶=截断双线性，另有一阶线性），B=底色
```

`λ = 0.090`（每源纹素，衰减尺度 τ≈11.1 纹素）。距离按**输出像素**计时 β_k = kλ，
**两档系数：β_2 = 0.180，β_4 = 0.360。**

## 两档验证结果（96×96 画布，128² 贴图居中缩小）

```text
[越界像素] 缩小2倍 x2: 5120 像素 (oob_pixels=5120, inside=4096, total=9216)
[越界像素] 缩小4倍 x4: 8192 像素 (oob_pixels=8192, inside=1024, total=9216)
```

越界区域**平均** RGB 色差（0..≈441.7），括号内为最大值：

| 档 | 截断平均(最大) | 带权外插平均(最大) | 平均下降 |
|---:|---:|---:|---:|
| 2× | 179.03 (283.6) | **4.02 (14.9)** | **97.8%** |
| 4× | 241.29 (288.7) | **0.74 (12.5)** | **99.7%** |

平均三通道绝对差（MAE）：2× 97.14→2.03；4× 129.56→0.37。
异配的高斯光斑场上沿用同一 λ，仍下降 86.4% / 97.1%。

## 代码与复现

- 实现：`texture/texture.go`（`DecayWeight`、`Overshoot`、`Sampler.Weighted`）
- 参考场/两档评测：`texture/check.go`
- 验收测试：`texture/texture_test.go`；默认产物哈希锁定：`pipe/identity_test.go`
- 验证命令：`go run ./cmd/texcheck`（重新生成 `artifacts/` 下全部图、JSON、报告）
- 测试：`go test -count=1 ./...`

## 对照产物（`artifacts/`）

- `clamp-to-edge.png` / `weighted-extrapolation.png`：截断、外插**两版各一份**总对照（上 2×、下 4×）
- `tex-clamp-x{2,4}.png(.pgm)` / `tex-extrap-x{2,4}.png(.pgm)`：每档两版单图
- `tex-compare-x{2,4}.png`：截断 ｜ 外插 ｜ 真值 ｜ 色差热图 四联对照
- `texture-report.md`、`texture-metrics.json`、`verification.txt`：完整报告、数值与验证记录

## 默认模式产物不变

既有渲染管线未改。`config.Default()` + 默认方块场景 PGM 的 sha256 仍为
`fbda3e5665174433272beab4f25172bc03466e3f8700bcf6007b32c3636f2dc3`（= HEAD fad87b8），
`cmp` 与 `TestDefaultArtifactByteIdentical` 均确认逐字节相同。
