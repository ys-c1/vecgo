// Command bench renders a synthetic scene with the cache enabled at two
// capacities, reports hit rates and peak memory, and writes the cache tables,
// eviction traces and a memory curve derived from runtime.MemStats samples.
//
// It is the reproducible harness behind artifacts/cache-report.md.
package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"vecgo/cache"
	"vecgo/config"
	"vecgo/geom"
	"vecgo/pipe"
)

type sample struct {
	triangles int
	peakBytes uint64
	heapBytes uint64
	hits      uint64
	misses    uint64
	evictions uint64
	warmRate  float64
}

func triScene(n int) [][]geom.Point {
	side := 1
	for side*side < n {
		side++
	}
	scene := make([][]geom.Point, 0, n)
	for i := 0; i < n; i++ {
		x := float64(i%side) * 3
		y := float64(i/side) * 3
		scene = append(scene, []geom.Point{
			{X: x + 1, Y: y + 1},
			{X: x + 4, Y: y + 1},
			{X: x + 2.5, Y: y + 4},
		})
	}
	return scene
}

// loopScene repeats a working set of `working` distinct triangles until it has
// n entries, laid out on a grid that does not depend on n. Because every
// distinct triangle is drawn on the same grid, the cache key for a given
// working-set index is identical across scene sizes. That makes peak footprint
// a function of the working set and capacity only, never of the triangle
// count, and gives a hit rate determined purely by LRU behaviour.
func loopScene(working, n int) [][]geom.Point {
	if working < 1 {
		working = 1
	}
	base := triScene(working)
	scene := make([][]geom.Point, 0, n)
	for i := 0; i < n; i++ {
		scene = append(scene, base[i%working])
	}
	return scene
}

func renderCached(scene [][]geom.Point, cfg config.Config, capacity int) (*cache.Cache, error) {
	c, err := cache.New(capacity)
	if err != nil {
		return nil, err
	}
	cfg.CacheEnabled = true
	cfg.CacheCapacity = capacity
	if _, err := pipe.RunCached(scene, cfg, c); err != nil {
		return nil, err
	}
	return c, nil
}
func peakHeap() uint64 {
	var ms runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&ms)
	return ms.HeapAlloc
}

// sweep renders loop scenes of increasing triangle count at a fixed capacity
// and records the peak cache footprint plus a post-GC heap sample. The working
// set is fixed and smaller than every capacity tier under test, so an ideal
// cache reaches a 100% warm hit rate and a bounded peak; each scene is
// rendered twice and the second pass measures that warm rate.
func sweep(cfg config.Config, capacity int, counts []int) ([]sample, float64) {
	const workingSet = 6
	out := make([]sample, 0, len(counts))
	for _, n := range counts {
		scene := loopScene(workingSet, n)
		before := peakHeap()
		c, err := renderCached(scene, cfg, capacity)
		if err != nil {
			fmt.Fprintln(os.Stderr, "渲染失败:", err)
			os.Exit(1)
		}
		// Warm pass over the same scene: its stats delta is the
		// steady-state hit rate at this scene size.
		warmBefore := c.Stats()
		cachedCfg := cfg
		cachedCfg.CacheEnabled = true
		cachedCfg.CacheCapacity = capacity
		if _, err := pipe.RunCached(scene, cachedCfg, c); err != nil {
			fmt.Fprintln(os.Stderr, "热渲染失败:", err)
			os.Exit(1)
		}
		s := c.Stats()
		warmHits := s.Hits - warmBefore.Hits
		warmTotal := warmHits + (s.Misses - warmBefore.Misses)
		warmRate := 0.0
		if warmTotal > 0 {
			warmRate = float64(warmHits) / float64(warmTotal)
		}
		after := peakHeap()
		heap := before
		if after > heap {
			heap = after
		}
		out = append(out, sample{
			triangles: n,
			peakBytes: uint64(s.PeakBytes),
			heapBytes: heap,
			hits:      s.Hits,
			misses:    s.Misses,
			evictions: s.Evictions,
			warmRate:  warmRate,
		})
	}
	return out, slope(out)
}

// slope is least-squares d(peakBytes)/d(triangles).
func slope(ss []sample) float64 {
	if len(ss) < 2 {
		return 0
	}
	var sx, sy, sxx, sxy float64
	for _, s := range ss {
		x, y := float64(s.triangles), float64(s.peakBytes)
		sx += x
		sy += y
		sxx += x * x
		sxy += x * y
	}
	n := float64(len(ss))
	den := n*sxx - sx*sx
	if den == 0 {
		return 0
	}
	return (n*sxy - sx*sy) / den
}

func writeCurve(path string, ss []sample) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	fmt.Fprintln(w, "triangles,peak_cache_bytes,heap_bytes,warm_hit_rate,hits,misses,evictions")
	for _, s := range ss {
		fmt.Fprintf(w, "%d,%d,%d,%.4f,%d,%d,%d\n",
			s.triangles, s.peakBytes, s.heapBytes, s.warmRate, s.hits, s.misses, s.evictions)
	}
	return w.Flush()
}

