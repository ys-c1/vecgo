package raster

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"vecgo/geom"
)

// Key is the coverage-cache key of one triangle: its three vertices, in the
// order they were given, as exact IEEE-754 bit patterns. There is no tolerance
// and no canonicalisation, so key equality is exactly vertex equality: two
// vertices that differ by one ULP produce two different keys.
type Key struct {
	X0, Y0, X1, Y1, X2, Y2 uint64
}

// TriangleKey builds the key of a triangle.
func TriangleKey(t []geom.Point) (Key, error) {
	if len(t) != 3 {
		return Key{}, fmt.Errorf("覆盖率缓存只支持三角形，收到 %d 个顶点", len(t))
	}
	return Key{
		X0: math.Float64bits(t[0].X), Y0: math.Float64bits(t[0].Y),
		X1: math.Float64bits(t[1].X), Y1: math.Float64bits(t[1].Y),
		X2: math.Float64bits(t[2].X), Y2: math.Float64bits(t[2].Y),
	}, nil
}

// HashFunc maps a Key to a hash. It is injectable so tests can force
// collisions; the cache stays correct for any hash.
type HashFunc func(Key) uint64

// FNVHash is the default 64-bit FNV-1a hash over the six key words.
func FNVHash(k Key) uint64 {
	const (
		offset = 14695981039346656037
		prime  = 1099511628211
	)
	h := uint64(offset)
	for _, w := range [...]uint64{k.X0, k.Y0, k.X1, k.Y1, k.X2, k.Y2} {
		for b := 0; b < 8; b++ {
			h ^= (w >> (8 * uint(b))) & 0xff
			h *= prime
		}
	}
	return h
}

// DefaultMaxBytes bounds a cache created with NewCache(0).
const DefaultMaxBytes = 8 << 20

// entryOverhead is the fixed per-entry bookkeeping accounted against the cap
// on top of the coverage payload itself (key, map entry, slice header). A
// constant keeps the reported memory deterministic across runs.
const entryOverhead = 96

// Conflict reports two distinct keys that hashed to the same value. Seq is the
// entry number being consulted; First is the entry number of the resident key
// that shares the hash.
type Conflict struct {
	Hash  uint64
	Seq   int
	First int
	Key   Key
}

// Full reports that storing a mask would exceed the cap. Seq is the entry
// number that did not fit; EvictedSeq is the entry number whose mask was
// dropped to make room (-1 when nothing was evicted). Rejected is true when
// the new mask alone is larger than the whole cap and was not stored at all.
type Full struct {
	Seq        int
	EvictedSeq int
	Rejected   bool
}

// Degenerate reports a triangle that cannot enclose area. Such a triangle is
// still cached and still rasterised, so output is unchanged; it is reported
// only.
type Degenerate struct {
	Seq    int
	Reason string
}

// MaxLogged bounds each per-event log in CacheStats. Counts stay exact; the
// lists stop growing at MaxLogged, so statistics cannot make memory linear in
// the triangle count.
const MaxLogged = 256

// CacheStats is the accounting of one or more FillCached runs.
type CacheStats struct {
	Lookups    int // triangle occurrences consulted
	Hits       int
	Misses     int
	Stores     int
	Evictions  int
	Rejected   int
	PeakBytes  int // peak accounted bytes (payload + per-entry overhead)
	PeakStored int // peak number of resident masks

	// Exact event counts, independent of the capped lists below.
	ConflictCount   int
	FullCount       int
	DegenerateCount int

	// Up to MaxLogged events of each kind, in occurrence order.
	Conflicts  []Conflict
	Full       []Full
	Degenerate []Degenerate
}

// appendLogged keeps the first MaxLogged events. The running event count is
// kept separately in CacheStats, so truncation loses detail, never a total.
func appendLogged[T any](dst []T, v T) []T {
	if len(dst) < MaxLogged {
		return append(dst, v)
	}
	return dst
}

// HitRate is Hits/Lookups, or 0 when nothing was looked up.
func (s CacheStats) HitRate() float64 {
	if s.Lookups == 0 {
		return 0
	}
	return float64(s.Hits) / float64(s.Lookups)
}

type hashOwner struct {
	key Key
	seq int
}

type orderEntry struct {
	key Key
	seq int
}

