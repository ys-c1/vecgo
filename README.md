# 五段失败归一与按段隔离

本次改动把 `config / geom / raster / shading / post` 五处的失败错误归到同一套
编码里，并让 `pipe` 按包分段做故障隔离：某段倒了只摘那一段，其余照常出图。

## 编码

编码规则：`Kind = Stage<<8 | point`，段位就是包分段。

| 段 | Stage | 编码范围 | 代表失败点 |
|---|---|---|---|
| config | 1 | 0x0101–0x0109 | 画幅/缩放/旋转/supersample/gamma/blur/着色/输出路径 |
| geom | 2 | 0x0201–0x0208 | 细分、裁剪、描边、重复点、顶点不足、坐标非有限 |
| raster | 3 | 0x0301–0x030B | 建图、光栅、导出（create/header/write/close）、缓冲不一致、超限 |
| shading | 4 | 0x0401–0x0404 | 模式、强度、nil 图、缓冲不一致 |
| post | 5 | 0x0501–0x0505 | gamma、blur 半径、nil 图、缓冲不一致 |
| snap | 6 | 0x0601–0x0604 | 基准读写/解析/比对（不属于五段渲染） |

`Stage` 永远由 `Kind` 的段位推导（`Kind.Stage()`），不单独存一份，所以“哪段出错”
和“摘哪段”不会对不上。`verr.Wrap` 把 `os.Create` 之类的底层错误包进指定编码，
并保留 `Unwrap` 链。

## 隔离语义

`pipe.RunEx` 返回 `*pipe.Result`：`Im` 永远非 nil，`Failures` 按发生顺序记录
每段失败（含编码、阶段、原始错误），`Dropped(stage)` 报告该段是否被摘掉。

| 段失败 | 摘掉什么 | 其余段 |
|---|---|---|
| config | 该份配置，退回 `config.Default()` | 全部照常（参数来自默认配置） |
| geom | 只剔掉不合法的那个多边形 | 其余多边形照常光栅 |
| raster | 该图清成全黑（同尺寸） | shading/post 仍在黑图上照常跑 |
| shading | 着色步骤 | 保留未着色的覆盖率图 |
| post | gamma 与 blur 各自独立，谁失败只跳谁 | 另一个照常执行 |

`pipe.Run`（旧签名）保持返回 `(*raster.Image, error)`，但现在有段失败时
**图与错误同时有效**，不会丢掉已经画出来的部分。

## 成功输出逐像素对齐

基准由 `snap` 包固化为 `snap/testdata/golden/`，按包分段、一包一套，互不借用：

- `config/config_matrix.pgm` — 1 张
- `geom/geom_primitives.pgm` — 1 张
- `raster/*.pgm` — 27 张（3 个场景 × 9 个配置）
- `shading/*.pgm` — 7 张（基底 + 2 模式 × 3 组强度）
- `post/*.pgm` — 9 张（5 个 gamma + 4 个 blur 半径）

基准是在**原始 HEAD（fad87b8）的独立 worktree** 上用同一份生成代码
（`snap/render.go`）跑出来的；当前树的输出也由同一份代码生成，再逐字节比对。
P5 PGM 无压缩，逐字节相等即逐像素相等。`go test ./snap -v` 会逐段打印结论：

```
config: 1/1 一致
geom: 1/1 一致
raster: 27/27 一致
shading: 7/7 一致
post: 9/9 一致
```

复现（基准已随仓库提交在 `snap/testdata/golden/`；下面的脚本用于重新
生成/复核，尤其当改动触及 `snap/render.go` 时）：

```bash
# 1) 基线树：原始 HEAD 的 worktree，放一份同一份生成代码 + 同模块名
git worktree add --detach /tmp/vecgo-baseline fad87b8
cp snap/render.go /tmp/vecgo-baseline/snap/render.go   # 目标目录需先生成
#   并在基线上跑 RenderAll（见 snap/README 的脚本）产出 golden 图
# 2) 当前树：生成实际结果并逐段比对
go run ./cmd/snapshots /tmp/current-snaps
go test ./snap -v
```

## 诊断

`diag` 默认关闭（所有入口空操作）；`cmd -diag` 常开。记录带**时间顺序**：
每条记录有单调递增 `Seq` 与自时钟起点的 `Offset`（纳秒），环形缓冲满后
保留最近 `Capacity` 条并计数 `Overflowed`。`pipe` 在每段前后写入记录，
失败记录带正确编码与段名，因此能按 `Seq` 还原现场。

## 分配计数

`pipe.TestDiagNoExtraAlloc` 用 `testing.AllocsPerRun` 取前后两次读数：

- 诊断关闭：`8 → 8`，差值 **0**
- 诊断常开：`8 → 8`，差值 **0**
- 常开 − 关闭：**0**（诊断本身没有多出任何堆分配）

失败路径同样：`12 → 12`，差值 **0**。

所有缓冲在 `diag.New` 时一次性申请；记录只做覆盖写，不做格式化、
不复制字符串（`AddText` 要求调用方传常量或长期持有的字符串）。

## 未兜住的失败点

见 [UNCOVERED.md](UNCOVERED.md)。摘要：

- 导出失败发生在渲染之后，只退出码，不摘段；
- `pipe.Run` 旧签名的调用方若 `if err != nil { return }` 仍会丢图；
- `diag` 的零分配依赖调用方传入零分配时钟；
- `geom` 的 Flatten/Clip/Stroke 已编码但不在 `pipe` 里调用；
- `SavePGM` 的 close 错误分支有代码、无真实注入测试；
- 真 OOM 无法 recover；
- 多边形自交/退化不产生错误，只产生“坏图”，本次不覆盖。
