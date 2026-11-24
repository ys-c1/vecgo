package main

import (
	"flag"
	"fmt"
	"os"

	"vecgo/config"
	"vecgo/format"
	"vecgo/geom"
	"vecgo/pipe"
)

func main() {
	version := flag.Int("version", format.VersionLegacy, "输出文件版本：0=不写版本字段（与旧版逐字节相同），1/2=写版本字段")
	decode := flag.Int("decode-fieldset", 0, "解码字段集：0=不解码，1=旧解码器，2=当前解码器")
	outPath := flag.String("out", "", "输出路径，留空沿用 config 默认")
	flag.Parse()

	cfg := config.Default()
	scene := [][]geom.Point{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}}
	im, err := pipe.Run(scene, cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "失败:", err)
		os.Exit(1)
	}
	if flag.NArg() > 0 {
		cfg.Output = flag.Arg(0)
	}
	if *outPath != "" {
		cfg.Output = *outPath
	}

	data, err := format.Encode(im, format.MetaFromConfig(cfg), *version)
	if err != nil {
		fmt.Fprintln(os.Stderr, "编码失败:", err)
		os.Exit(2)
	}
	if err := os.WriteFile(cfg.Output, data, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "导出失败:", err)
		os.Exit(2)
	}

	if *decode > 0 {
		d := format.DecoderV2
		if *decode == 1 {
			d = format.DecoderV1
		}
		dec, err := format.Decode(data, d)
		if err != nil {
			fmt.Fprintln(os.Stderr, "解码失败:", err)
			os.Exit(3)
		}
		fmt.Printf("decode: file_version=%d fieldset=%d downgraded=%v meta_known=%v on=%d\n",
			dec.FileVersion, dec.SelectedField, dec.Downgraded, dec.MetadataKnown, dec.Image.CountOn(128))
	}
	fmt.Printf("on=%d size=%dx%d version=%d bytes=%d\n",
		im.CountOn(128), im.W, im.H, *version, len(data))
}
