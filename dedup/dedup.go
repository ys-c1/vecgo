// Package dedup detects and merges repeated figures before rasterisation.
//
// A figure is identified by its vertex set.  The fixed-length fingerprint is
// computed over the sorted, normalised vertex coordinates, so a figure keeps
// the same fingerprint when its vertices are listed in a different order or
// when the ring is rotated/reversed.
//
// A fixed-length fingerprint is deliberately allowed to be short: the length
// is back-solved from the observed duplicate rate with the birthday bound
// (see RequiredBits).  Fingerprint equality is never trusted on its own; the
// canonical vertex ring is compared before two figures are merged.  If two
// different figures nevertheless share a fingerprint, the collision is
// reported together with both figure numbers instead of silently dropping a
// figure.
package dedup

import (
	"encoding/binary"
	"fmt"
	"math"
	"sort"

	"vecgo/geom"
)

// MinVertices is the smallest number of vertices a figure may have.
const MinVertices = 3

// TargetCollision bounds the probability that two distinct vertex sets share
// the same fixed-length fingerprint.  2^-32 keeps the birthday bound well
// below the resolution of the PGM output while remaining short.
const TargetCollision = 1.0 / 4294967296.0

// ErrorKind classifies the per-figure errors reported by this package.
type ErrorKind int

const (
	// ErrEmptyFigure means the figure has no vertices at all.
	ErrEmptyFigure ErrorKind = iota + 1
	// ErrTooFewVertices means the figure has fewer than MinVertices vertices.
	ErrTooFewVertices
	// ErrFingerprintCollision means two distinct figures share a fingerprint,
	// or share a vertex set but cannot be merged without changing the image.
	ErrFingerprintCollision
)

// FigureError reports a problem with one or more figures.  Indices are
// zero-based positions in the submitted scene, matching the Go slice index.
type FigureError struct {
	Kind            ErrorKind
	Indices         []int
	FingerprintBits int
	Detail          string
}

func (e *FigureError) Error() string {
	switch e.Kind {
	case ErrEmptyFigure:
		return fmt.Sprintf("图形 %d 为空", e.Indices[0])
	case ErrTooFewVertices:
		return fmt.Sprintf("图形 %d 顶点不足三（%s）", e.Indices[0], e.Detail)
	case ErrFingerprintCollision:
		return fmt.Sprintf("指纹冲突：图形 %d 与图形 %d %s（指纹长度 %d 位）",
			e.Indices[0], e.Indices[1], e.Detail, e.FingerprintBits)
	default:
		return "图形错误"
	}
}

// Stats describes one merge pass.
type Stats struct {
	Total           int     // submitted figures
	Unique          int     // figures left after deduplication
	Duplicates      int     // Total - Unique
	DuplicateRate   float64 // Duplicates / Total, in [0,1]
	RequiredBits    int     // fingerprint length back-solved from the duplicate rate
	FingerprintBits int     // length actually used; never greater than RequiredBits
}

// Result is the outcome of a merge.
type Result struct {
	// Unique holds the representative vertices, one per distinct figure, in
	// first-appearance order.  The slices are copies and keep the original
	// coordinates (they are not canonicalised for drawing).
	Unique [][]geom.Point
	// Representatives holds the original figure number of each entry in Unique.
	Representatives []int
	// Groups holds the original figure numbers of every duplicate group, in
	// first-appearance order.  The first index in each group is its representative.
	Groups [][]int
	// Stats holds the counts for this pass.
	Stats Stats
}

// Merge deduplicates scene using a fingerprint of the given bit length.
//
// Figures with no vertices, fewer than three vertices, or a fingerprint
// matching a different figure are rejected with a *FigureError naming the
// figure number(s).
func Merge(scene [][]geom.Point, bits int) (*Result, error) {
	if bits < 0 {
		return nil, fmt.Errorf("指纹长度不能为负: %d", bits)
	}
	return group(scene, func(p []geom.Point) string {
		return fingerprintKey(p, bits)
	}, bits)
}

// MergeAuto deduplicates scene and uses the observed duplicate rate to
// back-solve the fingerprint length (RequiredBits).  The length actually used
// is never greater than that value.
func MergeAuto(scene [][]geom.Point) (*Result, error) {
	// The probe pass uses the full vertex-set key, i.e. an unbounded
	// fingerprint, so it can measure the duplicate rate without relying on a
	// truncated fingerprint.
	probe, err := group(scene, setKey, 0)
	if err != nil {
		return nil, err
	}
	total := probe.Stats.Total
	unique := probe.Stats.Unique
	rate := 0.0
	if total > 0 {
		rate = float64(total-unique) / float64(total)
	}
	required := RequiredBits(total, rate, TargetCollision)
	bits := required
	if bits > 64 {
		bits = 64
	}
	if bits > required {
		return nil, fmt.Errorf("内部错误：实际指纹长度 %d 超过反解值 %d", bits, required)
	}

	res, err := Merge(scene, bits)
	if err != nil {
		return nil, err
	}
	res.Stats.Total = total
	res.Stats.Unique = unique
	res.Stats.Duplicates = total - unique
	res.Stats.DuplicateRate = rate
	res.Stats.RequiredBits = required
	res.Stats.FingerprintBits = bits
	return res, nil
}

