// 脏矩形增量渲染演示：五个规定样例 + 三类带矩形号的错误上报。
// 运行: go run ./cmd/dirtydemo
package main

import (
	"bytes"
	"fmt"
	"runtime"
	"time"

	"vecgo/config"
	"vecgo/dirty"
	"vecgo/geom"
	"vecgo/raster"
)

func sq(x0, y0, x1, y1 float64) []geom.Point {
	return []geom.Point{
		{X: x0, Y: y0}, {X: x1, Y: y0},
		{X: x1, Y: y1}, {X: x0, Y: y1},
	}
}

func demoCfg(w, h, blur int) config.Config {
	c := config.Default()
	c.Width, c.Height = w, h
	c.Blur = blur
	return c
}

func printStats(title string, st dirty.Stats) {
	fmt.Printf("  %-18s 原始矩形=%d 合并后=%d 重画像素=%d/%d 省下=%d 节省比例=%.2f%%\n",
		title, st.RawRects, st.MergedRects, st.RepaintPixels, st.TotalPixels,
		st.SavedPixels, 100*st.SavedRatio())
	for _, is := range st.Issues {
		fmt.Printf("      上报[%s] 矩形号=%d %s\n", is.Kind, is.Rect, is.Detail)
	}
	if st.Escalated {
		fmt.Printf("      字节对照失败已升级整帧重画\n")
	}
}

func memKB() uint64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.TotalAlloc / 1024
}

func main() {
	const W, H = 64, 64

	// ---------- 样例 1：单图形移动 ----------
	fmt.Println("== 样例1 单图形移动 ==")
	{
		cfg := demoCfg(W, H, 1)
		r := dirty.New(cfg)
		anchors := [][]geom.Point{sq(0, 0, 2, 2), sq(62, 62, 64, 64)}
		f0 := append(append([][]geom.Point{}, anchors...), sq(20, 20, 26, 26))
		out0, st0, _ := r.Render(f0)
		printStats("帧0(首帧)", st0)
		f1 := append(append([][]geom.Point{}, anchors...), sq(40, 40, 46, 46))
		out1, st1, _ := r.Render(f1)
		printStats("帧1(移动)", st1)
		ref1, _ := dirty.FullReference(f1, cfg)
		fmt.Printf("  字节对照: %v（%d 像素逐字节相等）\n", bytesEq(out1, ref1), len(out1.Pix))
		_ = out0
	}

	// ---------- 样例 2：多图形交替 ----------
	fmt.Println("== 样例2 多图形交替 ==")
	{
		cfg := demoCfg(W, H, 1)
		r := dirty.New(cfg)
		anchors := [][]geom.Point{sq(0, 0, 2, 2), sq(62, 62, 64, 64)}
		f0 := append(append([][]geom.Point{}, anchors...),
			sq(10, 10, 14, 14), sq(50, 50, 54, 54))
		r.Render(f0)
		f1 := append(append([][]geom.Point{}, anchors...),
			sq(16, 16, 20, 20), sq(50, 50, 54, 54)) // 只动图形 2
		_, st1, _ := r.Render(f1)
		printStats("帧1(动图形2)", st1)
		f2 := append(append([][]geom.Point{}, anchors...),
			sq(16, 16, 20, 20), sq(40, 40, 44, 44)) // 只动图形 3
		_, st2, _ := r.Render(f2)
		printStats("帧2(动图形3)", st2)
		f3 := append(append([][]geom.Point{}, anchors...),
			sq(10, 20, 14, 24), sq(46, 46, 50, 50)) // 两个都动
		out3, st3, _ := r.Render(f3)
		printStats("帧3(两个都动)", st3)
		ref3, _ := dirty.FullReference(f3, cfg)
		fmt.Printf("  字节对照: %v\n", bytesEq(out3, ref3))
	}

	// ---------- 样例 3：整帧失效 ----------
	fmt.Println("== 样例3 整帧失效（全局包围盒变化）==")
	{
		cfg := demoCfg(W, H, 1)
		r := dirty.New(cfg)
		f0 := [][]geom.Point{sq(10, 10, 30, 30)}
		r.Render(f0)
		f1 := [][]geom.Point{sq(10, 10, 60, 60)} // 扩展到新极值
		out1, st1, _ := r.Render(f1)
		printStats("帧1", st1)
		ref1, _ := dirty.FullReference(f1, cfg)
		fmt.Printf("  字节对照: %v\n", bytesEq(out1, ref1))
	}

	// ---------- 样例 4：零变化 ----------
	fmt.Println("== 样例4 零变化 ==")
	{
		cfg := demoCfg(W, H, 1)
		r := dirty.New(cfg)
		f0 := [][]geom.Point{sq(10, 10, 30, 30), sq(40, 40, 50, 50)}
		r.Render(f0)
		out1, st1, _ := r.Render(f0)
		printStats("帧1(相同场景)", st1)
		ref1, _ := dirty.FullReference(f0, cfg)
		fmt.Printf("  字节对照: %v\n", bytesEq(out1, ref1))
	}

	// ---------- 样例 5：极端长条 ----------
	fmt.Println("== 样例5 极端长条 ==")
	{
		cfg := demoCfg(W, H, 0) // 无 blur，细条节省最极致
		r := dirty.New(cfg)
		anchors := [][]geom.Point{sq(0, 0, 1, 1), sq(63, 63, 64, 64)}
		f0 := append(append([][]geom.Point{}, anchors...), sq(2, 30, 62, 31))
		r.Render(f0)
		f1 := append(append([][]geom.Point{}, anchors...), sq(2, 40, 62, 41))
		out1, st1, _ := r.Render(f1)
		printStats("帧1(长条平移)", st1)
		ref1, _ := dirty.FullReference(f1, cfg)
		fmt.Printf("  字节对照: %v\n", bytesEq(out1, ref1))
	}

	// ---------- 错误上报：越界 / 缺失上一帧 / 重叠冲突 ----------
	fmt.Println("== 错误上报（均带矩形号）==")
	{
		cfg := demoCfg(16, 16, 0)
		// 上一帧缺失
		r0 := dirty.New(cfg)
		_, _, err := r0.Apply([][]geom.Point{sq(2, 2, 8, 8)},
			[]dirty.Rect{{X0: 0, Y0: 0, X1: 4, Y1: 4}})
		fmt.Printf("  上一帧缺失: %v\n", err)

		// 先种一帧，再测越界与重叠
		r := dirty.New(cfg)
		scene := [][]geom.Point{sq(2, 2, 8, 8)}
		r.Render(scene)
		_, _, err = r.Apply(scene, []dirty.Rect{
			{X0: 0, Y0: 0, X1: 4, Y1: 4},
			{X0: 0, Y0: 0, X1: 99, Y1: 4}, // 矩形号 1 越界
		})
		fmt.Printf("  矩形越界:   %v\n", err)

		// 重叠冲突：普通模式合并并上报 Issue；严格模式报错
		_, st, err := r.Apply(scene, []dirty.Rect{
			{X0: 0, Y0: 0, X1: 6, Y1: 6},   // 矩形号 0
			{X0: 4, Y0: 4, X1: 10, Y1: 10}, // 矩形号 1，与 0 面积相交
			{X0: 12, Y0: 12, X1: 15, Y1: 15},
		})
		fmt.Printf("  重叠(普通): err=%v\n", err)
		printStats("合并结果", st)

		rs := dirty.New(cfg)
		rs.Strict = true
		rs.Render(scene)
		_, _, err = rs.Apply(scene, []dirty.Rect{
			{X0: 0, Y0: 0, X1: 6, Y1: 6},
			{X0: 4, Y0: 4, X1: 10, Y1: 10},
		})
		fmt.Printf("  重叠(严格): %v\n", err)
	}

	// ---------- 两个规模：耗时与内存峰值 ----------
	fmt.Println("\n== 规模对比（256x256 画幅，blur=1，各跑 30 帧；首帧全量，其后增量）==")
	for _, n := range []int{8, 64} {
		runScaleReport(n)
	}

	fmt.Printf("\n进程累计分配采样: %d KB\n", memKB())
}