// Cache stores per-triangle coverage masks at subsample resolution. A mask is
// independent of the other triangles in the scene because the bounds in Fill
// are scene-wide and with them the sample positions, so OR-combining cached
// masks reproduces the direct path's "any triangle contains the sample"
// exactly. Storage is FIFO-bounded to maxBytes, which is what keeps peak memory
// independent of the triangle count. Not safe for concurrent use.
type Cache struct {
	maxBytes int
	hash     HashFunc

	w, h, ss int
	// bounds are the scene-wide bounds the resident masks were sampled
	// against. A mask is only valid for the bounds it was built with, so a
	// change in bounds invalidates the whole cache even at the same grid.
	bounds [4]uint64
	planes map[Key][]uint64
	order  []orderEntry
	head   int
	byHash map[uint64]hashOwner
	bytes  int

	// acc and scratch are reused across runs: the OR accumulator and the
	// mask being computed. They keep a run at a constant number of
	// allocations and are counted in the ledger below.
	acc, scratchBuf []uint64

	// Err records that the last FillCached/Cache user fell back to the direct
	// path. It is written by pipe.RunWithOptions.
	Err error

	stats CacheStats
}

// NewCache returns a cache bounded to maxBytes of accounted storage; maxBytes
// <= 0 selects DefaultMaxBytes.
func NewCache(maxBytes int) *Cache {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBytes
	}
	return &Cache{
		maxBytes: maxBytes,
		hash:     FNVHash,
		planes:   make(map[Key][]uint64),
		byHash:   make(map[uint64]hashOwner),
	}
}

// SetHash replaces the hash function. Call it only on an empty cache: keyed
// maps do not survive a hash change, and the cache is reset if needed.
func (c *Cache) SetHash(h HashFunc) {
	if h == nil {
		h = FNVHash
	}
	if len(c.planes) != 0 {
		c.planes = make(map[Key][]uint64)
		c.byHash = make(map[uint64]hashOwner)
		c.order = nil
		c.head = 0
		c.bytes = 0
	}
	c.hash = h
}

// Stats returns a copy of the accounting gathered so far.
func (c *Cache) Stats() CacheStats {
	s := c.stats
	s.Conflicts = append([]Conflict(nil), s.Conflicts...)
	s.Full = append([]Full(nil), s.Full...)
	s.Degenerate = append([]Degenerate(nil), s.Degenerate...)
	return s
}

// ResetStats clears the accounting but keeps the cached masks and the peak
// figures (peaks describe the cache's lifetime, not the window).
func (c *Cache) ResetStats() {
	peakBytes, peakStored := c.stats.PeakBytes, c.stats.PeakStored
	c.stats = CacheStats{PeakBytes: peakBytes, PeakStored: peakStored}
}

// Len is the number of resident masks.
func (c *Cache) Len() int { return len(c.planes) }

// Bytes is the current accounted storage.
func (c *Cache) Bytes() int { return c.bytes }

func (c *Cache) configure(w, h, ss int, minx, miny, spanx, spany float64) {
	nb := [4]uint64{
		math.Float64bits(minx), math.Float64bits(miny),
		math.Float64bits(spanx), math.Float64bits(spany),
	}
	if c.w == w && c.h == h && c.ss == ss && c.bounds == nb {
		return
	}
	// Masks from a different grid or different scene bounds describe
	// different samples; drop them. Keeping them would let an equal key
	// return a mask for the wrong geometry.
	c.w, c.h, c.ss, c.bounds = w, h, ss, nb
	c.planes = make(map[Key][]uint64)
	c.byHash = make(map[uint64]hashOwner)
	c.order = nil
	c.head = 0
	c.bytes = 0
}

func (c *Cache) maskWords() int {
	n := c.w * c.ss * c.h * c.ss
	return (n + 63) / 64
}

// accumulate returns the zeroed OR accumulator for this grid.
func (c *Cache) accumulate(words int) []uint64 {
	if len(c.acc) != words {
		c.acc = make([]uint64, words)
	}
	return c.acc
}

// scratch returns the reusable mask buffer for this grid.
func (c *Cache) scratch(words int) []uint64 {
	if len(c.scratchBuf) != words {
		c.scratchBuf = make([]uint64, words)
	}
	return c.scratchBuf
}

func (c *Cache) notePeak() {
	if c.bytes > c.stats.PeakBytes {
		c.stats.PeakBytes = c.bytes
	}
	if len(c.planes) > c.stats.PeakStored {
		c.stats.PeakStored = len(c.planes)
	}
}

