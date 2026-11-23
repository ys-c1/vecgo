// Package pngx 自实现 PNG(8 位灰度) 编码、校验与解码。
// 不依赖第三方库, 也不使用标准库 image/png 与 compress/zlib:
// CRC32(IEEE)、Adler-32、zlib 存储块封装均为本包手写。
package pngx

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// Code 为独立错误码, 同时用作 pngcheck 的进程退出码。
type Code int

const (
	CodeZeroDim     Code = 1 // 宽高为零
	CodePixelRange  Code = 2 // 像素数据越界(数量与画幅不符)
	CodeCRCMismatch Code = 3 // CRC/校验和不匹配
	CodeFormat      Code = 4 // 其他结构错误(签名、截断、不支持的特性)
)

// Error 携带错误码; CRC 错误时 Offset 为出错 CRC 字段在文件中的字节偏移。
type Error struct {
	Code   Code
	Offset int64
	Msg    string
}

func (e *Error) Error() string {
	if e.Code == CodeCRCMismatch && e.Offset >= 0 {
		return fmt.Sprintf("png: %s (CRC 字段偏移 %d)", e.Msg, e.Offset)
	}
	return "png: " + e.Msg
}

// PNG 文件签名, 8 字节固定。
var sig = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}

// ---- CRC32 (IEEE 802.3, 多项式 0xEDB88320) ----

var crcTable [256]uint32

func init() {
	for i := 0; i < 256; i++ {
		c := uint32(i)
		for k := 0; k < 8; k++ {
			if c&1 != 0 {
				c = 0xEDB88320 ^ (c >> 1)
			} else {
				c >>= 1
			}
		}
		crcTable[i] = c
	}
}

func crc32Update(c uint32, data []byte) uint32 {
	c = ^c
	for _, b := range data {
		c = crcTable[byte(c)^b] ^ (c >> 8)
	}
	return ^c
}

// CRC32 计算 IEEE CRC-32, 供编码与自检使用。
func CRC32(data []byte) uint32 { return crc32Update(0, data) }

// ---- Adler-32 (zlib 尾部校验) ----

func adler32(data []byte) uint32 {
	var a, b uint32 = 1, 0
	// 5552 是 b 不溢出 32 位的最大块长, 分块取模避免逐字节取模。
	for len(data) > 0 {
		n := len(data)
		if n > 5552 {
			n = 5552
		}
		for _, x := range data[:n] {
			a += uint32(x)
			b += a
		}
		a %= 65521
		b %= 65521
		data = data[n:]
	}
	return b<<16 | a
}

// ---- zlib 封装( deflate 存储块, 不压缩 ) ----

func zlibStore(data []byte) []byte {
	out := []byte{0x78, 0x01} // CMF/FLG: deflate, 32K 窗口, 无字典, 校验位凑整
	sum := adler32(data)      // 对压缩前的原始数据计算
	for {
		n := len(data)
		if n > 65535 {
			n = 65535
		}
		final := byte(0)
		if n == len(data) {
			final = 1 // BFINAL
		}
		l := uint16(n)
		out = append(out, final, byte(l), byte(l>>8), byte(^l), byte(^l>>8))
		out = append(out, data[:n]...)
		data = data[n:]
		if final == 1 {
			break
		}
	}
	var sumb [4]byte
	binary.BigEndian.PutUint32(sumb[:], sum)
	return append(out, sumb[:]...)
}

// ---- 块写入 ----

func appendChunk(out []byte, typ string, data []byte) []byte {
	var lenb [4]byte
	binary.BigEndian.PutUint32(lenb[:], uint32(len(data)))
	out = append(out, lenb[:]...)
	out = append(out, typ...)
	out = append(out, data...)
	c := crc32Update(0, []byte(typ))
	c = crc32Update(c, data)
	var crcb [4]byte
	binary.BigEndian.PutUint32(crcb[:], c)
	return append(out, crcb[:]...)
}

