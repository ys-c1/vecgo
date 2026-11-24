package compose

import "vecgo/geom"

// DemoWidth and DemoHeight are the small logical grid on which per-pixel
// errors are reported.  Output PNGs are nearest-neighbor enlarged.
const (
	DemoWidth   = 8
	DemoHeight  = 8
	DemoSamples = 4
	DemoScale   = 32
)

// DemoBackground is opaque warm white.
var DemoBackground = [3]uint8{248, 246, 240}

type demoLayer struct {
	id    int
	depth int
	rgb   [3]uint8
	poly  []geom.Point
}

func rect(x0, y0, x1, y1 float64) []geom.Point {
	return []geom.Point{
		{X: x0, Y: y0},
		{X: x1, Y: y0},
		{X: x1, Y: y1},
		{X: x0, Y: y1},
	}
}

func demoLayers() []demoLayer {
	return []demoLayer{
		{
			id: 0, depth: 0, rgb: [3]uint8{230, 40, 40},
			poly: rect(0.5, 0.5, 5.5, 5.5),
		},
		{
			id: 1, depth: 1, rgb: [3]uint8{40, 200, 70},
			poly: rect(2.5, 1.5, 7.5, 6.5),
		},
		{
			id: 2, depth: 2, rgb: [3]uint8{50, 90, 230},
			poly: rect(1.5, 3.0, 6.5, 7.5),
		},
	}
}

// DemoPixelFragments returns W*H pixels, each containing the same three layer
// IDs.  Coverage is rasterized with DemoSamples squared point samples and
// then multiplied by the scene's fixed one-half opacity.
func DemoPixelFragments() [][]Fragment {
	layers := demoLayers()
	pixels := make([][]Fragment, DemoWidth*DemoHeight)

	for y := 0; y < DemoHeight; y++ {
		for x := 0; x < DemoWidth; x++ {
			fs := make([]Fragment, 0, len(layers))
			for _, layer := range layers {
				hits := 0
				for sy := 0; sy < DemoSamples; sy++ {
					py := float64(y) + (float64(sy)+0.5)/float64(DemoSamples)
					for sx := 0; sx < DemoSamples; sx++ {
						px := float64(x) + (float64(sx)+0.5)/float64(DemoSamples)
						if geom.Inside(layer.poly, px, py) {
							hits++
						}
					}
				}
				fs = append(fs, Fragment{
					ID:    layer.id,
					Depth: layer.depth,
					RGB:   layer.rgb,
					A:     AlphaForCoverage(hits, DemoSamples),
				})
			}
			pixels[y*DemoWidth+x] = fs
		}
	}
	return pixels
}

// Reorder applies one of the three named input permutations.  perm is a list
// of layer IDs in processing order.
func Reorder(frags []Fragment, perm []int) []Fragment {
	byID := make(map[int]Fragment, len(frags))
	for _, f := range frags {
		byID[f.ID] = f
	}
	out := make([]Fragment, 0, len(perm))
	for _, id := range perm {
		if f, ok := byID[id]; ok {
			out = append(out, f)
		}
	}
	return out
}

// DemoPermutations are the three deliberately different input orders.
var DemoPermutations = [3][]int{
	{0, 1, 2}, // canonical back-to-front
	{2, 1, 0}, // reversed
	{1, 0, 2}, // shuffled
}

// PermutationName returns a short label for a demo permutation.
func PermutationName(perm []int) string {
	names := map[int]byte{0: 'R', 1: 'G', 2: 'B'}
	b := make([]byte, 0, len(perm))
	for _, id := range perm {
		b = append(b, names[id])
	}
	return string(b)
}
