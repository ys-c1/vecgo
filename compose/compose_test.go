package compose

import (
	"math"
	"testing"
)

func TestAdjacentSwapIdentityAndBound(t *testing.T) {
	bg := [3]uint8{248, 246, 240}
	a := Fragment{ID: 0, RGB: [3]uint8{230, 40, 40}, A: Scale / 2}
	b := Fragment{ID: 1, RGB: [3]uint8{50, 90, 230}, A: Scale / 4}

	ab := StraightFloat(bg, []Fragment{a, b})
	ba := StraightFloat(bg, []Fragment{b, a})
	for c := 0; c < 3; c++ {
		want := 0.125 * (float64(b.RGB[c]) - float64(a.RGB[c]))
		got := ab[c] - ba[c]
		if math.Abs(got-want) > 1e-12 {
			t.Fatalf("channel %d swap identity = %.12f, want %.12f", c, got, want)
		}
	}

	bound := OrderBound([]Fragment{a, b})
	if diff := MaxChannelError(ab, ba); diff > bound+1e-12 {
		t.Fatalf("swap error %.9f exceeds bound %.9f", diff, bound)
	}
}

func TestStablePipelineMakesThreeOrdersIdentical(t *testing.T) {
	for idx, fs := range DemoPixelFragments() {
		var first [3]uint8
		for g, perm := range DemoPermutations {
			ordered := Reorder(fs, perm)
			got := CompositeQ(DemoBackground, ordered, true, true)
			if g == 0 {
				first = got
				continue
			}
			if got != first {
				t.Fatalf("pixel %d order %d = %v, want %v", idx, g, got, first)
			}
		}
	}
}

func TestStepRolesInAblation(t *testing.T) {
	stableMax := 0
	premultUnsortedMax := 0
	straightSortedMax := 0
	for _, fs := range DemoPixelFragments() {
		var stable [3][3]uint8
		var premultUnsorted [3][3]uint8
		var straightSorted [3][3]uint8
		for g, perm := range DemoPermutations {
			ordered := Reorder(fs, perm)
			stable[g] = CompositeQ(DemoBackground, ordered, true, true)
			premultUnsorted[g] = CompositeQ(DemoBackground, ordered, false, true)
			straightSorted[g] = CompositeQ(DemoBackground, fs, true, false)
		}
		for i := 0; i < 3; i++ {
			for j := i + 1; j < 3; j++ {
				stableMax = maxTestInt(stableMax, MaxU8ChannelDiff(stable[i], stable[j]))
				premultUnsortedMax = maxTestInt(premultUnsortedMax, MaxU8ChannelDiff(premultUnsorted[i], premultUnsorted[j]))
				straightSortedMax = maxTestInt(straightSortedMax, MaxU8ChannelDiff(straightSorted[i], straightSorted[j]))
			}
		}
	}

	if stableMax != 0 {
		t.Fatalf("complete pipeline max diff = %d, want 0", stableMax)
	}
	if straightSortedMax != 0 {
		t.Fatalf("depth sorting alone must canonicalize permutations, got diff %d", straightSortedMax)
	}
	if premultUnsortedMax == 0 {
		t.Fatal("premultiplication without canonical depth order is not commutative; scene should show a difference")
	}
}

func TestPremultiplyQRoundingBound(t *testing.T) {
	const maxCodeError = 255.0 / (2.0 * float64(Scale))
	for c := 0; c < 256; c++ {
		for hits := 0; hits <= DemoSamples*DemoSamples; hits++ {
			a := AlphaForCoverage(hits, DemoSamples)
			got := float64(PremultiplyQ(uint8(c), a)) / float64(Scale) * 255
			// Compare against the already Q16-rounded color input; conversion
			// error is a separate input-quantization term.
			want := (float64(ColorToQ(uint8(c))) / float64(Scale)) *
				(float64(a) / float64(Scale)) * 255
			if d := math.Abs(got - want); d > maxCodeError+1e-12 {
				t.Fatalf("c=%d hits=%d error %.9f exceeds %.9f", c, hits, d, maxCodeError)
			}
		}
	}
}

func TestOrderBoundHoldsForAllSixPermutations(t *testing.T) {
	ids := []int{0, 1, 2}
	perms := allPermutations(ids)
	for _, fs := range DemoPixelFragments() {
		bound := OrderBound(fs)
		for i := 0; i < len(perms); i++ {
			for j := i + 1; j < len(perms); j++ {
				a := StraightFloat(DemoBackground, Reorder(fs, perms[i]))
				b := StraightFloat(DemoBackground, Reorder(fs, perms[j]))
				if d := MaxChannelError(a, b); d > bound+1e-9 {
					t.Fatalf("error %.9f exceeds bound %.9f for %v vs %v", d, bound, perms[i], perms[j])
				}
			}
		}
	}
}

func allPermutations(in []int) [][]int {
	if len(in) == 0 {
		return [][]int{{}}
	}
	var out [][]int
	for i, v := range in {
		rest := append([]int{}, in[:i]...)
		rest = append(rest, in[i+1:]...)
		for _, p := range allPermutations(rest) {
			out = append(out, append([]int{v}, p...))
		}
	}
	return out
}

func maxTestInt(a, b int) int {
	if b > a {
		return b
	}
	return a
}
