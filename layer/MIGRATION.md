# 图层掩码迁移说明

当场景图层数变化时，用 `layer.Migrate(oldMask, oldLayers, newLayers)` 把旧掩码
映射到新图层。函数返回 `(新掩码, 迁移表, 错误)`，迁移表逐位说明每个旧位的去向。

## 映射规则（逐位）

| 旧位 i 的位置 | 去向 | 说明 |
|---|---|---|
| `i < oldLayers` 且 `i < newLayers` | 新掩码位 i | 同一层，可见性原样保留 |
| `i < oldLayers` 且 且 `i >= newLayers` | 丢弃（NewBit = -1） | 新图层数不含该层，该位从掩码中清除 |
| `i >= oldLayers` 且 `i < newLayers` | 新掩码位 i，置 1 | 新增图层，默认可见 |

位号与图层序号的对应关系不变：位 i 永远指 `scene[i]`，因此扩容/收缩只影响
尾部位，前面各层的开关状态不受重排影响。

## 失败语义

`oldLayers`/`newLayers` 为负或超过 `layer.MaxLayers`（64）时迁移失败：

- 返回值中的掩码是**原掩码**（旧状态原样保留，调用方可继续用它渲染）；
- 返回值中的迁移表包含能算出的全部行（位 0..63），供逐位排查；
- 错误信息指出超限的具体位号（如 65 层时的位 64）。

## 渲染等价性

迁移只重排可见性位，不改几何与画幅映射。若新增/保留/丢弃的图层几何覆盖
不变（例如新增层与已有层重合，或被丢弃的层本就不可见），迁移前后各渲一张
图，差异像素数为零。该性质由 `TestMigrationGrowRenderParity`、
`TestMigrationShrinkRenderParity`、`TestMigrationFailureKeepsState` 三个用例
保证。
