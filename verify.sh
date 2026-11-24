#!/usr/bin/env bash
# 复算并校验三份版本产物。可在仓库根目录直接运行。
set -euo pipefail
cd "$(dirname "$0")"

echo "== 生成三份产物 =="
go run ./cmd -version 0 -out artifacts/out_v0.pgm
go run ./cmd -version 1 -decode-fieldset 1 -out artifacts/out_v1.pgm
go run ./cmd -version 2 -decode-fieldset 1 -out artifacts/out_v2.pgm

echo "== v0 与旧版逐字节比对 =="
cmp artifacts/out_v0.pgm artifacts/baseline_legacy.pgm
echo "OK: out_v0.pgm == baseline_legacy.pgm（逐字节相同）"

echo "== 独立 P5 读取 + 逐像素比对 + v2 字段块校验 =="
python3 -I - <<'PY'
import binascii
import hashlib
import struct
import sys

def read_p5(path):
    data = open(path, "rb").read()
    pos = 0
    def token():
        nonlocal pos
        while True:
            while pos < len(data) and data[pos] in b" \t\r\n\v\f":
                pos += 1
            if pos < len(data) and data[pos] == ord("#"):
                while pos < len(data) and data[pos] != ord("\n"):
                    pos += 1
                if pos < len(data):
                    pos += 1
                continue
            break
        start = pos
        while pos < len(data) and data[pos] not in b" \t\r\n\v\f":
            pos += 1
        return data[start:pos]

    assert token() == b"P5", path
    w = int(token())
    h = int(token())
    assert int(token()) == 255
    assert data[pos] in b" \t\r\n\v\f"
    pos += 1
    pix = data[pos:]
    assert len(pix) == w * h, (path, len(pix), w, h)
    return data, w, h, pix

files = {v: f"artifacts/out_v{v}.pgm" for v in (0, 1, 2)}
pix = {}
for v, path in files.items():
    data, w, h, p = read_p5(path)
    pix[v] = p
    print(f"v{v}: {w}x{h} 文件 {len(data)} 字节 像素 SHA-256 {hashlib.sha256(p).hexdigest()[:16]}")

assert pix[0] == pix[1] == pix[2], "三份产物像素不一致"
print("OK: v0/v1/v2 像素逐像素相同（差异 0）")

# v2 字段块：独立解析字节序、符号与长度。
data = open(files[2], "rb").read()
comment = data.split(b"\n", 2)[1]
assert comment.startswith(b"#VCFM "), comment
raw = bytes.fromhex(comment[len(b"#VCFM "):].decode())
assert len(raw) == 22, f"v2 字段块应为 22 字节，实为 {len(raw)}"
magic = raw[0:4]
ver, flags, w, h = struct.unpack_from("<HHHH", raw, 4)
gamma_milli, = struct.unpack_from("<H", raw, 12)
ss, blur, shading, reserved = raw[14], raw[15], raw[16], raw[17]
crc, = struct.unpack_from("<I", raw, 18)
assert magic == b"VCFM", magic
assert (ver, flags) == (2, 1), (ver, flags)
assert (w, h) == (64, 64), (w, h)
assert 100 <= gamma_milli <= 10000, gamma_milli
assert 1 <= ss <= 255 and blur <= 255, (ss, blur)
assert shading in (0, 1), shading
assert reserved == 0, reserved
assert crc == binascii.crc32(pix[2]) & 0xFFFFFFFF, (hex(crc), hex(binascii.crc32(pix[2])))
print(f"OK: v2 字段块 magic={magic.decode()} version={ver} flags=0x{flags:04x} "
      f"w={w} h={h} gamma_milli={gamma_milli} ss={ss} blur={blur} "
      f"shading={shading} reserved={reserved} crc32={crc:#010x}")

# v1 字段块：12 字节。
data = open(files[1], "rb").read()
comment = data.split(b"\n", 2)[1]
raw = bytes.fromhex(comment[len(b"#VCFM "):].decode())
assert len(raw) == 12, f"v1 字段块应为 12 字节，实为 {len(raw)}"
ver, flags, w, h = struct.unpack_from("<HHHH", raw, 4)
assert (raw[0:4], ver, flags, w, h) == (b"VCFM", 1, 0, 64, 64), (raw[0:4], ver, flags, w, h)
print(f"OK: v1 字段块 12 字节 magic={raw[0:4].decode()} version={ver} flags=0x{flags:04x} w={w} h={h}")

# v0 必须没有版本字段。
assert b"VCFM" not in pix[0] and b"VCFM" not in open(files[0], "rb").read()[:64]
print("OK: v0 无版本字段")
PY

echo "所有校验通过。"
