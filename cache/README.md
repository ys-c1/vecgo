# LRU 覆盖率缓存

渲染器此前没有缓存；长任务反复光栅化同一批多边形，既浪费 CPU，也容易让人错误地以为“缓存只进不出”。本目录的 `cache` 包实现了带容量上限的 LRU，并接入了 `raster.Fill` 的等价路径。

## 语义

- **容量可配**：`cache.New(capacity)`，容量即条目数。达到上限后插入新键会淘汰最久未使用的条目。
- **结果不变**：缓存路径 (`Image.FillCached`) 与原始路径 (`Image.Fill`) 的输出逐字节相同。缓存存的是每个多边形在采样网格上的覆盖位掩码，多个多边形按位或合并，与 `Fill` 的“任一多边形命中即计数一次”语义一致。
- **键的构成**：掩码版本、画幅宽高、采样率、场景包围盒（决定采样坐标）、多边形顶点浮点位。任何一项不同都产生不同键，因此不同画幅或采样率之间不会串用掩码。
- **关闭缓存时**：`pipe.Run` 永远走未缓存路径，忽略 `Config` 中的缓存字段；输出与开启缓存时逐字节相同（`pipe.TestCacheByteIdentical` 等覆盖）。

## 统计

`Stats()` 提供命中、未命中、插入、更新、淘汰、拒绝、峰值条目、峰值字节数。`Entries()` 按插入序号排序列出每个存活条目的键与命中次数，顺序确定。导出接口：

- `WriteTable(w)`：人读表格，含全局计数、逐条目 `key/hits/bytes` 与淘汰轨迹。
- `WriteCSV(w)`：同内容 CSV，首行为 `#` 注释统计。
- `WriteEvictionTrace(w)`：每行一个被淘汰的键，末行 `#digest=<hex>`。

## 可复现的淘汰顺序

活跃度由单调递增的逻辑时钟驱动，从不依赖 map 迭代顺序。淘汰轨迹保留在容量为 `TraceLimit` 的环形缓冲中，同时对**全部**淘汰计算滚动 FNV-1a 摘要。两次相同输入运行的轨迹与摘要都应完全一致；即使轨迹环已回绕，摘要仍覆盖全量序列。

## 错误

三类输入都返回 `*cache.EntryError`，可用 `errors.Is` 判断哨兵，用 `errors.As` 取 `EntryInfo`：

| 输入 | 哨兵 | 附带信息 |
|---|---|---|
| 容量为 0 或负 | `ErrZeroCapacity` | `capacity`、当前条目数、字节数 |
| 键为空 | `ErrEmptyKey` | `key`、`capacity`、条目数、字节数 |
| 满后仍插入 | `ErrFull` | `key`、`capacity`、条目数、字节数 |

满后插入有两种 API：`Insert` 永不淘汰，直接返回 `ErrFull`；`Put` 默认按 LRU 淘汰后插入，`Options{NoEvict: true}` 时也会返回 `ErrFull`。

## 内存

缓存峰值字节数受容量约束，与三角形总数无关。`cmd/bench` 用递增的三角形数量采样 `PeakBytes`，做最小二乘求斜率，超过 `Config.CacheSlopeLimit()`（默认 4096 B/三角形，可配）即视为失败。有界容量下实测斜率为 0。

## 测试覆盖

| 类别 | 测试 |
|---|---|
| 冷启动 | `cache.TestColdStart`、`raster.TestCachedFillHotRerun` |
| 热循环 | `cache.TestHotLoop`、`pipe.TestCacheWarmRerunIdentical` |
| 容量抖动 | `cache.TestCapacityThrash` |
| 单条大三角形 | `raster.TestSingleLargeTriangle`、`pipe.TestSingleLargeTriangleCacheBounded` |
| 全不命中 | `cache.TestAllMiss` |
| 输出一致性 | `raster.TestCachedFillMatchesUncached`、`pipe.TestCacheByteIdentical`、`pipe.TestCacheDisableLeavesOutputUntouched` |
| 淘汰可复现 | `cache.TestEvictionOrderReproducible` |
| 斜率判失败 | `pipe.TestPeakMemoryDoesNotGrowWithTriangles`、`pipe.TestSlopeLimitDetectsGrowth` |
| 三类错误 | `cache.TestErrorInputs` |

## 命令行

```
go run ./cmd/bench -out artifacts/cache
```

参数：`-small-capacity`、`-large-capacity`、`-counts`、`-width`、`-height`、`-supersample`、`-max-bytes-per-triangle`、`-verify-identical`。输出 `cache-report.md`、两档 `curve-*.csv`（含运行时内存采样）、`entries-large.{txt,csv}`、`evictions-run{1,2}.txt`。

主程序注册了 `-cache`、`-cache-capacity`、`-cache-stats` 三个参数：

```
go run ./cmd -cache -cache-capacity 64 -cache-stats stats.txt out.pgm
```

`-cache-stats` 写出与 `WriteTable` 相同的逐条目报表；开启与不开启缓存生成的 PGM 逐字节相同。
