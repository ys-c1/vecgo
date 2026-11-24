package pipe

import (
	"math"

	"vecgo/config"
	"vecgo/geom"
	"vecgo/post"
	"vecgo/raster"
	"vecgo/shading"
)

// Result 是一次渲染的全部产物与面积核对数据。
type Result struct {
	// Weighted 是按覆盖率加权的灰度图（fill=coverage），或原采样填充结果（fill=supersample）。
	Weighted *raster.Image
	// Binary 是二值化结果，仅 fill=coverage 时生成，否则为 nil。
	Binary *raster.Image
	// Cov 保留逐像素覆盖率，用于面积核对；仅 fill=coverage 时非 nil。
	Cov *raster.Coverage

	ExactArea    float64 // 格林公式面积（世界坐标 × scale²）
	WeightedArea float64 // Σ 覆盖率 × 像素面积
	RelErr       float64 // |加权 − 精确| / 精确

	BinArea  float64 // 二值面积
	BinDiff  float64 // |二值面积 − 加权面积|
	BinL1    float64 // 逐像素差 × 像素面积
	BinBound float64 // 差异上界
	Partial  int     // 部分覆盖像素数

	SupersampleArea float64 // fill=supersample 时按像素值折算的面积，便于与精确面积对照
}

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

// Run 执行渲染。fill=coverage 时逐像素精确求交得到覆盖率，
// 加权面积与格林公式面积在浮点误差内严格相等；该模式下不做着色/伽马/模糊，
// 以免破坏覆盖率数值。fill=supersample 保留原来的采样填充与后期处理。
func Run(scene [][]geom.Point, cfg config.Config) (*Result, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	moved := make([][]geom.Point, len(scene))
	for i, poly := range scene {
		moved[i] = Transform(poly, cfg.Scale, cfg.Rotate)
	}
	if cfg.FillMode == "coverage" {
		return runCoverage(moved, cfg)
	}
	return runSupersample(moved, cfg)
}

func runCoverage(moved [][]geom.Point, cfg config.Config) (*Result, error) {
	cov, err := raster.Rasterize(moved, cfg.Width, cfg.Height)
	if err != nil {
		return nil, err
	}
	res := &Result{
		Weighted:     cov.Image(),
		Binary:       cov.BinaryImage(cfg.FillThreshold),
		Cov:          cov,
		WeightedArea: cov.WeightedArea(),
	}
	for _, p := range moved {
		res.ExactArea += geom.Area(p)
	}
	if res.ExactArea > 0 {
		res.RelErr = math.Abs(res.WeightedArea-res.ExactArea) / res.ExactArea
	}
	res.BinArea, res.BinL1, res.BinBound, res.BinDiff, res.Partial =
		cov.BinaryStats(cfg.FillThreshold)
	return res, nil
}

func runSupersample(moved [][]geom.Point, cfg config.Config) (*Result, error) {
	im, err := raster.New(cfg.Width, cfg.Height)
	if err != nil {
		return nil, err
	}
	if err := im.Fill(moved, cfg.Supersample); err != nil {
		return nil, err
	}
	// 采样填充把包围盒映射到画幅，因此像素面积 = 包围盒面积 / 像素数。
	// 面积按像素亮度折算；后期处理会改变亮度，故在着色之前算好。
	minx, miny, maxx, maxy := bounds(moved)
	sx, sy := maxx-minx, maxy-miny
	pixelArea := sx * sy / float64(cfg.Width*cfg.Height)
	sum := 0.0
	for _, v := range im.Pix {
		sum += float64(v) / 255
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
	res := &Result{
		Weighted:        out,
		ExactArea:       sceneArea(moved),
		SupersampleArea: sum * pixelArea,
	}
	return res, nil
}

func bounds(polys [][]geom.Point) (minx, miny, maxx, maxy float64) {
	minx, miny = polys[0][0].X, polys[0][0].Y
	maxx, maxy = minx, miny
	for _, p := range polys {
		for _, q := range p {
			minx = math.Min(minx, q.X)
			maxx = math.Max(maxx, q.X)
			miny = math.Min(miny, q.Y)
			maxy = math.Max(maxy, q.Y)
		}
	}
	return
}

// sceneArea 为各多边形格林公式面积之和。多边形两两不重叠时等于并集面积；
// 覆盖率填充按并集计算，重叠时两者不可比（RelErr 会体现差额）。
func sceneArea(polys [][]geom.Point) float64 {
	total := 0.0
	for _, p := range polys {
		total += geom.Area(p)
	}
	return total
}
