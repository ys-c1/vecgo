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
		out[i].X = scale*(p.X*c-p.Y*s)
		out[i].Y = scale*(p.X*s+p.Y*c)
	}
	return out
}

func Run(scene [][]geom.Point, cfg config.Config) (*raster.Image, error) {
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
	// 按旋转后的采样间距反解子采样数；θ=0 时反解值等于配置值，路径不变。
	effectiveN := EffectiveSupersample(cfg.Supersample, cfg.Rotate)
	if err := im.Fill(moved, effectiveN); err != nil {
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
