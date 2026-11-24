package pipe

import (
	"math"

	"vecgo/config"
	"vecgo/geom"
	"vecgo/post"
	"vecgo/raster"
	"vecgo/shading"
)

// Skipped 是 raster.Skipped 的重导出，调用方无需再引入 raster 包即可
// 读取跳过清单。
type Skipped = raster.Skipped

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

// Run 为传统整帧事务路径：任一图形非法即整体失败（返回 err，无图产出）。
// 行为与加入失败隔离之前完全一致。
func Run(scene [][]geom.Point, cfg config.Config) (*raster.Image, error) {
	im, _, err := run(scene, cfg, false)
	return im, err
}

// RunIsolated 开启失败隔离：单个图形出错只跳过它，其余图形照常绘制，
// 最终通过 SkippedList 汇总被跳过的图形编号（按编号升序）。
// 即使部分图形被跳过也返回有效图像与 nil 错误；仅当配置本身非法或
// 后处理失败时才返回错误。
func RunIsolated(scene [][]geom.Point, cfg config.Config) (*raster.Image, raster.SkippedList, error) {
	return run(scene, cfg, true)
}

func run(scene [][]geom.Point, cfg config.Config, isolate bool) (*raster.Image, raster.SkippedList, error) {
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

	var skipped raster.SkippedList
	if isolate {
		// 隔离路径：坏图形只记录编号并跳过，图像照常产出。
		skipped, err = im.FillIsolated(moved, cfg.Supersample)
	} else {
		// 传统路径：任一坏图形都会令整帧失败（或被 NaN/Inf 污染成全黑帧）。
		err = im.Fill(moved, cfg.Supersample)
	}
	if err != nil {
		return nil, skipped, err
	}
	if err := shading.Apply(im, cfg.Shading, 1.0, 0.0); err != nil {
		return nil, skipped, err
	}
	if err := post.Gamma(im, cfg.Gamma); err != nil {
		return nil, skipped, err
	}
	out, err := post.Blur(im, cfg.Blur)
	if err != nil {
		return nil, skipped, err
	}
	return out, skipped, nil
}