// asciiCurve renders a tiny SVG-free curve for a text report.
func asciiCurve(ss []sample) string {
	if len(ss) == 0 {
		return ""
	}
	maxY := uint64(0)
	for _, s := range ss {
		if s.peakBytes > maxY {
			maxY = s.peakBytes
		}
	}
	if maxY == 0 {
		maxY = 1
	}
	var b strings.Builder
	const height = uint64(10)
	for row := height; row >= 1; row-- {
		threshold := maxY * row / height
		fmt.Fprintf(&b, "%8d |", threshold)
		for _, s := range ss {
			if s.peakBytes >= threshold {
				b.WriteString(" #")
			} else {
				b.WriteString("  ")
			}
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "         +")
	for range ss {
		b.WriteString("--")
	}
	b.WriteString("\n          ")
	for _, s := range ss {
		fmt.Fprintf(&b, "%2d", s.triangles)
		if s.triangles < 10 {
			b.WriteString(" ")
		}
	}
	b.WriteString(" triangles\n")
	return b.String()
}

func main() {
	var (
		outDir   = flag.String("out", "artifacts/cache", "output directory for reports and curves")
		smallCap = flag.Int("small-capacity", 4, "small capacity tier (below the default working set)")
		largeCap = flag.Int("large-capacity", 256, "large capacity tier (above the default working set)")
		width    = flag.Int("width", 32, "image width")
		height   = flag.Int("height", 32, "image height")
		ss       = flag.Int("supersample", 2, "supersample factor (1..8)")
		limit    = flag.Float64("max-bytes-per-triangle", config.DefaultMaxBytesPerTriangle,
			"fail the slope check above this many bytes per triangle")
		countsArg = flag.String("counts", "8,16,32,64,128,256,512,1024",
			"comma separated triangle counts for the sweep")
		verify = flag.Bool("verify-identical", true, "assert cached output equals uncached byte-for-byte")
	)
	flag.Parse()

	var counts []int
	for _, part := range strings.Split(*countsArg, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		var n int
		if _, err := fmt.Sscanf(part, "%d", &n); err != nil || n <= 0 {
			fmt.Fprintf(os.Stderr, "无效的三角形数量: %q\n", part)
			os.Exit(2)
		}
		counts = append(counts, n)
	}
	sort.Ints(counts)
	if len(counts) < 2 {
		fmt.Fprintln(os.Stderr, "至少需要两个采样点")
		os.Exit(2)
	}

	cfg := config.Default()
	cfg.Width, cfg.Height = *width, *height
	cfg.Supersample = *ss
	cfg.CacheMaxBytesPerTriangle = *limit

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "创建输出目录失败:", err)
		os.Exit(1)
	}

	report := &strings.Builder{}
	fmt.Fprintf(report, "# cache report\n\n")
	fmt.Fprintf(report, "config: %dx%d supersample=%d counts=%v\n\n", *width, *height, *ss, counts)
	fmt.Fprintf(report, "command: go run ./cmd/bench \\\n")
	flag.VisitAll(func(f *flag.Flag) {
		fmt.Fprintf(report, "  -%s=%s \\\n", f.Name, f.Value.String())
	})
	fmt.Fprintf(report, "\n")

	tiers := []struct {
		name     string
		capacity int
	}{{"small", *smallCap}, {"large", *largeCap}}

	type tierResult struct {
		name string
		cap  int
		ss   []sample
		sl   float64
	}
	results := make([]tierResult, 0, len(tiers))

	failed := false
	for _, tier := range tiers {
		ssamples, sl := sweep(cfg, tier.capacity, counts)
		results = append(results, tierResult{name: tier.name, cap: tier.capacity, ss: ssamples, sl: sl})

		last := ssamples[len(ssamples)-1]
		var peak uint64
		for _, s := range ssamples {
			if s.peakBytes > peak {
				peak = s.peakBytes
			}
		}
		fmt.Fprintf(report, "## tier %s (capacity=%d)\n\n", tier.name, tier.capacity)
		fmt.Fprintf(report, "- warm hit rate @ %d triangles: %.4f\n", last.triangles, last.warmRate)
		fmt.Fprintf(report, "- peak cache bytes: %d\n", peak)
		fmt.Fprintf(report, "- peak-slope: %.3f bytes/triangle (limit %.3f)\n", sl, cfg.CacheSlopeLimit())
		if sl > cfg.CacheSlopeLimit() {
			fmt.Fprintf(report, "- RESULT: FAIL (slope above limit)\n")
			failed = true
		} else {
			fmt.Fprintf(report, "- RESULT: PASS\n")
		}
		fmt.Fprintf(report, "\n| triangles | peak cache B | heap B | warm hit rate | hits | misses | evictions |\n")
		fmt.Fprintf(report, "|---:|---:|---:|---:|---:|---:|---:|\n")
		for _, s := range ssamples {
			fmt.Fprintf(report, "| %d | %d | %d | %.4f | %d | %d | %d |\n",
				s.triangles, s.peakBytes, s.heapBytes, s.warmRate, s.hits, s.misses, s.evictions)
		}
		fmt.Fprintf(report, "\n```\n%s```\n\n", asciiCurve(ssamples))

		curvePath := filepath.Join(*outDir, "curve-"+tier.name+".csv")
		if err := writeCurve(curvePath, ssamples); err != nil {
			fmt.Fprintln(os.Stderr, "写曲线失败:", err)
			os.Exit(1)
		}
		fmt.Fprintf(report, "curve: %s\n\n", curvePath)
	}

	// Reproducible eviction order: two independent runs, compared byte for byte.
	traceScene := triScene(64)
	var traces [2][]byte
	var digests [2]string
	for i := 0; i < 2; i++ {
		c, err := renderCached(traceScene, cfg, *smallCap)
		if err != nil {
			fmt.Fprintln(os.Stderr, "轨迹渲染失败:", err)
			os.Exit(1)
		}
		var b strings.Builder
		if err := c.WriteEvictionTrace(&b); err != nil {
			fmt.Fprintln(os.Stderr, "写轨迹失败:", err)
			os.Exit(1)
		}
		traces[i] = []byte(b.String())
		digests[i] = c.EvictionDigest()
		tracePath := filepath.Join(*outDir, fmt.Sprintf("evictions-run%d.txt", i+1))
		if err := os.WriteFile(tracePath, traces[i], 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "写轨迹文件失败:", err)
			os.Exit(1)
		}
	}
	reproducible := string(traces[0]) == string(traces[1]) && digests[0] == digests[1]
	fmt.Fprintf(report, "## eviction order reproducibility (capacity=%d, 64 triangles)\n\n", *smallCap)
	fmt.Fprintf(report, "- run1 digest: %s\n- run2 digest: %s\n", digests[0], digests[1])
	if reproducible {
		fmt.Fprintf(report, "- RESULT: PASS (byte-identical traces)\n\n")
	} else {
		fmt.Fprintf(report, "- RESULT: FAIL\n\n")
		failed = true
	}

	// Tables and CSV for the large tier, listing key and hit counts. The
	// working set fits, so every entry accumulates real hit counts.
	tableScene := loopScene(6, 96)
	tableCFG := cfg
	tableCFG.CacheEnabled = true
	tableCFG.CacheCapacity = *largeCap
	c, err := renderCached(tableScene, cfg, *largeCap)
	if err != nil {
		fmt.Fprintln(os.Stderr, "表格渲染失败:", err)
		os.Exit(1)
	}
	if _, err := pipe.RunCached(tableScene, tableCFG, c); err != nil {
		fmt.Fprintln(os.Stderr, "表格热渲染失败:", err)
		os.Exit(1)
	}
	tablePath := filepath.Join(*outDir, "entries-large.txt")
	csvPath := filepath.Join(*outDir, "entries-large.csv")
	tf, err := os.Create(tablePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "创建表格失败:", err)
		os.Exit(1)
	}
	if err := c.WriteTable(tf); err != nil {
		tf.Close()
		fmt.Fprintln(os.Stderr, "写表格失败:", err)
		os.Exit(1)
	}
	tf.Close()
	cf, err := os.Create(csvPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "创建 CSV 失败:", err)
		os.Exit(1)
	}
	if err := c.WriteCSV(cf); err != nil {
		cf.Close()
		fmt.Fprintln(os.Stderr, "写 CSV 失败:", err)
		os.Exit(1)
	}
	cf.Close()
	fmt.Fprintf(report, "cache table: %s\ncache csv: %s\n\n", tablePath, csvPath)

	// Byte-identical output check across capacities.
	if *verify {
		scene := triScene(40)
		plain, err := pipe.Run(scene, cfg)
		if err != nil {
			fmt.Fprintln(os.Stderr, "关缓存渲染失败:", err)
			os.Exit(1)
		}
		for _, tier := range tiers {
			cachedCfg := cfg
			cachedCfg.CacheEnabled = true
			cachedCfg.CacheCapacity = tier.capacity
			c := mustNew(tier.capacity)
			got, err := pipe.RunCached(scene, cachedCfg, c)
			if err != nil {
				fmt.Fprintln(os.Stderr, "开缓存渲染失败:", err)
				os.Exit(1)
			}
			same := bytes.Equal(plain.Pix, got.Pix)
			fmt.Fprintf(report, "- byte-identical @ capacity=%d: %v\n", tier.capacity, same)
			if !same {
				failed = true
			}
		}
		fmt.Fprintln(report)
	}

	reportPath := filepath.Join(*outDir, "cache-report.md")
	if err := os.WriteFile(reportPath, []byte(report.String()), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "写报告失败:", err)
		os.Exit(1)
	}
	fmt.Println(report.String())
	fmt.Printf("report: %s\n", reportPath)
	if failed {
		fmt.Fprintln(os.Stderr, "存在失败的检查")
		os.Exit(1)
	}
}

func mustNew(capacity int) *cache.Cache {
	c, err := cache.New(capacity)
	if err != nil {
		panic(err)
	}
	return c
}
