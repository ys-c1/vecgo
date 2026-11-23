#!/usr/bin/env python3
"""独立 PNG 校验器(仅用标准库): 解析块、核对 CRC、zlib 解压,
还原 8 位灰度像素并与参考 PGM 逐字节比对。
用法: python3 -I verify_png.py <file.png> <参考.pgm>
"""
import struct
import sys
import zlib


def read_png(path):
    d = open(path, "rb").read()
    assert d[:8] == b"\x89PNG\r\n\x1a\n", "签名错误"
    pos = 8
    idat = b""
    w = h = None
    while pos < len(d):
        (n,) = struct.unpack(">I", d[pos : pos + 4])
        typ = d[pos + 4 : pos + 8]
        body = d[pos + 8 : pos + 8 + n]
        crc_off = pos + 8 + n
        (crc,) = struct.unpack(">I", d[crc_off : crc_off + 4])
        got = zlib.crc32(typ + body) & 0xFFFFFFFF
        assert got == crc, f"块 {typ} CRC 不匹配, CRC 字段偏移 {crc_off}"
        if typ == b"IHDR":
            w, h, bd, ct, comp, filt, inter = struct.unpack(">IIBBBBB", body)
            assert (bd, ct, comp, filt, inter) == (8, 0, 0, 0, 0), "非 8 位灰度"
        elif typ == b"IDAT":
            idat += body
        elif typ == b"IEND":
            break
        pos += 12 + n
    raw = zlib.decompress(idat)
    assert len(raw) == (w + 1) * h, "解压长度与画幅不符"
    pix = bytearray()
    for y in range(h):
        assert raw[y * (w + 1)] == 0, f"第 {y} 行滤波器非 None"
        pix += raw[y * (w + 1) + 1 : (y + 1) * (w + 1)]
    return w, h, bytes(pix)


def read_pgm(path):
    d = open(path, "rb").read()
    parts = d.split(b"\n", 3)
    assert parts[0] == b"P5", "非 P5 PGM"
    w, h = map(int, parts[1].split())
    assert parts[2] == b"255", "maxval 非 255"
    return w, h, parts[3]


def main():
    png_path, pgm_path = sys.argv[1], sys.argv[2]
    w, h, pix = read_png(png_path)
    w2, h2, ref = read_pgm(pgm_path)
    assert (w, h) == (w2, h2), f"画幅不一致: png {w}x{h}, pgm {w2}x{h2}"
    assert len(ref) == w * h, "PGM 像素量不符"
    assert pix == ref, "像素逐字节比对不一致"
    print(f"OK {png_path}: {w}x{h}, 像素与 {pgm_path} 逐字节一致 ({len(pix)} 字节)")


if __name__ == "__main__":
    main()
