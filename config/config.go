package config

import (
	"fmt"

	"vecgo/geom"
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
	// EdgeRule 选择贴边像素归属规则：
	// "half-pixel"（默认）中心采样 + 左上角规则；"legacy" 复现改动前的射线法产物。
	EdgeRule string
}

func Default() Config {
	return Config{Width: 64, Height: 64, Scale: 1, Rotate: 0, Supersample: 2, Shading: "flat", Gamma: 1, Blur: 0, Output: "out.pgm", EdgeRule: "half-pixel"}
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
	if c.EdgeRule != "half-pixel" && c.EdgeRule != "legacy" {
		return fmt.Errorf("edge-rule 非法: %s", c.EdgeRule)
	}
	if c.Output == "" {
		return fmt.Errorf("output 不能为空")
	}
	return nil
}

// ParseEdgeRule 把配置字符串映射为 geom.EdgeRule。
func ParseEdgeRule(s string) (geom.EdgeRule, error) {
	switch s {
	case "half-pixel":
		return geom.RuleHalfPixel, nil
	case "legacy":
		return geom.RuleLegacy, nil
	default:
		return geom.RuleHalfPixel, fmt.Errorf("edge-rule 非法: %s", s)
	}
}
