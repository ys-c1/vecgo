package main

import (
	"vecgo/freqsel"
	"vecgo/geom"
	"vecgo/raster"
)

// quad 返回轴对齐矩形多边形（顺时针四点）。
func quad(x0, y0, x1, y1 float64) []geom.Point {
	return []geom.Point{
		{X: x0, Y: y0}, {X: x1, Y: y0}, {X: x1, Y: y1}, {X: x0, Y: y1},
	}
}

// renderVecgoScene 用仓库自带的 raster.Fill 渲染一张含条形码竖条、细密横线、
// 小方格阵与三角斜边的高密度矢量场景，作为频率自适应选层的真实渲染输入。
// Fill 会把全部顶点的包围盒映射到 n×n 画幅；场景坐标取近正方形 [0,100]²。
func renderVecgoScene(n int) (*freqsel.Gray, error) {
	var scene [][]geom.Point

	// 左侧条形码竖条：间距约 3.9 坐标单位（映射后约 4 源像素/周期，高频）。
	for i := 0; i < 22; i++ {
		x := 2.0 + float64(i)*2.55
		w := 0.55
		if i%3 == 0 {
			w = 1.2
		}
		scene = append(scene, quad(x, 4, x+w, 46))
	}
	// 中部细密横线。
	for j := 0; j < 16; j++ {
		y := 52.0 + float64(j)*2.7
		h := 0.5
		if j%4 == 0 {
			h = 1.2
		}
		scene = append(scene, quad(4, y, 62, y+h))
	}
	// 右上小方格阵。
	for gy := 0; gy < 8; gy++ {
		for gx := 0; gx < 5; gx++ {
			x := 66.0 + float64(gx)*6.2
			y := 6.0 + float64(gy)*5.4
			scene = append(scene, quad(x, y, x+2.6, y+2.4))
		}
	}
	// 右下三角与一条粗对角带，提供干净的斜边/阶跃。
	scene = append(scene, []geom.Point{{X: 66, Y: 96}, {X: 98, Y: 96}, {X: 98, Y: 66}})
	scene = append(scene, []geom.Point{{X: 66, Y: 60}, {X: 70, Y: 60}, {X: 98, Y: 88}, {X: 98, Y: 94}})

	im, err := raster.New(n, n)
	if err != nil {
		return nil, err
	}
	if err := im.Fill(scene, 2); err != nil {
		return nil, err
	}
	return freqsel.FromUint8(n, n, im.Pix), nil
}