// RequiredBits back-solves the fingerprint length needed for a scene of total
// figures with the given duplicate rate.
//
// The number of distinct figures is d = round(total * (1 - duplicateRate)).
// For a b-bit fingerprint the birthday bound on the probability that two
// distinct figures collide is at most C(d,2) / 2^b, so requiring it to stay
// below target gives
//
//	b = ceil(log2(C(d,2) / target)).
//
// With target = 2^-32 this is ceil(log2(d*(d-1))) + 31 bits for d >= 2 and 0
// bits for d <= 1 (a single distinct figure needs no discriminating bits).
// Note that the more duplicates a scene has, the fewer distinct figures it
// contains and therefore the shorter the fingerprint it needs.
func RequiredBits(total int, duplicateRate, target float64) int {
	if total <= 0 {
		return 0
	}
	if duplicateRate < 0 {
		duplicateRate = 0
	}
	if duplicateRate > 1 {
		duplicateRate = 1
	}
	distinct := int(math.Round(float64(total) * (1 - duplicateRate)))
	if distinct < 1 {
		distinct = 1
	}
	return BirthdayBits(distinct, target)
}

// BirthdayBits returns the bit length at which the probability of a collision
// among distinct distinct figures is at most target.
func BirthdayBits(distinct int, target float64) int {
	if distinct <= 1 {
		return 0
	}
	if target <= 0 {
		target = TargetCollision
	}
	pairs := float64(distinct) * float64(distinct-1) / 2
	b := int(math.Ceil(math.Log2(pairs / target)))
	if b < 0 {
		b = 0
	}
	return b
}

// Fingerprint returns the fixed-length fingerprint of p's vertex set.  Vertex
// order is irrelevant: points are normalised and sorted first.
func Fingerprint(p []geom.Point, bits int) uint64 {
	if bits < 0 {
		bits = 0
	}
	pts := normalisedSorted(p)
	h := uint64(fnvOffset)
	h = fnvUint64(h, uint64(len(pts)))
	for _, q := range pts {
		h = fnvUint64(h, math.Float64bits(q.X))
		h = fnvUint64(h, math.Float64bits(q.Y))
	}
	return mask(h, bits)
}

const (
	fnvOffset = 14695981039346656037
	fnvPrime  = 1099511628211
)

func fnvByte(h uint64, b byte) uint64 {
	h ^= uint64(b)
	h *= fnvPrime
	return h
}

func fnvUint64(h, v uint64) uint64 {
	for i := 0; i < 8; i++ {
		h = fnvByte(h, byte(v>>(8*i)))
	}
	return h
}

func mask(h uint64, bits int) uint64 {
	if bits <= 0 {
		return 0
	}
	if bits >= 64 {
		return h
	}
	return h & ((uint64(1) << uint(bits)) - 1)
}

func fingerprintKey(p []geom.Point, bits int) string {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], Fingerprint(p, bits))
	return string(b[:])
}

func normalisedSorted(p []geom.Point) []geom.Point {
	// The fingerprint is taken over the vertex *set*, so a repeated vertex
	// does not create a new identity.
	out := make([]geom.Point, 0, len(p))
	for _, q := range p {
		out = append(out, normalise(q))
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].X != out[j].X {
			return out[i].X < out[j].X
		}
		return out[i].Y < out[j].Y
	})
	uniq := out[:0]
	for i, q := range out {
		if i > 0 && q == out[i-1] {
			continue
		}
		uniq = append(uniq, q)
	}
	return uniq
}

func normalise(p geom.Point) geom.Point {
	// Fold -0 into +0 so that a coordinate that prints identically also hashes
	// and compares identically.
	if p.X == 0 {
		p.X = 0
	}
	if p.Y == 0 {
		p.Y = 0
	}
	return p
}

func setKey(p []geom.Point) string {
	pts := normalisedSorted(p)
	buf := make([]byte, 0, 4+16*len(pts))
	var n [4]byte
	binary.BigEndian.PutUint32(n[:], uint32(len(pts)))
	buf = append(buf, n[:]...)
	var v [8]byte
	for _, q := range pts {
		binary.BigEndian.PutUint64(v[:], math.Float64bits(q.X))
		buf = append(buf, v[:]...)
		binary.BigEndian.PutUint64(v[:], math.Float64bits(q.Y))
		buf = append(buf, v[:]...)
	}
	return string(buf)
}

type groupState struct {
	total   int
	reps    []int
	rings   [][]geom.Point
	sets    []string
	members [][]int
}

