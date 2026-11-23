package cache

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func mustCache(t *testing.T, capacity int) *Cache {
	t.Helper()
	c, err := New(capacity)
	if err != nil {
		t.Fatalf("New(%d): %v", capacity, err)
	}
	return c
}

func put(t *testing.T, c *Cache, key string) {
	t.Helper()
	if err := c.Put(key, []uint64{uint64(len(key))}); err != nil {
		t.Fatalf("Put(%q): %v", key, err)
	}
}

func get(t *testing.T, c *Cache, key string) bool {
	t.Helper()
	_, ok, err := c.Lookup(key)
	if err != nil {
		t.Fatalf("Lookup(%q): %v", key, err)
	}
	return ok
}

// 冷启动：第一轮全部未命中并写入，第二轮全部命中。
func TestColdStart(t *testing.T) {
	c := mustCache(t, 8)
	keys := []string{"a", "b", "c", "d"}
	for _, k := range keys {
		if get(t, c, k) {
			t.Fatalf("冷启动阶段 %q 不应命中", k)
		}
		put(t, c, k)
	}
	if s := c.Stats(); s.Hits != 0 || s.Misses != 4 || s.Inserts != 4 {
		t.Fatalf("冷启动统计异常: %+v", s)
	}
	before := c.Stats()
	for _, k := range keys {
		if !get(t, c, k) {
			t.Fatalf("第二轮 %q 应命中", k)
		}
	}
	s := c.Stats()
	if s.Hits-before.Hits != 4 || s.Misses != before.Misses {
		t.Fatalf("热阶段命中率异常: %+v", s)
	}
	// 全生命周期命中率 = 4 命中 / 8 次请求。
	if s.HitRate != 0.5 {
		t.Fatalf("全生命周期命中率 %v, 期望 0.5", s.HitRate)
	}
}

// 热循环：同一个键反复访问，命中次数与请求次数一致，且不会触发淘汰。
func TestHotLoop(t *testing.T) {
	c := mustCache(t, 4)
	put(t, c, "hot")
	const n = 10000
	for i := 0; i < n; i++ {
		if !get(t, c, "hot") {
			t.Fatalf("第 %d 次应命中", i)
		}
	}
	s := c.Stats()
	if s.Hits != n || s.Misses != 0 || s.Evictions != 0 {
		t.Fatalf("热循环统计异常: %+v", s)
	}
	entries := c.Entries()
	if len(entries) != 1 || entries[0].Key != "hot" || entries[0].Hits != n {
		t.Fatalf("条目命中计数异常: %+v", entries)
	}
}

// 容量抖动：容量恒为 2，键序列反复进出，验证严格 LRU 淘汰顺序。
func TestCapacityThrash(t *testing.T) {
	c := mustCache(t, 2)
	put(t, c, "a")
	put(t, c, "b")
	if !get(t, c, "a") { // a 变为最近使用
		t.Fatal("a 应命中")
	}
	put(t, c, "c") // 淘汰 b
	if get(t, c, "b") {
		t.Fatal("b 已应被淘汰")
	}
	put(t, c, "d") // 淘汰 a
	if get(t, c, "a") {
		t.Fatal("a 已应被淘汰")
	}
	got := c.EvictionTrace()
	want := []string{"b", "a"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("淘汰顺序 %v, 期望 %v", got, want)
	}
	if s := c.Stats(); s.Evictions != 2 || s.Entries != 2 || s.Capacity != 2 {
		t.Fatalf("统计异常: %+v", s)
	}
}

// 全不命中：键各不相同，命中数为 0，淘汰数等于插入数减容量。
func TestAllMiss(t *testing.T) {
	const capN, total = 4, 200
	c := mustCache(t, capN)
	for i := 0; i < total; i++ {
		if get(t, c, fmt.Sprintf("k%d", i)) {
			t.Fatalf("k%d 不应命中", i)
		}
		put(t, c, fmt.Sprintf("k%d", i))
	}
	s := c.Stats()
	if s.Hits != 0 || s.Misses != 0 && s.Misses != total {
		// Lookup 计一次未命中；Put 不计未命中。
		t.Fatalf("全不命中统计异常: %+v", s)
	}
	if s.Misses != total {
		t.Fatalf("未命中数 %d, 期望 %d", s.Misses, total)
	}
	if s.Evictions != total-capN {
		t.Fatalf("淘汰数 %d, 期望 %d", s.Evictions, total-capN)
	}
	if s.Entries != capN {
		t.Fatalf("存活条目 %d, 期望 %d", s.Entries, capN)
	}
}

