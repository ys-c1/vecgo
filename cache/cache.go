// Package cache implements a small, deterministic LRU cache used by the
// renderer to memoize per-polygon coverage masks.
//
// Two properties matter for long-running renders:
//
//   - the cache is bounded: at most Capacity entries are retained, and when a
//     new entry arrives the least-recently-used one is evicted;
//   - eviction is reproducible: recency is driven by a monotonic operation
//     clock, never by map iteration order, so an identical sequence of
//     operations produces an identical eviction trace.
//
// Every hit, miss, insertion, rejection and eviction is counted. Counters and
// per-entry hit counts can be exported as a human-readable table or as CSV.
package cache

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"sort"
	"text/tabwriter"
)

const (
	// entryOverheadBytes approximates the bookkeeping cost of one entry
	// (map slot, list links, sequence numbers). It is included in the
	// reported cache footprint so the numbers are not misleadingly small.
	entryOverheadBytes = 64

	// DefaultTraceLimit bounds the in-memory eviction trace. The rolling
	// digest still covers *all* evictions, so two runs can be compared even
	// after the ring has wrapped.
	DefaultTraceLimit = 4096

	fnvOffset64 uint64 = 14695981039346656037
	fnvPrime64  uint64 = 1099511628211
)

var (
	// ErrZeroCapacity is returned when a cache is created (or used) with a
	// non-positive capacity.
	ErrZeroCapacity = errors.New("cache: capacity must be positive")
	// ErrEmptyKey is returned when an empty key is inserted or looked up.
	ErrEmptyKey = errors.New("cache: key must not be empty")
	// ErrFull is returned by Insert (and by Put on a cache configured with
	// NoEvict) when the cache is at capacity.
	ErrFull = errors.New("cache: full")
)

// EntryInfo describes the entry involved in an error, as far as it is known.
// It is attached to every error returned by this package so callers can log
// actionable context instead of an opaque message.
type EntryInfo struct {
	Key      string
	Capacity int
	Entries  int
	Bytes    int
}

func (i EntryInfo) String() string {
	return fmt.Sprintf("key=%q capacity=%d entries=%d bytes=%d",
		i.Key, i.Capacity, i.Entries, i.Bytes)
}

// EntryError is an error tied to one cache entry.
type EntryError struct {
	Op   string
	Err  error
	Info EntryInfo
}

func (e *EntryError) Error() string {
	return fmt.Sprintf("cache %s: %v [%s]", e.Op, e.Err, e.Info)
}

// Unwrap lets errors.Is/As reach the sentinel (ErrZeroCapacity, ErrEmptyKey,
// ErrFull).
func (e *EntryError) Unwrap() error { return e.Err }

// Entry returns the entry information carried by the error.
func (e *EntryError) Entry() EntryInfo { return e.Info }

// Options configures a Cache.
type Options struct {
	// Capacity is the maximum number of live entries. It must be positive.
	Capacity int
	// NoEvict makes Put refuse to evict: once the cache is full, Put returns
	// ErrFull instead of dropping the least-recently-used entry. Insert
	// always behaves this way.
	NoEvict bool
	// TraceLimit bounds the retained eviction trace. Zero selects
	// DefaultTraceLimit; a negative value disables the in-memory ring while
	// keeping the digest.
	TraceLimit int
}

// Stats is a snapshot of the cache counters.
type Stats struct {
	Capacity      int
	Entries       int
	Bytes         int
	PeakBytes     int
	PeakEntries   int
	Hits          uint64
	Misses        uint64
	Inserts       uint64
	Updates       uint64
	Evictions     uint64
	Rejected      uint64
	HitRate       float64
	EvictionTrace int
	EvictionSum   string
}

// EntryStat is one row of the exportable per-entry accounting.
type EntryStat struct {
	Key       string
	Hits      int
	Bytes     int
	InsertSeq uint64
	LastSeq   uint64
}

type node struct {
	key       string
	val       []uint64
	hits      int
	bytes     int
	insertSeq uint64
	lastSeq   uint64
	prev      *node
	next      *node
}

