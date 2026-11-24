package config

import "testing"

func TestDefaultEdgeRule(t *testing.T) {
	c := Default()
	if c.EdgeRule != "half-pixel" {
		t.Fatalf("默认 EdgeRule=%s，期望 half-pixel", c.EdgeRule)
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("默认配置应通过校验: %v", err)
	}
}

func TestEdgeRuleValidation(t *testing.T) {
	for _, ok := range []string{"half-pixel", "legacy"} {
		c := Default()
		c.EdgeRule = ok
		if err := c.Validate(); err != nil {
			t.Fatalf("合法规则 %s 不应报错: %v", ok, err)
		}
	}
	c := Default()
	c.EdgeRule = "corner-sample"
	if err := c.Validate(); err == nil {
		t.Fatal("非法 edge-rule 应报错")
	}
}

func TestParseEdgeRule(t *testing.T) {
	if r, err := ParseEdgeRule("half-pixel"); err != nil || r != 0 {
		t.Fatalf("half-pixel 解析错误: %v %v", r, err)
	}
	if r, err := ParseEdgeRule("legacy"); err != nil || r != 1 {
		t.Fatalf("legacy 解析错误: %v %v", r, err)
	}
	if _, err := ParseEdgeRule("nope"); err == nil {
		t.Fatal("非法值应返回错误")
	}
}
