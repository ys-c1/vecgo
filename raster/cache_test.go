package raster

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"math"
	"runtime"
	"testing"

	"vecgo/geom"
)

// tri builds a triangle.
func tri(x0, y0, x1, y1, x2, y2 float64) []geom.Point {
	return []geom.Point{{X: x0, Y: y0}, {X: x1, Y: y1}, {X: x2, Y: y2}}
}

func mustImage(t *testing.T, w, h int) *Image {
	t.Helper()
	im, err := New(w, h)
	if err != nil {
		t.Fatal(err)
	}
	return im
}

func fillDirect(t *testing.T, w, h, ss int, scene [][]geom.Point) []uint8 {
	t.Helper()
	im := mustImage(t, w, h)
	if err := im.Fill(scene, ss); err != nil {
		t.Fatal(err)
	}
	return im.Pix
}

func fillCached(t *testing.T, w, h, ss int, scene [][]geom.Point, c *Cache) []uint8 {
	t.Helper()
	im := mustImage(t, w, h)
	if err := im.FillCached(scene, ss, c); err != nil {
		t.Fatal(err)
	}
	return im.Pix
}

// theCases are the required scene shapes, drawn from a small pool of
// triangles at a few transforms so the scenes are not all the same picture.
func theCases() []struct {
	name  string
	scale int
	scene [][]geom.Point
} {
	base := tri(1, 1, 9, 2, 4, 9)
	other := tri(2, 3, 8, 3, 5, 8)
	sq := []geom.Point{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}
	rot := func(p []geom.Point, s, a float64) []geom.Point {
		out := make([]geom.Point, len(p))
		c, si := math.Cos(a), math.Sin(a)
		for i, q := range p {
			out[i] = geom.Point{X: s * (q.X*c - q.Y*si), Y: s * (q.X*si + q.Y*c)}
		}
		return out
	}

	single := [][]geom.Point{base}

	dup := [][]geom.Point{base, base, other, base}

	alt := [][]geom.Point{base, other, base, other, base, other}

	var big [][]geom.Point
	for i := 0; i < 300; i++ {
		f := float64(i%17) * 0.35
		big = append(big, rot(base, 1+f*0.05, f*0.2))
	}

	deg := [][]geom.Point{
		base,                  // 0 okay
		tri(2, 2, 2, 2, 7, 7), // 1 repeated vertex
		other,                 // 2 okay
		tri(3, 1, 7, 4, 5, 6), // 3 okay
		tri(1, 1, 5, 5, 9, 9), // 4 collinear, zero area
		tri(1, 1, 1, 1, 1, 1), // 5 all three equal
		rot(base, 1.3, 0.4),   // 6 okay
	}

	return []struct {
		name  string
		scale int
		scene [][]geom.Point
	}{
		{"single", 1, single},
		{"duplicate", 1, dup},
		{"alternating", 1, alt},
		{"large-300", 1, big},
		{"degenerate", 1, deg},
		{"square-nontriangle", 1, [][]geom.Point{sq}},
	}
}

// TestNoCacheOutputUnchanged locks the direct path to bytes captured before the
// coverage cache was introduced. These are the wire bytes, not a recount, so
// any change to the renderer's arithmetic fails here.
func TestNoCacheOutputUnchanged(t *testing.T) {
	golden := []struct {
		name  string
		w, h  int
		ss    int
		scene [][]geom.Point
		sum   [8]byte
	}{
		{"square-default", 64, 64, 2,
			[][]geom.Point{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}},
			[8]byte{0xf4, 0x7a, 0x8e, 0xc3, 0xe9, 0xaf, 0xf2, 0x31}},
		{"triangle-rotated", 48, 32, 2,
			[][]geom.Point{tri(1, 1, 9, 2, 4, 9)},
			[8]byte{0xd4, 0x73, 0x79, 0x2f, 0xfe, 0xa4, 0xb4, 0x1d}},
		{"dup", 24, 24, 2,
			[][]geom.Point{tri(1, 1, 9, 2, 4, 9), tri(1, 1, 9, 2, 4, 9), tri(2, 3, 8, 3, 5, 8)},
			[8]byte{0x5c, 0x44, 0xd4, 0x01, 0x8c, 0x76, 0xf4, 0x7c}},
		{"degenerate", 20, 20, 2,
			[][]geom.Point{tri(1, 1, 5, 5, 9, 9), tri(2, 2, 2, 2, 7, 7), tri(3, 1, 7, 4, 5, 6)},
			[8]byte{0xbb, 0xa8, 0x5b, 0x1c, 0x14, 0xe7, 0xe4, 0xa7}},
	}
	for _, g := range golden {
		got := fillDirect(t, g.w, g.h, g.ss, g.scene)
		full := sha256.Sum256(got)
		var sum [8]byte
		copy(sum[:], full[:8])
		if sum != g.sum {
			t.Errorf("%s: 直接路径输出已变: got %x want %x", g.name, sum, g.sum)
		}
	}
}

