package config

import (
	"math"
	"testing"

	"vecgo/verr"
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

// 每个失败点都必须落在 config 段（0x01xx）内的各自编码，方便聚合统计。
func TestValidateKinds(t *testing.T) {
	cases := []struct {
		mut  func(*Config)
		want verr.Kind
	}{
		{func(c *Config) { c.Width = -1 }, verr.KindConfigWidth},
		{func(c *Config) { c.Height = 5000 }, verr.KindConfigHeight},
		{func(c *Config) { c.Scale = 0 }, verr.KindConfigScale},
		{func(c *Config) { c.Scale = math.NaN() }, verr.KindConfigScale},
		{func(c *Config) { c.Rotate = math.Inf(1) }, verr.KindConfigRotate},
		{func(c *Config) { c.Supersample = 0 }, verr.KindConfigSupersample},
		{func(c *Config) { c.Gamma = 0 }, verr.KindConfigGamma},
		{func(c *Config) { c.Gamma = math.NaN() }, verr.KindConfigGamma},
		{func(c *Config) { c.Blur = -1 }, verr.KindConfigBlur},
		{func(c *Config) { c.Shading = "bogus" }, verr.KindConfigShading},
		{func(c *Config) { c.Output = "" }, verr.KindConfigOutput},
	}
	for i, tc := range cases {
		c := Default()
		tc.mut(&c)
		err := c.Validate()
		if err == nil {
			t.Fatalf("case %d 应报错", i)
		}
		k, ok := verr.Of(err)
		if !ok {
			t.Fatalf("case %d 未归一: %v", i, err)
		}
		if k != tc.want {
			t.Fatalf("case %d 编码: 期望 %s 实际 %s", i, tc.want.Hex(), k.Hex())
		}
		if k.Stage() != verr.StageConfig {
			t.Fatalf("case %d 段不对: %v", i, k.Stage())
		}
	}
}