// scaleScene 构造 n 个图形 + 四角锚点；末位图形随帧平移，其余静止。
func scaleScene(n int, shift float64) [][]geom.Point {
	scene := make([][]geom.Point, 0, n+2)
	scene = append(scene, sq(0, 0, 2, 2), sq(98, 98, 100, 100))
	for i := 0; i < n; i++ {
		x := 4 + float64(i)*0.7
		y := 4 + float64(i%8)*4
		scene = append(scene, sq(x, y, x+1.5, y+1.5))
	}
	scene[len(scene)-1] = sq(4+shift, 50, 5.5+shift, 51.5)
	return scene
}

func runScaleReport(n int) {
	c := config.Default()
	c.Width, c.Height = 256, 256
	c.Blur = 1
	const frames = 30

	// --- 全量路径（改造前的整帧重画）---
	var ms1, ms2 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&ms1)
	t0 := time.Now()
	var ref *raster.Image
	for f := 0; f < frames; f++ {
		im, err := dirty.FullReference(scaleScene(n, float64(f)*0.5), c)
		if err != nil {
			panic(err)
		}
		ref = im
	}
	fullDur := time.Since(t0)
	runtime.ReadMemStats(&ms2)
	fullPeakKB := (ms2.TotalAlloc - ms1.TotalAlloc) / 1024

	// --- 增量路径（关闭逐帧对照，测纯增量成本）---
	r := dirty.New(c)
	r.Verify = false
	runtime.GC()
	runtime.ReadMemStats(&ms1)
	t0 = time.Now()
	var last dirty.Stats
	var got *raster.Image
	for f := 0; f < frames; f++ {
		im, st, err := r.Render(scaleScene(n, float64(f)*0.5))
		if err != nil {
			panic(err)
		}
		got, last = im, st
	}
	incDur := time.Since(t0)
	runtime.ReadMemStats(&ms2)
	incPeakKB := (ms2.TotalAlloc - ms1.TotalAlloc) / 1024

	eq := bytesEq(got, ref)
	speedup := float64(fullDur) / float64(incDur)
	fmt.Printf("  图形数=%3d:\n", n)
	fmt.Printf("    全量: %v/帧均 %v  区间分配峰值 %d KB\n",
		fullDur, fullDur/frames, fullPeakKB)
	fmt.Printf("    增量: %v/帧均 %v  区间分配峰值 %d KB  (合并矩形=%d 重画像素=%d/%d 节省 %.2f%%)\n",
		incDur, incDur/frames, incPeakKB, last.MergedRects,
		last.RepaintPixels, last.TotalPixels, 100*last.SavedRatio())
	fmt.Printf("    加速 %.1fx  末帧字节对照=%v\n", speedup, eq)
}

func bytesEq(a, b *raster.Image) bool {
	return bytes.Equal(a.Pix, b.Pix)
}
