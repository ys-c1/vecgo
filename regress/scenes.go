// Package regress 提供渲染回归锚点：固定场景、基准图、逐像素差分与差异归因。
package regress

import (
	"fmt"

	"vecgo/config"
	"vecgo/geom"
	"vecgo/pipe"
	"vecgo/raster"
)

// Scene 是一组固定的回归场景：多边形 + 渲染配置。
type Scene struct {
	Name  string
	Polys [][]geom.Point
	Cfg   config.Config
}

func rect(x0, y0, x1, y1 float64) []geom.Point {
	return []geom.Point{{X: x0, Y: y0}, {X: x1, Y: y0}, {X: x1, Y: y1}, {X: x0, Y: y1}}
}

// Scenes 返回五组回归场景，覆盖：单图形、重叠、贴边、大画幅、退化。
func Scenes() []Scene {
	base := config.Default()

	// 1. 单图形：一个五边形（房子形）。不用矩形——矩形会填满整个包围盒，
	// 归一化后全图都是墨，锚不住"多了墨"这类回归。
	single := Scene{
		Name: "single",
		Polys: [][]geom.Point{{
			{X: 2, Y: 8}, {X: 8, Y: 8}, {X: 8, Y: 4}, {X: 5, Y: 1}, {X: 2, Y: 4},
		}},
		Cfg: base,
	}

	// 2. 重叠：横条、竖条、横条互相搭接；外框由上下两条横条决定，
	// 去掉中间竖条不会改变包围盒（注入"填充漏了"时靠它隔离变量）。
	overlap := Scene{
		Name: "overlap",
		Polys: [][]geom.Point{
			rect(0, 0, 14, 2),
			rect(4, 2, 10, 8),
			rect(0, 8, 14, 10),
		},
		Cfg: base,
	}

	// 3. 贴边：直角三角形的两条直角边压满包围盒的两条边。
	edge := Scene{
		Name:  "edge",
		Polys: [][]geom.Point{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 0, Y: 10}}},
		Cfg:   base,
	}

	// 4. 大画幅：512x384，贝塞尔压平的山形 + 两个矩形。
	largeCfg := base
	largeCfg.Width, largeCfg.Height = 512, 384
	hill, _ := geom.Flatten(
		geom.Point{X: 0, Y: 8}, geom.Point{X: 3, Y: 0},
		geom.Point{X: 7, Y: 0}, geom.Point{X: 10, Y: 8}, 16)
	hill = append(hill, geom.Point{X: 10, Y: 10}, geom.Point{X: 0, Y: 10})
	large := Scene{
		Name: "large",
		Polys: [][]geom.Point{
			hill,
			rect(1, 1, 4, 3),
			rect(6, 6, 9, 9),
		},
		Cfg: largeCfg,
	}

	// 5. 退化：正常矩形 + 一个零面积的共线细长条（三个顶点共线），
	// 渲染不得崩溃，且退化多边形不贡献像素。
	degenerate := Scene{
		Name: "degenerate",
		Polys: [][]geom.Point{
			rect(0, 0, 10, 8),
			{{X: 5, Y: 9}, {X: 5, Y: 9.5}, {X: 5, Y: 9}},
		},
		Cfg: base,
	}

	return []Scene{single, overlap, edge, large, degenerate}
}

// SceneByName 按名字取场景（每次返回全新拷贝，可安全改动用于注入偏差）。
func SceneByName(name string) (Scene, error) {
	for _, sc := range Scenes() {
		if sc.Name == name {
			return sc, nil
		}
	}
	return Scene{}, fmt.Errorf("未知场景: %s", name)
}

// Render 用管线渲染一个场景。
func Render(sc Scene) (*raster.Image, error) {
	return pipe.Run(sc.Polys, sc.Cfg)
}
