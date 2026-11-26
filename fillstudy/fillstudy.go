// Package fillstudy measures how the even-odd and non-zero fill rules differ on
// self-intersecting polygons. It reports, for a shape, the covered-pixel counts
// under both rules, the crossing-direction statistics, and named witnesses for
// samples where the rules disagree.
package fillstudy

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"vecgo/geom"
	"vecgo/raster"
)

// Shape is a named closed path plus the provenance of its self-intersections.
type Shape struct {
	Name string
	// Why explains in one line what self-intersections this shape has.
	Why  string
	Poly []geom.Point
}

// RuleStats is one shape measured under both rules on one sampling grid.
type RuleStats struct {
	Shape Shape
	// Frame is the sampling frame, so every reported coordinate is
	// reproducible from it.
	Frame raster.Frame
	// Supersample is the per-axis supersample count used.
	Supersample int
	// Width, Height are the image dimensions.
	Width, Height int

	NonZeroOn int // pixels with coverage >= threshold
	EvenOddOn int
	// NonZeroAny, EvenOddAny count pixels with any coverage at all.
	NonZeroAny, EvenOddAny int

	// DisagreeSamples counts supersamples where the two rules disagree.
	DisagreeSamples int
	// DisagreePixels counts pixels where the coverage ratio differs.
	DisagreePixels int
	// CoverageNonZero and CoverageEvenOdd sum the 8-bit coverage over all
	// pixels. Their difference divided by 255 is the covered-pixel equivalent
	// of the disagreement, and is always non-negative because even-odd's
	// coverage is pointwise <= non-zero's.
	CoverageNonZero, CoverageEvenOdd int
	// PatternHistogram counts disagreement samples by their crossing-direction
	// pattern, e.g. "+1 +1" or "-1 +1 +1 +1". This is the evidence for the
	// necessity/sufficiency condition: every key has an even number of
	// crossings, an even sum, and a non-zero sum.
	PatternHistogram map[string]int
	// WindingHistogram counts disagreement samples by winding number W.
	WindingHistogram map[int]int

	// Witnesses are representative disagreement samples, one per distinct
	// pattern, chosen deterministically.
	Witnesses []Witness
}

// DeltaOn is the covered-pixel difference (non-zero minus even-odd).
func (s RuleStats) DeltaOn() int { return s.NonZeroOn - s.EvenOddOn }

// DeltaAny is the any-coverage pixel difference (non-zero minus even-odd).
func (s RuleStats) DeltaAny() int { return s.NonZeroAny - s.EvenOddAny }

// CoverageDelta is the fractional-coverage difference in pixel equivalents,
// (sumNonZero - sumEvenOdd)/255. It is the exact area-equivalent disagreement
// on the supersampled grid, unlike the thresholded pixel counts.
func (s RuleStats) CoverageDelta() float64 {
	return float64(s.CoverageNonZero-s.CoverageEvenOdd) / 255
}

// Witness is one sample on which the two rules disagree, with the crossings
// that cause it.
type Witness struct {
	Col, Row int
	SI, SJ   int
	P        geom.Point
	// Crossings are the +x ray crossings from P, in polygon segment order
	// (ascending segment index), the order geom.Summarize returns them.
	Crossings []geom.Crossing
	// W is the winding number sum_k d_k.
	W int
	// Unpaired is the subset of Crossings that even-odd cancels in pairs and
	// non-zero keeps. These are the crossings that cause the disagreement.
	Unpaired []geom.Crossing
	// Directions is the crossing direction sequence.
	Directions []int
	// Pattern is the space-separated list of crossing directions.
	Pattern string
}

// Threshold is the coverage value at or above which a pixel is "covered" for
// the headline counts.
const Threshold uint8 = 128

