# 失败隔离（fault isolation）

## 背景：黑帧是怎么来的

隔离关闭时，`pipe.Run` 是整帧事务：任何一个图形出问题，整帧都无法产出，
其余图形跟着陪葬。实测有两条路径：

1. **顶点不足（< 3 个点）→ 显式失败**
   `raster.Image.Fill` 直接返回 `多边形至少三个顶点`，`pipe.Run` 整体
   返回错误，命令退出码 1，没有任何 PGM 落盘。
2. **坐标含 `Inf`/`NaN` → 静默黑帧（更隐蔽）**
   包围盒被 `Inf` 污染（`maxx = +Inf`），每个像素的采样坐标经
   `minx + …*spanx/...` 算出 `NaN`；`geom.Inside` 中所有比较对 `NaN`
   均为 false，于是没有任何像素被点亮，整帧全零。管线**不报错、退出码 0**，
   照常写出一张纯黑 PGM。

## 实现

- `raster.Fill(polys, ss)`：传统路径，行为与改造前完全一致（内部调用
  共享的 `rasterize` 核心，外围校验逻辑原样保留）。
- `raster.FillIsolated(polys, ss) (SkippedList, error)`：逐图形调用
  `validatePolygon`（顶点数 ≥ 3、坐标不含 NaN/Inf），坏图形只记录编号
  和原因并跳过，好图形进入同一个 `rasterize` 核心绘制。
- `pipe.Run` 保持原签名走传统路径；新增 `pipe.RunIsolated` 返回
  `(图像, 跳过清单, 错误)`。两条路径共用同一光栅化核心与同一套后处理
  （shading/gamma/blur），所以同一组合法图形的输出逐像素相同。
- 全部图形都被跳过时返回尺寸正确的全黑图（已无图形可画），不视为错误；
  空场景同样安全，不再触发 `polys[0]` 越界 panic。
- 跳过清单按输入顺序追加，编号天然升序；`SkippedList.IDs()` 返回
  `[]int` 供程序读取，CLI 的 `-skip-list` 输出纯编号、一行一个，
  可直接被 shell/awk 等脚本读取。

## 命令用法

```bash
go build -o vecgo ./cmd

# 隔离开启（坏图形跳过，其余照常绘制）
./vecgo -isolate -scene scenes/abnormal.json \
  -out render.pgm -skip-list skipped.txt

# 隔离关闭（传统整帧事务，用于对照）
./vecgo -scene scenes/abnormal.json -out render.pgm
```

场景 JSON 坐标除普通数字外，支持 `"Inf"`、`"-Inf"`、`"NaN"` 三个字符串
token，用于显式构造坏坐标。

## 一键校验

```bash
scripts/verify_isolation.sh
```

校验内容：

1. 正常场景隔离 on/off 的两份 PGM `cmp` 逐字节相同；
2. 异常场景隔离后非黑帧，跳过清单升序、一行一个；
3. 对照：隔离关闭时同一输入整体失败（无图）或静默黑帧（全零 PGM）。

单元测试：`raster/raster_isolate_test.go`、`pipe/pipe_isolate_test.go`，
覆盖逐像素一致性、跳过编号、全坏/空场景、两种黑帧机制对照。
