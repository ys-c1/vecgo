# 渲染确定性（逐字节可复现）排查与验证

## 背景

同一场景连续渲染两次，产物 PGM 的字节曾出现不一致。按三个方向逐一排查：
**遍历顺序、浮点累加、缓冲残留**。

## 三处排查结论与修复

### 1. 遍历顺序

- 全代码库无 map 迭代、无 goroutine、无 rand/time 依赖；所有像素循环均为
  切片下标序，本身确定。
- 唯一的隐患在 `raster.Fill`：多边形命中测试是"首个命中即 break"，
  结果依赖调用方传入多边形的顺序。调用方换序时，重叠边界像素理论上可变化。
- 修复：`Fill` 入口对多边形按几何内容（`polygonKey`）做
  `sort.SliceStable` 确定性排序，遍历顺序只取决于输入内容，与切片排列无关。
  回归测试：`raster.TestFillOrderIndependent`。

### 2. 浮点累加

- 出字节的热路径（`raster.Fill` 的 `hit`、`post.Blur` 的 `sum`）都是
  **整数**累加，遍历顺序固定，不存在浮点舍入随运行次数漂移的问题。
- 全代码库唯一的浮点累加器是 `geom.Length` 的 `total += math.Hypot(...)`。
- 修复：改为 Kahan 补偿求和，补偿项吸收低位舍入，结果只取决于输入内容。
  回归测试：`geom.TestLengthKahanAccuracy`（万段 1e-4 微线段，总长须贴近 1.0）。

### 3. 缓冲残留

- 旧代码靠两件"隐性"事保证干净：`raster.New` 的 `make` 返回零值缓冲、
  `Fill` 恰好写满每个下标。一旦复用同一个 `Image` 二次 `Fill`，
  未被新场景覆盖的下标就会残留上一次的像素。
- 修复：新增 `Image.Clear()` 并在 `Fill` 入口显式清零，缓冲安全成为显式契约。
  回归测试：`raster.TestFillClearsBufferResidue`（脏缓冲与全新缓冲渲染结果逐字节相等）。

## 复现步骤（关键：中间不重启进程）

测量工具：`tools/determinism`。它在**同一个进程内**对同一场景按指定画幅
各渲染 N 次，逐文件输出 sha256，并在每行打印 PID 以便确认进程未重启。

```bash
# 构建（无需 CGO，纯 Go）
go build -o /tmp/det ./tools/determinism

# 三档画幅在【一次调用、同一 PID】内顺序各跑 10 次；
# 进程在整个循环期间不重启，画幅切换也在同一进程内完成。
/tmp/det -sizes 64x64,128x128,256x256 -runs 10 -outdir /tmp/det-out | tee /tmp/det-log.txt
```

确认"不重启进程"：输出首行与每行均带同一 `pid=`，且

```bash
# 所有渲染行的不同 PID 数必须为 1（本次实测 30 行全部为同一 PID）
grep -o 'pid=[0-9]*' /tmp/det-log.txt | sort -u | wc -l
```

一次性冷启动基准（每档单独起一个全新进程，只跑 1 次），用于定位首次分歧：

```bash
/tmp/det -width 64  -height 64  -runs 1 -outdir /tmp/cold
/tmp/det -width 128 -height 128 -runs 1 -outdir /tmp/cold
/tmp/det -width 256 -height 256 -runs 1 -outdir /tmp/cold
```

判定标准：同档 10 份哈希彼此相同，且与该档冷启动基准哈希相同；
若某次开始与基准不同，首个不同的迭代即为首次分歧点。

## 老场景输出保持原样

- 修复前二进制与修复后二进制对默认老场景（64x64 单位正方形）各出一张图，
  `cmp` 逐字节相同，sha256 一致。
- 三档各 10 份产物在修复前/后两个目录间 `diff -r` 完全一致：
  加固只把确定性从"隐性"变成"显式"，未改变任何输出字节。

## 测试

```bash
gofmt -l .          # 无输出
go vet ./...
go test ./...
go test -race ./...
go test -count=20 ./...   # 压力重复，全部通过
```
