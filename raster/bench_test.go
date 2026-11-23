package raster

import (
	"testing"

	"vecgo/geom"
)

// benchScene builds n distinct triangles, each duplicated once, so the
// hit-rate ceiling is 50% and the cache still thrashes when capped.
func benchScene(n int) [][]geom.Point {
	scene := make([][]geom.Point, 0, 2*n)
	for i := 0; i < n; i++ {
		f := float64(i) * 0.037
		tr := tri(1+0.5*f, 1, 9, 2+0.3*f, 4+0.2*f, 9)
		scene = append(scene, tr, tr)
	}
	return scene
}

// benchWarmScene is the triangle list the warm benchmark draws; its masks are
// preloaded by the benchmark itself.
func benchWarmScene(n int) [][]geom.Point { return benchScene(n) }

const benchCap = 1 << 20

func BenchmarkFillDirect(b *testing.B) {
	for _, n := range []int{200, 2000} {
		scene := benchScene(n)
		im, err := New(64, 64)
		if err != nil {
			b.Fatal(err)
		}
		b.Run(sizeName(n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if err := im.Fill(scene, 2); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkFillCachedCapped(b *testing.B) {
	for _, n := range []int{200, 2000} {
		scene := benchScene(n)
		im, err := New(64, 64)
		if err != nil {
			b.Fatal(err)
		}
		b.Run(sizeName(n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				c := NewCache(benchCap)
				if err := im.FillCached(scene, 2, c); err != nil {
					b.Fatal(err)
				}
			}
			c := NewCache(benchCap)
			_ = im.FillCached(scene, 2, c)
			s := c.Stats()
			b.ReportMetric(s.HitRate(), "hitrate")
			b.ReportMetric(float64(s.PeakBytes), "peak-cache-B")
		})
	}
}

// BenchmarkFillCachedWarm measures per-hit cost with the whole mesh resident.
// The cap is deliberately large enough to hold every mask, so hit rate is 1
// and the numbers show hit cost, not capacity behaviour (see the capped
// benchmark and TestMemoryCurveReport for the memory story).
func BenchmarkFillCachedWarm(b *testing.B) {
	for _, n := range []int{200, 2000} {
		c := NewCache(DefaultMaxBytes)
		im, err := New(64, 64)
		if err != nil {
			b.Fatal(err)
		}
		// Draw once to store the hot set (the second copies in the list), then
		// measure a run made only of residents: per-hit cost by n.
		if err := im.FillCached(benchScene(n), 2, c); err != nil {
			b.Fatal(err)
		}
		hot := make([][]geom.Point, 0, c.Len())
		for i := 0; i < n; i++ {
			f := float64(i) * 0.037
			hot = append(hot, tri(1+0.5*f, 1, 9, 2+0.3*f, 4+0.2*f, 9))
		}
		b.Run(sizeName(n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				c.ResetStats()
				if err := im.FillCached(hot, 2, c); err != nil {
					b.Fatal(err)
				}
			}
			s := c.Stats()
			b.ReportMetric(s.HitRate(), "hitrate")
			b.ReportMetric(float64(c.Len()), "entries")
		})
	}
}

// TestLogsAreCappedNotLinear pins the rule that statistics cost is constant:
// a scene far larger than MaxLogged still reports exact counts, with logs cut
// at MaxLogged.
func TestLogsAreCappedNotLinear(t *testing.T) {
	scene := dupScene(4 * MaxLogged)
	c := NewCache(1 << 16)
	im := mustImage(t, 32, 32)
	if err := im.FillCached(scene, 2, c); err != nil {
		t.Fatal(err)
	}
	s := c.Stats()
	if s.FullCount < MaxLogged || len(s.Full) != MaxLogged {
		t.Fatalf("写满事件应计数精确、日志封顶: count=%d logged=%d", s.FullCount, len(s.Full))
	}
	if s.Lookups != len(scene) || s.Hits+s.Misses != len(scene) {
		t.Fatalf("查询计数应精确: %+v", s)
	}
}

func sizeName(n int) string {
	switch n {
	case 200:
		return "200unique"
	case 2000:
		return "2000unique"
	}
	return "other"
}