// Encode 把 w×h 的 8 位灰度像素编码为 PNG。
// 宽高非正返回 CodeZeroDim; 像素数量与画幅不符返回 CodePixelRange。
// 编码完成后立即回验全部 CRC(自检), 失败返回 CodeCRCMismatch。
func Encode(w, h int, pix []byte) ([]byte, *Error) {
	if w <= 0 || h <= 0 {
		return nil, &Error{Code: CodeZeroDim, Offset: -1,
			Msg: fmt.Sprintf("宽高必须为正: %dx%d", w, h)}
	}
	if len(pix) != w*h {
		return nil, &Error{Code: CodePixelRange, Offset: -1,
			Msg: fmt.Sprintf("像素数据长度 %d 越界, 画幅 %dx%d 需要 %d", len(pix), w, h, w*h)}
	}
	// 每行前置一个滤波器字节 0 (None)。
	raw := make([]byte, 0, (w+1)*h)
	for y := 0; y < h; y++ {
		raw = append(raw, 0)
		raw = append(raw, pix[y*w:(y+1)*w]...)
	}
	out := append([]byte{}, sig...)
	var ihdr [13]byte
	binary.BigEndian.PutUint32(ihdr[0:4], uint32(w))
	binary.BigEndian.PutUint32(ihdr[4:8], uint32(h))
	ihdr[8] = 8 // 位深 8
	ihdr[9] = 0 // 颜色类型 0: 灰度
	// ihdr[10..12] = 0: 压缩=deflate, 滤波=标准, 隔行=无
	out = appendChunk(out, "IHDR", ihdr[:])
	out = appendChunk(out, "IDAT", zlibStore(raw))
	out = appendChunk(out, "IEND", nil)
	if e := Validate(out); e != nil { // 自检
		return nil, e
	}
	return out, nil
}

// Validate 校验 PNG 字节流: 签名、块结构、逐块 CRC。
// CRC 不匹配时返回 CodeCRCMismatch, Offset 为该 CRC 字段的字节偏移。
func Validate(data []byte) *Error {
	if len(data) < 8 || !bytes.Equal(data[:8], sig) {
		return &Error{Code: CodeFormat, Offset: -1, Msg: "签名错误, 不是 PNG"}
	}
	pos := 8
	first := true
	for pos < len(data) {
		chunkStart := pos
		if pos+8 > len(data) {
			return &Error{Code: CodeFormat, Offset: int64(pos), Msg: "块头截断"}
		}
		n := binary.BigEndian.Uint32(data[pos:])
		typ := string(data[pos+4 : pos+8])
		pos += 8
		if uint64(pos)+uint64(n)+4 > uint64(len(data)) {
			return &Error{Code: CodeFormat, Offset: int64(chunkStart),
				Msg: fmt.Sprintf("块 %s 数据截断", typ)}
		}
		body := data[pos : pos+int(n)]
		crcOff := pos + int(n)
		want := binary.BigEndian.Uint32(data[crcOff:])
		got := crc32Update(crc32Update(0, []byte(typ)), body)
		if want != got {
			return &Error{Code: CodeCRCMismatch, Offset: int64(crcOff),
				Msg: fmt.Sprintf("块 %s (块起始偏移 %d) CRC 不匹配: 记录 %08x, 实算 %08x",
					typ, chunkStart, want, got)}
		}
		if first && typ != "IHDR" {
			return &Error{Code: CodeFormat, Offset: int64(chunkStart), Msg: "首块必须是 IHDR"}
		}
		first = false
		pos = crcOff + 4
		if typ == "IEND" {
			return nil
		}
	}
	return &Error{Code: CodeFormat, Offset: -1, Msg: "缺少 IEND"}
}

