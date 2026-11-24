# 配置键名迁移说明

配置键名经历过两轮改名，共三代。旧加载逻辑会把不认识的键静默忽略，
字段回落到默认值，出图跟着偏移且没有任何提示。本次改动在
`config` 包加入迁移加载器（`config.Load` / `config.LoadFile`）：
旧键自动映射到当前键，每个被替换的键都打一条弃用告警，未知键也不再静默丢弃。

## 键名映射表（导出于 `config.DeprecatedKeys`）

| 当前键 (v3) | 第 1 代旧名 (v1) | 第 2 代旧名 (v2) | 含义 |
|---|---|---|---|
| `width` | `imgW` | `w` | 画幅宽 |
| `height` | `imgH` | `h` | 画幅高 |
| `scale` | `zoom` | `s` | 缩放 |
| `rotate` | `angle` | `rot` | 旋转角（弧度） |
| `supersample` | `samples` | `aa` | 超采样数 |
| `shading` | `fill` | `shade` | 着色模式 |
| `gamma` | `gammaV` | `g` | 伽马值 |
| `blur` | `soften` | `b` | 模糊半径 |
| `output` | `dest` | `out` | 输出路径 |

三代键名的字面量两两不相交：任何键的解释都是唯一的，不存在"同一个字面量
在不同代含义不同"的二义性。代际常量为 `config.GenerationV1`、
`config.GenerationV2`、`config.GenerationCurrent`，可用
`config.LookupDeprecated(oldKey)` 程序化查询。

## 告警类别

每条告警是一个 `config.Warning`（`cmd` 启动时打印到 stderr）：

- `KindDeprecated`：文件出现映射表中的弃用旧键。**每个被替换的键恰好一条**，
  无论它最终是否生效；
- `KindConflict`：同一目标字段被多个键同时提供，按下面的优先级只取一个；
- `KindUnknown`：键既不是当前键也不在映射表中。过去这类键被静默丢掉，
  现在显式告警。

取值类型错误（如 `"width": "wide"`）和越界值不走告警，直接返回 error
（越界由 `Config.Validate` 统一拦截，迁移后的值也一样校验）。

## 冲突规则与样例

优先级从高到低：

1. **当前键（v3）优先于一切旧键。**
2. 只有旧键时，**代际新者优先：v2 > v1**。
3. 落败的键不生效，但仍计入弃用告警，并额外发一条冲突告警说明取舍。

### 样例（新旧键同时出现，规则 1）

```json
{
  "width": 32, "height": 32,
  "s": 9.0,
  "scale": 3.5,
  "shading": "flat", "output": "conflict.pgm"
}
```

字段 `scale` 同时被第 2 代旧键 `s`（值 9.0）和当前键 `scale`（值 3.5）
提供。按规则 1，**`scale` 的 3.5 生效，`s` 的 9.0 忽略**，产生两条告警：

```
弃用告警: 键 "s" 是第 2 代旧名，已替换为 "scale"，请更新配置文件
冲突: 字段 "scale" 同时由旧键 "s" 和新键 "scale" 提供；新键优先，旧值已忽略
```

**取舍理由**：旧键已处于弃用周期，文件里同时写出新键，说明作者正在更新
配置，新键代表当前意图。若让旧键赢，迁移层会把一份已经改对的配置再改回
旧值——正是本次要消灭的"静默出图偏移"。两代旧键同场时同理，取更接近
当前代的 v2（由 `TestNewerGenerationWinsOnConflict` 覆盖）。

## 逐像素验收

四份老配置夹具（`config/testdata/`，v1/v2 各两份）各出一张图：

- `v1_1.json`、`v1_2.json`：全第 1 代键；
- `v2_1.json`、`v2_2.json`：全第 2 代键；
- 每份旁边的 `*.twin.json` 是同值的当前键对照件。

`tools/gengolden` 用 twin 渲出 golden 基线（`pipe/testdata/golden/*.pgm`），
代表"改动前应得的图"。验收测试 `pipe.TestLegacyConfigsPixelParity` 对每份
老配置做两路逐字节比对：老配置渲染结果 vs golden 基线、vs twin 现渲结果，
**任一像素不同即失败**并报告第一个差异像素的坐标与两值；全过即差值为零。
四份夹具的参数两两不同，覆盖全部 9 个字段及 flat/gradient 两种着色、
不同 gamma/blur/supersample 组合。
