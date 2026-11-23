package config

import "fmt"

type Config struct {
	Width, Height int
	Scale         float64
	Rotate        float64
	Supersample   int
	Shading       string
	Gamma         float64
	Blur          int
	Output        string

	// CacheEnabled turns on the per-polygon mask cache. When false the
	// renderer takes the uncached path and CacheCapacity is ignored.
	CacheEnabled bool
	// CacheCapacity is the LRU entry bound. It must be positive when the
	// cache is enabled.
	CacheCapacity int
	// CacheMaxBytesPerTriangle bounds the growth slope of peak cache bytes
	// as a function of triangle count. Zero selects the default.
	CacheMaxBytesPerTriangle float64
}

// DefaultCacheCapacity is used by Default.
const DefaultCacheCapacity = 1024

// DefaultMaxBytesPerTriangle is the default slope limit for the memory
// scaling check: peak cache footprint may grow by at most this many bytes per
// additional triangle.
const DefaultMaxBytesPerTriangle = 4096

func Default() Config {
	return Config{Width: 64, Height: 64, Scale: 1, Rotate: 0, Supersample: 2, Shading: "flat", Gamma: 1, Blur: 0, Output: "out.pgm",
		CacheEnabled: false, CacheCapacity: DefaultCacheCapacity, CacheMaxBytesPerTriangle: DefaultMaxBytesPerTriangle}
}

func (c Config) Validate() error {
	if c.Width <= 0 || c.Width > 4096 {
		return fmt.Errorf("width 超出范围: %d", c.Width)
	}
	if c.Height <= 0 || c.Height > 4096 {
		return fmt.Errorf("height 超出范围: %d", c.Height)
	}
	if c.Scale <= 0 || c.Scale > 100 {
		return fmt.Errorf("scale 超出范围: %v", c.Scale)
	}
	if c.Supersample < 1 || c.Supersample > 8 {
		return fmt.Errorf("supersample 超出范围: %d", c.Supersample)
	}
	if c.Gamma < 0.1 || c.Gamma > 10 {
		return fmt.Errorf("gamma 超出范围: %v", c.Gamma)
	}
	if c.Blur < 0 || c.Blur > 16 {
		return fmt.Errorf("blur 超出范围: %d", c.Blur)
	}
	if c.Shading != "flat" && c.Shading != "gradient" {
		return fmt.Errorf("shading 非法: %s", c.Shading)
	}
	if c.Output == "" {
		return fmt.Errorf("output 不能为空")
	}
	if c.CacheEnabled {
		if c.CacheCapacity <= 0 {
			return fmt.Errorf("cache 容量必须为正: %d", c.CacheCapacity)
		}
		if c.CacheMaxBytesPerTriangle < 0 {
			return fmt.Errorf("cache 斜率上限不能为负: %v", c.CacheMaxBytesPerTriangle)
		}
	}
	return nil
}

// CacheSlopeLimit returns the effective slope limit for the memory scaling
// check.
func (c Config) CacheSlopeLimit() float64 {
	if c.CacheMaxBytesPerTriangle > 0 {
		return c.CacheMaxBytesPerTriangle
	}
	return DefaultMaxBytesPerTriangle
}
