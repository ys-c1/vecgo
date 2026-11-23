// Package cull implements batch frustum culling for bounding spheres.
//
// A frustum is six planes written as N·x + D = 0. The positive half-space
// N·x + D >= 0 is the inside half-space. Normals are not required to be unit
// length: every plane is normalized once, before any sphere is tested.
//
// For a normalized plane (unit N) and a sphere with center c and radius r,
// the support point in the +N direction is c + rN. Its signed distance,
//
//	maxDist = N·c + D + r
//
// is the largest signed distance reached by any point of the sphere. The
// support point in the -N direction gives
//
//	minDist = N·c + D - r.
//
// Therefore:
//
//	maxDist < 0   => the whole sphere is outside this half-space,
//	minDist > 0   => the whole sphere is inside this half-space,
//	otherwise     => the sphere touches or crosses the plane.
//
// A sphere is outside the frustum as soon as it is outside one plane, inside
// when it is inside all six planes, and intersecting otherwise. Batch culling
// never reports "outside" unless maxDist is negative beyond the derived
// round-off tolerance, so a sphere that intersects the frustum is never
// dropped. With u = 2⁻⁵³, the tolerance is an upper bound on the absolute
// error of both maxDist and minDist:
//
//	Tol = 24u·(||c||₂ + |D| + r + |s| + |maxDist|) + 8u
//
// where D is the normalized plane offset and s = N·c + D is the center's
// signed distance. Derivation:
//
//   - Normalization divides each component by the exact m = max|Nᵢ| and by
//     the rounded norm. The quotient is within u; squaring, summing and the
//     square root add about 4u; the final division another u. Together with
//     the component's own rounding, every normalized component and D itself
//     is within 7u relative error of exact.
//   - A normalized component's absolute error is thus ≤ 7u, and the dot
//     product's normalization error is at most 7u·Σ|cᵢ| ≤ 7√3·u·||c||₂.
//   - Float evaluation of the 3-term dot product adds at most 3u·||c||₂ by
//     Cauchy–Schwarz (each product, and the two additions, are ≤ u of the
//     running magnitude).
//   - Together the dot product is within 16u·||c||₂ of exact. Adding D then
//     adds ≤ 7u·|D| and one rounding ≤ u·|s|; adding r adds one rounding
//     ≤ u·|maxDist|.
//   - For minDist = s - r, |minDist| ≤ 2|s| + |maxDist|, so the same
//     magnitudes cover its single rounded subtraction.
//
// The coefficient 24u therefore dominates the 16u, 7u and u terms with
// margin, and the +8u term covers subnormal rounding. TestToleranceBoundHolds
// checks measured error against this bound.
package cull

import (
	"errors"
	"fmt"
	"math"
)

// unitRoundoff is 2⁻⁵³, the unit roundoff for float64 round-to-nearest.
const unitRoundoff = 0x1p-53

// Vec3 is a point or direction in 3D.
type Vec3 struct {
	X, Y, Z float64
}

// Sphere is a bounding sphere.
type Sphere struct {
	Center Vec3
	Radius float64
}

// Plane is the plane N·x + D = 0. N is the normal and need not be unit length.
type Plane struct {
	N Vec3
	D float64
}

// Frustum is the six planes of a view frustum.
type Frustum struct {
	Planes [6]Plane
}

// State is the three-state result of sphere/frustum classification.
type State uint8

const (
	// Outside means the sphere is completely outside at least one plane.
	Outside State = iota
	// Intersect means the sphere touches or crosses at least one plane and
	// is not completely outside any plane. It is conservative: it may be
	// reported for a sphere that misses the frustum near an edge or corner.
	Intersect
	// Inside means the sphere is completely inside all six planes.
	Inside
)

func (s State) String() string {
	switch s {
	case Outside:
		return "outside"
	case Intersect:
		return "intersect"
	case Inside:
		return "inside"
	default:
		return "unknown"
	}
}

// Sentinel errors reported by Cull, wrapped in *CullError.
var (
	ErrEmptyScene      = errors.New("场景为空")
	ErrDegeneratePlane = errors.New("平面退化")
	ErrZeroRadius      = errors.New("包围球半径为零")
	ErrInvalidSphere   = errors.New("包围球非法")
)

// CullError reports which object or plane made a batch invalid. Object and
// Plane are zero-based indices, or -1 when the error has no such subject.
type CullError struct {
	Kind   error
	Object int
	Plane  int
	Detail string
}

func (e *CullError) Error() string {
	switch {
	case errors.Is(e.Kind, ErrEmptyScene):
		return "场景为空: 物体数 0（物体号: 无）"
	case errors.Is(e.Kind, ErrDegeneratePlane):
		if e.Detail != "" {
			return fmt.Sprintf("平面 %d 退化: %s（物体号: 无）", e.Plane, e.Detail)
		}
		return fmt.Sprintf("平面 %d 退化（物体号: 无）", e.Plane)
	case errors.Is(e.Kind, ErrZeroRadius):
		return fmt.Sprintf("物体 %d 的包围球半径为零（物体号: %d）", e.Object, e.Object)
	case errors.Is(e.Kind, ErrInvalidSphere):
		return fmt.Sprintf("物体 %d 的包围球非法: %s（物体号: %d）", e.Object, e.Detail, e.Object)
	default:
		return e.Kind.Error()
	}
}

