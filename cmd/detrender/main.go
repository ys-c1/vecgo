// Command detrender 按固定输入渲染确定性核验场景，
// 输出 PGM 并打印每个场景的 sha256，供两档优化逐字节比对。
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"vecgo/pipe"
	"vecgo/scene"
)

func main() {
	outDir := flag.String("out", "", "输出目录（必填）")
	only := flag.String("scene", "", "只渲染指定场景，默认全部")
	flag.Parse()

	if *outDir == "" {
		fmt.Fprintln(os.Stderr, "失败: 必须指定 -out 输出目录")
		os.Exit(2)
	}
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "失败:", err)
		os.Exit(2)
	}

	cases := scene.Cases()
	if *only != "" {
		c, ok := scene.ByName(*only)
		if !ok {
			fmt.Fprintf(os.Stderr, "失败: 未知场景 %q\n", *only)
			os.Exit(2)
		}
		cases = []scene.Case{c}
	}

	for _, c := range cases {
		cfg := c.Cfg
		path := filepath.Join(*outDir, string(c.Name)+".pgm")
		cfg.Output = path
		im, err := pipe.Run(c.Geom, cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "失败: 场景 %s: %v\n", c.Name, err)
			os.Exit(1)
		}
		if err := im.SavePGM(path); err != nil {
			fmt.Fprintf(os.Stderr, "导出失败: 场景 %s: %v\n", c.Name, err)
			os.Exit(1)
		}
		sum := sha256.Sum256(im.Pix)
		// 哈希只覆盖像素字节；PGM 头为固定 ASCII，整文件比对另行做。
		fmt.Printf("%s  %s  %d bytes\n", hex.EncodeToString(sum[:]), c.Name, len(im.Pix))
	}
}