// Decode 校验并解码本包产物(8 位灰度, 滤波器 None, zlib 存储块)。
// IHDR 宽高为零返回 CodeZeroDim; 解压后像素量与画幅不符返回 CodePixelRange。
func Decode(data []byte) (w, h int, pix []byte, err *Error) {
	if e := Validate(data); e != nil {
		return 0, 0, nil, e
	}
	ihdr := data[16:29] // 签名 8 + 长度 4 + "IHDR" 4
	w = int(binary.BigEndian.Uint32(ihdr[0:4]))
	h = int(binary.BigEndian.Uint32(ihdr[4:8]))
	if w == 0 || h == 0 {
		return 0, 0, nil, &Error{Code: CodeZeroDim, Offset: 16,
			Msg: fmt.Sprintf("IHDR 宽高为零: %dx%d", w, h)}
	}
	if w > 1<<24 || h > 1<<24 {
		return 0, 0, nil, &Error{Code: CodeFormat, Offset: 16, Msg: "画幅超出安全上限"}
	}
	if ihdr[8] != 8 || ihdr[9] != 0 {
		return 0, 0, nil, &Error{Code: CodeFormat, Offset: 16,
			Msg: fmt.Sprintf("仅支持 8 位灰度, 实际位深 %d 颜色类型 %d", ihdr[8], ihdr[9])}
	}
	var idat []byte
	pos := 8
	for pos+8 <= len(data) {
		n := int(binary.BigEndian.Uint32(data[pos:]))
		typ := string(data[pos+4 : pos+8])
		if typ == "IDAT" {
			idat = append(idat, data[pos+8:pos+8+n]...)
		}
		pos += 8 + n + 4
		if typ == "IEND" {
			break
		}
	}
	raw, e := inflateStored(idat)
	if e != nil {
		return 0, 0, nil, e
	}
	if len(raw) != (w+1)*h {
		return 0, 0, nil, &Error{Code: CodePixelRange, Offset: -1,
			Msg: fmt.Sprintf("解压得 %d 字节, 画幅 %dx%d 需要 %d", len(raw), w, h, (w+1)*h)}
	}
	pix = make([]byte, w*h)
	for y := 0; y < h; y++ {
		if raw[y*(w+1)] != 0 {
			return 0, 0, nil, &Error{Code: CodeFormat, Offset: int64(y * (w + 1)),
				Msg: fmt.Sprintf("第 %d 行滤波器非 None", y)}
		}
		copy(pix[y*w:], raw[y*(w+1)+1:(y+1)*(w+1)])
	}
	return w, h, pix, nil
}

// inflateStored 解开 zlib 封装的 deflate 存储块, 并核对 Adler-32。
func inflateStored(d []byte) ([]byte, *Error) {
	if len(d) < 6 {
		return nil, &Error{Code: CodeFormat, Offset: -1, Msg: "zlib 流截断"}
	}
	cmf, flg := d[0], d[1]
	if cmf&0x0f != 8 || (int(cmf)<<8|int(flg))%31 != 0 {
		return nil, &Error{Code: CodeFormat, Offset: 0, Msg: "zlib 头非法"}
	}
	if flg&0x20 != 0 {
		return nil, &Error{Code: CodeFormat, Offset: 1, Msg: "不支持预设字典"}
	}
	pos := 2
	end := len(d) - 4 // 尾部 4 字节为 Adler-32
	var out []byte
	for {
		if pos+5 > end {
			return nil, &Error{Code: CodeFormat, Offset: int64(pos), Msg: "deflate 块头截断"}
		}
		hdr := d[pos]
		final := hdr & 1
		btype := (hdr >> 1) & 3
		if btype != 0 {
			return nil, &Error{Code: CodeFormat, Offset: int64(pos),
				Msg: fmt.Sprintf("仅支持存储块, 遇到块类型 %d", btype)}
		}
		n := int(d[pos+1]) | int(d[pos+2])<<8
		nl := int(d[pos+3]) | int(d[pos+4])<<8
		if n != nl^0xffff {
			return nil, &Error{Code: CodeFormat, Offset: int64(pos + 1),
				Msg: "deflate LEN/NLEN 不匹配"}
		}
		pos += 5
		if pos+n > end {
			return nil, &Error{Code: CodeFormat, Offset: int64(pos), Msg: "deflate 数据截断"}
		}
		out = append(out, d[pos:pos+n]...)
		pos += n
		if final == 1 {
			break
		}
	}
	if pos != end {
		return nil, &Error{Code: CodeFormat, Offset: int64(pos), Msg: "IDAT 尾部多余数据"}
	}
	want := binary.BigEndian.Uint32(d[end:])
	if got := adler32(out); want != got {
		return nil, &Error{Code: CodeCRCMismatch, Offset: -1,
			Msg: fmt.Sprintf("zlib Adler-32 不匹配: 记录 %08x, 实算 %08x", want, got)}
	}
	return out, nil
}
