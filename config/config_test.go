package config

import (
	"errors"
	"math"
	"testing"

	"vecgo/param"
)

func TestDefaultOK(t *testing.T) {
	if err := Default().Validate(); err != nil {
		t.Fatalf("默认配置应通过: %v", err)
	}
}

func TestBadWidth(t *testing.T) {
	c := Default()
	c.Width = 0
	assertField(t, c.Validate(), "config.Width")
}

func TestBadShading(t *testing.T) {
	c := Default()
	c.Shading = "bogus"
	assertField(t, c.Validate(), "config.Shading")
}

// 以前这些值会被静默接受（NaN 比较恒为假），现在必须拒绝。
func TestNaNAndInfRejected(t *testing.T) {
	for _, tc := range []struct {
		name string
		set  func(*Config)
	}{
		{"Scale NaN", func(c *Config) { c.Scale = math.NaN() }},
		{"Scale +Inf", func(c *Config) { c.Scale = math.Inf(1) }},
		{"Rotate NaN", func(c *Config) { c.Rotate = math.NaN() }},
		{"Rotate Inf", func(c *Config) { c.Rotate = math.Inf(-1) }},
		{"Gamma NaN", func(c *Config) { c.Gamma = math.NaN() }},
	} {
		c := Default()
		tc.set(&c)
		if err := c.Validate(); err == nil {
			t.Fatalf("%s 应被拒绝", tc.name)
		}
	}
}

func TestMultipleErrorsReported(t *testing.T) {
	c := Default()
	c.Width = -1
	c.Gamma = 0
	err := c.Validate()
	var list *param.ErrorList
	if !errors.As(err, &list) || len(list.Errors) != 2 {
		t.Fatalf("应汇总两处非法参数，实际: %v", err)
	}
}

func assertField(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("应报错并指明 %s", want)
	}
	var pe *param.Error
	if !errors.As(err, &pe) {
		t.Fatalf("错误类型应为 *param.Error，实际 %T: %v", err, err)
	}
	if pe.Field != want {
		t.Fatalf("字段应为 %s，实际 %s", want, pe.Field)
	}
}
