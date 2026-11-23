package raster

import (
	"bytes"
	"testing"

	"vecgo/cache"
	"vecgo/geom"
)

func square(x, y, size float64) []geom.Point {
	return []geom.Point{
		{X: x, Y: y}, {X: x + size, Y: y},
		{X: x + size, Y: y + size}, {X: x, Y: y + size},
	}
}

func runBoth(t *testing.T, polys [][]geom.Point, w, h, ss int) (*Image, *Image, *cache.Cache) {
	t.Helper()
	base, err := New(w, h)
	if err != nil {
		t.Fatal(err)
	}
	if err := base.Fill(polys, ss); err != nil {
		t.Fatal(err)
	}
	c, err := cache.New(16)
	if err != nil {
		t.Fatal(err)
	}
	cached, err := New(w, h)
	if err != nil {
		t.Fatal(err)
	}
	if err := cached.FillCached(polys, ss, c); err != nil {
		t.Fatal(err)
	}
	return base, cached, c
}

// 缓存路径必须与 Fill 逐字节一致，覆盖单三角形、重叠、相交与贴边情形。
func TestCachedFillMatchesUncached(t *testing.T) {
	cases := []struct {
		name  string
		polys [][]geom.Point
	}{
		{"single", [][]geom.Point{square(0, 0, 10)}},
		{"overlap", [][]geom.Point{square(0, 0, 10), square(5, 5, 10)}},
		{"disjoint", [][]geom.Point{square(0, 0, 4), square(6, 6, 4)}},
		{"offscreen", [][]geom.Point{square(-50, -50, 30), square(9, 9, 5)}},
		{"nested", [][]geom.Point{square(0, 0, 10), square(2, 2, 4)}},
		{"triangle", [][]geom.Point{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 5, Y: 10}}}},
		{"degenerate", [][]geom.Point{{{X: 0, Y: 0}, {X: 5, Y: 5}, {X: 10, Y: 10}}}},
	}
	for _, tc := range cases {
		for ss := 1; ss <= 4; ss++ {
			t.Run(tc.name, func(t *testing.T) {
				base, cached, _ := runBoth(t, tc.polys, 21, 17, ss)
				if !bytes.Equal(base.Pix, cached.Pix) {
					t.Fatalf("ss=%d 输出不一致", ss)
				}
			})
		}
	}
}

// 单条大三角形：一次填充覆盖整个画幅，掩码应覆盖全部像素。
func TestSingleLargeTriangle(t *testing.T) {
	huge := []geom.Point{{X: -1e4, Y: -1e4}, {X: 1e4, Y: -1e4}, {X: 0, Y: 1e4}}
	c, err := cache.New(1)
	if err != nil {
		t.Fatal(err)
	}
	for _, ss := range []int{1, 2, 4, 8} {
		base, err := New(40, 40)
		if err != nil {
			t.Fatal(err)
		}
		if err := base.Fill([][]geom.Point{huge}, ss); err != nil {
			t.Fatal(err)
		}
		im, err := New(40, 40)
		if err != nil {
			t.Fatal(err)
		}
		if err := im.FillCached([][]geom.Point{huge}, ss, c); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(base.Pix, im.Pix) {
			t.Fatalf("ss=%d 大三角形输出不一致", ss)
		}
		// 大三角形覆盖画幅下半部分以上，应有大量像素点亮。
		if im.CountOn(1) < 400 {
			t.Fatalf("ss=%d 亮像素过少: %d", ss, im.CountOn(1))
		}
	}
	// 每轮 supersample 不同，网格参数进入键，因此应各插入一次、无命中。
	if s := c.Stats(); s.Inserts != 4 || s.Hits != 0 {
		t.Fatalf("不同采样率应为不同键: %+v", s)
	}
}

// 同一个缓存复用在不同画幅上时，网格参数进入键，不能串用掩码。
func TestCacheKeyIncludesGrid(t *testing.T) {
	c, _ := cache.New(4)
	polys := [][]geom.Point{square(0, 0, 10)}
	for _, size := range [][2]int{{10, 10}, {20, 20}, {10, 20}} {
		base, err := New(size[0], size[1])
		if err != nil {
			t.Fatal(err)
		}
		if err := base.Fill(polys, 2); err != nil {
			t.Fatal(err)
		}
		im, _ := New(size[0], size[1])
		if err := im.FillCached(polys, 2, c); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(base.Pix, im.Pix) {
			t.Fatalf("画幅 %v 输出不一致", size)
		}
	}
	if s := c.Stats(); s.Inserts != 3 || s.Hits != 0 {
		t.Fatalf("不同画幅应为不同键: %+v", s)
	}
}

// 重复渲染同一场景：首次全部未命中，第二次全部命中。
func TestCachedFillHotRerun(t *testing.T) {
	polys := [][]geom.Point{square(0, 0, 5), square(3, 3, 5), square(6, 6, 5)}
	c, _ := cache.New(8)
	im, _ := New(16, 16)
	if err := im.FillCached(polys, 2, c); err != nil {
		t.Fatal(err)
	}
	first := c.Stats()
	if first.Misses != 3 || first.Hits != 0 {
		t.Fatalf("首轮应 3 次未命中: %+v", first)
	}
	if err := im.FillCached(polys, 2, c); err != nil {
		t.Fatal(err)
	}
	second := c.Stats()
	if second.Misses != first.Misses || second.Hits != 3 {
		t.Fatalf("第二轮应 3 次命中: %+v", second)
	}
}

func TestFillCachedRejectsBadInput(t *testing.T) {
	im, _ := New(4, 4)
	c, _ := cache.New(2)
	if err := im.FillCached([][]geom.Point{square(0, 0, 1)}, 0, c); err == nil {
		t.Fatal("supersample=0 应报错")
	}
	if err := im.FillCached([][]geom.Point{{{X: 0, Y: 0}, {X: 1, Y: 1}}}, 2, c); err == nil {
		t.Fatal("少于三个顶点应报错")
	}
}

// supersample 超过 8 时每像素需要多个 uint64，掩码布局必须仍然正确。
func TestCachedFillHighSupersample(t *testing.T) {
	for _, ss := range []int{9, 16, 17} {
		base, cached, _ := runBoth(t, [][]geom.Point{square(0, 0, 9)}, 13, 11, ss)
		if !bytes.Equal(base.Pix, cached.Pix) {
			t.Fatalf("ss=%d 输出不一致", ss)
		}
	}
}
