# vecgo 输出文件格式（PGM P5 + 版本字段）

本文档是 `raster.SavePGM` / `format.Encode` / `format.Decode` 的契约。所有多字节字段
**一律小端序（little-endian）**。

## 1. 三种输出

| 版本 | 命令 | 文件头 | 与旧版关系 |
|---|---|---|---|
| v0 | `vecgo -version 0 -out out_v0.pgm` | 无版本字段 | **与旧版 `SavePGM` 逐字节相同** |
| v1 | `vecgo -version 1 -out out_v1.pgm` | 一行 PGM 注释 + 4 字节公共字段 | 旧解码器/普通 PGM 读者可读 |
| v2 | `vecgo -version 2 -out out_v2.pgm` | 一行 PGM 注释 + 4 字节公共字段 + 18 字节扩展 | 旧解码器可**降级**读取 |

v0 字节布局（不变）：

```
"P5\n" W " " H "\n255\n" Pix[W*H]
```

v1/v2 字节布局：

```
"P5\n"
"#VCFM " <hex(payload)> "\n"        ← 版本字段，位于 P5 之后、画幅之前的注释行
W " " H "\n255\n"
Pix[W*H]                            ← 位置与长度与 v0 相同
```

版本字段放在 PGM 注释里是兼容性的关键：PGM 规定注释可出现在画幅字段之前，普通
P5 解码器（netpbm、PIL、ImageMagick 等）会跳过它，因此 v1/v2 仍是合法 PGM，且像素区
**不会因为加版本字段而移动**。`payload` 是字段块的十六进制（小写）编码，不含空白。

## 2. 字段块

### 2.1 公共字段（v1 与 v2 均有的前 12 字节）

| 偏移 | 长度 | 类型 | 名称 | 说明 |
|---|---|---|---|---|
| 0 | 4 | ASCII | magic | 固定 `"VCFM"` |
| 4 | 2 | uint16 LE | version | 1 或 2 |
| 6 | 2 | uint16 LE | flags | 布局标志，见 2.2 |
| 8 | 2 | uint16 LE | width | 像素宽，必须等于 PGM 画幅宽 |
| 10 | 2 | uint16 LE | height | 像素高，必须等于 PGM 画幅高 |

### 2.2 v2 扩展字段（偏移 12..21，共 10 字节）

| 偏移 | 长度 | 类型 | 名称 | 取值/说明 |
|---|---|---|---|---|
| 12 | 2 | uint16 LE | gamma_milli | round(gamma×1000)，合法 [100, 10000] |
| 14 | 1 | uint8 | supersample | [1, 255] |
| 15 | 1 | uint8 | blur | [0, 255] |
| 16 | 1 | uint8 | shading | 0=flat，1=gradient |
| 17 | 1 | uint8 | reserved | 必须为 0 |
| 18 | 4 | uint32 LE | pixel_crc32 | IEEE CRC32（与 Go `crc32.ChecksumIEEE`、Python `binascii.crc32` 相同），仅覆盖像素区 |

**符号性**：字段块中所有数值字段均为**无符号**。没有负数字段；gamma 用非负毫值表示。

**长度**：v1 字段块恰好 12 字节（24 个十六进制字符）；v2 恰好 22 字节（44 个字符）。
解码时长度必须精确匹配，否则拒绝（见 §4）。

### 2.3 flags

| 位 | 名称 | 含义 |
|---|---|---|
| 0 | `EXTENDED_META` | 字段块在公共字段后追加 v2 扩展字段；不移动像素区 |
| 1..15 | 保留 | 必须为 0 |

`flags` 的规则：v1 要求 `flags==0`；v2 要求 `flags==0x0001`。任何未登记位被置位都视为
**未知布局**并明确拒绝。这个设计的意图是：**新版本若只追加元数据（bit0 语义），旧解码器
可以按字段集 1 降级；若将来出现移动/改写像素区的布局，旧解码器必须拒绝而不是读错。**

## 3. 解码按版本选字段集

解码器声明的字段集只有 1 和 2（`format.DecoderV1` / `format.DecoderV2`）：

| 文件版本 | 字段集 1（旧解码器） | 字段集 2（当前解码器） |
|---|---|---|
| v0（无版本字段） | 可读，字段集 1 | 可读，字段集 1 |
| v1 | 可读，字段集 1 | 可读，字段集 1 |
| v2 | **可降级读**：用字段集 1，保留像素，`Downgraded=true`，元数据不可见 | 可读，字段集 2，含元数据 + 校验和 |

选择规则（`format.Decode`）：

1. 文件无版本字段 → 字段集 1。
2. 版本字段 `version` 不在 {1,2} → `ErrUnsupportedVersion`。
3. `flags` 含未登记位或与版本不符 → `ErrUnsupportedLayout`。
4. 否则 `SelectedField = min(decoder.FieldSet, version)`；`version > decoder.FieldSet` 时
   `Downgraded = true`。
5. 仅当 `SelectedField == 2` 时读取 v2 扩展字段并校验 CRC32；字段集 1 不读也不校验扩展元数据。
6. 公共字段中的 width/height 必须与 PGM 画幅一致，否则 `ErrBadHeader`。

## 4. 明确拒绝的情形

| 情形 | 错误类别 |
|---|---|
| 版本不在 {1,2}（例如 3） | `ErrUnsupportedVersion` |
| 未知 flags / 保留位非 0 / reserved 字节非 0（字段集 2） | `ErrUnsupportedLayout` |
| 字段块长度与版本不符 | `ErrBadHeader` |
| magic 错误、多行版本字段、十六进制非法 | `ErrBadHeader` |
| 画幅声明与 PGM 画幅不一致 | `ErrBadHeader` |
| 画幅为 0、超过 uint16、像素区长度不等于 W×H | `ErrBadHeader` |
| PGM 非 P5、maxval≠255 | `ErrBadHeader` |
| 字段集 2 下 CRC32 不符、gamma 毫值越界、supersample=0、着色码未知 | `ErrCorrupt` |
| 解码器字段集不是 1 或 2 | 普通错误 |

注意：CRC 与扩展元数据只在字段集 2 下校验。**字段集 1 是降级路径，只保证像素**；
因此 v2 中损坏的扩展字段不影响旧解码器的降级读取，但字段集 2 会拒绝。

## 5. 降级后的比对

三份产物的像素区 SHA-256 与 CRC 完全一致；`format_test.go` 的 `TestDowngradeIsPixelIdentical`
与仓库内 `verify.sh` 分别用 Go 解码器和独立 PGM 读取器逐像素比对，差异为 0。

## 6. 复算

```sh
go run ./cmd -version 0 -out artifacts/out_v0.pgm
go run ./cmd -version 1 -decode-fieldset 1 -out artifacts/out_v1.pgm
go run ./cmd -version 2 -decode-fieldset 1 -out artifacts/out_v2.pgm
./verify.sh
```

`verify.sh` 会：
1. 复算 v0 是否与 `artifacts/baseline_legacy.pgm` 逐字节相同；
2. 用独立 P5 解析（跳过注释）解出三份文件的像素并逐像素比对；
3. 校验 v2 字段块的字节序、符号、长度以及 CRC32。