// Measure renders shape under both rules on a width x height image with the
// given supersample count, then cross-checks the per-pixel counts against an
// independent per-supersample pass that uses the same frame.
func Measure(shape Shape, width, height, supersample int) (RuleStats, error) {
	rs := RuleStats{Shape: shape, Supersample: supersample, Width: width, Height: height}
	rs.Frame = raster.Bounds([][]geom.Point{shape.Poly})
	rs.Frame.W, rs.Frame.H = width, height
	rs.PatternHistogram = map[string]int{}
	rs.WindingHistogram = map[int]int{}

	imgs := map[geom.FillRule]*raster.Image{}
	for _, rule := range geom.Rules {
		im, err := raster.New(width, height)
		if err != nil {
			return rs, err
		}
		if err := im.FillRule([][]geom.Point{shape.Poly}, supersample, rule); err != nil {
			return rs, err
		}
		imgs[rule] = im
	}

	nz, eo := imgs[geom.NonZero], imgs[geom.EvenOdd]
	rs.NonZeroOn, rs.EvenOddOn = nz.CountOn(Threshold), eo.CountOn(Threshold)
	rs.NonZeroAny, rs.EvenOddAny = nz.CountCovered(), eo.CountCovered()
	rs.CoverageNonZero, rs.CoverageEvenOdd = sumPix(nz), sumPix(eo)

	// Independent per-sample audit. This both counts disagreements and checks
	// that the pixel-level coverage the renderer wrote matches the predicate.
	den := supersample * supersample
	witnessByPattern := map[string]Witness{}
	for row := 0; row < height; row++ {
		for col := 0; col < width; col++ {
			nzHits, eoHits := 0, 0
			for sj := 0; sj < supersample; sj++ {
				for si := 0; si < supersample; si++ {
					p := rs.Frame.Sample(col, row, si, sj, supersample)
					sum := geom.Summarize(shape.Poly, p.X, p.Y)
					if sum.NonZero() {
						nzHits++
					}
					if sum.EvenOdd() {
						eoHits++
					}
					if sum.Agree() {
						continue
					}
					rs.DisagreeSamples++
					pat := patternOf(sum.Directions())
					rs.PatternHistogram[pat]++
					rs.WindingHistogram[sum.Winding]++
					if _, ok := witnessByPattern[pat]; !ok {
						witnessByPattern[pat] = Witness{
							Col:        col,
							Row:        row,
							SI:         si,
							SJ:         sj,
							P:          p,
							Crossings:  sum.Crossings,
							W:          sum.Winding,
							Unpaired:   geom.Unpaired(sum.Crossings),
							Directions: sum.Directions(),
							Pattern:    pat,
						}
					}
				}
			}
			if got := int(uint8(255 * nzHits / den)); got != int(nz.Pix[row*width+col]) {
				return rs, fmt.Errorf("%s: non-zero coverage mismatch at (%d,%d): render %d audit %d",
					shape.Name, col, row, nz.Pix[row*width+col], got)
			}
			if got := int(uint8(255 * eoHits / den)); got != int(eo.Pix[row*width+col]) {
				return rs, fmt.Errorf("%s: even-odd coverage mismatch at (%d,%d): render %d audit %d",
					shape.Name, col, row, eo.Pix[row*width+col], got)
			}
			if nz.Pix[row*width+col] != eo.Pix[row*width+col] {
				rs.DisagreePixels++
			}
		}
	}

	for _, pat := range sortedKeys(rs.PatternHistogram) {
		rs.Witnesses = append(rs.Witnesses, witnessByPattern[pat])
	}
	return rs, nil
}

func patternOf(dirs []int) string {
	parts := make([]string, len(dirs))
	for i, d := range dirs {
		parts[i] = fmt.Sprintf("%+d", d)
	}
	return strings.Join(parts, " ")
}

func sortedKeys(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ShapeLibrary is the fixed set of self-intersecting shapes used by the study
// and its tests. Each entry exercises a different disagreement pattern:
//
//   - Pentagram (5/2): two upward crossings, winding 2, one pattern.
//   - Double-traced triangle: same edge visited twice, winding 2 everywhere,
//     so even-odd discards the whole shape.
//   - Star 9/4: up to six crossings with mixed directions, winding 2, 4 and 6.
func ShapeLibrary() []Shape {
	return []Shape{
		Pentagram(),
		DoubleTracedTriangle(),
		Star94(),
	}
}

// Pentagram is the 5/2 star polygon. Every ray that lands in its central
// pentagon meets two upward crossings, so the rules disagree there: even-odd
// leaves the pentagon empty, non-zero fills it.
func Pentagram() Shape {
	return Shape{
		Name: "pentagram",
		Why:  "5/2 star: the central pentagon has winding 2, so even-odd drops it",
		Poly: regularStar(5, 2, 100),
	}
}

// DoubleTracedTriangle is one triangle traversed twice in the same direction.
// Every interior ray meets two upward crossings with winding 2; even-odd makes
// the whole shape empty. The two crossings of a ray sit at the same geometric
// point because the edge is visited twice.
func DoubleTracedTriangle() Shape {
	tri := []geom.Point{{X: 0, Y: -90}, {X: 78, Y: 45}, {X: -78, Y: 45}}
	return Shape{
		Name: "double-traced-triangle",
		Why:  "a triangle traced twice the same way: winding 2 everywhere, even-odd empties it",
		Poly: append(append([]geom.Point{}, tri...), tri...),
	}
}

// Star94 is the 9/4 star polygon. Its deep pockets are covered four or six
// times and some rays meet a mix of upward and downward crossings, so the
// direction histogram is richer than the single-pattern star cases while the
// winding number stays even and non-zero.
func Star94() Shape {
	return Shape{
		Name: "star-9/4",
		Why:  "9/4 star: pockets covered 2, 4 or 6 times, rays with mixed crossing directions",
		Poly: regularStar(9, 4, 100),
	}
}

func regularStar(n, step int, r float64) []geom.Point {
	verts := make([]geom.Point, n)
	for k := 0; k < n; k++ {
		a := (-90 + 360*float64(k)/float64(n)) * math.Pi / 180
		verts[k] = geom.Point{X: r * math.Cos(a), Y: r * math.Sin(a)}
	}
	out := make([]geom.Point, n)
	for k := 0; k < n; k++ {
		out[k] = verts[(k*step)%n]
	}
	return out
}

func sumPix(im *raster.Image) int {
	s := 0
	for _, v := range im.Pix {
		s += int(v)
	}
	return s
}