// Unwrap lets errors.Is(err, ErrZeroRadius) and friends work.
func (e *CullError) Unwrap() error { return e.Kind }

// NormalizePlane divides N and D by ||N||₂. It is safe against overflow and
// underflow from squaring or adding the normal components. A zero or
// non-finite normal, or an offset that cannot be represented after
// normalization, is rejected as a degenerate plane.
func NormalizePlane(p Plane) (Plane, error) {
	if !isFinite(p.N.X) || !isFinite(p.N.Y) || !isFinite(p.N.Z) || !isFinite(p.D) {
		return Plane{}, fmt.Errorf("%w: 法向或偏移含非有限值", ErrDegeneratePlane)
	}
	m := math.Max(math.Abs(p.N.X), math.Max(math.Abs(p.N.Y), math.Abs(p.N.Z)))
	if m == 0 {
		return Plane{}, fmt.Errorf("%w: 法向长度为零", ErrDegeneratePlane)
	}
	ux, uy, uz := p.N.X/m, p.N.Y/m, p.N.Z/m
	l := math.Sqrt(ux*ux + uy*uy + uz*uz)
	if !(l > 0) || !isFinite(l) {
		return Plane{}, fmt.Errorf("%w: 法向长度不可用", ErrDegeneratePlane)
	}
	d := (p.D / m) / l
	if !isFinite(d) {
		return Plane{}, fmt.Errorf("%w: 归一化后的偏移量溢出", ErrDegeneratePlane)
	}
	return Plane{
		N: Vec3{X: ux / l, Y: uy / l, Z: uz / l},
		D: d,
	}, nil
}

// support returns the extreme signed distances of the sphere along the plane
// normal plus the derived absolute-error bound for those distances. Plane p
// must already be normalized.
func support(p Plane, s Sphere) (maxDist, minDist, tol float64, ok bool) {
	ax := p.N.X * s.Center.X
	ay := p.N.Y * s.Center.Y
	az := p.N.Z * s.Center.Z
	dot := ax + ay + az
	mid := dot + p.D
	maxDist = mid + s.Radius
	minDist = mid - s.Radius

	if !isFinite(dot) || !isFinite(mid) || !isFinite(minDist) {
		return maxDist, minDist, tol, false
	}

	// Sum the magnitudes in a scaled form so the final multiply back cannot
	// overflow before it is multiplied by 24u.
	normC := scaledNorm(s.Center)
	m := math.Max(normC, math.Max(math.Abs(p.D), s.Radius))
	m = math.Max(m, math.Max(math.Abs(mid), math.Abs(maxDist)))
	var sum float64
	if m > 0 {
		sum = normC/m + math.Abs(p.D)/m + s.Radius/m +
			math.Abs(mid)/m + math.Abs(maxDist)/m
	}
	tol = 24*unitRoundoff*m*sum + 8*unitRoundoff

	if !isFinite(tol) {
		return maxDist, minDist, tol, false
	}
	return maxDist, minDist, tol, true
}

// scaledNorm returns m·||v||₂/ m for m = max|vᵢ|, avoiding intermediate
// overflow and underflow.
func scaledNorm(v Vec3) float64 {
	m := math.Max(math.Abs(v.X), math.Max(math.Abs(v.Y), math.Abs(v.Z)))
	if m == 0 {
		return 0
	}
	x, y, z := v.X/m, v.Y/m, v.Z/m
	return m * math.Sqrt(x*x+y*y+z*z)
}

// MaxSignedDistance normalizes p and returns the largest signed distance from
// any point of s to the plane, along with the derived absolute-error bound.
// The support point realizing the maximum is c + rN.
func MaxSignedDistance(p Plane, s Sphere) (maxDist, tol float64, err error) {
	np, err := NormalizePlane(p)
	if err != nil {
		return 0, 0, err
	}
	if !finiteSphere(s) {
		return 0, 0, ErrInvalidSphere
	}
	maxDist, _, tol, ok := support(np, s)
	if !ok {
		return 0, 0, errors.New("距离计算溢出")
	}
	return maxDist, tol, nil
}

// ClassifySphere is the per-object reference classification for a frustum.
// It keeps the original one-object-at-a-time interface and is intentionally
// simple; Cull is the batch path. Invalid input is classified conservatively
// as Intersect, never Outside.
func ClassifySphere(f Frustum, s Sphere) State {
	if math.IsNaN(s.Radius) || math.IsInf(s.Radius, 0) || s.Radius < 0 || !finiteCenter(s.Center) {
		return Intersect
	}
	state := Inside
	for i := range f.Planes {
		np, err := NormalizePlane(f.Planes[i])
		if err != nil {
			return Intersect
		}
		maxDist, minDist, tol, ok := support(np, s)
		if !ok {
			return Intersect
		}
		if maxDist < -tol {
			return Outside
		}
		if minDist <= tol && state == Inside {
			state = Intersect
		}
	}
	return state
}

