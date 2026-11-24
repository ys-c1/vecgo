package config

import (
	"math"

	"vecgo/verr"
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
}

func Default() Config {
	return Config{Width: 64, Height: 64, Scale: 1, Rotate: 0, Supersample: 2, Shading: "flat", Gamma: 1, Blur: 0, Output: "out.pgm"}
}

func (c Config) Validate() error {
	if c.Width <= 0 || c.Width > 4096 {
		return verr.Errorf(verr.KindConfigWidth, "width 超出范围: %d", c.Width)
	}
	if c.Height <= 0 || c.Height > 4096 {
		return verr.Errorf(verr.KindConfigHeight, "height 超出范围: %d", c.Height)
	}
	if math.IsNaN(c.Scale) || math.IsInf(c.Scale, 0) || c.Scale <= 0 || c.Scale > 100 {
		return verr.Errorf(verr.KindConfigScale, "scale 超出范围: %v", c.Scale)
	}
	if math.IsNaN(c.Rotate) || math.IsInf(c.Rotate, 0) {
		return verr.Errorf(verr.KindConfigRotate, "rotate 非有限: %v", c.Rotate)
	}
	if c.Supersample < 1 || c.Supersample > 8 {
		return verr.Errorf(verr.KindConfigSupersample, "supersample 超出范围: %d", c.Supersample)
	}
	if math.IsNaN(c.Gamma) || math.IsInf(c.Gamma, 0) || c.Gamma < 0.1 || c.Gamma > 10 {
		return verr.Errorf(verr.KindConfigGamma, "gamma 超出范围: %v", c.Gamma)
	}
	if c.Blur < 0 || c.Blur > 16 {
		return verr.Errorf(verr.KindConfigBlur, "blur 超出范围: %d", c.Blur)
	}
	if c.Shading != "flat" && c.Shading != "gradient" {
		return verr.Errorf(verr.KindConfigShading, "shading 非法: %s", c.Shading)
	}
	if c.Output == "" {
		return verr.Errorf(verr.KindConfigOutput, "output 不能为空")
	}
	return nil
}
