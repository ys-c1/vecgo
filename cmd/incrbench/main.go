// Command incrbench 跑通增量渲染的五类场景，逐帧与全量路径做字节比对，
// 并登记两种脏矩形来源下的两档耗时与合并后脏矩形个数。
//
// 场景里都放了两枚固定不动的「锚点」小三角形，把世界包围盒钉住。
// raster.Fill 的采样映射由整帧包围盒决定，包围盒一旦随图形移动而变化，
// 每个像素的采样点都会移动，增量渲染就无从谈起。锚点让映射保持稳定，
// 于是 pipe.Run（既有全量路径）与会话增量路径可以直接逐字节对比。
//
// 用法:
//
//	go run ./cmd/incrbench                  # 全部场景，两种来源
//	go run ./cmd/incrbench -filter 长条      # 只跑名字包含该子串的场景
//	go run ./cmd/incrbench -w 128 -h 128    # 改画幅
//	go run ./cmd/incrbench -bench 5         # 每个场景计时重复轮数（取每帧最好成绩）
package main

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"sort"
	"strings"
	"time"

	"vecgo/config"
	"vecgo/geom"
	"vecgo/incr"
	"vecgo/pipe"
	"vecgo/raster"
)

// 世界坐标是固定的；画幅只影响像素密度。
const world = 100.0

type marksFn func(frame int, prev, cur [][]geom.Point, m raster.Mapping) []raster.Rect

type scene struct {
	name     string
	frames   [][][]geom.Point
	marksFor marksFn // nil 表示该场景不跑标记来源
	cfgW     int
	cfgH     int
}

func square(x, y, s float64) []geom.Point {
	return []geom.Point{{X: x, Y: y}, {X: x + s, Y: y}, {X: x + s, Y: y + s}, {X: x, Y: y + s}}
}

// anchors 把世界包围盒钉死在 [0,world]²，且自身每帧完全相同。
func anchors() [][]geom.Point {
	return [][]geom.Point{
		{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 0, Y: 1}},
		{{X: world, Y: world}, {X: world - 1, Y: world}, {X: world, Y: world - 1}},
	}
}

func main() {
	w := flag.Int("w", 96, "画幅宽")
	h := flag.Int("h", 96, "画幅高")
	total := flag.Int("n", 24, "每个场景的帧数")
	rounds := flag.Int("bench", 3, "计时重复轮数，取每帧最好成绩")
	filter := flag.String("filter", "", "按场景名过滤")
	flag.Parse()

	cfg := config.Default()
	cfg.Width, cfg.Height = *w, *h
	cfg.Scale, cfg.Rotate = 1, 0
	cfg.Supersample = 2
	cfg.Shading = "flat"
	cfg.Gamma = 1.4
	cfg.Blur = 1

	failed := false
	for _, sc := range buildScenes(*total, cfg.Width, cfg.Height) {
		if *filter != "" && !strings.Contains(sc.name, *filter) {
			continue
		}
		for _, src := range []incr.Source{incr.SourceDiff, incr.SourceMarks} {
			if src == incr.SourceMarks && sc.marksFor == nil {
				continue
			}
			if err := runScene(sc, src, cfg, *rounds); err != nil {
				fmt.Fprintf(os.Stderr, "场景 %s / %s 失败: %v\n", sc.name, src, err)
				failed = true
			}
		}
	}
	if failed {
		os.Exit(1)
	}
}

