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
	if cfg.Shading == "gradient" && cfg.Rotate != 0 {
		// 旋转后的渐变必须在多边形局部坐标系中采样，
		// 否则等强度线仍平行于画布行，斜看就与多边形错位。
		minx, miny, maxx, maxy := raster.BBox(moved)
		frame := shading.NewLocalFrame(cfg.Scale, cfg.Rotate, minx, miny, maxx, maxy, cfg.Width, cfg.Height)
		if err := shading.ApplyGradient(im, frame, 1.0, 0.0); err != nil {
			return nil, err
		}
	} else if err := shading.Apply(im, cfg.Shading, 1.0, 0.0); err != nil {
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
