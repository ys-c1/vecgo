// pngcheck 是自写的 PNG 校验器: 校验签名、块结构、逐块 CRC,
// 解码像素; 可选与参考 PGM 逐字节比对。
// 退出码即 pngx 错误码: 1=宽高为零 2=像素越界 3=CRC 不匹配 4=其他结构错误 5=像素比对不一致。
package main

import (
	"fmt"
	"os"

	"vecgo/pngx"
)

func fail(e *pngx.Error) {
	fmt.Fprintln(os.Stderr, "校验失败:", e)
	os.Exit(int(e.Code))
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "用法: pngcheck <file.png> [参考.pgm]")
		os.Exit(64)
	}
	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "读取失败:", err)
		os.Exit(66)
	}
	if e := pngx.Validate(data); e != nil {
		fail(e)
	}
	w, h, pix, e := pngx.Decode(data)
	if e != nil {
		fail(e)
	}
	fmt.Printf("OK %s: %dx%d, 文件 %d 字节, 像素 %d 字节, CRC 全部通过\n",
		os.Args[1], w, h, len(data), len(pix))
	if len(os.Args) > 2 {
		ref, err := readPGM(os.Args[2])
		if err != nil {
			fmt.Fprintln(os.Stderr, "参考 PGM 读取失败:", err)
			os.Exit(66)
		}
		if len(ref) != len(pix) {
			fmt.Fprintf(os.Stderr, "像素量不一致: png %d, pgm %d\n", len(pix), len(ref))
			os.Exit(5)
		}
		for i := range pix {
			if pix[i] != ref[i] {
				fmt.Fprintf(os.Stderr, "像素比对不一致于偏移 %d: png=%d pgm=%d\n", i, pix[i], ref[i])
				os.Exit(5)
			}
		}
		fmt.Printf("像素与 %s 逐字节一致 (%d 字节)\n", os.Args[2], len(pix))
	}
}

// readPGM 读取 SavePGM 写出的 P5 文件, 返回像素区。
func readPGM(path string) ([]byte, error) {
	d, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	// 头部固定三行: "P5\n<w> <h>\n255\n"
	nl := 0
	for i, b := range d {
		if b == '\n' {
			nl++
			if nl == 3 {
				return d[i+1:], nil
			}
		}
	}
	return nil, fmt.Errorf("PGM 头不完整")
}
