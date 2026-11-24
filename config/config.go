package config

import "fmt"

type Config struct {
	Width, Height int
	Scale         float64
	Rotate        float64
	Supersample   int
	// Samples 是每像素独立随机采样数。为 0（默认）时渲染走确定性的
	// Supersample×Supersample 网格，产物与旧版逐字节相同；>0 时改走
	// raster.FillRandom 的二项蒙特卡洛覆盖估计。
	Samples int
	// Seed 是随机采样的固定种子，保证可复现；Samples=0 时不使用。
	Seed    int64
	Shading string
	Gamma   float64
	Blur    int
	Output  string
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
	if c.Samples < 0 || c.Samples > 1000000 {
		return fmt.Errorf("samples 超出范围: %d", c.Samples)
	}
	if c.Gamma < 0.1 || c.Gamma > 10 {
		return fmt.Errorf("gamma 超出范围: %v", c.Gamma)
	}
	if c.Blur < 0 || c.Blur > 16 {
		return fmt.Errorf("blur 超出范围: %d", c.Blur)
	}
	if c.Shading != "flat" && c.Shading != "gradient" {
		return fmt.Errorf("shading 非法: %s", c.Shading)
	}
	if c.Output == "" {
		return fmt.Errorf("output 不能为空")
	}
	return nil
}