// TestTwoStateByteIdentical is the core invariant: cache off, cache cold and
// cache warm must produce byte-identical pixels, for every required scene.
// It reports both states (cold: all misses; warm: hits reused) per scene.
func TestTwoStateByteIdentical(t *testing.T) {
	for _, tc := range theCases() {
		for _, ss := range []int{1, 2, 3} {
			direct := fillDirect(t, 40, 32, ss, tc.scene)

			if tc.name == "square-nontriangle" {
				// Non-triangles are the cache's one hard limit: it must
				// refuse rather than silently fall back.
				if err := mustImage(t, 40, 32).FillCached(tc.scene, ss, NewCache(0)); err == nil {
					t.Fatal("四边形应被覆盖率缓存拒绝")
				}
				continue
			}

			cold := NewCache(0)
			gotCold := fillCached(t, 40, 32, ss, tc.scene, cold)
			if !bytes.Equal(direct, gotCold) {
				t.Fatalf("%s ss=%d 冷缓存输出与直接路径不同", tc.name, ss)
			}

			warm := NewCache(0)
			if err := func() error {
				im := mustImage(t, 40, 32)
				return im.FillCached(tc.scene, ss, warm)
			}(); err != nil {
				t.Fatal(err)
			}
			warm.ResetStats()
			gotWarm := fillCached(t, 40, 32, ss, tc.scene, warm)
			if !bytes.Equal(direct, gotWarm) {
				t.Fatalf("%s ss=%d 热缓存输出与直接路径不同", tc.name, ss)
			}

			cs, ws := cold.Stats(), warm.Stats()
			t.Logf("%-20s ss=%d cold: hit=%d miss=%d bytes=%d | warm: hit=%d miss=%d hitrate=%.2f | bytes_equal=%v",
				tc.name, ss, cs.Hits, cs.Misses, cs.PeakBytes,
				ws.Hits, ws.Misses, ws.HitRate(), bytes.Equal(direct, gotWarm))

			// Cold means no mask was stored before this run; a repeated
			// triangle within the scene is a hit, by design.
			if cs.Hits+cs.Misses != len(tc.scene) {
				t.Fatalf("%s 冷缓存查询数应等于三角形数: %+v", tc.name, cs)
			}
			if ws.Misses != 0 || ws.Hits != len(tc.scene) {
				t.Fatalf("%s 热缓存应全命中: %+v", tc.name, ws)
			}
		}
	}
}

// TestBoundsChangeInvalidates: the subsample grid is scene-wide, so the same
// triangle in a scene with a different bounding box needs a different mask. A
// cache warmed on one scene must not answer for the other.
func TestBoundsChangeInvalidates(t *testing.T) {
	a := tri(1, 1, 9, 2, 4, 9)
	far := tri(300, 300, 320, 300, 310, 320)
	scene1 := [][]geom.Point{a}
	scene2 := [][]geom.Point{a, far}

	c := NewCache(0)
	got1 := fillCached(t, 40, 32, 2, scene1, c)
	if want := fillDirect(t, 40, 32, 2, scene1); !bytes.Equal(got1, want) {
		t.Fatal("首场景输出不符")
	}
	c.ResetStats()
	got2 := fillCached(t, 40, 32, 2, scene2, c)
	if want := fillDirect(t, 40, 32, 2, scene2); !bytes.Equal(got2, want) {
		t.Fatal("边界变化后复用了过期掩码")
	}
	if s := c.Stats(); s.Hits != 0 {
		t.Fatalf("边界变化后不应命中: %s", FormatStats(s))
	}
}

// TestCacheDisabledByteIdentical ensures a nil cache goes down the exact same
// code path as before the feature.
func TestCacheDisabledByteIdentical(t *testing.T) {
	scene := theCases()[3].scene
	for _, ss := range []int{1, 2} {
		direct := fillDirect(t, 32, 32, ss, scene)
		off := fillCached(t, 32, 32, ss, scene, nil)
		if !bytes.Equal(direct, off) {
			t.Fatalf("ss=%d 关闭缓存时输出改变", ss)
		}
	}
}

