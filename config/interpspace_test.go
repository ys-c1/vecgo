package config

import "testing"

func TestInterpSpaceNormalize(t *testing.T) {
	cases := map[string]string{
		"":             "encoded",
		"encoded":      "encoded",
		"srgb":         "encoded",
		"linear":       "linear",
		"linear-light": "linear",
		"lab":          "perceptual",
		"lstar":        "perceptual",
	}
	for in, want := range cases {
		if got := NormalizeSpace(in); got != want {
			t.Fatalf("NormalizeSpace(%q)=%q want %q", in, got, want)
		}
	}
}

func TestInterpSpaceValidate(t *testing.T) {
	for _, sp := range []string{"", "encoded", "linear", "perceptual", "srgb", "lab"} {
		c := Default()
		c.InterpSpace = sp
		if err := c.Validate(); err != nil {
			t.Fatalf("space=%q 应合法: %v", sp, err)
		}
	}
	c := Default()
	c.InterpSpace = "banana"
	if err := c.Validate(); err == nil {
		t.Fatal("非法 interpSpace 应报错")
	}
}

func TestDefaultDoesNotOwnEncoding(t *testing.T) {
	if Default().TransferOwnsEncoding() {
		t.Fatal("默认配置必须仍走历史 gamma 阶段，不能由 shading 私有编码")
	}
	c := Default()
	c.InterpSpace = "linear"
	if !c.TransferOwnsEncoding() {
		t.Fatal("linear 空间应自带 sRGB 编码并跳过 post.Gamma")
	}
}
