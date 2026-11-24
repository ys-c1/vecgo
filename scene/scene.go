// Package scene 定义确定性核验用的标准场景与配置，
// 供渲染驱动命令和测试共用，保证两档优化渲染的是同一份输入。
package scene

import (
	"vecgo/config"
	"vecgo/geom"
)

// Name 是标准场景的标识，顺序固定，报告按该顺序出表。
type Name string

const (
	Single     Name = "single"
	Overlap    Name = "overlap"
	Large      Name = "large"
	Sliver     Name = "sliver"
	Degenerate Name = "degenerate"
)

// Cases 按固定顺序返回全部标准场景。
func Cases() []Case {
	return []Case{
		{
			Name: Single,
			Desc: "单三角形",
			Geom: [][]geom.Point{
				{{X: 1, Y: 1}, {X: 9, Y: 2}, {X: 4, Y: 9}},
			},
			Cfg: base(64, 64),
		},
		{
			Name: Overlap,
			Desc: "重叠三角形",
			Geom: [][]geom.Point{
				{{X: 0.5, Y: 0.5}, {X: 8.5, Y: 1}, {X: 3, Y: 9.5}},
				{{X: 2.5, Y: 1.5}, {X: 9.5, Y: 3}, {X: 6, Y: 8.5}},
			},
			Cfg: base(64, 64),
		},
		{
			Name: Large,
			Desc: "大画幅",
			Geom: [][]geom.Point{
				{{X: 120, Y: 80}, {X: 1900, Y: 260}, {X: 760, Y: 1980}},
				{{X: 300, Y: 1500}, {X: 1750, Y: 1700}, {X: 1050, Y: 120}},
			},
			Cfg: base(2048, 2048),
		},
		{
			Name: Sliver,
			Desc: "细长三角形",
			Geom: [][]geom.Point{
				{{X: 0, Y: 0}, {X: 64, Y: 63.99}, {X: 64, Y: 64.01}},
			},
			Cfg: base(128, 128),
		},
		{
			Name: Degenerate,
			Desc: "退化（共线）三角形",
			Geom: [][]geom.Point{
				{{X: 0, Y: 0}, {X: 5, Y: 5}, {X: 10, Y: 10}},
			},
			Cfg: base(64, 64),
		},
	}
}

// Case 是一个标准渲染场景。
type Case struct {
	Name Name
	Desc string
	Geom [][]geom.Point
	Cfg  config.Config
}

func base(w, h int) config.Config {
	c := config.Default()
	c.Width, c.Height = w, h
	c.Shading = "gradient"
	c.Gamma = 1.8
	c.Blur = 1
	c.Output = "detrender.pgm" // 非空占位，驱动命令会按输出目录覆盖
	return c
}

// ByName 按标识查找场景，找不到返回 false。
func ByName(name string) (Case, bool) {
	for _, c := range Cases() {
		if string(c.Name) == name {
			return c, true
		}
	}
	return Case{}, false
}