// 淘汰顺序可复现：两次独立运行产生完全相同的轨迹与摘要，且轨迹有界。
func TestEvictionOrderReproducible(t *testing.T) {
	run := func(traceLimit int) *Cache {
		c, err := NewWithOptions(Options{Capacity: 3, TraceLimit: traceLimit})
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 500; i++ {
			put(t, c, fmt.Sprintf("t%d", i%37))
			if i%5 == 0 {
				get(t, c, fmt.Sprintf("t%d", (i*7)%37))
			}
		}
		return c
	}
	a, b := run(0), run(0)
	if !reflect.DeepEqual(a.EvictionTrace(), b.EvictionTrace()) {
		t.Fatal("两次运行的淘汰轨迹不一致")
	}
	if a.EvictionDigest() != b.EvictionDigest() {
		t.Fatalf("两次运行的淘汰摘要不一致: %s != %s", a.EvictionDigest(), b.EvictionDigest())
	}
	// 摘要覆盖全部淘汰，即使内存轨迹环已回绕。
	limited := run(4)
	if len(limited.EvictionTrace()) != 4 {
		t.Fatalf("轨迹环应有 4 条, 实际 %d", len(limited.EvictionTrace()))
	}
	if limited.EvictionDigest() != a.EvictionDigest() {
		t.Fatal("有界轨迹不应改变全量淘汰摘要")
	}
	if !bytes.Equal(traceBytes(t, a), traceBytes(t, b)) {
		t.Fatal("导出的淘汰轨迹字节不一致")
	}
}

