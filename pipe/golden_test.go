package pipe

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"vecgo/config"
	"vecgo/geom"
)

// 这些哈希取自增量渲染改动之前的全量路径输出。
// 全量路径的像素不得因增量改造而改变。
func TestFullPathOutputUnchanged(t *testing.T) {
	scene := [][]geom.Point{
		{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}},
		{{X: 4, Y: 4}, {X: 9, Y: 4}, {X: 9, Y: 9}, {X: 4, Y: 9}},
	}
	cases := []struct {
		name string
		cfg  config.Config
		want string
	}{
		{"default", config.Default(), "f47a8ec3e9aff2318d896942282ad4fe37d6391c82914f54a5da8a37de1300c6"},
		{"post", func() config.Config {
			c := config.Default()
			c.Width, c.Height = 64, 64
			c.Supersample = 3
			c.Gamma = 1.7
			c.Blur = 2
			c.Shading = "gradient"
			return c
		}(), "bad85f4321f1b585142b13d81892fc5873cf58de3ace35de42c6aabe87968deb"},
		{"odd", func() config.Config {
			c := config.Default()
			c.Width, c.Height = 33, 17
			c.Scale = 2.5
			c.Rotate = 0.4
			c.Supersample = 4
			c.Gamma = 0.6
			c.Blur = 3
			return c
		}(), "398f5c87222848eeb3f8c2cd025686440f3c5d94aa9f63cfbb4e0a7665845a27"},
	}
	for _, tc := range cases {
		im, err := Run(scene, tc.cfg)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		sum := sha256.Sum256(im.Pix)
		if got := hex.EncodeToString(sum[:]); got != tc.want {
			t.Fatalf("%s 全量输出已改变: %s != %s", tc.name, got, tc.want)
		}
	}
}
