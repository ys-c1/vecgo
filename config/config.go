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
	// InterpSpace 选择渐变插值所在的颜色空间：
	//   "" / "encoded" / "srgb" / "gamma"  —— 直接在编码灰度上线性插值（历史行为，默认，产物逐字节不变）
	//   "linear" / "linear-light"          —— 先解码到线性光，在线性光上插值，再编码回 sRGB
	//   "perceptual" / "lab" / "lstar"     —— 在 CIELAB 明度 L* 上均匀插值，再转回 sRGB
	InterpSpace string
	Output      string
}

// NormalizeSpace 把 InterpSpace 的各种别名归一到 ""(=encoded) / "linear" / "perceptual"。
// 空串与无法识别的值按默认编码空间处理；是否非法由 Validate 判定。
func NormalizeSpace(s string) string {
	switch s {
	case "", "encoded", "encode", "srgb", "sRGB", "gamma", "default":
		return "encoded"
	case "linear", "linear-light", "linearlight", "rgb-linear":
		return "linear"
	case "perceptual", "lab", "lstar", "l*", "cilab":
		return "perceptual"
	default:
		return s
	}
}

// TransferOwnsEncoding 报告该空间是否由 shading 一次性输出最终 sRGB 编码字节。
// 为 true 时 pipe 不再叠加 post.Gamma，以免二次传递。
func (c Config) TransferOwnsEncoding() bool {
	s := NormalizeSpace(c.InterpSpace)
	return s == "linear" || s == "perceptual"
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
	switch NormalizeSpace(c.InterpSpace) {
	case "encoded", "linear", "perceptual":
	default:
		return fmt.Errorf("interpSpace 非法: %s", c.InterpSpace)
	}
	if c.Output == "" {
		return fmt.Errorf("output 不能为空")
	}
	return nil
}