func runScene(sc scene, src incr.Source, cfg config.Config, rounds int) error {
	if rounds < 1 {
		rounds = 1
	}
	var logbuf bytes.Buffer
	opts := incr.SessionOptions{Source: src, FallbackLog: log.New(&logbuf, "", 0)}

	var incTotal, fullTotal time.Duration
	var incSamples, fullSamples []time.Duration
	var lastStats incr.Stats
	allEqual := true
	incrementalFrames := 0
	firstLog := ""
	mergedCounts := map[int]int{}
	var wholeCount, fallbackCount int

	for i, cur := range sc.frames {
		incBest := time.Duration(1<<63 - 1)
		fullBest := time.Duration(1<<63 - 1)
		var got, want *raster.Image
		var stats incr.Stats

		for r := 0; r < rounds; r++ { //nolint:gosec // 帧数来自命令行
			s, err := incr.NewSession(cfg, opts)
			if err != nil {
				return err
			}
			// 回放到第 i 帧之前：不计时，只为拿到正确的会话状态。
			if err := replay(s, sc, cfg, i); err != nil {
				return err
			}
			marks := marksAt(sc, s, i)
			t0 := time.Now()
			img, st, err := s.Frame(cur, marks)
			el := time.Since(t0)
			if err != nil {
				return fmt.Errorf("帧 %d 增量: %w", i, err)
			}
			if el < incBest {
				incBest, got, stats = el, img, st
			}
			t1 := time.Now()
			ref, err := pipe.Run(cur, cfg)
			el = time.Since(t1)
			if err != nil {
				return fmt.Errorf("帧 %d 全量: %w", i, err)
			}
			if el < fullBest {
				fullBest, want = el, ref
			}
		}
		incTotal += incBest
		fullTotal += fullBest
		lastStats = stats
		if stats.FellBack {
			if firstLog == "" {
				firstLog = stats.FallbackWhy
			}
		}
		if !bytes.Equal(got.Bytes(), want.Bytes()) {
			allEqual = false
			fmt.Fprintf(os.Stderr, "  帧 %d 字节不一致 (场景 %s / %s)\n", i, sc.name, src)
			reportFirstDiff(got, want)
		}
		mergedCounts[stats.RectsMerged]++
		if stats.WholeFrame {
			wholeCount++
		}
		if stats.FellBack {
			fallbackCount++
		}
		// 只有真正的增量帧才纳入「四成」判定；整帧失效与回退按设计就是全量。
		if !stats.WholeFrame && !stats.FellBack {
			incrementalFrames++
			incSamples = append(incSamples, incBest)
			fullSamples = append(fullSamples, fullBest)
		}
	}

	ratio := 0.0
	if fullTotal > 0 {
		ratio = float64(incTotal) / float64(fullTotal)
	}
	status := "OK "
	if !allEqual {
		status = "BAD"
	}
	medInc, medFull := median(incSamples), median(fullSamples)
	medRatio := 0.0
	if medFull > 0 {
		medRatio = float64(medInc) / float64(medFull)
	}
	// 「四成」只对真正的增量帧作要求：整帧失效与回退帧按定义就是全量渲染。
	budget := "✓"
	if incrementalFrames > 0 && medRatio > 0.4 {
		status = "SLOW"
		budget = "✗"
	}
	// 合并后脏矩形的个数按帧报一遍，便于核对「合并后个数」这一项。
	fmt.Printf("%s %-14s %-6s 帧=%-3d 全量=%9s 增量=%9s 合计占比=%5.1f%% 增量帧中位=%6.1f%%%s (%s/%s, n=%d) 脏矩形 %d→%d 覆盖=%d/%d 整帧=%d 回退=%d 合并后分布=%s\n",
		status, sc.name, src, len(sc.frames),
		fullTotal.Round(time.Microsecond), incTotal.Round(time.Microsecond),
		100*ratio, 100*medRatio, budget, medInc.Round(time.Microsecond), medFull.Round(time.Microsecond), incrementalFrames,
		lastStats.RectsIn, lastStats.RectsMerged,
		lastStats.DirtyPixels, lastStats.TotalPixels,
		wholeCount, fallbackCount, formatCounts(mergedCounts))
	if logbuf.Len() > 0 {
		lines := strings.Split(strings.TrimSpace(logbuf.String()), "\n")
		fmt.Printf("    回退日志 %d 条；首条: %s\n", len(lines), firstLog)
	}
	_ = firstLog
	return nil
}

