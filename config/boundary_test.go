package config

import (
	"math"
	"testing"
)

// TestBoundaryThreeParams 对三个参数各试一次分界：
//   - scale：上界 100 与刚好越界
//   - rotate：有限值上界与 Inf（rotate 没有数值上界，只有有限性分界）
//   - gamma：下界 0.1 与刚好越界
//
// 每个参数都同时验证“放行侧确实放行”和“拒绝侧确实拒绝”，
// 并把放行理由打到测试日志里，供 PARAMS.md 引用。
func TestBoundaryThreeParams(t *testing.T) {
	cases := []struct {
		param  string
		value  any
		reject bool
		reason string
		mutate func(*Config)
	}{
		{
			param: "scale", value: 100.0, reject: false,
			reason: "100 是设计上界，仍能正常缩放，不改变渲染的有效性",
			mutate: func(c *Config) { c.Scale = 100 },
		},
		{
			param: "scale", value: 100.0000001, reject: true,
			mutate: func(c *Config) { c.Scale = 100.0000001 },
		},
		{
			param: "rotate", value: math.MaxFloat64, reject: false,
			reason: "有限角度总有确定的 cos/sin，渲染可复现，只是精度随角度增大而下降",
			mutate: func(c *Config) { c.Rotate = math.MaxFloat64 },
		},
		{
			param: "rotate", value: math.Inf(1), reject: true,
			mutate: func(c *Config) { c.Rotate = math.Inf(1) },
		},
		{
			param: "gamma", value: 0.1, reject: false,
			reason: "0.1 是设计下界，底数为正，幂运算有定义且输出可复现",
			mutate: func(c *Config) { c.Gamma = 0.1 },
		},
		{
			param: "gamma", value: 0.0999, reject: true,
			mutate: func(c *Config) { c.Gamma = 0.0999 },
		},
	}
	for _, tc := range cases {
		c := Default()
		tc.mutate(&c)
		err := c.Validate()
		if tc.reject && err == nil {
			t.Fatalf("%s=%v 应被拒绝，实际放行", tc.param, tc.value)
		}
		if !tc.reject && err != nil {
			t.Fatalf("%s=%v 应放行，实际拒绝: %v", tc.param, tc.value, err)
		}
		if !tc.reject {
			t.Logf("放行 %s=%v：%s", tc.param, tc.value, tc.reason)
		} else {
			t.Logf("拒绝 %s=%v：%v", tc.param, tc.value, err)
		}
	}
}
