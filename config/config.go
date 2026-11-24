package config

import "fmt"

type Config struct {
	Width, Height int
	Scale         float64
	Rotate        float64
	Supersample   int
	Shading       string
	Gamma         float64
	Blur          int
	// PyramidLevels > 0 时走“固定层数”金字塔重建路径（用恰好该层数）。
	// 0 表示不启用金字塔，渲染管线与原来逐字节一致。
	PyramidLevels int
	// TargetError > 0 且 PyramidLevels == 0 时走“按目标误差自适应选层”
	// 路径：在当前图上拟合 E(n)=A q^n 并取满足误差的最小整数层。
	// 取值为 0..255 的灰度级 RMSE；0 表示不启用。
	TargetError float64
	Output      string
}

func Default() Config {
	return Config{Width: 64, Height: 64, Scale: 1, Rotate: 0, Supersample: 2, Shading: "flat", Gamma: 1, Blur: 0, Output: "out.pgm"}
}

func (c Config) Validate() error {
	if c.Width <= 0 || c.Width > 4096 {
		return fmt.Errorf("width 超出范围: %d", c.Width)
	}
	if c.Height <= 0 || c.Height > 4096 {
		return fmt.Errorf("height 超出范围: %d", c.Height)
	}
	if c.Scale <= 0 || c.Scale > 100 {
		return fmt.Errorf("scale 超出范围: %v", c.Scale)
	}
	if c.Supersample < 1 || c.Supersample > 8 {
		return fmt.Errorf("supersample 超出范围: %d", c.Supersample)
	}
	if c.Gamma < 0.1 || c.Gamma > 10 {
		return fmt.Errorf("gamma 超出范围: %v", c.Gamma)
	}
	if c.Blur < 0 || c.Blur > 16 {
		return fmt.Errorf("blur 超出范围: %d", c.Blur)
	}
	if c.PyramidLevels < 0 || c.PyramidLevels > 64 {
		return fmt.Errorf("pyramid_levels 超出范围: %d", c.PyramidLevels)
	}
	if c.TargetError < 0 || c.TargetError > 255 {
		return fmt.Errorf("target_error 超出范围: %v", c.TargetError)
	}
	if c.Shading != "flat" && c.Shading != "gradient" {
		return fmt.Errorf("shading 非法: %s", c.Shading)
	}
	if c.Output == "" {
		return fmt.Errorf("output 不能为空")
	}
	return nil
}
