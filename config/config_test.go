package config

import (
	"strings"
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

func TestBadDitherKernel(t *testing.T) {
	c := Default()
	c.Dither = true
	c.DitherKernel = "bogus"
	err := c.Validate()
	if err == nil {
		t.Fatal("非法抖动核应报错")
	}
	if !strings.Contains(err.Error(), "ditherKernel") {
		t.Fatalf("错误信息应带参数名 ditherKernel: %v", err)
	}
}

func TestBadDitherStrength(t *testing.T) {
	c := Default()
	c.Dither = true
	c.DitherStrength = -0.5
	err := c.Validate()
	if err == nil {
		t.Fatal("ditherStrength<0 应报错")
	}
	if !strings.Contains(err.Error(), "ditherStrength") {
		t.Fatalf("错误信息应带参数名 ditherStrength: %v", err)
	}
}

func TestBadDitherDepth(t *testing.T) {
	c := Default()
	c.Dither = true
	c.DitherDepth = 9
	err := c.Validate()
	if err == nil {
		t.Fatal("ditherDepth=9 应报错")
	}
	if !strings.Contains(err.Error(), "ditherDepth") {
		t.Fatalf("错误信息应带参数名 ditherDepth: %v", err)
	}
}

// 未开启抖动时，抖动字段的零值不应影响校验（既有路径不受影响）。
func TestDitherOffSkipsDitherChecks(t *testing.T) {
	c := Default()
	c.Dither = false
	c.DitherKernel = ""
	c.DitherStrength = 0
	c.DitherDepth = 0
	if err := c.Validate(); err != nil {
		t.Fatalf("关闭抖动时不应校验抖动参数: %v", err)
	}
}