// bounds 把若干组世界坐标点经会话映射换算成像素矩形（bench 只传画幅）。
func bounds(m raster.Mapping, w, h int, groups ...[]geom.Point) raster.Rect {
	var r raster.Rect
	for _, g := range groups {
		r = r.Union(m.PixelBBox(g, w, h))
	}
	return r
}

// replay 把新会话推进到第 n 帧之前。
func replay(s *incr.Session, sc scene, cfg config.Config, n int) error {
	for j := 0; j < n; j++ {
		if _, _, err := s.Frame(sc.frames[j], marksAt(sc, s, j)); err != nil {
			return fmt.Errorf("回放帧 %d: %w", j, err)
		}
	}
	return nil
}

func marksAt(sc scene, s *incr.Session, i int) []raster.Rect {
	if sc.marksFor == nil {
		return nil
	}
	var prev [][]geom.Point
	if i > 0 {
		prev = sc.frames[i-1]
	}
	return sc.marksFor(i, prev, sc.frames[i], s.Mapping())
}

func reportFirstDiff(a, b *raster.Image) {
	if a.W != b.W || a.H != b.H {
		fmt.Fprintf(os.Stderr, "    尺寸不同: %dx%d vs %dx%d\n", a.W, a.H, b.W, b.H)
		return
	}
	for i := range a.Pix {
		if a.Pix[i] != b.Pix[i] {
			fmt.Fprintf(os.Stderr, "    首个差异 像素(%d,%d): 增量=%d 全量=%d\n", i%a.W, i/a.W, a.Pix[i], b.Pix[i])
			return
		}
	}
}

func buildScenes(n int, w, h int) []scene {
	if n < 2 {
		n = 2
	}
	return []scene{
		singleMove(n, w, h),
		multiAlternate(n, w, h),
		fullInvalidate(n, w, h),
		zeroChange(n, w, h),
		longStrip(n, w, h),
		badMarksScene(n, w, h),
	}
}

// 1. 单图形移动：每帧平移一个方块，其余图形不动。
func singleMove(n, w, h int) scene {
	frames := make([][][]geom.Point, n)
	for i := 0; i < n; i++ {
		f := anchors()
		f = append(f,
			square(5+float64(i%12)*1.2, 5+float64(i%7)*1.1, 20),
			square(60, 15, 18), square(20, 60, 16), square(70, 70, 12))
		frames[i] = f
	}
	return scene{
		name:   "单图形移动",
		frames: frames,
		marksFor: func(i int, prev, cur [][]geom.Point, m raster.Mapping) []raster.Rect {
			if i == 0 {
				return nil
			}
			return []raster.Rect{bounds(m, w, h, cur[2], prev[2])}
		},
	}
}

// 2. 多图形交替：两个图形按相反相位移动，第三个不动。
func multiAlternate(n, w, h int) scene {
	frames := make([][][]geom.Point, n)
	for i := 0; i < n; i++ {
		t := float64(i)
		f := anchors()
		f = append(f,
			square(10+10*sin(t*0.5), 20, 14),
			square(55, 40+10*sin(t*0.5+3.14), 14),
			square(40, 75, 10))
		frames[i] = f
	}
	return scene{
		name:   "多图形交替",
		frames: frames,
		marksFor: func(i int, prev, cur [][]geom.Point, m raster.Mapping) []raster.Rect {
			if i == 0 {
				return nil
			}
			var out []raster.Rect
			for k := range cur {
				if samePts(prev[k], cur[k]) {
					continue
				}
				out = append(out, bounds(m, w, h, prev[k], cur[k]))
			}
			return out
		},
	}
}

