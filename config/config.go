package config

import "vecgo/param"

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

// Validate 校验整份配置。所有字段的边界都来自 param 包，与各包内 API 的校验
// 共用同一套口径；出错时返回 *param.Error（多处非法时为 *param.ErrorList），
// Field 指明 config.<字段>。非法值只报错，不夹取、不兜底、不忽略。
func (c Config) Validate() error {
	return param.All(
		param.IntIn("config.Width", c.Width, param.SideMin, param.SideMax),
		param.IntIn("config.Height", c.Height, param.SideMin, param.SideMax),
		param.FloatInOpenLo("config.Scale", c.Scale, param.ScaleMin, param.ScaleMax),
		param.Finite("config.Rotate", c.Rotate),
		param.IntIn("config.Supersample", c.Supersample, param.SupersampleMin, param.SupersampleMax),
		param.Choice("config.Shading", c.Shading, param.ShadingModes),
		param.FloatIn("config.Gamma", c.Gamma, param.GammaMin, param.GammaMax),
		param.IntIn("config.Blur", c.Blur, param.BlurMin, param.BlurMax),
		param.NonEmpty("config.Output", c.Output),
	)
}
