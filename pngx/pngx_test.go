package pngx

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"
)

func gradient(w, h int) []byte {
	pix := make([]byte, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			pix[y*w+x] = byte((x + y) % 256)
		}
	}
	return pix
}

func solid(w, h int, v byte) []byte {
	pix := make([]byte, w*h)
	for i := range pix {
		pix[i] = v
	}
	return pix
}

func roundtrip(t *testing.T, w, h int, pix []byte) []byte {
	t.Helper()
	data, err := Encode(w, h, pix)
	if err != nil {
		t.Fatalf("Encode 失败: %v", err)
	}
	if e := Validate(data); e != nil {
		t.Fatalf("Validate 失败: %v", e)
	}
	gw, gh, gpix, e := Decode(data)
	if e != nil {
		t.Fatalf("Decode 失败: %v", e)
	}
	if gw != w || gh != h {
		t.Fatalf("画幅不一致: 得 %dx%d, 要 %dx%d", gw, gh, w, h)
	}
	if !bytes.Equal(gpix, pix) {
		t.Fatal("像素逐字节比对不一致")
	}
	return data
}

// 五类用例: 小图、大图、全黑、全白、单像素。

func TestSmallImage(t *testing.T)  { roundtrip(t, 3, 2, gradient(3, 2)) }
func TestLargeImage(t *testing.T)  { roundtrip(t, 1024, 768, gradient(1024, 768)) }
func TestAllBlack(t *testing.T)    { roundtrip(t, 64, 64, solid(64, 64, 0)) }
func TestAllWhite(t *testing.T)    { roundtrip(t, 64, 64, solid(64, 64, 255)) }
func TestSinglePixel(t *testing.T) { roundtrip(t, 1, 1, []byte{200}) }

// 三类错误输入, 各配最小可复现输入。

func TestErrZeroDim(t *testing.T) {
	// 最小输入: 宽为 0
	if _, e := Encode(0, 1, nil); e == nil || e.Code != CodeZeroDim {
		t.Fatalf("期望 CodeZeroDim, 得 %v", e)
	}
	if _, e := Encode(1, 0, nil); e == nil || e.Code != CodeZeroDim {
		t.Fatalf("期望 CodeZeroDim, 得 %v", e)
	}
}

func TestErrPixelRange(t *testing.T) {
	// 最小输入: 2x2 画幅只给 3 个像素
	if _, e := Encode(2, 2, []byte{1, 2, 3}); e == nil || e.Code != CodePixelRange {
		t.Fatalf("期望 CodePixelRange, 得 %v", e)
	}
	// 多给也不行
	if _, e := Encode(1, 1, []byte{1, 2}); e == nil || e.Code != CodePixelRange {
		t.Fatalf("期望 CodePixelRange, 得 %v", e)
	}
}

func TestErrCRCMismatch(t *testing.T) {
	// 最小输入: 1x1 合法产物, 翻转 IEND 的 CRC 字段最低字节
	data, err := Encode(1, 1, []byte{7})
	if err != nil {
		t.Fatalf("Encode 失败: %v", err)
	}
	bad := append([]byte{}, data...)
	off := len(bad) - 4 // IEND 的 CRC 字段
	bad[off] ^= 0xFF
	e := Validate(bad)
	if e == nil || e.Code != CodeCRCMismatch {
		t.Fatalf("期望 CodeCRCMismatch, 得 %v", e)
	}
	if e.Offset != int64(off) {
		t.Fatalf("偏移量错误: 报 %d, 实际 %d", e.Offset, off)
	}
	t.Logf("自检报告: %v", e)
}

// 解码端同样能识别三类错误(针对文件输入)。

func TestDecodeZeroDim(t *testing.T) {
	// 构造: 合法产物把 IHDR 宽改为 0 并重算 CRC
	data, _ := Encode(4, 4, make([]byte, 16))
	bad := append([]byte{}, data...)
	for i := 16; i < 20; i++ { // IHDR 宽度字段
		bad[i] = 0
	}
	binary.BigEndian.PutUint32(bad[29:33], CRC32(bad[12:29])) // 重算 IHDR CRC
	if _, _, _, e := Decode(bad); e == nil || e.Code != CodeZeroDim {
		t.Fatalf("期望 CodeZeroDim, 得 %v", e)
	}
}

func TestDecodePixelRange(t *testing.T) {
	// 构造: 合法产物把 IHDR 高 2 改为 3, 像素量即与画幅不符
	data, _ := Encode(2, 2, []byte{1, 2, 3, 4})
	bad := append([]byte{}, data...)
	binary.BigEndian.PutUint32(bad[20:24], 3) // IHDR 高度字段
	binary.BigEndian.PutUint32(bad[29:33], CRC32(bad[12:29]))
	if _, _, _, e := Decode(bad); e == nil || e.Code != CodePixelRange {
		t.Fatalf("期望 CodePixelRange, 得 %v", e)
	}
}

// 两档画幅: 各报产物体积与编码耗时。

func TestEncodeMetrics(t *testing.T) {
	for _, s := range []int{64, 1024} {
		pix := gradient(s, s)
		start := time.Now()
		data, err := Encode(s, s, pix)
		if err != nil {
			t.Fatalf("Encode 失败: %v", err)
		}
		ms := float64(time.Since(start).Microseconds()) / 1000
		t.Logf("%dx%d: png=%d 字节, 编码=%.3f ms", s, s, len(data), ms)
	}
}