// Cache is a bounded, deterministic LRU cache of []uint64 values.
//
// A Cache is not safe for concurrent use.
type Cache struct {
	cap     int
	noEvict bool

	m          map[string]*node
	head, tail *node // head is least recently used, tail most recently used

	clock uint64

	curBytes    int
	peakBytes   int
	peakEntries int

	hits, misses, inserts, updates, evictions, rejected uint64

	traceBuf   []string
	traceStart int
	traceN     int

	traceDigest uint64
}

// New returns a bounded LRU cache with the given entry capacity.
func New(capacity int) (*Cache, error) {
	return NewWithOptions(Options{Capacity: capacity})
}

// NewWithOptions returns a cache configured by opts.
func NewWithOptions(opts Options) (*Cache, error) {
	if opts.Capacity <= 0 {
		return nil, &EntryError{
			Op:   "New",
			Err:  ErrZeroCapacity,
			Info: EntryInfo{Capacity: opts.Capacity},
		}
	}
	traceLimit := opts.TraceLimit
	if traceLimit == 0 {
		traceLimit = DefaultTraceLimit
	}
	if traceLimit < 0 {
		traceLimit = 0
	}
	// Do not pre-size the map from an absurd user supplied capacity.
	mapHint := opts.Capacity
	if mapHint > 4096 {
		mapHint = 4096
	}
	return &Cache{
		cap:         opts.Capacity,
		noEvict:     opts.NoEvict,
		m:           make(map[string]*node, mapHint),
		traceBuf:    make([]string, traceLimit),
		traceN:      0,
		traceDigest: fnvOffset64,
	}, nil
}

// Capacity returns the configured entry capacity.
func (c *Cache) Capacity() int {
	if c == nil {
		return 0
	}
	return c.cap
}

// Len returns the number of live entries.
func (c *Cache) Len() int {
	if c == nil {
		return 0
	}
	return len(c.m)
}

func (c *Cache) info(key string) EntryInfo {
	if c == nil {
		return EntryInfo{Key: key}
	}
	return EntryInfo{Key: key, Capacity: c.cap, Entries: len(c.m), Bytes: c.curBytes}
}

func (c *Cache) zeroErr(key, op string) error {
	return &EntryError{Op: op, Err: ErrZeroCapacity, Info: c.info(key)}
}

// Lookup returns the value stored under key and advances its recency. The
// returned slice is owned by the cache and must be treated as read-only.
func (c *Cache) Lookup(key string) ([]uint64, bool, error) {
	if c == nil {
		return nil, false, &EntryError{Op: "Lookup", Err: ErrZeroCapacity, Info: EntryInfo{Key: key}}
	}
	if key == "" {
		c.misses++
		return nil, false, &EntryError{Op: "Lookup", Err: ErrEmptyKey, Info: c.info(key)}
	}
	if c.cap <= 0 {
		c.misses++
		return nil, false, c.zeroErr(key, "Lookup")
	}
	n, ok := c.m[key]
	if !ok {
		c.misses++
		return nil, false, nil
	}
	c.touch(n)
	c.hits++
	n.hits++
	return n.val, true, nil
}

// Get is Lookup without the error result. Invalid empty keys count as a miss.
func (c *Cache) Get(key string) ([]uint64, bool) {
	v, ok, _ := c.Lookup(key)
	return v, ok
}

// GetCopy returns a copy of the cached value so the caller may modify it.
func (c *Cache) GetCopy(key string) ([]uint64, bool) {
	v, ok := c.Get(key)
	if !ok {
		return nil, false
	}
	out := make([]uint64, len(v))
	copy(out, v)
	return out, true
}

// Put stores val under key, evicting the least-recently-used entry when the
// cache is full. Updating an existing key does not change the entry count.
// The cache keeps its own copy, so the caller may reuse val afterwards.
func (c *Cache) Put(key string, val []uint64) error {
	return c.write("Put", key, val, true)
}

// Insert is Put without eviction: it returns ErrFull (carrying the entry
// information) if the cache is already at capacity. It is the strict variant
// for callers that must never silently drop a cached value.
func (c *Cache) Insert(key string, val []uint64) error {
	return c.write("Insert", key, val, false)
}

