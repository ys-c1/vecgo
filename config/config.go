package config

import "fmt"

type Config struct {
	Width, Height  int
	Scale          float64
	Rotate         float64
	Supersample    int
	Shading        string
	Gamma          float64
	Blur           int
	Dither         bool
	DitherKernel   string
	DitherStrength float64
	DitherDepth    int
	Output         string
}

func Default() Config {
	return Config{
		Width: 64, Height: 64, Scale: 1, Rotate: 0, Supersample: 2,
		Shading: "flat", Gamma: 1, Blur: 0,
		Dither: false, DitherKernel: "floyd-steinberg", DitherStrength: 1.0, DitherDepth: 4,
		Output: "out.pgm",
	}
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
	if c.Shading != "flat" && c.Shading != "gradient" {
		return fmt.Errorf("shading 非法: %s", c.Shading)
	}
	if c.Dither {
		if c.DitherKernel != "floyd-steinberg" && c.DitherKernel != "atkinson" {
			return fmt.Errorf("ditherKernel 非法: %s", c.DitherKernel)
		}
		if c.DitherStrength < 0 {
			return fmt.Errorf("ditherStrength 不能为负: %v", c.DitherStrength)
		}
		if c.DitherDepth < 1 || c.DitherDepth > 8 {
			return fmt.Errorf("ditherDepth 超出范围 [1,8]: %d", c.DitherDepth)
		}
	}
	if c.Output == "" {
		return fmt.Errorf("output 不能为空")
	}
	return nil
}