// TestKeyCollisionReported forces every key into one hash bucket and checks
// that the collision is reported with both entry numbers while correctness
// survives: the cache stays a hint, never authoritative.
func TestKeyCollisionReported(t *testing.T) {
	c := NewCache(0)
	c.SetHash(func(Key) uint64 { return 7 })
	a := tri(1, 1, 9, 2, 4, 9)
	b := tri(2, 2, 8, 8, 3, 7)
	d := tri(5, 1, 9, 9, 6, 4)
	scene := [][]geom.Point{a, b, d}

	if got, want := fillCached(t, 24, 24, 2, scene, c), fillDirect(t, 24, 24, 2, scene); !bytes.Equal(got, want) {
		t.Fatal("键冲突改变了输出")
	}
	s := c.Stats()
	got := map[int]bool{}
	for _, k := range s.Conflicts {
		got[k.Seq] = true
		if k.First != 0 {
			t.Fatalf("冲突应指向条目 0，得到 %d", k.First)
		}
	}
	if !got[1] || !got[2] || len(got) != 2 {
		t.Fatalf("冲突条目号应为 {1,2}，得到 %v (%s)", got, FormatStats(s))
	}
	if s.Hits != 0 {
		t.Fatalf("键不同不应命中: %s", FormatStats(s))
	}
	t.Logf("冲突报告:\n%s", FormatStats(s))
}

// TestCacheFullReported drives the FIFO cap to eviction and checks the entry
// numbers plus that output is unaffected by the eviction.
func TestCacheFullReported(t *testing.T) {
	var scene [][]geom.Point
	for i := 0; i < 40; i++ {
		scene = append(scene, tri(float64(i%5), 1, 9, float64(i%7), 4, 9))
	}
	// 24x24 ss=2 needs 2304 bits = 36 words = 288 B + 96 overhead; 3 entries.
	c := NewCache(3 * (288 + 96))
	got := fillCached(t, 24, 24, 2, scene, c)
	if want := fillDirect(t, 24, 24, 2, scene); !bytes.Equal(got, want) {
		t.Fatal("缓存写满后输出改变")
	}
	s := c.Stats()
	if s.FullCount == 0 || s.Evictions == 0 {
		t.Fatalf("应报告写满/逐出: %s", FormatStats(s))
	}
	if s.Evictions != s.FullCount || len(s.Full) != s.FullCount {
		t.Fatalf("逐出计数与报告条数不一致: evictions=%d full=%d logged=%d",
			s.Evictions, s.FullCount, len(s.Full))
	}
	for _, f := range s.Full {
		if f.EvictedSeq < 0 || f.Rejected {
			t.Fatalf("完整写满应报告被逐条目号: %+v", f)
		}
	}
	if s.PeakBytes > c.maxBytes {
		t.Fatalf("峰值 %d 超过上限 %d", s.PeakBytes, c.maxBytes)
	}
	t.Logf("写满报告:\n%s", FormatStats(s))

	// One mask larger than the whole cap: reported as rejected, not evicted.
	small := NewCache(64)
	got2 := fillCached(t, 24, 24, 2, scene[:2], small)
	if want := fillDirect(t, 24, 24, 2, scene[:2]); !bytes.Equal(got2, want) {
		t.Fatal("拒绝存储时输出改变")
	}
	s2 := small.Stats()
	if s2.Rejected != 2 || small.Len() != 0 {
		t.Fatalf("超限掩码应被拒绝: %s", FormatStats(s2))
	}
	t.Logf("拒绝报告:\n%s", FormatStats(s2))
}

// TestRepeatedVerticesReported checks the degenerate entry numbers.
func TestRepeatedVerticesReported(t *testing.T) {
	scene := [][]geom.Point{
		tri(1, 1, 9, 2, 4, 9), // 0 fine
		tri(2, 2, 2, 2, 7, 7), // 1 repeated vertex
		tri(3, 1, 7, 4, 5, 6), // 2 fine
		tri(1, 1, 5, 5, 9, 9), // 3 collinear: zero area
		tri(4, 4, 4, 4, 4, 4), // 4 all equal
	}
	c := NewCache(0)
	got := fillCached(t, 24, 24, 2, scene, c)
	if want := fillDirect(t, 24, 24, 2, scene); !bytes.Equal(got, want) {
		t.Fatal("退化三角形改变了输出")
	}
	s := c.Stats()
	var seqs []int
	for _, d := range s.Degenerate {
		seqs = append(seqs, d.Seq)
	}
	want := []int{1, 3, 4}
	if fmt.Sprint(seqs) != fmt.Sprint(want) {
		t.Fatalf("退化条目号应为 %v，得到 %v (%s)", want, seqs, FormatStats(s))
	}
	t.Logf("退化报告:\n%s", FormatStats(s))
}

// TestKeyIsExactVertexIdentity pins the key to raw bits: one ULP apart is a
// different triangle.
func TestKeyIsExactVertexIdentity(t *testing.T) {
	a := tri(1, 1, 9, 2, 4, 9)
	b := tri(1, 1, math.Nextafter(9, 10), 2, 4, 9)
	ka, err := TriangleKey(a)
	if err != nil {
		t.Fatal(err)
	}
	kb, err := TriangleKey(b)
	if err != nil {
		t.Fatal(err)
	}
	if ka == kb {
		t.Fatal("1 ULP 差异应产生不同键")
	}
	if _, err := TriangleKey([]geom.Point{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 1, Y: 1}, {X: 0, Y: 1}}); err == nil {
		t.Fatal("四边形不应有三角形键")
	}
}

