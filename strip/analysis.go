package strip

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"

	"vecgo/geom"
	"vecgo/raster"
)

// Report contains the counting and pixel-equality evidence for one mesh.
type Report struct {
	Name                   string  `json:"name"`
	Triangles              int     `json:"triangles"`
	Components             int     `json:"components"`
	IndependentVertices    int     `json:"independentVertices"`
	LowerBound             int     `json:"lowerBound"`
	GreedyStripCount       int     `json:"greedyStripCount"`
	GreedyPaths            [][]int `json:"greedyPaths"`
	SlotVertices           int     `json:"slotVertices"`
	StreamLengths          []int   `json:"streamLengths"`
	StripVertices          int     `json:"stripVertices"`
	LowerBoundToStripRatio float64 `json:"lowerBoundToStripRatio"`
	StripToLowerBoundRatio float64 `json:"stripToLowerBoundRatio"`
	PixelCount             int     `json:"pixelCount"`
	DifferentPixels        int     `json:"differentPixels"`
	IndependentSHA256      string  `json:"independentSha256"`
	StripSHA256            string  `json:"stripSha256"`
	Streams                [][]int `json:"streams,omitempty"`
}

// TrianglePolygons flattens the independent triangle list.
func TrianglePolygons(m *Mesh) [][]geom.Point {
	out := make([][]geom.Point, 0, len(m.Triangles))
	for _, tri := range m.Triangles {
		out = append(out, []geom.Point{
			m.Vertices[tri[0]], m.Vertices[tri[1]], m.Vertices[tri[2]],
		})
	}
	return out
}

// StreamPolygons expands all sliding windows of all encoded strip streams.
func StreamPolygons(m *Mesh, streams [][]int) [][]geom.Point {
	var out [][]geom.Point
	for _, s := range streams {
		for _, tri := range StreamTriangles(s) {
			out = append(out, []geom.Point{
				m.Vertices[tri[0]], m.Vertices[tri[1]], m.Vertices[tri[2]],
			})
		}
	}
	return out
}

// RenderCoverage uses the repository's ordinary polygon sampler. Rendering
// both inputs through this same function makes the pixel comparison direct.
func RenderCoverage(polys [][]geom.Point, width, height, supersample int) (*raster.Image, error) {
	im, err := raster.New(width, height)
	if err != nil {
		return nil, err
	}
	if err := im.Fill(polys, supersample); err != nil {
		return nil, err
	}
	return im, nil
}

// Frame is an explicit coordinate window for reproducible renders.
type Frame struct {
	MinX, MinY, MaxX, MaxY float64
}

// RenderFrame samples polygons in a fixed coordinate frame using the same
// center-of-subpixel rule as raster.Image.Fill.
func RenderFrame(polys [][]geom.Point, width, height, supersample int, f Frame) (*raster.Image, error) {
	if supersample < 1 {
		return nil, errors.New("supersample 必须为正")
	}
	im, err := raster.New(width, height)
	if err != nil {
		return nil, err
	}
	spanX, spanY := f.MaxX-f.MinX, f.MaxY-f.MinY
	den := supersample * supersample
	for row := 0; row < im.H; row++ {
		for col := 0; col < im.W; col++ {
			hit := 0
			for sj := 0; sj < supersample; sj++ {
				for si := 0; si < supersample; si++ {
					px := f.MinX + (float64(col)+(float64(si)+0.5)/float64(supersample))*spanX/float64(im.W)
					py := f.MinY + (float64(row)+(float64(sj)+0.5)/float64(supersample))*spanY/float64(im.H)
					for _, p := range polys {
						if geom.Inside(p, px, py) {
							hit++
							break
						}
					}
				}
			}
			im.Pix[row*im.W+col] = uint8(255 * hit / den)
		}
	}
	return im, nil
}

// CountDifferentPixels returns the number of unequal grayscale pixels.
func CountDifferentPixels(a, b *raster.Image) int {
	n := 0
	for i := range a.Pix {
		if a.Pix[i] != b.Pix[i] {
			n++
		}
	}
	return n
}

// Analyze runs greedy ordering, encodes streams, and compares rendered pixels.
func Analyze(m *Mesh, width, height, supersample int) (Report, error) {
	adj := Adjacency(m)
	c := Components(adj)
	paths := GreedyPaths(m)
	streams := EncodePaths(m, paths)

	streamLengths := make([]int, 0, len(streams))
	stripVertices := 0
	for _, s := range streams {
		streamLengths = append(streamLengths, len(s))
		stripVertices += len(s)
	}
	t := len(m.Triangles)
	lb := LowerBound(t, c)

	independent, err := RenderCoverage(TrianglePolygons(m), width, height, supersample)
	if err != nil {
		return Report{}, err
	}
	stripped, err := RenderCoverage(StreamPolygons(m, streams), width, height, supersample)
	if err != nil {
		return Report{}, err
	}
	diff := CountDifferentPixels(independent, stripped)
	sumA := sha256.Sum256(independent.Pix)
	sumB := sha256.Sum256(stripped.Pix)

	return Report{
		Name:                   m.Name,
		Triangles:              t,
		Components:             c,
		IndependentVertices:    3 * t,
		LowerBound:             lb,
		GreedyStripCount:       len(paths),
		GreedyPaths:            paths,
		SlotVertices:           SlotVertices(t, len(paths)),
		StreamLengths:          streamLengths,
		StripVertices:          stripVertices,
		LowerBoundToStripRatio: float64(lb) / float64(stripVertices),
		StripToLowerBoundRatio: float64(stripVertices) / float64(lb),
		PixelCount:             width * height,
		DifferentPixels:        diff,
		IndependentSHA256:      hex.EncodeToString(sumA[:]),
		StripSHA256:            hex.EncodeToString(sumB[:]),
		Streams:                streams,
	}, nil
}

// Ratio returns lowerBound / vertices as a float.
func Ratio(lowerBound, vertices int) float64 {
	return float64(lowerBound) / float64(vertices)
}
