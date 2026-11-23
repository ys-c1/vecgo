package pipe

import (
	"fmt"
	"math"

	"vecgo/cache"
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

// Run renders the scene without consulting any cache. Cache-related fields in
// cfg are ignored, so the output is the uncached reference for the same input.
func Run(scene [][]geom.Point, cfg config.Config) (*raster.Image, error) {
	return render(scene, cfg, nil)
}

// RunCached is Run with an optional shared cache. Caching is active only when
// cfg.CacheEnabled is true; in that case c must be non-nil and its capacity
// must equal cfg.CacheCapacity. When caching is disabled c is ignored and the
// output is byte-identical to any cached run of the same input.
func RunCached(scene [][]geom.Point, cfg config.Config, c *cache.Cache) (*raster.Image, error) {
	if cfg.CacheEnabled {
		if c == nil {
			return nil, &cache.EntryError{Op: "RunCached", Err: cache.ErrZeroCapacity,
				Info: cache.EntryInfo{Capacity: cfg.CacheCapacity}}
		}
		if c.Capacity() != cfg.CacheCapacity {
			return nil, fmt.Errorf("缓存容量 %d 与配置 %d 不一致", c.Capacity(), cfg.CacheCapacity)
		}
		return render(scene, cfg, c)
	}
	return render(scene, cfg, nil)
}

func render(scene [][]geom.Point, cfg config.Config, c *cache.Cache) (*raster.Image, error) {
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
	if c != nil {
		err = im.FillCached(moved, cfg.Supersample, c)
	} else {
		err = im.Fill(moved, cfg.Supersample)
	}
	if err != nil {
		return nil, err
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