func group(scene [][]geom.Point, keyOf func([]geom.Point) string, bits int) (*Result, error) {
	st := &groupState{total: len(scene)}
	buckets := make(map[string][]int, len(scene))
	for i, poly := range scene {
		if len(poly) == 0 {
			return nil, &FigureError{Kind: ErrEmptyFigure, Indices: []int{i}}
		}
		if len(poly) < MinVertices {
			return nil, &FigureError{
				Kind:    ErrTooFewVertices,
				Indices: []int{i},
				Detail:  fmt.Sprintf("%d 个", len(poly)),
			}
		}
		ring := canonicalRing(poly)
		set := setKey(poly)
		key := keyOf(poly)
		bucket := buckets[key]

		matched := -1
		for _, g := range bucket {
			if ringEqual(st.rings[g], ring) {
				matched = g
				break
			}
		}
		if matched >= 0 {
			st.members[matched] = append(st.members[matched], i)
			continue
		}
		if len(bucket) > 0 {
			j := bucket[0]
			indices := []int{j, i}
			sort.Ints(indices)
			detail := "碰撞但顶点集合不同"
			if st.sets[j] == set {
				detail = "顶点集合相同但环序不同，合并会改变画面"
			}
			return nil, &FigureError{
				Kind:            ErrFingerprintCollision,
				Indices:         indices,
				FingerprintBits: bits,
				Detail:          detail,
			}
		}
		buckets[key] = append(bucket, len(st.reps))
		st.reps = append(st.reps, i)
		st.rings = append(st.rings, ring)
		st.sets = append(st.sets, set)
		st.members = append(st.members, []int{i})
	}

	res := &Result{
		Unique:          make([][]geom.Point, 0, len(st.reps)),
		Representatives: make([]int, 0, len(st.reps)),
		Groups:          make([][]int, 0, len(st.reps)),
	}
	for g, rep := range st.reps {
		poly := append([]geom.Point(nil), scene[rep]...)
		res.Unique = append(res.Unique, poly)
		res.Representatives = append(res.Representatives, rep)
		res.Groups = append(res.Groups, append([]int(nil), st.members[g]...))
	}
	total := st.total
	unique := len(st.reps)
	rate := 0.0
	if total > 0 {
		rate = float64(total-unique) / float64(total)
	}
	res.Stats = Stats{
		Total:           total,
		Unique:          unique,
		Duplicates:      total - unique,
		DuplicateRate:   rate,
		RequiredBits:    0,
		FingerprintBits: bits,
	}
	return res, nil
}

// canonicalRing returns the ring in a canonical rotation and orientation.
//
// Even-odd filling (the rule used by raster.Image.Fill) depends only on the
// set of undirected edges, so rotating the ring or reversing its orientation
// describes the same filled figure.  Zero-length edges are also no-ops, so
// consecutive duplicate vertices (including a duplicated closing vertex) are
// folded away first; this does not change the vertex set, so the bounding box
// used by the rasteriser is untouched.  Choosing the lexicographically
// smallest rotation of the forward and reversed rings makes equivalent
// submissions compare equal.
func canonicalRing(poly []geom.Point) []geom.Point {
	pts := make([]geom.Point, len(poly))
	for i, p := range poly {
		pts[i] = normalise(p)
	}
	pts = foldRing(pts)
	best := minRotation(pts)
	rev := make([]geom.Point, len(pts))
	for i := range pts {
		rev[i] = pts[len(pts)-1-i]
	}
	if r := minRotation(rev); ringLess(r, best) {
		return r
	}
	return best
}

// foldRing removes zero-length edges from a closed ring by dropping vertices
// equal to their predecessor, including the wrap-around pair.
func foldRing(pts []geom.Point) []geom.Point {
	out := make([]geom.Point, 0, len(pts))
	for i, p := range pts {
		if i > 0 && p == pts[i-1] {
			continue
		}
		out = append(out, p)
	}
	if len(out) > 1 && out[len(out)-1] == out[0] {
		out = out[:len(out)-1]
	}
	return out
}

func minRotation(pts []geom.Point) []geom.Point {
	best := 0
	for i := 1; i < len(pts); i++ {
		if rotationLess(pts, i, best) {
			best = i
		}
	}
	out := make([]geom.Point, len(pts))
	for k := range out {
		out[k] = pts[(best+k)%len(pts)]
	}
	return out
}

func rotationLess(pts []geom.Point, i, j int) bool {
	n := len(pts)
	for k := 0; k < n; k++ {
		a := pts[(i+k)%n]
		b := pts[(j+k)%n]
		if a.X != b.X {
			return a.X < b.X
		}
		if a.Y != b.Y {
			return a.Y < b.Y
		}
	}
	return false
}

func ringLess(a, b []geom.Point) bool {
	for i := range a {
		if a[i].X != b[i].X {
			return a[i].X < b[i].X
		}
		if a[i].Y != b[i].Y {
			return a[i].Y < b[i].Y
		}
	}
	return false
}

func ringEqual(a, b []geom.Point) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].X != b[i].X || a[i].Y != b[i].Y {
			return false
		}
	}
	return true
}
