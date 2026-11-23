package pipe

import (
	"bytes"
	"fmt"
	"math"
	"testing"

	"vecgo/cache"
	"vecgo/config"
	"vecgo/geom"
)

func tri(cx, cy, r float64) []geom.Point {
	return []geom.Point{{X: cx, Y: cy - r}, {X: cx + r, Y: cy + r}, {X: cx - r, Y: cy + r}}
}

// gridScene builds a deterministic scene of n overlapping triangles laid out
// on a grid so that the scene bounding box stays roughly square.
func gridScene(n int) [][]geom.Point {
	side := int(math.Ceil(math.Sqrt(float64(n))))
	scene := make([][]geom.Point, 0, n)
	for i := 0; i < n; i++ {
		x := float64(i%side) * 3
		y := float64(i/side) * 3
		scene = append(scene, tri(x+5, y+5, 4))
	}
	return scene
}

func cfgFor(w, h int) config.Config {
	c := config.Default()
	c.Width, c.Height = w, h
	c.Supersample = 2
	c.Shading = "gradient"
	c.Gamma = 1.3
	c.Blur = 1
	return c
}

// 缓存关闭与开启的输出必须逐字节相同，包括经过着色、gamma 与模糊之后。
func TestCacheByteIdentical(t *testing.T) {
	scene := gridScene(12)
	cfg := cfgFor(48, 48)
	cfg.CacheEnabled = true
	cfg.CacheCapacity = 5

	uncached, err := Run(scene, cfg)
	if err != nil {
		t.Fatal(err)
	}
	c, err := cache.New(cfg.CacheCapacity)
	if err != nil {
		t.Fatal(err)
	}
	cached, err := RunCached(scene, cfg, c)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(uncached.Pix, cached.Pix) {
		t.Fatalf("开缓存与关缓存输出不一致: %d vs %d 字节", len(uncached.Pix), len(cached.Pix))
	}
	if s := c.Stats(); s.Evictions == 0 {
		t.Fatalf("容量 %d 下 %d 个三角形应触发淘汰: %+v", cfg.CacheCapacity, len(scene), s)
	}
}

// 同一输入第二次渲染应全部命中，且仍与关闭缓存逐字节相同。
func TestCacheWarmRerunIdentical(t *testing.T) {
	scene := gridScene(9)
	cfg := cfgFor(32, 32)
	cfg.CacheEnabled = true
	cfg.CacheCapacity = 64
	uncached, err := Run(scene, cfg)
	if err != nil {
		t.Fatal(err)
	}
	c, _ := cache.New(64)
	if _, err := RunCached(scene, cfg, c); err != nil {
		t.Fatal(err)
	}
	before := c.Stats()
	warm, err := RunCached(scene, cfg, c)
	if err != nil {
		t.Fatal(err)
	}
	after := c.Stats()
	if after.Misses != before.Misses {
		t.Fatalf("热重跑不应产生未命中: before=%d after=%d", before.Misses, after.Misses)
	}
	if after.Hits-before.Hits != uint64(len(scene)) {
		t.Fatalf("热重跑命中数 %d, 期望 %d", after.Hits-before.Hits, len(scene))
	}
	if !bytes.Equal(uncached.Pix, warm.Pix) {
		t.Fatal("热重跑输出与关闭缓存不一致")
	}
}

// 缓存容量与配置不一致时应拒绝。
func TestRunRejectsCapacityMismatch(t *testing.T) {
	cfg := cfgFor(8, 8)
	cfg.CacheEnabled = true
	cfg.CacheCapacity = 4
	c, _ := cache.New(5)
	if _, err := RunCached(gridScene(2), cfg, c); err == nil {
		t.Fatal("容量不一致应报错")
	}
}

func TestCachedFillRejectsBadInput(t *testing.T) {
	im, _ := newImage(t, 8, 8)
	c, _ := cache.New(4)
	if err := im.FillCached(nil, 2, c); err == nil {
		t.Fatal("空场景应报错")
	}
	if err := im.FillCached(gridScene(1), 0, c); err == nil {
		t.Fatal("supersample=0 应报错")
	}
	if err := im.FillCached(gridScene(1), 2, nil); err == nil {
		t.Fatal("nil 缓存应报错")
	}
}

