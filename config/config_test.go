package config

import (
	"math"
	"testing"
)

func TestDefaultOK(t *testing.T) {
	if err := Default().Validate(); err != nil {
		t.Fatalf("默认配置应通过: %v", err)
	}
}

func TestBadWidth(t *testing.T) {
	c := Default()
	c.Width = 0
	if err := c.Validate(); err == nil {
		t.Fatal("width=0 应报错")
	}
}

func TestBadShading(t *testing.T) {
	c := Default()
	c.Shading = "bogus"
	if err := c.Validate(); err == nil {
		t.Fatal("非法着色模式应报错")
	}
}

// TestRejectsNonFinite 覆盖本次补上的边界：NaN 以前能穿过所有区间比较，
// 静默产出一张全黑图；rotate 以前完全没校验。
func TestRejectsNonFinite(t *testing.T) {
	cases := []struct {
		name  string
		param string
		mut   func(*Config)
	}{
		{"scale=NaN", "scale", func(c *Config) { c.Scale = math.NaN() }},
		{"scale=+Inf", "scale", func(c *Config) { c.Scale = math.Inf(1) }},
		{"scale=-Inf", "scale", func(c *Config) { c.Scale = math.Inf(-1) }},
		{"rotate=NaN", "rotate", func(c *Config) { c.Rotate = math.NaN() }},
		{"rotate=+Inf", "rotate", func(c *Config) { c.Rotate = math.Inf(1) }},
		{"rotate=-Inf", "rotate", func(c *Config) { c.Rotate = math.Inf(-1) }},
		{"gamma=NaN", "gamma", func(c *Config) { c.Gamma = math.NaN() }},
		{"gamma=+Inf", "gamma", func(c *Config) { c.Gamma = math.Inf(1) }},
		{"gamma=-Inf", "gamma", func(c *Config) { c.Gamma = math.Inf(-1) }},
	}
	for _, tc := range cases {
		c := Default()
		tc.mut(&c)
		issues := c.Issues()
		if len(issues) != 1 || issues[0].Param != tc.param {
			t.Fatalf("%s: 应只拒绝 %s，实际 %+v", tc.name, tc.param, issues)
		}
		if c.Validate() == nil {
			t.Fatalf("%s: Validate 应报错", tc.name)
		}
	}
}

// TestReportsAllIssues 确认多个非法参数一次性全部报出，而不是只报第一条。
func TestReportsAllIssues(t *testing.T) {
	c := Default()
	c.Width = 0
	c.Scale = math.NaN()
	c.Rotate = math.Inf(1)
	c.Gamma = 0
	c.Output = ""
	issues := c.Issues()
	if len(issues) != 5 {
		t.Fatalf("应报 5 条，实际 %d: %+v", len(issues), issues)
	}
	want := []string{"width", "scale", "rotate", "gamma", "output"}
	for i, p := range want {
		if issues[i].Param != p {
			t.Fatalf("第 %d 条应为 %s，实际 %s", i, p, issues[i].Param)
		}
	}
}

// TestAcceptedBoundaries 锁住仍然放行的合法极值，防止以后误收紧。
func TestAcceptedBoundaries(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*Config)
	}{
		{"width=1", func(c *Config) { c.Width = 1 }},
		{"width=4096", func(c *Config) { c.Width = 4096 }},
		{"height=4096", func(c *Config) { c.Height = 4096 }},
		{"scale=最小正数", func(c *Config) { c.Scale = math.SmallestNonzeroFloat64 }},
		{"scale=100", func(c *Config) { c.Scale = 100 }},
		{"rotate=1e300", func(c *Config) { c.Rotate = 1e300 }},
		{"rotate=-1e300", func(c *Config) { c.Rotate = -1e300 }},
		{"rotate=pi", func(c *Config) { c.Rotate = math.Pi }},
		{"supersample=1", func(c *Config) { c.Supersample = 1 }},
		{"supersample=8", func(c *Config) { c.Supersample = 8 }},
		{"shading=gradient", func(c *Config) { c.Shading = "gradient" }},
		{"gamma=0.1", func(c *Config) { c.Gamma = 0.1 }},
		{"gamma=10", func(c *Config) { c.Gamma = 10 }},
		{"blur=0", func(c *Config) { c.Blur = 0 }},
		{"blur=16", func(c *Config) { c.Blur = 16 }},
	}
	for _, tc := range cases {
		c := Default()
		tc.mut(&c)
		if err := c.Validate(); err != nil {
			t.Fatalf("%s 应放行，实际: %v", tc.name, err)
		}
	}
}
