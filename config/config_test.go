package config

import "testing"

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

func TestBadGain(t *testing.T) {
	for _, g := range []float64{0, -1, 11} {
		c := Default()
		c.Gain = g
		if err := c.Validate(); err == nil {
			t.Fatalf("gain=%v 应报错", g)
		}
	}
}