// dupScene draws n distinct triangles, each twice back to back — the
// motivating case ("the same triangle drawn twice"). The second draw of a pair
// hits even when the cap evicts between pairs.
func dupScene(n int) [][]geom.Point {
	scene := make([][]geom.Point, 0, 2*n)
	for i := 0; i < n; i++ {
		f := float64(i) * 0.037
		tr := tri(1+0.5*f, 1, 9, 2+0.3*f, 4+0.2*f, 9)
		scene = append(scene, tr, tr)
	}
	return scene
}

func runCache(t *testing.T, scene [][]geom.Point, capBytes int) CacheStats {
	t.Helper()
	c := NewCache(capBytes)
	im := mustImage(t, 64, 64)
	if err := im.FillCached(scene, 2, c); err != nil {
		t.Fatal(err)
	}
	return c.Stats()
}

// TestMemoryBoundedNotLinear runs two triangle counts against the same cap and
// reports hit rate and peak memory for each. Peak accounted storage must be
// bounded by the cap and identical across the two counts, while storing every
// mask would cost n times a mask.
func TestMemoryBoundedNotLinear(t *testing.T) {
	const capBytes = 1 << 20
	var peaks [2]int
	for i, n := range []int{2000, 8000} {
		s := runCache(t, dupScene(n), capBytes)
		if s.PeakBytes > capBytes {
			t.Fatalf("n=%d 峰值 %d 超上限 %d", n, s.PeakBytes, capBytes)
		}
		peaks[i] = s.PeakBytes
		t.Logf("n=%5d 三角形=%5d 命中率=%.3f 峰值条目=%d 峰值字节=%d 逐出=%d 全存需=%d字节",
			n, 2*n, s.HitRate(), s.PeakStored, s.PeakBytes, s.Evictions, 2*n*(36*8+entryOverhead))
	}
	if peaks[0] != peaks[1] {
		t.Fatalf("峰值内存随三角形数增长: %d vs %d", peaks[0], peaks[1])
	}
}

// TestHeapBoundedNotLinear measures real heap growth while the cache is alive.
// The ledger above is an estimate; this is the process heap. Both sizes are
// past the cap, so the cache's retained heap must be flat rather than
// proportional to the triangle count.
func TestHeapBoundedNotLinear(t *testing.T) {
	const capBytes = 1 << 20
	measure := func(n int) uint64 {
		scene := dupScene(n)
		c := NewCache(capBytes)
		im := mustImage(t, 64, 64)
		runtime.GC()
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		if err := im.FillCached(scene, 2, c); err != nil {
			t.Fatal(err)
		}
		runtime.GC()
		runtime.ReadMemStats(&after)
		runtime.KeepAlive(c)
		runtime.KeepAlive(im)
		runtime.KeepAlive(scene)
		return after.HeapAlloc - before.HeapAlloc
	}
	small := measure(2000)
	large := measure(8000)
	t.Logf("堆增长: 2000 唯一=%d 字节, 8000 唯一=%d 字节, 上限=%d", small, large, capBytes)
	if large > 4*small || large > 3*capBytes {
		t.Fatalf("堆随三角形数线性增长: %d vs %d", small, large)
	}
}

// TestMemoryCurveReport is the closing curve: hit rate and peak memory over
// four scene sizes, all against one cap. Run with -v to read the table.
func TestMemoryCurveReport(t *testing.T) {
	const capBytes = 1 << 20
	t.Logf("%8s %10s %10s %10s %10s %10s %10s", "unique", "occurrences", "hitrate", "peak_stored", "peak_bytes", "evictions", "unbounded_B")
	var peaks []int
	for _, n := range []int{100, 400, 1600, 6400} {
		s := runCache(t, dupScene(n), capBytes)
		if s.PeakBytes > capBytes {
			t.Fatalf("n=%d 峰值 %d 超上限 %d", n, s.PeakBytes, capBytes)
		}
		peaks = append(peaks, s.PeakBytes)
		t.Logf("%8d %10d %10.3f %10d %10d %10d %10d",
			n, 2*n, s.HitRate(), s.PeakStored, s.PeakBytes, s.Evictions, 2*n*(36*8+entryOverhead))
	}
	// The last two sizes are past capacity; their peaks must be identical,
	// i.e. flat rather than linear in the triangle count.
	if peaks[len(peaks)-1] != peaks[len(peaks)-2] {
		t.Fatalf("饱和后峰值仍随三角形数增长: %v", peaks)
	}
}
