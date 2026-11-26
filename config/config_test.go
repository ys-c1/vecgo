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

func TestFillRuleAccepted(t *testing.T) {
	for _, r := range []string{"non-zero", "even-odd"} {
		c := Default()
		c.FillRule = r
		if err := c.Validate(); err != nil {
			t.Fatalf("规则 %q 应被接受: %v", r, err)
		}
	}
}

func TestFillRuleRejected(t *testing.T) {
	c := Default()
	c.FillRule = "winding"
	if err := c.Validate(); err == nil {
		t.Fatal("非法填充规则应报错")
	}
	c.FillRule = ""
	if err := c.Validate(); err == nil {
		t.Fatal("空填充规则应报错")
	}
}
