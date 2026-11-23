package pipe

import (
	"math"

	"vecgo/config"
	"vecgo/dedup"
	"vecgo/geom"
	"vecgo/post"
	"vecgo/raster"
	"vecgo/shading"
)

func Transform(pts []geom.Point, scale, rotate float64) []geom.Point {
	c, s := math.Cos(rotate), math.Sin(rotate)
	out := make([]geom.Point, len(pts))
	for i, p := range pts {
		out[i] = geom.Point{}
		out[i].X = scale * (p.X*c - p.Y*s)
		out[i].Y = scale * (p.X*s + p.Y*c)
	}
	return out
}

// Stats describes what a Run actually did.
type Stats struct {
	// Submitted is the number of figures in the scene as given.
	Submitted int
	// Unique is the number of figures left after deduplication.
	Unique int
	// Duplicates is Submitted - Unique.
	Duplicates int
	// Drawn is the number of figures the rasteriser was asked to draw.
	Drawn int
	// RequiredBits is the fingerprint length back-solved from the duplicate rate.
	RequiredBits int
	// FingerprintBits is the length actually used; it never exceeds RequiredBits.
	FingerprintBits int
}

// Run renders scene.  Repeated figures are merged before rasterisation so that
// each distinct figure is drawn exactly once; the resulting image is
// byte-identical to drawing every submission.  The function signature is
// unchanged.  Use RunWithStats to inspect the deduplication counts.
func Run(scene [][]geom.Point, cfg config.Config) (*raster.Image, error) {
	im, _, err := RunWithStats(scene, cfg)
	return im, err
}

// RunWithStats is Run that also reports the deduplication and drawing counts.
func RunWithStats(scene [][]geom.Point, cfg config.Config) (*raster.Image, Stats, error) {
	return runWith(scene, cfg, func(scene [][]geom.Point) (*dedup.Result, error) {
		return dedup.MergeAuto(scene)
	})
}

// RenderDeduped renders scene with an explicit fingerprint length.  It is
// mainly useful to exercise (and report) collisions: a deliberately short
// length makes them deterministic.  A collision is reported as a
// *dedup.FigureError naming both figure numbers.
func RenderDeduped(scene [][]geom.Point, cfg config.Config, bits int) (*raster.Image, error) {
	im, _, err := runWith(scene, cfg, func(scene [][]geom.Point) (*dedup.Result, error) {
		return dedup.Merge(scene, bits)
	})
	return im, err
}

func runWith(scene [][]geom.Point, cfg config.Config, merge func([][]geom.Point) (*dedup.Result, error)) (*raster.Image, Stats, error) {
	var stats Stats
	if err := cfg.Validate(); err != nil {
		return nil, stats, err
	}
	res, err := merge(scene)
	if err != nil {
		return nil, stats, err
	}
	stats = Stats{
		Submitted:       res.Stats.Total,
		Unique:          res.Stats.Unique,
		Duplicates:      res.Stats.Duplicates,
		RequiredBits:    res.Stats.RequiredBits,
		FingerprintBits: res.Stats.FingerprintBits,
	}

	im, err := raster.New(cfg.Width, cfg.Height)
	if err != nil {
		return nil, stats, err
	}
	moved := make([][]geom.Point, len(res.Unique))
	for i, poly := range res.Unique {
		moved[i] = Transform(poly, cfg.Scale, cfg.Rotate)
	}
	// The per-figure drawing entry point is unchanged; the counter reports how
	// many figures it was handed.
	if err := im.FillCounting(moved, cfg.Supersample, &stats.Drawn); err != nil {
		return nil, stats, err
	}
	if err := shading.Apply(im, cfg.Shading, 1.0, 0.0); err != nil {
		return nil, stats, err
	}
	if err := post.Gamma(im, cfg.Gamma); err != nil {
		return nil, stats, err
	}
	out, err := post.Blur(im, cfg.Blur)
	if err != nil {
		return nil, stats, err
	}
	return out, stats, nil
}

// RenderUndeduplicated draws every submitted figure once, without merging.
// It is the baseline RunWithStats is checked against: both must produce the
// same bytes.  It exists for verification; normal rendering should use Run.
func RenderUndeduplicated(scene [][]geom.Point, cfg config.Config) (*raster.Image, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	im, err := raster.New(cfg.Width, cfg.Height)
	if err != nil {
		return nil, err
	}
	moved := make([][]geom.Point, len(scene))
	for i, poly := range scene {
		moved[i] = Transform(poly, cfg.Scale, cfg.Rotate)
	}
	if err := im.Fill(moved, cfg.Supersample); err != nil {
		return nil, err
	}
	if err := shading.Apply(im, cfg.Shading, 1.0, 0.0); err != nil {
		return nil, err
	}
	if err := post.Gamma(im, cfg.Gamma); err != nil {
		return nil, err
	}
	return post.Blur(im, cfg.Blur)
}
