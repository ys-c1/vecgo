package config

import (
	"fmt"

	"vecgo/level"
)

type Config struct {
	Width, Height int
	Scale         float64
	Rotate        float64
	Supersample   int
	Shading       string
	Gamma         float64
	Blur          int
	Output        string
	// LevelDist 是色阶沿参数轴的分布："" 或 "linear"（默认，历史产物逐字节不变）、
	// "perceptual"（幂律感知均匀，配合 LevelAlpha，缺省 2）、"lab"（CIE L* 交叉验证）。
	LevelDist string
	// LevelAlpha 是幂律感知假设 P(t)=t^LevelAlpha 的指数；0 表示用 level.DefaultAlpha(=2)。
	LevelAlpha float64
}

func Default() Config {
	return Config{Width: 64, Height: 64, Scale: 1, Rotate: 0, Supersample: 2, Shading: "flat", Gamma: 1, Blur: 0, Output: "out.pgm", LevelDist: "linear", LevelAlpha: 0}
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
	if _, err := level.Normalize(c.LevelDist); err != nil {
		return err
	}
	if _, err := level.CleanAlpha(c.LevelAlpha); err != nil {
		return err
	}
	if c.Output == "" {
		return fmt.Errorf("output 不能为空")
	}
	return nil
}
