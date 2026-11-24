package pipe

import (
	"bytes"
	"testing"

	"vecgo/config"
	"vecgo/geom"
)

// TestRunWithStatsSamePixels 验证统计路径与普通渲染输出逐字节相同。
func TestRunWithStatsSamePixels(t *testing.T) {
	scenes := map[string][][]geom.Point{
		"square": {{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}},
		"seam": {
			{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}},
			{{X: 0, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}},
		},
	}
	for name, scene := range scenes {
		for _, cfg := range []config.Config{
			func() config.Config {
				c := config.Default()
				c.Width, c.Height = 24, 24
				return c
			}(),
			func() config.Config {
				c := config.Default()
				c.Width, c.Height = 24, 24
				c.Gamma = 2.2
				c.Blur = 2
				c.Shading = "gradient"
				return c
			}(),
		} {
			plain, err := Run(scene, cfg)
			if err != nil {
				t.Fatal(err)
			}
			measured, st, err := RunWithStats(scene, cfg)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(plain.Pix, measured.Pix) {
				t.Fatalf("%s: 统计路径改变了输出像素", name)
			}
			if st == nil {
				t.Fatalf("%s: 未返回统计", name)
			}
			if issues := st.Verify(); len(issues) != 0 {
				t.Fatalf("%s: 账目不符: %v", name, issues)
			}
		}
	}
}

// TestRunWithStatsRejectsBadConfig 统计路径同样要校验配置。
func TestRunWithStatsRejectsBadConfig(t *testing.T) {
	scene := [][]geom.Point{{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 1, Y: 1}}}
	cfg := config.Default()
	cfg.Blur = -1
	if _, _, err := RunWithStats(scene, cfg); err == nil {
		t.Fatal("blur=-1 应报错")
	}
}

// TestRunWithStatsSeamDoubleWrite 接缝场景应记录到 2 次覆盖像素，
// 且覆盖总次数大于面积和。
func TestRunWithStatsSeamDoubleWrite(t *testing.T) {
	scene := [][]geom.Point{
		{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}},
		{{X: 0, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}},
	}
	cfg := config.Default()
	cfg.Width, cfg.Height = 32, 32
	_, st, err := RunWithStats(scene, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if st.Buckets()[1].Pixels == 0 {
		t.Fatal("接缝应产生 2 次覆盖像素")
	}
	if st.Diff() <= 0 {
		t.Fatalf("接缝重复写入应使差值大于 0，实际 %v", st.Diff())
	}
}