func (c *Cache) write(op, key string, val []uint64, mayEvict bool) error {
	if c == nil {
		return &EntryError{Op: op, Err: ErrZeroCapacity, Info: EntryInfo{Key: key}}
	}
	if key == "" {
		c.rejected++
		return &EntryError{Op: op, Err: ErrEmptyKey, Info: c.info(key)}
	}
	if c.cap <= 0 {
		c.rejected++
		return c.zeroErr(key, op)
	}
	bytes := len(key) + 8*len(val) + entryOverheadBytes
	stored := append([]uint64(nil), val...)
	if n, ok := c.m[key]; ok {
		c.curBytes += bytes - n.bytes
		n.val = stored
		n.bytes = bytes
		c.updates++
		c.touch(n)
		c.notePeak()
		return nil
	}
	if len(c.m) >= c.cap {
		if !mayEvict || c.noEvict {
			c.rejected++
			return &EntryError{Op: op, Err: ErrFull, Info: c.info(key)}
		}
		c.evictOne()
	}
	c.clock++
	n := &node{
		key:       key,
		val:       stored,
		bytes:     bytes,
		insertSeq: c.clock,
		lastSeq:   c.clock,
	}
	c.m[key] = n
	c.pushBack(n)
	c.curBytes += bytes
	c.inserts++
	c.notePeak()
	return nil
}

func (c *Cache) notePeak() {
	if c.curBytes > c.peakBytes {
		c.peakBytes = c.curBytes
	}
	if len(c.m) > c.peakEntries {
		c.peakEntries = len(c.m)
	}
}

func (c *Cache) touch(n *node) {
	c.clock++
	n.lastSeq = c.clock
	c.unlink(n)
	c.pushBack(n)
}

func (c *Cache) evictOne() {
	n := c.head
	c.unlink(n)
	delete(c.m, n.key)
	c.curBytes -= n.bytes
	c.evictions++
	c.recordEviction(n.key)
}

func (c *Cache) recordEviction(key string) {
	// Rolling FNV-1a digest over the full eviction sequence. Two runs that
	// evict the same keys in the same order end with equal digests even when
	// the in-memory trace ring has wrapped.
	for i := 0; i < len(key); i++ {
		c.traceDigest ^= uint64(key[i])
		c.traceDigest *= fnvPrime64
	}
	c.traceDigest ^= 0xff
	c.traceDigest *= fnvPrime64

	if len(c.traceBuf) == 0 {
		return
	}
	if c.traceN < len(c.traceBuf) {
		c.traceBuf[c.traceN] = key
		c.traceN++
		return
	}
	c.traceBuf[c.traceStart] = key
	c.traceStart = (c.traceStart + 1) % len(c.traceBuf)
}

func (c *Cache) pushBack(n *node) {
	n.prev = c.tail
	n.next = nil
	if c.tail != nil {
		c.tail.next = n
	} else {
		c.head = n
	}
	c.tail = n
}

func (c *Cache) unlink(n *node) {
	if n.prev != nil {
		n.prev.next = n.next
	} else {
		c.head = n.next
	}
	if n.next != nil {
		n.next.prev = n.prev
	} else {
		c.tail = n.prev
	}
	n.prev, n.next = nil, nil
}

// EvictionTrace returns the retained eviction keys in chronological order.
// It is a copy; mutating it does not affect the cache.
func (c *Cache) EvictionTrace() []string {
	if c == nil || c.traceN == 0 {
		return nil
	}
	out := make([]string, c.traceN)
	for i := 0; i < c.traceN; i++ {
		out[i] = c.traceBuf[(c.traceStart+i)%len(c.traceBuf)]
	}
	return out
}

// EvictionDigest is a hex FNV-1a digest of every eviction, including those no
// longer retained in the bounded trace ring.
func (c *Cache) EvictionDigest() string {
	if c == nil {
		return fmt.Sprintf("%016x", fnvOffset64)
	}
	return fmt.Sprintf("%016x", c.traceDigest)
}

// Stats returns a snapshot of counters and footprint.
func (c *Cache) Stats() Stats {
	if c == nil {
		return Stats{EvictionSum: fmt.Sprintf("%016x", fnvOffset64)}
	}
	s := Stats{
		Capacity:      c.cap,
		Entries:       len(c.m),
		Bytes:         c.curBytes,
		PeakBytes:     c.peakBytes,
		PeakEntries:   c.peakEntries,
		Hits:          c.hits,
		Misses:        c.misses,
		Inserts:       c.inserts,
		Updates:       c.updates,
		Evictions:     c.evictions,
		Rejected:      c.rejected,
		EvictionTrace: c.traceN,
		EvictionSum:   c.EvictionDigest(),
	}
	if total := c.hits + c.misses; total > 0 {
		s.HitRate = float64(c.hits) / float64(total)
	}
	return s
}

