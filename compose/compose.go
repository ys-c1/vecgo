// Package compose contains the order-experiment compositing pipeline.
//
// The default vecgo pipeline remains grayscale; this package is additive and
// is exercised by cmd/composedemo.
package compose

import (
	"math"
	"sort"
)

// FractionBits is the number of binary fractional bits in the internal
// fixed-point representation.  Normalized values are q / 2^FractionBits;
// q == Scale encodes 1.0 and fits in uint32.  Multiplication is widened to
// uint64.
const FractionBits = 16

const (
	Scale     = uint64(1) << FractionBits
	halfScale = Scale / 2
)

// Fragment is one colored translucent sample at a pixel.
type Fragment struct {
	ID    int
	Depth int
	RGB   [3]uint8
	// A is source coverage/alpha in Q16.16 normalized fixed point.
	A uint64
}

// AlphaForCoverage maps supersample coverage to the scene's one-half opacity.
// With 4x supersampling this is coverage / 32, exactly representable in Q16.
func AlphaForCoverage(coverage, samples int) uint64 {
	den := uint64(2 * samples * samples)
	return (Scale*uint64(coverage) + den/2) / den
}

// ColorToQ rounds an 8-bit sRGB code/channel value to Q16.
func ColorToQ(c uint8) uint64 {
	return (uint64(c)*Scale + 127) / 255
}

// MulQ computes round_half_up(a*b/Scale) in Q16.
func MulQ(a, b uint64) uint64 {
	return (a*b + halfScale) >> FractionBits
}

func clampQ(v uint64) uint64 {
	if v > Scale {
		return Scale
	}
	return v
}

func ordered(frags []Fragment, byDepth bool) []Fragment {
	out := append([]Fragment(nil), frags...)
	if byDepth {
		// Stable tie-break by (depth, id) makes equal-depth fragments total.
		sort.SliceStable(out, func(i, j int) bool {
			if out[i].Depth != out[j].Depth {
				return out[i].Depth < out[j].Depth
			}
			return out[i].ID < out[j].ID
		})
	}
	return out
}

func qToU8(v uint64) uint8 {
	v = clampQ(v)
	return uint8((v*255 + halfScale) / Scale)
}

// PremultiplyQ returns round_Q((c/255) * (a/Scale)) in Q16.
func PremultiplyQ(c uint8, a uint64) uint64 {
	return MulQ(ColorToQ(c), a)
}

// CompositeQValues is CompositeQ before final 8-bit output rounding.
func CompositeQValues(bg [3]uint8, frags []Fragment, byDepth, premult bool) [3]uint64 {
	fs := ordered(frags, byDepth)
	var dst [3]uint64
	for c := range bg {
		dst[c] = ColorToQ(bg[c]) // opaque background: premultiplied == straight
	}

	for _, f := range fs {
		a := f.A
		if a > Scale {
			a = Scale
		}
		oneMinusA := Scale - a
		for c := 0; c < 3; c++ {
			cq := ColorToQ(f.RGB[c])
			if premult {
				src := MulQ(cq, a)
				dst[c] = clampQ(src + MulQ(dst[c], oneMinusA))
			} else {
				srcPart := MulQ(a, cq)
				dstPart := MulQ(oneMinusA, dst[c])
				dst[c] = clampQ(srcPart + dstPart)
			}
		}
	}
	return dst
}

// CompositeQ runs the deterministic fixed-point pipeline and rounds once to
// 8 bits per channel.
func CompositeQ(bg [3]uint8, frags []Fragment, byDepth, premult bool) [3]uint8 {
	dst := CompositeQValues(bg, frags, byDepth, premult)
	var out [3]uint8
	for c := range dst {
		out[c] = qToU8(dst[c])
	}
	return out
}

// StraightFloat is the reference non-premultiplied blend:
// out = alpha*color + (1-alpha)*destination, in supplied fragment order.
// Returned channels are in 8-bit code units [0,255].
func StraightFloat(bg [3]uint8, frags []Fragment) [3]float64 {
	var dst [3]float64
	for c := range bg {
		dst[c] = float64(bg[c])
	}
	for _, f := range frags {
		a := float64(f.A) / float64(Scale)
		for c := 0; c < 3; c++ {
			dst[c] = a*float64(f.RGB[c]) + (1-a)*dst[c]
		}
	}
	return dst
}

// CompositeFloatQInputs is the unrounded rational counterpart of CompositeQ.
// It uses the same quantized Q16 input values but performs no MulQ rounding.
// Returned channels are normalized values multiplied by 255 (8-bit units).
func CompositeFloatQInputs(bg [3]uint8, frags []Fragment, byDepth, premult bool) [3]float64 {
	fs := ordered(frags, byDepth)
	var dst [3]float64
	for c := range bg {
		dst[c] = float64(ColorToQ(bg[c])) / float64(Scale)
	}
	for _, f := range fs {
		a := float64(f.A) / float64(Scale)
		oneMinusA := 1 - a
		for c := 0; c < 3; c++ {
			cq := float64(ColorToQ(f.RGB[c])) / float64(Scale)
			if premult {
				src := cq * a
				dst[c] = src + dst[c]*oneMinusA
			} else {
				dst[c] = a*cq + oneMinusA*dst[c]
			}
		}
	}
	var out [3]float64
	for c := range dst {
		out[c] = dst[c] * 255
	}
	return out
}

// MaxChannelError returns max_c |a[c]-b[c]|.
func MaxChannelError(a, b [3]float64) float64 {
	m := 0.0
	for c := 0; c < 3; c++ {
		if d := math.Abs(a[c] - b[c]); d > m {
			m = d
		}
	}
	return m
}

// MaxU8ChannelDiff returns the maximum integer channel difference.
func MaxU8ChannelDiff(a, b [3]uint8) int {
	m := 0
	for c := 0; c < 3; c++ {
		d := int(a[c]) - int(b[c])
		if d < 0 {
			d = -d
		}
		if d > m {
			m = d
		}
	}
	return m
}

// OrderBound is B(p)=sum_{i<j} alpha_i alpha_j max_c |C_i-C_j| in 8-bit
// code units.  It bounds the real-valued difference between any two fragment
// permutations under StraightFloat.
func OrderBound(frags []Fragment) float64 {
	b := 0.0
	for i := 0; i < len(frags); i++ {
		ai := float64(frags[i].A) / float64(Scale)
		for j := i + 1; j < len(frags); j++ {
			aj := float64(frags[j].A) / float64(Scale)
			d := 0.0
			for c := 0; c < 3; c++ {
				if v := math.Abs(float64(frags[i].RGB[c]) - float64(frags[j].RGB[c])); v > d {
					d = v
				}
			}
			b += ai * aj * d
		}
	}
	return b
}

// PairSwapBound gives the tighter adjacent-swap bound for two specific
// permutations and a bubble-sort path.  The supplied swaps are pairs of
// fragment IDs; downstream survival is computed from the fragments that
// follow the swapped adjacent pair in the path state.  For simplicity and
// robustness callers can use OrderBound; this helper evaluates a provided
// conservative attenuation for each pair.
func PairSwapBound(swaps []SwapTerm) float64 {
	b := 0.0
	for _, s := range swaps {
		b += s.AlphaI * s.AlphaJ * s.ColorDistance * s.DownstreamSurvival
	}
	return b
}

// SwapTerm is one adjacent transposition in a bubble-sort proof.
type SwapTerm struct {
	AlphaI             float64
	AlphaJ             float64
	ColorDistance      float64
	DownstreamSurvival float64
}
