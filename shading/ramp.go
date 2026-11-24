package shading

import (
	"vecgo/ramp"
	"vecgo/raster"
)

// ApplyCubicRamp 用控制点的三次 Hermite 渐变逐行填充图像（第 r 行对应
// x=r/(H-1)，全行同值）。optimize=false 使用会过冲的朴素平均切线（事故版本）；
// optimize=true 先用同号 + Fritsch–Carlson 约束做最优切线扫描，消除过冲。
//
// 返回填充所用的 *ramp.Curve，调用方可继续做过冲/偏离计量。
// 原有的线性 Apply 行为完全不变。
func ApplyCubicRamp(im *raster.Image, stops []ramp.Stop, optimize bool) (*ramp.Curve, error) {
	c, err := ramp.New(stops)
	if err != nil {
		return nil, err
	}
	if optimize {
		c.Optimize(2000, 1e-13)
	}
	if err := c.FillImage(im); err != nil {
		return nil, err
	}
	return c, nil
}
