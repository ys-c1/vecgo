package depth_test

import (
	"math"
	"strings"
	"testing"

	"vecgo/depth"
	"vecgo/raster"
)

// quad 造一个 0..16 的正方形图层，四角各带齐次深度 (z, w)。
func quad(z [4]float64, w float64, shade uint8, alpha float64) depth.Layer {
	return depth.Layer{
		Poly: []depth.Vertex{
			{X: 0, Y: 0, Z: z[0], W: w},
			{X: 16, Y: 0, Z: z[1], W: w},
			{X: 16, Y: 16, Z: z[2], W: w},
			{X: 0, Y: 16, Z: z[3], W: w},
		},
		Shade: shade,
		Alpha: alpha,
	}
}

func render(t *testing.T, layers []depth.Layer, ss int) *raster.Image {
	t.Helper()
	im, err := raster.New(16, 16)
	if err != nil {
		t.Fatal(err)
	}
	if err := depth.Render(im, layers, ss); err != nil {
		t.Fatal(err)
	}
	return im
}

// 前后遮挡：近处小方块盖住远处背景，与绘制顺序无关。
func TestOcclusion(t *testing.T) {
	back := quad([4]float64{5, 5.1, 5.1, 5}, 1, 100, 1)
	front := depth.Layer{
		Poly: []depth.Vertex{
			{X: 4, Y: 4, Z: 2, W: 1},
			{X: 12, Y: 4, Z: 2.05, W: 1},
			{X: 12, Y: 12, Z: 2.1, W: 1},
			{X: 4, Y: 12, Z: 2.05, W: 1},
		},
		Shade: 200,
		Alpha: 1,
	}
	im := render(t, []depth.Layer{back, front}, 1)
	if got := im.Pix[8*16+8]; got != 200 {
		t.Fatalf("中心像素应为近图层 200，得 %d", got)
	}
	if got := im.Pix[0]; got != 100 {
		t.Fatalf("角落像素应为远图层 100，得 %d", got)
	}
	// 交换绘制顺序，深度决定结果，图像必须不变。
	rev := render(t, []depth.Layer{front, back}, 1)
	n, err := depth.Diff(im, rev)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("绘制顺序不应影响深度消隐结果，差异像素 %d", n)
	}
}

// 交叉：两个斜面在深度上交叉，左半图层 0 在前，右半图层 1 在前。
func TestIntersection(t *testing.T) {
	a := quad([4]float64{1, 9, 9, 1}, 1, 100, 1) // 左近右远
	b := quad([4]float64{9, 1, 1, 9}, 1, 200, 1) // 左远右近
	im := render(t, []depth.Layer{a, b}, 1)
	if got := im.Pix[8*16+2]; got != 100 {
		t.Fatalf("左侧应为图层 0 (100)，得 %d", got)
	}
	if got := im.Pix[8*16+13]; got != 200 {
		t.Fatalf("右侧应为图层 1 (200)，得 %d", got)
	}
}

// 共面：两层同一斜面，深度处处相同，按图层号定序，小号在前。
func TestCoplanar(t *testing.T) {
	a := quad([4]float64{1, 9, 9, 1}, 1, 100, 1)
	b := quad([4]float64{1, 9, 9, 1}, 1, 200, 1)
	im := render(t, []depth.Layer{a, b}, 1)
	for i, v := range im.Pix {
		if v != 100 {
			t.Fatalf("像素 %d 应为图层号小的 100，得 %d", i, v)
		}
	}
	rev := render(t, []depth.Layer{b, a}, 1)
	for i, v := range rev.Pix {
		if v != 200 {
			t.Fatalf("交换后像素 %d 应为新的小号图层 200，得 %d", i, v)
		}
	}
}

