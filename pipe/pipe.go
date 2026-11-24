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
	if err := shading.ApplySpace(im, cfg.Shading, 1.0, 0.0, cfg.InterpSpace); err != nil {
		return nil, err
	}
	// linear/perceptual 空间已在 shading 末端完成 sRGB 编码，不能再叠 gamma 传递。
	// 默认 encoded 空间保持历史调用：即使 gamma=1 也照走原 LUT（其浮点截断行为被逐字节锁定）。
	if !cfg.TransferOwnsEncoding() {
		if err := post.Gamma(im, cfg.Gamma); err != nil {
			return nil, err
		}
	}
	out, err := post.Blur(im, cfg.Blur)
	if err != nil {
		return nil, err
	}
	return out, nil
}
