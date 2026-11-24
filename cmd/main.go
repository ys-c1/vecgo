package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"vecgo/config"
	"vecgo/diag"
	"vecgo/geom"
	"vecgo/pipe"
)

func main() {
	diagOn := flag.Bool("diag", false, "常开诊断（记录按时间顺序打印，零堆分配）")
	out := flag.String("out", "out.pgm", "输出 PGM 路径")
	flag.Parse()

	cfg := config.Default()
	cfg.Output = *out
	scene := [][]geom.Point{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}}

	var rec *diag.Recorder
	if *diagOn {
		rec = diag.New(true, time.Now)
	}
	res := pipe.RunEx(scene, cfg, pipe.RunOpts{Diag: rec})

	// 部分段失败时仍出图：只摘掉失败的那一段，其余照常渲染。
	if err := res.Im.SavePGM(cfg.Output); err != nil {
		fmt.Fprintln(os.Stderr, "导出失败:", err)
		os.Exit(2)
	}

	// 失败按段报，格式与错误编码一致：<stage>[0xNNNN]: <原因>
	if res.Failed() {
		for _, f := range res.Failures {
			fmt.Fprintf(os.Stderr, "段失败 %s[%s]: %v\n", f.Stage, f.Kind.Hex(), f.Err)
		}
		// 仍然打印图的统计，便于确认其余段照常出图。
	}
	fmt.Printf("on=%d size=%dx%d failures=%d\n", res.Im.CountOn(128), res.Im.W, res.Im.H, len(res.Failures))

	if rec != nil {
		// 诊断按 Seq / Offset 顺序输出，还原现场用。
		for _, r := range rec.Records() {
			fmt.Printf("diag seq=%d offset=%dns event=%s kind=0x%04X stage=%s a=%d b=%d\n",
				r.Seq, r.Offset, r.Event, uint32(r.Kind), r.Stage, r.A, r.B)
		}
	}

	if res.Failed() {
		os.Exit(1)
	}
}
