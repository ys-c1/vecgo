package config

import "fmt"

// Config 的字段顺序即配置键、字段快照的固定顺序。
// 每个字段末尾注释里的“默认值”必须与 Default() 一致，
// 由 load_test.go 的 TestCommentsMatchDefault 直接解析源码核对。
//
// 零值 Config 不是默认配置：除 Rotate、Blur 外，零值都落在 Validate 的
// 非法区间内。需要“缺省即默认”的语义时，请走 Load/LoadFile（从 Default
// 出发，只覆盖文件中出现的键），不要直接构造 Config{}。
type Config struct {
	Width       int     `json:"width"`       // 默认值: 64
	Height      int     `json:"height"`      // 默认值: 64
	Scale       float64 `json:"scale"`       // 默认值: 1
	Rotate      float64 `json:"rotate"`      // 默认值: 0
	Supersample int     `json:"supersample"` // 默认值: 2
	Shading     string  `json:"shading"`     // 默认值: "flat"
	Gamma       float64 `json:"gamma"`       // 默认值: 1
	Blur        int     `json:"blur"`        // 默认值: 0
	Output      string  `json:"output"`      // 默认值: "out.pgm"
}

// Default 返回默认配置。返回值的每个字段与 Config 字段注释里的“默认值”一致，
// 由 load_test.go 的 TestCommentsMatchDefault 直接解析源码双向核对。
func Default() Config {
	return Config{
		Width:       64,        // 默认值: 64
		Height:      64,        // 默认值: 64
		Scale:       1,         // 默认值: 1
		Rotate:      0,         // 默认值: 0
		Supersample: 2,         // 默认值: 2
		Shading:     "flat",    // 默认值: "flat"
		Gamma:       1,         // 默认值: 1
		Blur:        0,         // 默认值: 0
		Output:      "out.pgm", // 默认值: "out.pgm"
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
	if c.Output == "" {
		return fmt.Errorf("output 不能为空")
	}
	return nil
}
