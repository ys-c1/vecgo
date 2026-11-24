package config

import "fmt"

type Config struct {
	Width, Height int
	Scale         float64
	Rotate        float64
	Supersample   int
	FillMode      string
	FillThreshold float64
	Shading       string
	Gamma         float64
	Blur          int
	Output        string
	BinaryOutput  string
}

func Default() Config {
	return Config{
		Width: 64, Height: 64, Scale: 1, Rotate: 0, Supersample: 2,
		FillMode: "coverage", FillThreshold: 0.5,
		Shading: "flat", Gamma: 1, Blur: 0,
		Output: "out.pgm", BinaryOutput: "out-bin.pgm",
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
	if c.FillMode != "coverage" && c.FillMode != "supersample" {
		return fmt.Errorf("fill mode 非法: %s", c.FillMode)
	}
	if c.FillThreshold < 0 || c.FillThreshold > 1 {
		return fmt.Errorf("fill threshold 超出范围: %v", c.FillThreshold)
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
	if c.BinaryOutput == "" {
		return fmt.Errorf("binary output 不能为空")
	}
	return nil
}
