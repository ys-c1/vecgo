# 参数校验与诊断图

## 问题

参数写错时不报错，照样出图，只是画面黑了一大片。实测（改动前）：

| 输入 | 改动前行为 |
|---|---|
| `scale=NaN` | 静默输出**全黑**图（64×64 全 0） |
| `rotate=NaN` / `rotate=Inf` | 静默输出**全黑**图 |
| `gamma=NaN` | 放行，渲染结果与默认完全一样，错误被吞掉 |
| `rotate=1e300` | 放行，出图正常；但角度这么大时 cos/sin 精度有限，图上看到的内容未必是用户想要的 |

前三条是本次要修的 bug。第四条不是 bug——有限角度在数学上有定义，
所以维持放行，只是把“精度随角度下降”写进放行理由。

根因有两条：

1. **NaN 能穿过所有区间比较**。`NaN < 0.1` 和 `NaN > 10` 都为假，
   所以 `validate` 里的 `c.Gamma < 0.1 || c.Gamma > 10` 对 NaN 恒为假。
   `scale`、`gamma` 同理。
2. **`rotate` 完全没有校验**。`Inf` 进 `math.Cos/Sin` 得到 NaN，整幅图变黑。

## 改动

- `config.Config.Issues()`：逐参数校验，**先查有限性再查区间**，
  一次性列出全部问题（不再只报第一条）。`Validate()` 由它派生，
  单条错误文本与改动前一致。
- `post.Gamma`、`shading.Apply` 也补了同样的有限性检查。它们是被
  别的包直接调用的库函数，不能只靠 `config` 把门；`raster.Fill`
  在 `supersample` 上本来就有下界检查。
- `config.Config.Validate()` 现在拒绝：`scale`/`gamma`/`rotate` 的
  NaN/±Inf，以及此前就有的区间越界。
- `diag` 包：用 `Safe(cfg)`（非法值回落默认、合法值原样保留）渲染
  参考图，再把受影响区域标红，导出为 PNG。
- `cmd`：新增参数化入口 `-width/-height/-scale/-rotate/...` 与
  `-diag`、`-diag-out`。**合法参数**下无论带不带 `-diag`，输出都与
  改动前逐字节一致；位置参数仍然可用。非法参数在没有 `-diag` 时
  以退出码 1 拒绝，不再静默出图。

## 诊断图

- 触发：`-diag` 且存在非法参数。非法参数不进渲染，避免写出一张
  看不出问题的黑图。
- 输出：PNG（工单附件通用格式；灰度 PGM 装不下红色标记）。
  默认路径 `<输出>.diag.png`，可用 `-diag-out` 指定。
- 标记方式：
  - **几何覆盖区域**（`scale`/`rotate`/`supersample`/`shading`）：
    参考图中被图形覆盖的像素打红色斜纹，底图明暗仍可见。
  - **整幅画面**（`width`/`height`/`gamma`/`blur`/`output`）：红色边框。
- 自解释：拒绝原因与影响范围写进 PNG 的 `iTXt` 文本块（关键字
  `vecgo-diag`），图片单独转发或贴进工单时说明不丢。
  例如 `python3 -c "from PIL import Image; print(Image.open('x.png').text)"`。
- 退出码：诊断模式拒绝渲染时为 `3`，便于脚本区分“参数错”与
  “渲染/导出失败”。

注意：仓库自带的演示场景是一个铺满画幅的正方形（`raster.Fill`
会把场景包围盒归一化到画幅），所以对 `scale`/`rotate` 生成的诊断
图看起来是满屏斜纹——这符合事实：几何区域确实铺满整幅。局部标
记见 `diag` 包测试里用三角形场景的用例。

## 拒绝 / 放行一览

| 参数 | 拒绝 | 放行 | 放行理由（若放行） |
|---|---|---|---|
| `width` | `<=0`、`>4096` | `1..4096` | 整数像素宽，4096 是设计上界 |
| `height` | `<=0`、`>4096` | `1..4096` | 同上 |
| `scale` | NaN、±Inf、`<=0`、`>100` | `(0,100]` 的有限值 | 正有限倍数都能确定地映射几何 |
| `rotate` | NaN、±Inf | 任意**有限**值 | 有限角度总有确定的 cos/sin，渲染可复现 |
| `supersample` | `<1`、`>8` | `1..8` | 整数采样数，8 是设计上界 |
| `shading` | 非 `flat`/`gradient` | `flat`、`gradient` | 仅这两种实现存在 |
| `gamma` | NaN、±Inf、`<0.1`、`>10` | `[0.1,10]` 的有限值 | 底数为正，幂运算有定义且可复现 |
| `blur` | `<0`、`>16` | `0..16` | 整数半径，0 表示不模糊 |
| `output` | 空串 | 任意非空路径 | 空路径无法落盘 |

## 三个参数的分界实验

来自 `go test ./config/ -run Boundary -v`：

| 参数 | 放行侧 | 拒绝侧 | 放行侧理由 |
|---|---|---|---|
| `scale` | `100` | `100.0000001` | 100 是设计上界，仍能正常缩放，不改变渲染的有效性 |
| `rotate` | `math.MaxFloat64` | `+Inf` | 有限角度总有确定的 cos/sin，渲染可复现，只是精度随角度增大而下降 |
| `gamma` | `0.1` | `0.0999` | 0.1 是设计下界，底数为正，幂运算有定义且输出可复现 |

## 诊断关闭时输出逐字节相同的证据

`pipe/testdata/*.golden` 是**改动前**的代码生成的基准输出，覆盖
默认配置、极小 scale + 低 gamma、宽画幅 + 旋转 + 渐变 + 大模糊、
负旋转 + ss=1、blur=0 五种组合。`TestGoldenByteIdentical` 用改动后
的代码重渲并逐字节比较。

另外用改动前后的两个二进制在真实调用下对比：

```
$ go build -o /tmp/vecgo-old ./cmd   # 在 HEAD (fad87b8) 的 worktree 里
$ /tmp/vecgo-old /tmp/baseline.pgm && sha256sum /tmp/baseline.pgm
fbda3e5665174433272beab4f25172bc03466e3f8700bcf6007b32c3636f2dc3
$ /tmp/vecgo-new /tmp/new-pos.pgm && sha256sum /tmp/new-pos.pgm
fbda3e5665174433272beab4f25172bc03466e3f8700bcf6007b32c3636f2dc3
```

位置参数调用与默认调用（在 /tmp 下不带参数）都逐字节一致。

## 测试

```
go test ./...                       # 全部通过
go test ./config/ -run Boundary -v  # 分界实验
go test ./pipe/ -run Golden -v      # 逐字节回归
```
