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

// Run 渲染场景，不收集任何统计。
func Run(scene [][]geom.Point, cfg config.Config) (*raster.Image, error) {
	return run(scene, cfg, nil)
}

func run(scene [][]geom.Point, cfg config.Config, st *Stats) (*raster.Image, error) {
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
	var fill *raster.FillStats
	if st != nil && st.Enabled {
		st.Fill.Enabled = true
		st.Fill.KeepPerPixel = st.KeepPerPixel
		st.Fill.Reset()
		if err := st.theorize(moved, cfg); err != nil {
			return nil, err
		}
		fill = &st.Fill
	}
	if err := im.FillWithStats(moved, cfg.Supersample, fill); err != nil {
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
