package config

import (
	"errors"
	"fmt"
	"math"
	"strings"
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

// Issue 描述一个被拒绝的参数。Param 是参数名，Value 是被拒绝值的文本形式，
// Reason 是只讲原因的短句，Msg 是 "参数名 原因" 的完整信息
// （与 Validate 返回的错误文本一致），便于直接贴进工单。
type Issue struct {
	Param  string
	Value  string
	Reason string
	Msg    string
}

// Issues 按固定顺序列出所有不合法参数。以前 NaN 能穿过区间比较
// （NaN < x 与 NaN > x 都为假），rotate 则完全没有校验，因此这里对
// 每个浮点参数先做有限性检查，再做区间检查。
func (c Config) Issues() []Issue {
	var out []Issue
	add := func(param string, value any, format string, args ...any) {
		reason := fmt.Sprintf(format, args...)
		out = append(out, Issue{
			Param:  param,
			Value:  fmt.Sprint(value),
			Reason: reason,
			Msg:    param + " " + reason,
		})
	}

	if c.Width <= 0 || c.Width > 4096 {
		add("width", c.Width, "超出范围: %d", c.Width)
	}
	if c.Height <= 0 || c.Height > 4096 {
		add("height", c.Height, "超出范围: %d", c.Height)
	}
	if !finite(c.Scale) {
		add("scale", c.Scale, "非有限值: %v", c.Scale)
	} else if c.Scale <= 0 || c.Scale > 100 {
		add("scale", c.Scale, "超出范围: %v", c.Scale)
	}
	// 旋转角以 2π 为周期，任何有限值都有确定的 cos/sin，因此只拒绝非有限值。
	if !finite(c.Rotate) {
		add("rotate", c.Rotate, "非有限值: %v", c.Rotate)
	}
	if c.Supersample < 1 || c.Supersample > 8 {
		add("supersample", c.Supersample, "超出范围: %d", c.Supersample)
	}
	if c.Shading != "flat" && c.Shading != "gradient" {
		add("shading", c.Shading, "非法: %s", c.Shading)
	}
	if !finite(c.Gamma) {
		add("gamma", c.Gamma, "非有限值: %v", c.Gamma)
	} else if c.Gamma < 0.1 || c.Gamma > 10 {
		add("gamma", c.Gamma, "超出范围: %v", c.Gamma)
	}
	if c.Blur < 0 || c.Blur > 16 {
		add("blur", c.Blur, "超出范围: %d", c.Blur)
	}
	if c.Output == "" {
		add("output", c.Output, "不能为空")
	}
	return out
}

func finite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}

func (c Config) Validate() error {
	issues := c.Issues()
	if len(issues) == 0 {
		return nil
	}
	if len(issues) == 1 {
		return errors.New(issues[0].Msg)
	}
	msgs := make([]string, len(issues))
	for i, is := range issues {
		msgs[i] = is.Msg
	}
	return errors.New(strings.Join(msgs, "; "))
}