// 全透明：Alpha=0 的图层不遮挡；半透明图层按 Shade*Alpha 贡献。
func TestTransparent(t *testing.T) {
	back := quad([4]float64{5, 5.1, 5.1, 5}, 1, 200, 1)
	ghost := quad([4]float64{2, 2.1, 2.1, 2}, 1, 100, 0)
	im := render(t, []depth.Layer{back, ghost}, 1)
	for i, v := range im.Pix {
		if v != 200 {
			t.Fatalf("全透明图层不应遮挡，像素 %d 应为 200，得 %d", i, v)
		}
	}
	half := quad([4]float64{2, 2.1, 2.1, 2}, 1, 100, 0.5)
	im2 := render(t, []depth.Layer{back, half}, 1)
	for i, v := range im2.Pix {
		if v != 50 {
			t.Fatalf("半透明图层像素 %d 应为 100*0.5=50，得 %d", i, v)
		}
	}
}

// 单层：单个斜面图层正常渲染。
func TestSingleLayer(t *testing.T) {
	only := quad([4]float64{3, 3.2, 3.2, 3}, 1, 255, 1)
	im := render(t, []depth.Layer{only}, 1)
	for i, v := range im.Pix {
		if v != 255 {
			t.Fatalf("像素 %d 应为 255，得 %d", i, v)
		}
	}
}

// 三类非法输入：NaN 深度、空图层、深度范围为零，都要报图层号。
func TestErrors(t *testing.T) {
	good := quad([4]float64{1, 2, 2, 1}, 1, 100, 1)
	nan := quad([4]float64{1, 2, 2, 1}, 1, 100, 1)
	nan.Poly[2].Z = math.NaN()
	flat := quad([4]float64{4, 4, 4, 4}, 1, 100, 1)
	empty := depth.Layer{Poly: nil, Shade: 1, Alpha: 1}

	cases := []struct {
		name   string
		layers []depth.Layer
		want   string
	}{
		{"NaN 深度", []depth.Layer{good, nan}, "图层 1 深度为 NaN"},
		{"空图层", []depth.Layer{empty, good}, "图层 0 为空"},
		{"深度范围为零", []depth.Layer{good, good, flat}, "图层 2 深度范围为零"},
	}
	for _, c := range cases {
		im, err := raster.New(4, 4)
		if err != nil {
			t.Fatal(err)
		}
		err = depth.Render(im, c.layers, 1)
		if err == nil {
			t.Fatalf("%s: 应报错", c.name)
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Fatalf("%s: 错误 %q 应包含 %q", c.name, err, c.want)
		}
	}
}

// composite 造一个含遮挡、交叉、全透明和 w≠1 图层的综合场景。
func composite() []depth.Layer {
	back := quad([4]float64{5, 5.4, 5.4, 5}, 1, 60, 1)
	crossA := quad([4]float64{1, 4, 4, 1}, 1, 120, 1)
	crossB := quad([4]float64{4, 1, 1, 4}, 1, 180, 1)
	wide := quad([4]float64{6, 6.4, 6.4, 6}, 2, 240, 1) // d = z/w = 3..3.2
	ghost := quad([4]float64{0.5, 0.6, 0.6, 0.5}, 1, 255, 0)
	return []depth.Layer{back, crossA, crossB, wide, ghost}
}

// 验证一：z-buffer 消隐结果与逐像素排序参照完全一致。
func TestMatchesReference(t *testing.T) {
	layers := composite()
	a := render(t, layers, 2)
	b, err := raster.New(16, 16)
	if err != nil {
		t.Fatal(err)
	}
	if err := depth.RenderReference(b, layers, 2); err != nil {
		t.Fatal(err)
	}
	n, err := depth.Diff(a, b)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("深度表:")
	for i, l := range layers {
		t.Logf("  图层 %d shade=%d alpha=%v d(z/w)=%v", i, l.Shade, l.Alpha, depth.Depths(l))
	}
	t.Logf("差异像素数: %d", n)
	if n != 0 {
		t.Fatalf("消隐结果与逐像素排序参照不一致，差异像素 %d", n)
	}
}

// 验证二：同一输入两次运行结果相同。
func TestDeterministic(t *testing.T) {
	layers := composite()
	a := render(t, layers, 2)
	b := render(t, layers, 2)
	n, err := depth.Diff(a, b)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("同一输入两次运行不同，差异像素 %d", n)
	}
}
