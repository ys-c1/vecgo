// Package format 定义 vecgo 输出文件的版本化编码与解码。
//
// 兼容性原则：
//   - version 0（不写版本字段）输出与旧版 SavePGM 逐字节相同。
//   - 带版本的输出仍是合法 PGM（P5），版本字段与元数据放在一行 PGM 注释里，
//     因此普通 PGM 解码器可以跳过注释继续读像素。
//   - 像素区在各版本之间位置与布局不变，所以旧解码器（字段集 1）可以把
//     新文件（字段集 2）按字段集 1 降级读取，得到的像素与旧版逐字节一致。
//   - 未登记的版本、未知布局标志、长度/尺寸不一致等一律明确拒绝。
//
// 字段的字节序、符号性与长度见仓库根目录 FORMAT.md。
package format

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash/crc32"
	"math"
	"strings"

	"vecgo/config"
	"vecgo/raster"
)

const (
	// VersionLegacy 表示不写版本字段，输出与旧版逐字节相同。
	VersionLegacy = 0
	// Version1 是旧字段集：版本、标志、宽、高。
	Version1 = 1
	// Version2 追加 gamma、超采样、模糊、着色与像素校验和。
	Version2 = 2
)

const (
	// magic 是字段块的前 4 个字节，ASCII。
	magic = "VCFM"
	// commentPrefix 是带版本文件头的 PGM 注释前缀。
	commentPrefix = "#VCFM "

	payloadV1Len = 12
	payloadV2Len = 22

	// flagExtendedMeta 表示字段块在 v1 公共字段之后追加了 v2 元数据。
	// 该标志只增加元数据，不移动像素区，因此旧解码器可以降级。
	flagExtendedMeta uint16 = 1 << 0
	knownFlags       uint16 = flagExtendedMeta

	shadingFlat     uint8 = 0
	shadingGradient uint8 = 1
)

// 明确拒绝的错误类别，调用方可用 errors.Is 判断。
var (
	ErrUnsupportedVersion = errors.New("format: 不支持的版本")
	ErrUnsupportedLayout  = errors.New("format: 不支持的布局")
	ErrCorrupt            = errors.New("format: 文件损坏")
	ErrBadHeader          = errors.New("format: 文件头非法")
)

// Meta 是 v2 字段块携带的渲染参数。
type Meta struct {
	Gamma       float64
	Supersample int
	Blur        int
	Shading     string
}

// MetaFromConfig 按 config 生成可编码的元数据。
func MetaFromConfig(c config.Config) Meta {
	return Meta{Gamma: c.Gamma, Supersample: c.Supersample, Blur: c.Blur, Shading: c.Shading}
}

// Decoder 表示某个解码器支持的字段集。
type Decoder struct {
	FieldSet int
}

// DecoderV1 是旧解码器：只使用字段集 1，可把 v2 降级读取。
var DecoderV1 = Decoder{FieldSet: 1}

// DecoderV2 是当前解码器：字段集 1、2 都能完整读取。
var DecoderV2 = Decoder{FieldSet: 2}

// Decoded 是解码结果。
type Decoded struct {
	Image         *raster.Image
	Meta          Meta
	MetadataKnown bool // false 表示该版本字段集没有携带这些元数据
	FileVersion   int  // 0 表示文件未写版本字段
	NativeVersion int  // 文件自身字段集对应的版本
	SelectedField int  // 本次解码实际使用的字段集
	Downgraded    bool // 文件版本字段集高于解码器字段集
}