// Entries returns per-entry accounting sorted by insertion sequence, which is
// deterministic. Only live entries are reported; evicted keys appear in the
// trace instead, keeping the metadata footprint bounded by Capacity.
func (c *Cache) Entries() []EntryStat {
	if c == nil || len(c.m) == 0 {
		return nil
	}
	out := make([]EntryStat, 0, len(c.m))
	for _, n := range c.m {
		out = append(out, EntryStat{
			Key:       n.key,
			Hits:      n.hits,
			Bytes:     n.bytes,
			InsertSeq: n.insertSeq,
			LastSeq:   n.lastSeq,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].InsertSeq != out[j].InsertSeq {
			return out[i].InsertSeq < out[j].InsertSeq
		}
		return out[i].Key < out[j].Key
	})
	return out
}

// WriteTable writes a deterministic human-readable report: global counters,
// one row per live entry (key and hit count), and the eviction trace.
func (c *Cache) WriteTable(w io.Writer) error {
	s := c.Stats()
	if _, err := fmt.Fprintf(w, "cache capacity=%d entries=%d bytes=%d peak_bytes=%d peak_entries=%d\n",
		s.Capacity, s.Entries, s.Bytes, s.PeakBytes, s.PeakEntries); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "hits=%d misses=%d hit_rate=%.6f inserts=%d updates=%d evictions=%d rejected=%d\n",
		s.Hits, s.Misses, s.HitRate, s.Inserts, s.Updates, s.Evictions, s.Rejected); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "eviction_trace=%d eviction_digest=%s\n", s.EvictionTrace, s.EvictionSum); err != nil {
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "seq\tlast\tkey\thits\tbytes"); err != nil {
		return err
	}
	for _, e := range c.Entries() {
		if _, err := fmt.Fprintf(tw, "%d\t%d\t%s\t%d\t%d\n",
			e.InsertSeq, e.LastSeq, e.Key, e.Hits, e.Bytes); err != nil {
			return err
		}
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	if trace := c.EvictionTrace(); len(trace) > 0 {
		if _, err := fmt.Fprintln(w, "evictions (oldest first):"); err != nil {
			return err
		}
		for _, k := range trace {
			if _, err := fmt.Fprintln(w, k); err != nil {
				return err
			}
		}
	}
	return nil
}

// WriteCSV writes per-entry accounting as CSV with a stable header and row
// order. The eviction digest is emitted as a comment line prefixed with '#'.
func (c *Cache) WriteCSV(w io.Writer) error {
	s := c.Stats()
	if _, err := fmt.Fprintf(w, "# capacity=%d entries=%d hits=%d misses=%d evictions=%d eviction_digest=%s\n",
		s.Capacity, s.Entries, s.Hits, s.Misses, s.Evictions, s.EvictionSum); err != nil {
		return err
	}
	cw := csv.NewWriter(w)
	if err := cw.Write([]string{"key", "hits", "bytes", "insert_seq", "last_seq"}); err != nil {
		return err
	}
	for _, e := range c.Entries() {
		if err := cw.Write([]string{
			e.Key,
			fmt.Sprint(e.Hits),
			fmt.Sprint(e.Bytes),
			fmt.Sprint(e.InsertSeq),
			fmt.Sprint(e.LastSeq),
		}); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

// WriteEvictionTrace writes one evicted key per line followed by a final
// "#digest=<hex>" line. Two runs can be compared byte-for-byte.
func (c *Cache) WriteEvictionTrace(w io.Writer) error {
	for _, k := range c.EvictionTrace() {
		if _, err := fmt.Fprintln(w, k); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintf(w, "#digest=%s\n", c.EvictionDigest())
	return err
}

// Clear drops all entries and resets the footprint, keeping cumulative
// counters and the eviction digest.
func (c *Cache) Clear() {
	if c == nil {
		return
	}
	c.m = make(map[string]*node, c.cap)
	c.head, c.tail = nil, nil
	c.curBytes = 0
}
