package pipe

import (
	"math"

	"vecgo/config"
	"vecgo/geom"
	"vecgo/pipe/timing"
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
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	im, err := raster.New(cfg.Width, cfg.Height)
	if err != nil {
		return nil, err
	}
	moved := make([][]geom.Point, len(scene))
	timing.Start(timing.StageTransform)
	for i, poly := range scene {
		moved[i] = Transform(poly, cfg.Scale, cfg.Rotate)
	}
	timing.Done(timing.StageTransform)
	timing.Start(timing.StageRasterize)
	if err := im.Fill(moved, cfg.Supersample); err != nil {
		timing.Done(timing.StageRasterize)
		return nil, err
	}
	timing.Done(timing.StageRasterize)
	timing.Start(timing.StageShading)
	if err := shading.Apply(im, cfg.Shading, 1.0, 0.0); err != nil {
		timing.Done(timing.StageShading)
		return nil, err
	}
	timing.Done(timing.StageShading)
	timing.Start(timing.StageGamma)
	if err := post.Gamma(im, cfg.Gamma); err != nil {
		timing.Done(timing.StageGamma)
		return nil, err
	}
	timing.Done(timing.StageGamma)
	timing.Start(timing.StageBlur)
	out, err := post.Blur(im, cfg.Blur)
	timing.Done(timing.StageBlur)
	if err != nil {
		return nil, err
	}
	return out, nil
}