// Encode 按 version 编码图像。
//
// version=0 时不写版本字段，字节与旧版 SavePGM 完全一致。
// version=1、2 时写入 PGM 注释形式的版本字段块。
func Encode(im *raster.Image, m Meta, version int) ([]byte, error) {
	if im == nil || im.W <= 0 || im.H <= 0 {
		return nil, errors.New("format: 空图不能编码")
	}
	if len(im.Pix) != im.W*im.H {
		return nil, fmt.Errorf("format: 像素数 %d 与 %dx%d 不一致", len(im.Pix), im.W, im.H)
	}
	if im.W > 65535 || im.H > 65535 {
		return nil, fmt.Errorf("format: 画幅 %dx%d 超出 uint16 字段范围", im.W, im.H)
	}

	switch version {
	case VersionLegacy:
		return encodeLegacy(im), nil
	case Version1:
		p, err := encodePayload(im, m, Version1)
		if err != nil {
			return nil, err
		}
		return wrapVersioned(im, p), nil
	case Version2:
		p, err := encodePayload(im, m, Version2)
		if err != nil {
			return nil, err
		}
		return wrapVersioned(im, p), nil
	default:
		return nil, fmt.Errorf("%w: 写入版本 %d（仅支持 0、1、2）", ErrUnsupportedVersion, version)
	}
}

func encodeLegacy(im *raster.Image) []byte {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "P5\n%d %d\n255\n", im.W, im.H)
	buf.Write(im.Pix)
	return buf.Bytes()
}

func encodePayload(im *raster.Image, m Meta, version int) ([]byte, error) {
	p := make([]byte, payloadV1Len)
	if version == Version2 {
		p = make([]byte, payloadV2Len)
	}
	copy(p[0:4], magic)
	binary.LittleEndian.PutUint16(p[4:6], uint16(version))
	flags := uint16(0)
	if version == Version2 {
		flags = flagExtendedMeta
	}
	binary.LittleEndian.PutUint16(p[6:8], flags)
	binary.LittleEndian.PutUint16(p[8:10], uint16(im.W))
	binary.LittleEndian.PutUint16(p[10:12], uint16(im.H))

	if version == Version2 {
		if math.IsNaN(m.Gamma) || m.Gamma < 0.1 || m.Gamma > 10 {
			return nil, fmt.Errorf("format: gamma 超出可编码范围: %v", m.Gamma)
		}
		gm := int(math.Round(m.Gamma * 1000))
		if gm < 100 || gm > 10000 {
			return nil, fmt.Errorf("format: gamma 毫值超出 uint16 范围: %d", gm)
		}
		if m.Supersample < 1 || m.Supersample > 255 {
			return nil, fmt.Errorf("format: supersample 超出 uint8 范围: %d", m.Supersample)
		}
		if m.Blur < 0 || m.Blur > 255 {
			return nil, fmt.Errorf("format: blur 超出 uint8 范围: %d", m.Blur)
		}
		var sc uint8
		switch m.Shading {
		case "flat":
			sc = shadingFlat
		case "gradient":
			sc = shadingGradient
		default:
			return nil, fmt.Errorf("format: 未知着色模式: %q", m.Shading)
		}
		binary.LittleEndian.PutUint16(p[12:14], uint16(gm))
		p[14] = uint8(m.Supersample)
		p[15] = uint8(m.Blur)
		p[16] = sc
		p[17] = 0 // reserved，写入必须为 0
		binary.LittleEndian.PutUint32(p[18:22], crc32.ChecksumIEEE(im.Pix))
	}
	return p, nil
}

func wrapVersioned(im *raster.Image, payload []byte) []byte {
	var buf bytes.Buffer
	buf.WriteString("P5\n")
	buf.WriteString(commentPrefix)
	buf.WriteString(hex.EncodeToString(payload))
	buf.WriteByte('\n')
	fmt.Fprintf(&buf, "%d %d\n255\n", im.W, im.H)
	buf.Write(im.Pix)
	return buf.Bytes()
}

