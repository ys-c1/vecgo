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
	im, _, err := run(scene, cfg, false)
	return im, err
}

// RunWithStats 与 Run 渲染出逐字节相同的图像，并返回填充阶段的覆盖率
// 统计（哪些像素被多少个图形覆盖、分桶与面积核对）。统计只观察填充，
// 不改变最终的像素值。
func RunWithStats(scene [][]geom.Point, cfg config.Config) (*raster.Image, *raster.CoverageStats, error) {
	return run(scene, cfg, true)
}

func run(scene [][]geom.Point, cfg config.Config, withStats bool) (*raster.Image, *raster.CoverageStats, error) {
	if err := cfg.Validate(); err != nil {
		return nil, nil, err
	}
	im, err := raster.New(cfg.Width, cfg.Height)
	if err != nil {
		return nil, nil, err
	}
	moved := make([][]geom.Point, len(scene))
	for i, poly := range scene {
		moved[i] = Transform(poly, cfg.Scale, cfg.Rotate)
	}
	var stats *raster.CoverageStats
	if withStats {
		stats, err = im.FillStats(moved, cfg.Supersample)
	} else {
		err = im.Fill(moved, cfg.Supersample)
	}
	if err != nil {
		return nil, nil, err
	}
	if err := shading.Apply(im, cfg.Shading, 1.0, 0.0); err != nil {
		return nil, nil, err
	}
	if err := post.Gamma(im, cfg.Gamma); err != nil {
		return nil, nil, err
	}
	out, err := post.Blur(im, cfg.Blur)
	if err != nil {
		return nil, nil, err
	}
	return out, stats, nil
}
