package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"vecgo/config"
	"vecgo/geom"
	"vecgo/incr"
	"vecgo/pipe"
)

func main() {
	incrFrames := flag.Int("incr", 0, "额外渲染的增量帧数（0 表示保持原有的单帧全量输出）")
	out := flag.String("o", "", "输出文件名；不给时沿用配置里的 Output")
	flag.Parse()

	cfg := config.Default()
	// 原有的演示场景：一个 10×10 方块。
	scene := [][]geom.Point{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}}

	// 全量路径保持不变：同样的场景、同样的配置、同样的默认文件名。
	im, err := pipe.Run(scene, cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "失败:", err)
		os.Exit(1)
	}
	path := cfg.Output
	if *out != "" {
		path = *out
	} else if flag.NArg() > 0 {
		// 兼容旧用法：第一个位置参数就是输出名。
		path = flag.Arg(0)
	}
	if err := im.SavePGM(path); err != nil {
		fmt.Fprintln(os.Stderr, "导出失败:", err)
		os.Exit(2)
	}
	fmt.Printf("on=%d size=%dx%d\n", im.CountOn(128), im.W, im.H)

	if *incrFrames > 0 {
		if err := runIncremental(cfg, *incrFrames); err != nil {
			fmt.Fprintln(os.Stderr, "增量失败:", err)
			os.Exit(4)
		}
	}
}

// runIncremental 演示增量路径：固定视图下图形逐帧平移，
// 每帧都与同场景的全量输出逐字节比对。
func runIncremental(cfg config.Config, frames int) error {
	anchors := [][]geom.Point{
		{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 0, Y: 1}},
		{{X: 100, Y: 100}, {X: 99, Y: 100}, {X: 100, Y: 99}},
	}
	box := func(dx float64) []geom.Point {
		return []geom.Point{{X: dx, Y: 20}, {X: dx + 30, Y: 20}, {X: dx + 30, Y: 50}, {X: dx, Y: 50}}
	}
	sess, err := incr.NewSession(cfg, incr.SessionOptions{
		Source:      incr.SourceDiff,
		FallbackLog: log.New(os.Stderr, "incr: ", 0),
	})
	if err != nil {
		return err
	}
	for i := 0; i <= frames; i++ {
		s := append(append([][]geom.Point{}, anchors...), box(10+float64(i)*4))
		var img []byte
		var stats incr.Stats
		if i == 0 {
			got, st, err := sess.Frame(s, nil)
			if err != nil {
				return err
			}
			img, stats = got.Bytes(), st
		} else {
			got, st, err := sess.Incremental(s, nil)
			if err != nil {
				return err
			}
			img, stats = got.Bytes(), st
		}
		want, err := sess.Full(s)
		if err != nil {
			return err
		}
		if string(img) != string(want.Bytes()) {
			return fmt.Errorf("帧 %d 增量与全量不一致", i)
		}
		fmt.Printf("帧 %d: 脏矩形 %d→%d 覆盖=%d/%d 整帧=%v\n",
			i, stats.RectsIn, stats.RectsMerged, stats.DirtyPixels, stats.TotalPixels, stats.WholeFrame)
	}
	return nil
}