// Decode 解析文件，并按 d.FieldSet 选择字段集。
//
// 无版本字段（v0）以及 v1 文件都按字段集 1 读取；v2 文件在
// DecoderV1 下按字段集 1 降级读取，在 DecoderV2 下按字段集 2 读取。
func Decode(data []byte, d Decoder) (*Decoded, error) {
	if d.FieldSet != 1 && d.FieldSet != 2 {
		return nil, fmt.Errorf("format: 解码器字段集非法: %d", d.FieldSet)
	}
	w, h, comments, pix, err := parsePGM(data)
	if err != nil {
		return nil, err
	}

	out := &Decoded{
		FileVersion:   0,
		NativeVersion: Version1,
		SelectedField: 1,
		Meta:          Meta{},
		MetadataKnown: false,
	}

	var verComment string
	for _, c := range comments {
		if strings.HasPrefix(c, magic) || strings.HasPrefix(c, magic+" ") {
			if verComment != "" {
				return nil, fmt.Errorf("%w: 出现多个版本字段注释", ErrBadHeader)
			}
			verComment = c
		}
	}

	if verComment != "" {
		if !strings.HasPrefix(verComment, magic+" ") {
			return nil, fmt.Errorf("%w: 版本字段注释缺少空格分隔", ErrBadHeader)
		}
		raw, err := hex.DecodeString(strings.TrimSpace(verComment[len(magic)+1:]))
		if err != nil {
			return nil, fmt.Errorf("%w: 版本字段块不是合法十六进制: %v", ErrBadHeader, err)
		}
		if len(raw) < payloadV1Len {
			return nil, fmt.Errorf("%w: 版本字段块长度 %d，至少需要 %d", ErrBadHeader, len(raw), payloadV1Len)
		}
		if string(raw[0:4]) != magic {
			return nil, fmt.Errorf("%w: 版本字段块魔数错误", ErrBadHeader)
		}
		ver := binary.LittleEndian.Uint16(raw[4:6])
		flags := binary.LittleEndian.Uint16(raw[6:8])
		fw := int(binary.LittleEndian.Uint16(raw[8:10]))
		fh := int(binary.LittleEndian.Uint16(raw[10:12]))

		switch ver {
		case Version1:
			if len(raw) != payloadV1Len {
				return nil, fmt.Errorf("%w: v1 字段块长度 %d，应为 %d", ErrBadHeader, len(raw), payloadV1Len)
			}
			if flags != 0 {
				return nil, fmt.Errorf("%w: v1 布局标志 0x%04x，应为 0", ErrUnsupportedLayout, flags)
			}
		case Version2:
			if len(raw) != payloadV2Len {
				return nil, fmt.Errorf("%w: v2 字段块长度 %d，应为 %d", ErrBadHeader, len(raw), payloadV2Len)
			}
			if flags&^knownFlags != 0 || flags != flagExtendedMeta {
				return nil, fmt.Errorf("%w: v2 布局标志 0x%04x，仅允许 bit0", ErrUnsupportedLayout, flags)
			}
		default:
			return nil, fmt.Errorf("%w: 版本 %d（仅登记 1、2）", ErrUnsupportedVersion, ver)
		}

		if fw != w || fh != h {
			return nil, fmt.Errorf("%w: 版本字段声明画幅 %dx%d 与 PGM 画幅 %dx%d 不一致", ErrBadHeader, fw, fh, w, h)
		}

		out.FileVersion = int(ver)
		out.NativeVersion = int(ver)
		out.SelectedField = d.FieldSet
		if int(ver) < d.FieldSet {
			out.SelectedField = int(ver)
		}
		out.Downgraded = d.FieldSet < int(ver)

		if ver == Version2 && d.FieldSet == 2 {
			gm := binary.LittleEndian.Uint16(raw[12:14])
			ss := raw[14]
			blur := raw[15]
			sh := raw[16]
			reserved := raw[17]
			wantCRC := binary.LittleEndian.Uint32(raw[18:22])
			if reserved != 0 {
				return nil, fmt.Errorf("%w: v2 保留字节为 %d，应为 0", ErrUnsupportedLayout, reserved)
			}
			if gm < 100 || gm > 10000 {
				return nil, fmt.Errorf("%w: gamma 毫值 %d 超出 [100,10000]", ErrCorrupt, gm)
			}
			if ss < 1 {
				return nil, fmt.Errorf("%w: supersample 为 0", ErrCorrupt)
			}
			if sh != shadingFlat && sh != shadingGradient {
				return nil, fmt.Errorf("%w: 着色码 %d 未知", ErrCorrupt, sh)
			}
			if got := crc32.ChecksumIEEE(pix); got != wantCRC {
				return nil, fmt.Errorf("%w: 像素 CRC32 不符（文件 %08x，实算 %08x）", ErrCorrupt, wantCRC, got)
			}
			out.Meta = Meta{
				Gamma:       float64(gm) / 1000,
				Supersample: int(ss),
				Blur:        int(blur),
				Shading:     shadingName(sh),
			}
			out.MetadataKnown = true
		}
	}

	im, err := raster.New(w, h)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadHeader, err)
	}
	copy(im.Pix, pix)
	out.Image = im
	return out, nil
}

