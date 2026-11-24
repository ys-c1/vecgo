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

func TestPyramidFieldsValidation(t *testing.T) {
	for _, lv := range []int{-1, 65} {
		c := Default()
		c.PyramidLevels = lv
		if err := c.Validate(); err == nil {
			t.Fatalf("pyramid_levels=%d 应报错", lv)
		}
	}
	for _, te := range []float64{-0.1, 256} {
		c := Default()
		c.TargetError = te
		if err := c.Validate(); err == nil {
			t.Fatalf("target_error=%v 应报错", te)
		}
	}
	// 合法的金字塔配置应通过。
	c := Default()
	c.PyramidLevels = 4
	c.TargetError = 3.5
	if err := c.Validate(); err != nil {
		t.Fatalf("合法金字塔配置不应报错: %v", err)
	}
}
