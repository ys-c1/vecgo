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
	// 默认（"" 或 "linear"）走历史路径，调用序列与运算顺序完全不变，
	// 因而默认产物与改动前逐字节相同（由 TestDefaultArtifactsByteIdentical 锁定）。
	if cfg.LevelDist == "" || cfg.LevelDist == "linear" {
		if err := shading.Apply(im, cfg.Shading, 1.0, 0.0); err != nil {
			return nil, err
		}
	} else {
		if err := shading.ApplyDist(im, cfg.Shading, 1.0, 0.0, cfg.LevelDist, cfg.LevelAlpha); err != nil {
			return nil, err
		}
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
