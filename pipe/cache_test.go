package pipe

import (
	"bytes"
	"crypto/sha256"
	"testing"

	"vecgo/config"
	"vecgo/geom"
	"vecgo/raster"
)

func sceneOf(tris ...[]geom.Point) [][]geom.Point { return tris }

func tri(x0, y0, x1, y1, x2, y2 float64) []geom.Point {
	return []geom.Point{{X: x0, Y: y0}, {X: x1, Y: y1}, {X: x2, Y: y2}}
}

func cfgOf(w, h, ss int) config.Config {
	c := config.Default()
	c.Width, c.Height, c.Supersample = w, h, ss
	return c
}

// TestPipelineGolden locks the whole chain (fill -> shading -> gamma -> blur)
// to bytes from the pre-cache commit. A cached run must land on the same bytes.
func TestPipelineGolden(t *testing.T) {
	cases := []struct {
		name  string
		cfg   config.Config
		scene [][]geom.Point
		want  [8]byte
	}{
		{
			// A square is not a triangle: the cache must refuse it, so only
			// the direct golden applies here.
			"square-nontriangle", config.Default(),
			sceneOf([]geom.Point{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}),
			[8]byte{0xf4, 0x7a, 0x8e, 0xc3, 0xe9, 0xaf, 0xf2, 0x31},
		},
		{
			"triangle-shaded", func() config.Config {
				c := config.Default()
				c.Width, c.Height = 48, 32
				c.Shading, c.Gamma, c.Blur = "gradient", 2.2, 1
				c.Rotate, c.Scale = 0.3, 1.5
				return c
			}(),
			sceneOf(tri(1, 1, 9, 2, 4, 9)),
			[8]byte{0x33, 0xea, 0x43, 0x8a, 0xdc, 0xca, 0x23, 0xab},
		},
		{
			"duplicate-triangles", cfgOf(24, 24, 2),
			sceneOf(tri(1, 1, 9, 2, 4, 9), tri(1, 1, 9, 2, 4, 9), tri(2, 3, 8, 3, 5, 8)),
			[8]byte{0x6f, 0x7b, 0xf1, 0xfb, 0xd5, 0x44, 0x37, 0x8e},
		},
		{
			"degenerate", cfgOf(20, 20, 2),
			sceneOf(tri(1, 1, 5, 5, 9, 9), tri(2, 2, 2, 2, 7, 7), tri(3, 1, 7, 4, 5, 6)),
			[8]byte{0x8d, 0x04, 0x9e, 0xcb, 0x8d, 0xdd, 0x78, 0x1d},
		},
	}
	for _, tc := range cases {
		im, err := Run(tc.scene, tc.cfg)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(im.Pix)
		if !bytes.Equal(sum[:8], tc.want[:]) {
			t.Errorf("%s 直接管线输出已变: got %x want %x", tc.name, sum[:8], tc.want)
		}

		cache := raster.NewCache(0)
		if tc.name == "square-nontriangle" {
			if _, err := RunWithOptions(tc.scene, tc.cfg, Options{CoverageCache: cache}); err == nil {
				t.Error("四边形场景不应被缓存")
			}
			continue
		}
		cached, err := RunWithOptions(tc.scene, tc.cfg, Options{CoverageCache: cache})
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(im.Pix, cached.Pix) {
			t.Errorf("%s 缓存管线与直接管线输出不同", tc.name)
		}
		s := cache.Stats()
		t.Logf("%-19s hitrate=%.2f peak_bytes=%d pix_equal=true", tc.name, s.HitRate(), s.PeakBytes)
	}
}

// TestRunWithCacheReportsHitRate runs the same triangle twice and checks the
// second draw is a hit while the image is unchanged.
func TestRunWithCacheReportsHitRate(t *testing.T) {
	tr := tri(1, 1, 9, 2, 4, 9)
	cfg := cfgOf(32, 32, 2)
	a, err := Run(sceneOf(tr), cfg)
	if err != nil {
		t.Fatal(err)
	}
	c := raster.NewCache(0)
	b, err := RunWithOptions(sceneOf(tr, tr), cfg, Options{CoverageCache: c})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a.Pix, b.Pix) {
		t.Fatal("重复三角形改变了输出")
	}
	s := c.Stats()
	if s.Hits != 1 || s.Misses != 1 || s.HitRate() != 0.5 {
		t.Fatalf("命中率异常: %s", raster.FormatStats(s))
	}
}

// TestCacheFallbackIsNotSilent: a non-triangle with AllowCacheOnError keeps the
// direct path byte-for-byte and records why.
func TestCacheFallbackIsNotSilent(t *testing.T) {
	sq := []geom.Point{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}
	cfg := cfgOf(16, 16, 2)
	want, err := Run(sceneOf(sq), cfg)
	if err != nil {
		t.Fatal(err)
	}

	c := raster.NewCache(0)
	got, err := RunWithOptions(sceneOf(sq), cfg, Options{CoverageCache: c, AllowCacheOnError: true})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(want.Pix, got.Pix) {
		t.Fatal("回退路径输出不同")
	}
	if c.Err == nil {
		t.Fatal("回退应留下原因")
	}
	t.Logf("回退原因: %v", c.Err)

	if _, err := RunWithOptions(sceneOf(sq), cfg, Options{CoverageCache: raster.NewCache(0)}); err == nil {
		t.Fatal("未允许回退时应报错")
	}
}