func shadingName(code uint8) string {
	if code == shadingGradient {
		return "gradient"
	}
	return "flat"
}

func parsePGM(data []byte) (w, h int, comments []string, pix []byte, err error) {
	pos := 0
	tok, err := nextToken(data, &pos, &comments)
	if err != nil {
		return 0, 0, nil, nil, err
	}
	if tok != "P5" {
		return 0, 0, nil, nil, fmt.Errorf("%w: 魔数 %q 不是 P5", ErrBadHeader, tok)
	}
	tok, err = nextToken(data, &pos, &comments)
	if err != nil {
		return 0, 0, nil, nil, err
	}
	if w, err = parseDim(tok); err != nil {
		return 0, 0, nil, nil, err
	}
	tok, err = nextToken(data, &pos, &comments)
	if err != nil {
		return 0, 0, nil, nil, err
	}
	if h, err = parseDim(tok); err != nil {
		return 0, 0, nil, nil, err
	}
	tok, err = nextToken(data, &pos, &comments)
	if err != nil {
		return 0, 0, nil, nil, err
	}
	if tok != "255" {
		return 0, 0, nil, nil, fmt.Errorf("%w: maxval %q 不是 255", ErrBadHeader, tok)
	}
	if pos >= len(data) || !isSpace(data[pos]) {
		return 0, 0, nil, nil, fmt.Errorf("%w: maxval 后缺少分隔空白", ErrBadHeader)
	}
	pos++ // PGM 规定 maxval 后恰好一个空白字节
	want, ok := mulOK(w, h)
	if !ok {
		return 0, 0, nil, nil, fmt.Errorf("%w: 画幅 %dx%d 过大", ErrBadHeader, w, h)
	}
	if len(data)-pos != want {
		return 0, 0, nil, nil, fmt.Errorf("%w: 像素区长度 %d，应为 %d", ErrBadHeader, len(data)-pos, want)
	}
	return w, h, comments, data[pos:], nil
}

func parseDim(tok string) (int, error) {
	n := 0
	if tok == "" {
		return 0, fmt.Errorf("%w: 画幅字段为空", ErrBadHeader)
	}
	for i := 0; i < len(tok); i++ {
		c := tok[i]
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("%w: 画幅字段 %q 非十进制", ErrBadHeader, tok)
		}
		n = n*10 + int(c-'0')
		if n > 65535 {
			return 0, fmt.Errorf("%w: 画幅字段 %q 超出 uint16", ErrBadHeader, tok)
		}
	}
	if n <= 0 {
		return 0, fmt.Errorf("%w: 画幅必须为正", ErrBadHeader)
	}
	return n, nil
}

func mulOK(w, h int) (int, bool) {
	if w <= 0 || h <= 0 {
		return 0, false
	}
	if w > (1<<31-1)/h {
		return 0, false
	}
	return w * h, true
}

func nextToken(data []byte, pos *int, comments *[]string) (string, error) {
	for {
		for *pos < len(data) && isSpace(data[*pos]) {
			*pos++
		}
		if *pos < len(data) && data[*pos] == '#' {
			start := *pos + 1
			end := start
			for end < len(data) && data[end] != '\n' {
				end++
			}
			*comments = append(*comments, string(data[start:end]))
			*pos = end
			if *pos < len(data) && data[*pos] == '\n' {
				*pos++
			}
			continue
		}
		break
	}
	start := *pos
	for *pos < len(data) && !isSpace(data[*pos]) {
		*pos++
	}
	if start == *pos {
		return "", fmt.Errorf("%w: 文件头意外结束", ErrBadHeader)
	}
	return string(data[start:*pos]), nil
}

func isSpace(b byte) bool {
	switch b {
	case ' ', '\t', '\n', '\r', '\v', '\f':
		return true
	}
	return false
}
