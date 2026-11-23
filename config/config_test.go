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

func TestBadBlocksWorkers(t *testing.T) {
	c := Default()
	c.Blocks = 0
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "blocks") {
		t.Fatalf("blocks=0 应报带字段名的错误: %v", err)
	}
	c = Default()
	c.Workers = -1
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "workers") {
		t.Fatalf("workers 为负应报带字段名的错误: %v", err)
	}
}

func TestDefaultSingleThreaded(t *testing.T) {
	d := Default()
	if d.Blocks != 1 || d.Workers != 1 {
		t.Fatalf("默认配置必须保持串行基线: blocks=%d workers=%d", d.Blocks, d.Workers)
	}
}