// Visible reports whether a sphere should be kept. Intersect and Inside are
// visible; only Outside is culled.
func Visible(f Frustum, s Sphere) bool {
	return ClassifySphere(f, s) != Outside
}

// Stats summarizes one Cull call.
type Stats struct {
	// Total is the number of input spheres.
	Total int
	// Inside, Intersect and Outside are the three-state counts.
	Inside, Intersect, Outside int
	// ObjectVisits is the number of distinct objects that received at least
	// one sphere/plane test.
	ObjectVisits int
	// PlaneTests is the number of sphere/plane tests actually performed.
	// Early rejection makes this smaller than 6*Total.
	PlaneTests int
	// Culled is the number of objects rejected by one failing plane test.
	// It always equals Outside.
	Culled int
	// MaxTolerance is the largest derived tolerance bound used.
	MaxTolerance float64
}

// Result is the batch classification plus its summary.
type Result struct {
	States []State
	Stats
}

// Cull classifies every sphere against the frustum in batches. The six planes
// are normalized once; each batch then walks an active list and removes
// spheres as soon as any plane proves them outside. The returned states are
// exactly equal to calling ClassifySphere on each sphere with the same input.
//
// An empty scene, a degenerate plane, or a zero/negative/invalid sphere radius
// is rejected with a *CullError naming the offending object or plane.
func Cull(f Frustum, spheres []Sphere) (*Result, error) {
	if len(spheres) == 0 {
		return nil, &CullError{Kind: ErrEmptyScene, Object: -1, Plane: -1}
	}

	var planes [6]Plane
	for i := range f.Planes {
		p, err := NormalizePlane(f.Planes[i])
		if err != nil {
			return nil, &CullError{
				Kind:   ErrDegeneratePlane,
				Object: -1,
				Plane:  i,
				Detail: err.Error(),
			}
		}
		planes[i] = p
	}

	for i, s := range spheres {
		if !finiteCenter(s.Center) || math.IsNaN(s.Radius) || math.IsInf(s.Radius, 0) {
			return nil, &CullError{
				Kind:   ErrInvalidSphere,
				Object: i,
				Plane:  -1,
				Detail: "球心或半径含非有限值",
			}
		}
		if s.Radius == 0 {
			return nil, &CullError{Kind: ErrZeroRadius, Object: i, Plane: -1}
		}
		if s.Radius < 0 {
			return nil, &CullError{
				Kind:   ErrInvalidSphere,
				Object: i,
				Plane:  -1,
				Detail: "半径为负",
			}
		}
	}

	states := make([]State, len(spheres))
	for i := range states {
		states[i] = Inside
	}

	active := make([]int, len(spheres))
	for i := range active {
		active[i] = i
	}

	stats := Stats{
		Total:        len(spheres),
		ObjectVisits: len(spheres),
	}

	cullCount := 0
	for pi := 0; pi < len(planes) && len(active) > 0; pi++ {
		p := planes[pi]
		keep := active[:0]
		for _, idx := range active {
			maxDist, minDist, tol, ok := support(p, spheres[idx])
			if !ok {
				return nil, &CullError{
					Kind:   ErrInvalidSphere,
					Object: idx,
					Plane:  pi,
					Detail: "距离计算溢出",
				}
			}
			stats.PlaneTests++
			if tol > stats.MaxTolerance {
				stats.MaxTolerance = tol
			}
			if maxDist < -tol {
				// The support point in the plane-normal direction is
				// strictly behind the plane, beyond round-off: no point of
				// the sphere can be inside this half-space.
				states[idx] = Outside
				cullCount++
				continue
			}
			if minDist <= tol && states[idx] == Inside {
				states[idx] = Intersect
			}
			keep = append(keep, idx)
		}
		active = keep
	}

	for _, st := range states {
		switch st {
		case Inside:
			stats.Inside++
		case Intersect:
			stats.Intersect++
		case Outside:
			stats.Outside++
		}
	}
	stats.Culled = cullCount
	return &Result{States: states, Stats: stats}, nil
}

// SkippedPlaneTests reports how many sphere/plane tests early rejection saved
// relative to testing every sphere against all six planes.
func (s Stats) SkippedPlaneTests() int {
	return 6*s.Total - s.PlaneTests
}

// Summary renders the three-state counts, the number of visited objects and
// the derived tolerance bound in one line, for end-of-run reporting.
func (s Stats) Summary() string {
	return fmt.Sprintf(
		"total=%d inside=%d intersect=%d outside=%d visits=%d planeTests=%d culled=%d skipped=%d maxTol=%.3g",
		s.Total, s.Inside, s.Intersect, s.Outside,
		s.ObjectVisits, s.PlaneTests, s.Culled, s.SkippedPlaneTests(), s.MaxTolerance,
	)
}

func finiteCenter(c Vec3) bool {
	return isFinite(c.X) && isFinite(c.Y) && isFinite(c.Z)
}

func finiteSphere(s Sphere) bool {
	return finiteCenter(s.Center) && isFinite(s.Radius)
}

func isFinite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}