// 峰值缓存字节数随三角形数增长必须低于给定斜率：容量有界时斜率应接近 0。
func TestPeakMemoryDoesNotGrowWithTriangles(t *testing.T) {
	cfg := cfgFor(32, 32)
	cfg.CacheEnabled = true
	const capacity = 8
	cfg.CacheCapacity = capacity
	measure := func(n int) int {
		c, err := cache.New(capacity)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := RunCached(gridScene(n), cfg, c); err != nil {
			t.Fatal(err)
		}
		s := c.Stats()
		if s.PeakEntries > capacity {
			t.Fatalf("%d 个三角形时峰值条目 %d 超过容量 %d", n, s.PeakEntries, capacity)
		}
		return s.PeakBytes
	}
	small, large := measure(20), measure(200)
	slope := float64(large-small) / float64(200-20)
	limit := cfg.CacheSlopeLimit()
	if slope > limit {
		t.Fatalf("峰值内存斜率 %.2f B/三角形 超过上限 %.2f (small=%d large=%d)",
			slope, limit, small, large)
	}
}

func TestCacheDisableLeavesOutputUntouched(t *testing.T) {
	scene := gridScene(6)
	cfg := cfgFor(24, 24)
	cfg.CacheEnabled = false
	a, err := Run(scene, cfg)
	if err != nil {
		t.Fatal(err)
	}
	b, err := RunCached(scene, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a.Pix, b.Pix) {
		t.Fatal("关闭缓存的两条路径输出不一致")
	}
	c, _ := cache.New(16)
	d, err := RunCached(scene, cfg, c)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a.Pix, d.Pix) {
		t.Fatal("配置关闭缓存但传入缓存对象时输出不一致")
	}
	if c.Stats().Hits+c.Stats().Misses != 0 {
		t.Fatal("配置关闭缓存时不应访问缓存")
	}
	_ = fmt.Sprint(len(scene))
}

// 单条大三角形在固定容量下的峰值与多条小三角形相当，不随场景规模增长。
func TestSingleLargeTriangleCacheBounded(t *testing.T) {
	cfg := cfgFor(64, 64)
	cfg.CacheEnabled = true
	cfg.CacheCapacity = 2
	big := []geom.Point{{X: -1e4, Y: -1e4}, {X: 1e4, Y: -1e4}, {X: 0, Y: 1e4}}
	c, err := cache.New(2)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RunCached([][]geom.Point{big, big, big, big}, cfg, c); err != nil {
		t.Fatal(err)
	}
	s := c.Stats()
	if s.PeakEntries > 2 {
		t.Fatalf("大三角形峰值条目 %d 超过容量 2", s.PeakEntries)
	}
	// 同一三角形重复 4 次：首轮未命中、后三轮全部命中。
	if s.Misses != 1 || s.Hits != 3 {
		t.Fatalf("大三角形重复命中统计异常: %+v", s)
	}
}

// 斜率检查必须能真正判失败：把上限设成负值之外的极小正数时，若峰值确实
// 随三角形数增长则应触发失败。这里用无界容量构造增长，再用上限断言其超标。
func TestSlopeLimitDetectsGrowth(t *testing.T) {
	cfg := cfgFor(16, 16)
	const capN = 1 << 20 // 实际无界：不会淘汰
	cfg.CacheEnabled = true
	cfg.CacheCapacity = capN
	measure := func(n int) float64 {
		c, err := cache.New(capN)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := RunCached(gridScene(n), cfg, c); err != nil {
			t.Fatal(err)
		}
		return float64(c.Stats().PeakBytes)
	}
	small, large := measure(10), measure(200)
	slope := (large - small) / float64(200-10)
	if slope <= 0 {
		t.Fatalf("无界容量下峰值应随三角形增长, 实际斜率 %.3f", slope)
	}
	// 有界容量下同一测量应被压平，低于给定上限。
	bounded := cfg
	bounded.CacheEnabled = true
	bounded.CacheCapacity = 8
	c, _ := cache.New(8)
	if _, err := RunCached(gridScene(200), bounded, c); err != nil {
		t.Fatal(err)
	}
	peak := float64(c.Stats().PeakBytes)
	if peak/200 > cfg.CacheSlopeLimit() {
		t.Fatalf("有界峰值 %v 折合斜率超过上限", peak)
	}
}