// 3. 整帧失效：多数帧只移动一个图形；第 5 帧与第 17 帧图形数量增加，
// 这两帧必须整帧重画。整段动画的增量耗时应仍远低于全量。
func fullInvalidate(n, w, h int) scene {
	extraAt := func(i int) int {
		switch {
		case i < 5:
			return 0
		case i < 17:
			return 1
		default:
			return 2
		}
	}
	frames := make([][][]geom.Point, n)
	for i := 0; i < n; i++ {
		f := anchors()
		f = append(f, square(10+float64(i%8)*2, 10, 18), square(70, 70, 18))
		for k := 0; k < extraAt(i); k++ {
			f = append(f, square(25+float64(k)*12, 35, 6))
		}
		frames[i] = f
	}
	return scene{
		name:   "整帧失效",
		frames: frames,
		marksFor: func(i int, prev, cur [][]geom.Point, m raster.Mapping) []raster.Rect {
			if i == 0 {
				return nil
			}
			if len(prev) != len(cur) {
				// 调用方知道这一帧图形增减，直接标整幅。
				return []raster.Rect{{X0: 0, Y0: 0, X1: w, Y1: h}}
			}
			return []raster.Rect{bounds(m, w, h, cur[2], prev[2])}
		},
	}
}

// 4. 零变化：每帧几何完全相同。
func zeroChange(n, w, h int) scene {
	base := append(anchors(), square(10, 10, 25), square(55, 55, 20))
	frames := make([][][]geom.Point, n)
	for i := 0; i < n; i++ {
		frames[i] = cloneScene(base)
	}
	return scene{
		name:   "零变化",
		frames: frames,
		marksFor: func(i int, prev, cur [][]geom.Point, m raster.Mapping) []raster.Rect {
			return nil
		},
	}
}

// 5. 极端长条：一个 1 像素高、横跨整幅的长条上下移动。
func longStrip(n, w, h int) scene {
	frames := make([][][]geom.Point, n)
	for i := 0; i < n; i++ {
		y := 10 + float64(i%10)*0.8
		f := anchors()
		f = append(f,
			[]geom.Point{{X: 3, Y: y}, {X: 97, Y: y}, {X: 97, Y: y + 1.5}, {X: 3, Y: y + 1.5}},
			square(40, 70, 14))
		frames[i] = f
	}
	return scene{
		name:   "极端长条",
		frames: frames,
		marksFor: func(i int, prev, cur [][]geom.Point, m raster.Mapping) []raster.Rect {
			if i == 0 {
				return nil
			}
			return []raster.Rect{bounds(m, w, h, cur[2], prev[2])}
		},
	}
}

// 6. 标记出错回退：调用方每第 5 帧漏标真正移动的图形，
// 该帧必须回退全量并留下日志；其余帧按标记增量。
func badMarksScene(n, w, h int) scene {
	frames := make([][][]geom.Point, n)
	for i := 0; i < n; i++ {
		f := anchors()
		f = append(f, square(10+float64(i%10)*3, 10, 16), square(70, 70, 14))
		frames[i] = f
	}
	return scene{
		name:   "标记出错回退",
		frames: frames,
		marksFor: func(i int, prev, cur [][]geom.Point, m raster.Mapping) []raster.Rect {
			if i == 0 {
				return nil
			}
			if i%5 == 4 {
				// 只标静止的第二个图形，漏掉真正移动的第一个。
				return []raster.Rect{bounds(m, w, h, cur[3])}
			}
			return []raster.Rect{bounds(m, w, h, cur[2], prev[2])}
		},
	}
}

func cloneScene(in [][]geom.Point) [][]geom.Point {
	out := make([][]geom.Point, len(in))
	for i, p := range in {
		out[i] = append([]geom.Point{}, p...)
	}
	return out
}

func samePts(a, b []geom.Point) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func sin(x float64) float64 { return math.Sin(x) }

func median(ds []time.Duration) time.Duration {
	if len(ds) == 0 {
		return 0
	}
	c := append([]time.Duration{}, ds...)
	for i := 1; i < len(c); i++ {
		for j := i; j > 0 && c[j] < c[j-1]; j-- {
			c[j], c[j-1] = c[j-1], c[j]
		}
	}
	return c[len(c)/2]
}

// formatCounts 把「合并后脏矩形个数 → 出现帧数」排成稳定字符串。
func formatCounts(m map[int]int) string {
	keys := make([]int, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteString(" ")
		}
		fmt.Fprintf(&b, "%d个×%d帧", k, m[k])
	}
	return b.String()
}
