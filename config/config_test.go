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

func TestCacheValidation(t *testing.T) {
	c := Default()
	if c.CacheEnabled {
		t.Fatal("默认应关闭缓存")
	}
	c.CacheEnabled = true
	c.CacheCapacity = 0
	if err := c.Validate(); err == nil {
		t.Fatal("开启缓存但容量为 0 应报错")
	}
	c.CacheCapacity = 16
	if err := c.Validate(); err != nil {
		t.Fatalf("合法缓存配置应通过: %v", err)
	}
	c.CacheMaxBytesPerTriangle = -1
	if err := c.Validate(); err == nil {
		t.Fatal("负斜率上限应报错")
	}
}

func TestCacheSlopeLimitDefault(t *testing.T) {
	c := Default()
	c.CacheMaxBytesPerTriangle = 0
	if got := c.CacheSlopeLimit(); got != DefaultMaxBytesPerTriangle {
		t.Fatalf("零值应回退到默认上限, 实际 %v", got)
	}
	c.CacheMaxBytesPerTriangle = 12.5
	if got := c.CacheSlopeLimit(); got != 12.5 {
		t.Fatalf("显式上限应生效, 实际 %v", got)
	}
}
