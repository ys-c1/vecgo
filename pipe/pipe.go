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
	return run(scene, cfg, nil)
}

// RunDiag 与 Run 相同，但把每次失败和每个阶段耗时记入 d。
// d 为 nil 时行为与 Run 完全一致。
func RunDiag(scene [][]geom.Point, cfg config.Config, d *Diag) (*raster.Image, error) {
	return run(scene, cfg, d)
}

func run(scene [][]geom.Point, cfg config.Config, d *Diag) (*raster.Image, error) {
	if err := d.Time("validate", cfg.Validate); err != nil {
		return nil, d.Fail("config", err)
	}
	var im *raster.Image
	err := d.Time("new", func() error {
		var err error
		im, err = raster.New(cfg.Width, cfg.Height)
		return err
	})
	if err != nil {
		return nil, d.Fail("raster", err)
	}
	moved := make([][]geom.Point, len(scene))
	err = d.Time("transform", func() error {
		for i, poly := range scene {
			moved[i] = Transform(poly, cfg.Scale, cfg.Rotate)
		}
		return nil
	})
	if err != nil {
		return nil, d.Fail("pipe", err)
	}
	if err := d.Time("fill", func() error { return im.Fill(moved, cfg.Supersample) }); err != nil {
		return nil, d.Fail("raster", err)
	}
	if err := d.Time("shading", func() error { return shading.Apply(im, cfg.Shading, 1.0, 0.0) }); err != nil {
		return nil, d.Fail("shading", err)
	}
	if err := d.Time("gamma", func() error { return post.Gamma(im, cfg.Gamma) }); err != nil {
		return nil, d.Fail("post", err)
	}
	var out *raster.Image
	err = d.Time("blur", func() error {
		var err error
		out, err = post.Blur(im, cfg.Blur)
		return err
	})
	if err != nil {
		return nil, d.Fail("post", err)
	}
	return out, nil
}
