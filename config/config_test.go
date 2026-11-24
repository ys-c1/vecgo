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

func TestLevelDistDefaultAndValidation(t *testing.T) {
	c := Default()
	if c.LevelDist != "linear" {
		t.Fatalf("缺省色阶分布应为 linear, got %q", c.LevelDist)
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("默认 LevelDist 应合法: %v", err)
	}
	for _, good := range []string{"", "linear", "perceptual", "lab"} {
		c := Default()
		c.LevelDist = good
		if err := c.Validate(); err != nil {
			t.Fatalf("LevelDist=%q 应合法: %v", good, err)
		}
	}
	c = Default()
	c.LevelDist = "nope"
	if err := c.Validate(); err == nil {
		t.Fatal("非法 LevelDist 应报错")
	}
	c = Default()
	c.LevelAlpha = -2
	if err := c.Validate(); err == nil {
		t.Fatal("非法 LevelAlpha 应报错")
	}
}
