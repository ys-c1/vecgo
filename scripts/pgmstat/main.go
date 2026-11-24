// pgmstat 读取 P5 格式 PGM，输出宽、高与非零像素数，供校验脚本判断黑帧。
package main

import (
	"bufio"
	"fmt"
	"os"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "用法: pgmstat <file.pgm>")
		os.Exit(2)
	}
	f, err := os.Open(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer f.Close()

	r := bufio.NewReader(f)
	var magic string
	var w, h, maxv int
	if _, err := fmt.Fscanln(r, &magic); err != nil || magic != "P5" {
		fmt.Fprintln(os.Stderr, "不是 P5 PGM")
		os.Exit(1)
	}
	if _, err := fmt.Fscan(r, &w, &h, &maxv); err != nil {
		fmt.Fprintln(os.Stderr, "PGM 头解析失败:", err)
		os.Exit(1)
	}
	if _, err := r.ReadByte(); err != nil { // 吃掉最大值后的单个换行
		fmt.Fprintln(os.Stderr, "PGM 头不完整:", err)
		os.Exit(1)
	}

	pix := make([]byte, w*h)
	if _, err := readFull(r, pix); err != nil {
		fmt.Fprintln(os.Stderr, "PGM 像素数据不完整:", err)
		os.Exit(1)
	}
	on := 0
	for _, v := range pix {
		if v != 0 {
			on++
		}
	}
	fmt.Printf("file=%s w=%d h=%d nonzero=%d black=%v\n",
		os.Args[1], w, h, on, on == 0)
}

func readFull(r *bufio.Reader, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := r.Read(buf[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}