func (c *Cache) store(seq int, k Key, mask []uint64) {
	size := len(mask)*8 + entryOverhead
	if size > c.maxBytes {
		c.stats.Rejected++
		c.stats.FullCount++
		c.stats.Full = appendLogged(c.stats.Full, Full{Seq: seq, EvictedSeq: -1, Rejected: true})
		return
	}
	for c.bytes+size > c.maxBytes && c.head < len(c.order) {
		victim := c.order[c.head]
		c.head++
		c.evict(victim)
		c.stats.FullCount++
		c.stats.Full = appendLogged(c.stats.Full, Full{Seq: seq, EvictedSeq: victim.seq})
	}
	if c.bytes+size > c.maxBytes {
		// Only reachable if the accounting and the FIFO drifted apart.
		c.stats.Rejected++
		c.stats.FullCount++
		c.stats.Full = appendLogged(c.stats.Full, Full{Seq: seq, EvictedSeq: -1, Rejected: true})
		return
	}
	if c.head > 1024 && c.head*2 > len(c.order) {
		// Compact in place: copy overlaps, so no second buffer and no
		// transient growth. The backing array is bounded because len(order)
		// never exceeds the number of masks the cap admits.
		n := copy(c.order, c.order[c.head:])
		c.order = c.order[:n]
		c.head = 0
	}
	c.order = append(c.order, orderEntry{key: k, seq: seq})
	// The caller computes into a reused buffer, so the cache keeps its own
	// copy; this is the one allocation per stored mask.
	c.planes[k] = append([]uint64(nil), mask...)
	c.bytes += size
	h := c.hash(k)
	if _, ok := c.byHash[h]; !ok {
		c.byHash[h] = hashOwner{key: k, seq: seq}
	}
	c.stats.Stores++
	c.notePeak()
}

func (c *Cache) evict(e orderEntry) {
	if m, ok := c.planes[e.key]; ok {
		c.bytes -= len(m)*8 + entryOverhead
		delete(c.planes, e.key)
	}
	h := c.hash(e.key)
	if o, ok := c.byHash[h]; ok && o.key == e.key {
		delete(c.byHash, h)
	}
	c.stats.Evictions++
}

func (c *Cache) lookup(seq int, k Key) ([]uint64, bool) {
	c.stats.Lookups++
	if m, ok := c.planes[k]; ok {
		c.stats.Hits++
		return m, true
	}
	c.stats.Misses++
	h := c.hash(k)
	if o, ok := c.byHash[h]; ok && o.key != k {
		c.stats.ConflictCount++
		c.stats.Conflicts = appendLogged(c.stats.Conflicts, Conflict{Hash: h, Seq: seq, First: o.seq, Key: k})
	}
	return nil, false
}

func (c *Cache) noteDegenerate(seq int, t []geom.Point) {
	for i := 0; i < 3; i++ {
		for j := i + 1; j < 3; j++ {
			if t[i] == t[j] {
				c.stats.DegenerateCount++
				c.stats.Degenerate = appendLogged(c.stats.Degenerate, Degenerate{Seq: seq, Reason: "顶点重复"})
				return
			}
		}
	}
	area := (t[1].X-t[0].X)*(t[2].Y-t[0].Y) - (t[2].X-t[0].X)*(t[1].Y-t[0].Y)
	if area == 0 {
		c.stats.DegenerateCount++
		c.stats.Degenerate = appendLogged(c.stats.Degenerate, Degenerate{Seq: seq, Reason: "面积为零"})
	}
}

// FormatStats renders a multi-line report, including per-event entry numbers,
// for logs and the benchmark harness.
func FormatStats(s CacheStats) string {
	var b strings.Builder
	fmt.Fprintf(&b, "lookups=%d hits=%d misses=%d stores=%d hitrate=%.4f\n",
		s.Lookups, s.Hits, s.Misses, s.Stores, s.HitRate())
	fmt.Fprintf(&b, "peak_stored=%d peak_bytes=%d evictions=%d rejected=%d\n",
		s.PeakStored, s.PeakBytes, s.Evictions, s.Rejected)
	fmt.Fprintf(&b, "conflicts=%d entries=[%s]\n", s.ConflictCount, joinInts(conflictSeqs(s.Conflicts)))
	fmt.Fprintf(&b, "full=%d entries=[%s]\n", s.FullCount, joinInts(fullSeqs(s.Full)))
	fmt.Fprintf(&b, "degenerate=%d entries=[%s]\n", s.DegenerateCount, joinInts(degenerateSeqs(s.Degenerate)))
	return b.String()
}

func conflictSeqs(cs []Conflict) []int {
	out := make([]int, len(cs))
	for i, c := range cs {
		out[i] = c.Seq
	}
	return out
}

func fullSeqs(fs []Full) []int {
	out := make([]int, len(fs))
	for i, f := range fs {
		out[i] = f.Seq
	}
	return out
}

func degenerateSeqs(ds []Degenerate) []int {
	out := make([]int, len(ds))
	for i, d := range ds {
		out[i] = d.Seq
	}
	return out
}

func joinInts(v []int) string {
	sort.Ints(v)
	parts := make([]string, len(v))
	for i, n := range v {
		parts[i] = fmt.Sprint(n)
	}
	return strings.Join(parts, ",")
}
