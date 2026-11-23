package pipe

import (
	"math"

	"vecgo/config"
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

func Run(scene [][]geom.Point, cfg config.Config) (*raster.Image, error) {
	return RunWithOptions(scene, cfg, Options{})
}

// Options selects the coverage-cache behaviour of RunWithOptions. The zero
// value is the original direct path, byte for byte.
type Options struct {
	// CoverageCache enables the per-triangle coverage cache. The scene must
	// then consist of triangles only.
	CoverageCache *raster.Cache
	// AllowCacheOnError keeps the direct path when the scene contains a
	// non-triangle: instead of failing, cache.Err records why the cache was
	// not used. Off by default so a silently uncached run cannot masquerade
	// as a cached one.
	AllowCacheOnError bool
}

// ErrCacheUnused reports that a coverage cache was requested but the scene
// could not be cached; the image was produced by the direct path.
type ErrCacheUnused struct{ Reason error }

func (e ErrCacheUnused) Error() string {
	return "覆盖率缓存未启用: " + e.Reason.Error()
}

// RunWithOptions is Run with the coverage cache wired in. Without a cache it
// is exactly Run. With a cache the result must be byte-identical: the direct
// path is used for the cache-miss computation and masks are OR-combined, so
// only the number of times geom.Inside runs differs.
func RunWithOptions(scene [][]geom.Point, cfg config.Config, opt Options) (*raster.Image, error) {
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
	if opt.CoverageCache != nil {
		opt.CoverageCache.Err = nil
	}
	fillErr := im.FillCached(moved, cfg.Supersample, opt.CoverageCache)
	if fillErr != nil && opt.CoverageCache != nil && opt.AllowCacheOnError {
		if err := im.Fill(moved, cfg.Supersample); err != nil {
			return nil, err
		}
		opt.CoverageCache.Err = ErrCacheUnused{Reason: fillErr}
	} else if fillErr != nil {
		return nil, fillErr
	}
	if err := shading.Apply(im, cfg.Shading, 1.0, 0.0); err != nil {
		return nil, err
	}
	if err := post.Gamma(im, cfg.Gamma); err != nil {
		return nil, err
	}
	out, err := post.Blur(im, cfg.Blur)
	if err != nil {
		return nil, err
	}
	return out, nil
}
