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
	Output        string

	// Frame 选择几何→像素的映射/缩放路径：
	//   "" 或 "bbox"：原始路径，按多边形包围盒归一化到整幅（默认，产物与历史一致）；
	//   "world"    ：固定世界视口，使用 MinX/MinY/PPU，缩放会让边在固定像素网格上移动。
	Frame string
	MinX  float64
	MinY  float64
	PPU   float64 // 仅 Frame=="world" 时使用，必须为正（像素/世界单位）
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
	if c.Shading != "flat" && c.Shading != "gradient" {
		return fmt.Errorf("shading 非法: %s", c.Shading)
	}
	switch c.Frame {
	case "", "bbox":
	case "world":
		if c.PPU <= 0 {
			return fmt.Errorf("world 视口需要正的 PPU: %v", c.PPU)
		}
	default:
		return fmt.Errorf("frame 非法: %s（应为 bbox 或 world）", c.Frame)
	}
	if c.Output == "" {
		return fmt.Errorf("output 不能为空")
	}
	return nil
}