func traceBytes(t *testing.T, c *Cache) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := c.WriteEvictionTrace(&buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// 命中/淘汰统计可导出为表，按条目列出键与命中次数。
func TestTableExport(t *testing.T) {
	c := mustCache(t, 3)
	put(t, c, "alpha")
	put(t, c, "beta")
	put(t, c, "gamma")
	for i := 0; i < 5; i++ {
		get(t, c, "beta")
	}
	get(t, c, "alpha")
	var buf bytes.Buffer
	if err := c.WriteTable(&buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"key", "hits", "alpha", "beta", "gamma", "eviction_digest"} {
		if !strings.Contains(out, want) {
			t.Fatalf("表格缺少 %q:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "beta") || !strings.Contains(out, "5") {
		t.Fatalf("beta 的命中次数未导出:\n%s", out)
	}
	// 行顺序由插入序号决定，两次导出必须一致。
	var buf2 bytes.Buffer
	if err := c.WriteTable(&buf2); err != nil {
		t.Fatal(err)
	}
	if buf.String() != buf2.String() {
		t.Fatal("同一缓存两次导出不一致")
	}
}

func TestCSVExport(t *testing.T) {
	c := mustCache(t, 2)
	put(t, c, "x")
	get(t, c, "x")
	var buf bytes.Buffer
	if err := c.WriteCSV(&buf); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("CSV 行数 %d, 期望 3:\n%s", len(lines), buf.String())
	}
	if !strings.HasPrefix(lines[0], "# capacity=") {
		t.Fatalf("缺少统计注释行: %s", lines[0])
	}
	if lines[1] != "key,hits,bytes,insert_seq,last_seq" {
		t.Fatalf("表头异常: %s", lines[1])
	}
	if !strings.HasPrefix(lines[2], "x,1,") {
		t.Fatalf("条目行异常: %s", lines[2])
	}
}

// 三类错误输入：容量为零、键为空、缓存满后仍插入（Insert 不淘汰）。
func TestErrorInputs(t *testing.T) {
	t.Run("zero capacity", func(t *testing.T) {
		_, err := New(0)
		if !errors.Is(err, ErrZeroCapacity) {
			t.Fatalf("期望 ErrZeroCapacity, 实际 %v", err)
		}
		var ee *EntryError
		if !errors.As(err, &ee) {
			t.Fatalf("期望 *EntryError, 实际 %T", err)
		}
		if ee.Entry().Capacity != 0 || !strings.Contains(err.Error(), "capacity=0") {
			t.Fatalf("错误缺少条目信息: %v", err)
		}
	})

	t.Run("empty key", func(t *testing.T) {
		c := mustCache(t, 2)
		_, _, err := c.Lookup("")
		if !errors.Is(err, ErrEmptyKey) {
			t.Fatalf("Lookup 期望 ErrEmptyKey, 实际 %v", err)
		}
		err = c.Put("", []uint64{1})
		if !errors.Is(err, ErrEmptyKey) {
			t.Fatalf("Put 期望 ErrEmptyKey, 实际 %v", err)
		}
		var ee *EntryError
		if !errors.As(err, &ee) || ee.Entry().Key != "" || ee.Entry().Capacity != 2 {
			t.Fatalf("错误缺少条目信息: %v", err)
		}
		if !strings.Contains(err.Error(), "key=") || !strings.Contains(err.Error(), "capacity=2") {
			t.Fatalf("错误文本缺少条目信息: %v", err)
		}
	})

	t.Run("insert when full", func(t *testing.T) {
		c := mustCache(t, 2)
		put(t, c, "a")
		put(t, c, "b")
		err := c.Insert("c", []uint64{3})
		if !errors.Is(err, ErrFull) {
			t.Fatalf("期望 ErrFull, 实际 %v", err)
		}
		var ee *EntryError
		if !errors.As(err, &ee) {
			t.Fatalf("期望 *EntryError, 实际 %T", err)
		}
		info := ee.Entry()
		if info.Key != "c" || info.Capacity != 2 || info.Entries != 2 {
			t.Fatalf("错误缺少条目信息: %+v", info)
		}
		if !strings.Contains(err.Error(), "key=\"c\"") || !strings.Contains(err.Error(), "entries=2") {
			t.Fatalf("错误文本缺少条目信息: %v", err)
		}
		// 满时 Insert 必须拒绝而不是淘汰。
		if c.Len() != 2 {
			t.Fatalf("拒绝插入后条目数应为 2, 实际 %d", c.Len())
		}
		if s := c.Stats(); s.Evictions != 0 || s.Rejected != 1 {
			t.Fatalf("拒绝插入统计异常: %+v", s)
		}
	})

	t.Run("noevict put when full", func(t *testing.T) {
		c, err := NewWithOptions(Options{Capacity: 1, NoEvict: true})
		if err != nil {
			t.Fatal(err)
		}
		put(t, c, "a")
		err = c.Put("b", []uint64{2})
		if !errors.Is(err, ErrFull) {
			t.Fatalf("NoEvict 下期望 ErrFull, 实际 %v", err)
		}
	})
}

func TestNilCacheReturnsEntryInfo(t *testing.T) {
	var c *Cache
	_, _, err := c.Lookup("k")
	if !errors.Is(err, ErrZeroCapacity) {
		t.Fatalf("期望 ErrZeroCapacity, 实际 %v", err)
	}
	if err := c.Put("k", []uint64{1}); !errors.Is(err, ErrZeroCapacity) {
		t.Fatalf("期望 ErrZeroCapacity, 实际 %v", err)
	}
}

// 更新已有键不增加条目数，也不触发淘汰。
func TestUpdateExisting(t *testing.T) {
	c := mustCache(t, 2)
	if err := c.Put("a", []uint64{1, 2, 3}); err != nil {
		t.Fatal(err)
	}
	before := c.Stats().Bytes
	if err := c.Put("a", []uint64{9}); err != nil {
		t.Fatal(err)
	}
	s := c.Stats()
	if s.Entries != 1 || s.Evictions != 0 || s.Updates != 1 {
		t.Fatalf("更新统计异常: %+v", s)
	}
	if s.Bytes >= before {
		t.Fatalf("缩小更新后字节数应下降: %d -> %d", before, s.Bytes)
	}
	v, ok := c.GetCopy("a")
	if !ok || !reflect.DeepEqual(v, []uint64{9}) {
		t.Fatalf("更新后值异常: %v %v", v, ok)
	}
}

func TestHitRateZeroOnNoTraffic(t *testing.T) {
	c := mustCache(t, 4)
	if s := c.Stats(); s.HitRate != 0 {
		t.Fatalf("无流量命中率应为 0, 实际 %v", s.HitRate)
	}
}

func TestPeakEntriesBounded(t *testing.T) {
	c := mustCache(t, 5)
	for i := 0; i < 100; i++ {
		put(t, c, fmt.Sprintf("p%d", i))
	}
	if s := c.Stats(); s.PeakEntries > 5 || s.Entries > 5 {
		t.Fatalf("条目数溢出容量: %+v", s)
	}
}

// Put 必须持有自己的副本：调用方复用传入切片不得污染缓存内容。
func TestPutCopiesValue(t *testing.T) {
	c := mustCache(t, 2)
	buf := []uint64{1, 2, 3}
	if err := c.Put("k", buf); err != nil {
		t.Fatal(err)
	}
	buf[0] = 999
	got, ok := c.GetCopy("k")
	if !ok || !reflect.DeepEqual(got, []uint64{1, 2, 3}) {
		t.Fatalf("缓存内容被外部修改: %v", got)
	}
}
