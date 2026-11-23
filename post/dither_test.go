package post

import (
	"math"
	"strings"
	"testing"

	"vecgo/raster"
)

// refDither 是逐像素手算的参考实现：与 Dither 同一算法，但刻意写得直白，
// 用于交叉核对正式实现的每一个像素。
func refDither(im *raster.Image, k Kernel, strength float64, depth int) []uint8 {
	sum := 0.0
	for _, o := range k.Offsets {
		sum += o[2]
	}
	norm := 1.0
	if k.Normalize && sum != 0 {
		norm = 1 / sum
	}
	maxLevel := (1 << uint(depth)) - 1
	levels := float64(maxLevel)
	buf := make([]float64, im.W*im.H)
	for i, v := range im.Pix {
		buf[i] = float64(v)
	}
	out := make([]uint8, im.W*im.H)
	for y := 0; y < im.H; y++ {
		for xi := 0; xi < im.W; xi++ {
			x := xi
			if y%2 == 1 {
				x = im.W - 1 - xi
			}
			vc := buf[y*im.W+x]
			if vc < 0 {
				vc = 0
			}
			if vc > 255 {
				vc = 255
			}
			q := math.Round(vc/255*levels) * 255 / levels
			out[y*im.W+x] = uint8(q)
			e := (vc - q) * strength
			for _, o := range k.Offsets {
				dx := int(o[0])
				if y%2 == 1 {
					dx = -dx
				}
				nx, ny := x+dx, y+int(o[1])
				if nx >= 0 && nx < im.W && ny >= 0 && ny < im.H {
					buf[ny*im.W+nx] += e * o[2] * norm
				}
			}
		}
	}
	return out
}

func gradientImage(w, h int) *raster.Image {
	im, _ := raster.New(w, h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			im.Pix[y*w+x] = uint8(math.Round(255 * float64(x) / float64(w-1)))
		}
	}
	return im
}

func solidImage(w, h int, v uint8) *raster.Image {
	im, _ := raster.New(w, h)
	for i := range im.Pix {
		im.Pix[i] = v
	}
	return im
}

func textureImage(w, h int) *raster.Image {
	im, _ := raster.New(w, h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if (x+y)%2 == 0 {
				im.Pix[y*w+x] = 255
			}
		}
	}
	return im
}

func assertMatchesReference(t *testing.T, im *raster.Image, k Kernel, strength float64, depth int) *raster.Image {
	t.Helper()
	out, err := Dither(im, k, strength, depth)
	if err != nil {
		t.Fatal(err)
	}
	ref := refDither(im, k, strength, depth)
	for i := range out.Pix {
		if out.Pix[i] != ref[i] {
			t.Fatalf("像素 %d 与手算参考不符: 得 %d, 参考 %d", i, out.Pix[i], ref[i])
		}
	}
	return out
}

// 用例 1：灰阶渐变。环带数必须不到现有实现（仅量化）的一半。
// 环带 = 面积 >= 一个完整量化色带（128 宽 4 位深下为 8 列 x 32 行 = 256 像素）的等值连通区域。
func TestDitherGradient(t *testing.T) {
	src := gradientImage(128, 32)
	k := FloydSteinberg()
	out := assertMatchesReference(t, src, k, 1.0, 4)
	quant, err := Quantize(src, 4)
	if err != nil {
		t.Fatal(err)
	}
	before := CountBands(quant, 256)
	after := CountBands(out, 256)
	if before == 0 {
		t.Fatal("仅量化的渐变应出现环带")
	}
	if after*2 >= before {
		t.Fatalf("环带数应少于现有实现的一半: 抖动后 %d, 仅量化 %d", after, before)
	}
}

// 用例 2：纯色块。均值应贴近原色，且只出现合法量化级。
func TestDitherSolidBlock(t *testing.T) {
	src := solidImage(32, 32, 128)
	k := FloydSteinberg()
	out := assertMatchesReference(t, src, k, 1.0, 4)
	sum := 0
	for _, v := range out.Pix {
		if v%17 != 0 { // 4 位深下合法级别均为 17 的倍数
			t.Fatalf("出现非法量化级: %d", v)
		}
		sum += int(v)
	}
	mean := float64(sum) / float64(len(out.Pix))
	if math.Abs(mean-128) > 9 { // 半个量化级(8.5)以内
		t.Fatalf("纯色块均值漂移过大: %v", mean)
	}
}

// 用例 3：细密纹理（棋盘格）。不得发散，且与手算参考一致。
func TestDitherFineTexture(t *testing.T) {
	src := textureImage(31, 17)
	k := FloydSteinberg()
	out := assertMatchesReference(t, src, k, 1.0, 4)
	on := out.CountOn(128)
	if on == 0 || on == len(out.Pix) {
		t.Fatalf("纹理不应被抹成全黑或全白: on=%d", on)
	}
}

// 用例 4：单像素。
func TestDitherSinglePixel(t *testing.T) {
	im, _ := raster.New(1, 1)
	im.Pix[0] = 200
	out, err := Dither(im, FloydSteinberg(), 1.0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if out.Pix[0] != 255 {
		t.Fatalf("200 在 1 位深下应量化为 255, 得 %d", out.Pix[0])
	}
	im.Pix[0] = 100
	out, err = Dither(im, FloydSteinberg(), 1.0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if out.Pix[0] != 0 {
		t.Fatalf("100 在 1 位深下应量化为 0, 得 %d", out.Pix[0])
	}
}

// 用例 5：全黑。输出必须仍为全黑。
func TestDitherAllBlack(t *testing.T) {
	src := solidImage(16, 16, 0)
	out := assertMatchesReference(t, src, FloydSteinberg(), 1.0, 4)
	for i, v := range out.Pix {
		if v != 0 {
			t.Fatalf("全黑输入像素 %d 应为 0, 得 %d", i, v)
		}
	}
}

// 逐像素手算：4x1 全 100，1 位深，Floyd–Steinberg，strength=1。
// x0: q=0, err=100, x1 += 43.75 -> 143.75
// x1: q=255, err=-111.25, x2 -= 48.671875 -> 51.328125
// x2: q=0, err=51.328125, x3 += 22.455... -> 122.455...
// x3: q=0
func TestDitherHandComputedRow(t *testing.T) {
	src := solidImage(4, 1, 100)
	out, err := Dither(src, FloydSteinberg(), 1.0, 1)
	if err != nil {
		t.Fatal(err)
	}
	want := []uint8{0, 255, 0, 0}
	for i, v := range want {
		if out.Pix[i] != v {
			t.Fatalf("像素 %d: 得 %d, 手算 %d", i, out.Pix[i], v)
		}
	}
}

// 蛇形推进：反向行应镜像 dx。2 行输入，第二行从右往左扩散。
func TestDitherSerpentine(t *testing.T) {
	im, _ := raster.New(2, 2)
	for i := range im.Pix {
		im.Pix[i] = 100
	}
	k := Kernel{
		Name:      "right-only",
		Offsets:   [][3]float64{{1, 0, 1}},
		Normalize: true,
	}
	out := assertMatchesReference(t, im, k, 1.0, 1)
	// 行 0 从左到右: [0,255]；行 1 从右到左，误差向左扩散: [255,0]
	want := []uint8{0, 255, 255, 0}
	for i, v := range want {
		if out.Pix[i] != v {
			t.Fatalf("像素 %d: 得 %d, 手算 %d", i, out.Pix[i], v)
		}
	}
}

func TestDitherBadKernel(t *testing.T) {
	im, _ := raster.New(2, 2)
	cases := []Kernel{
		{Name: "empty", Offsets: nil, Normalize: true},
		{Name: "backward", Offsets: [][3]float64{{-1, 0, 1}}, Normalize: true},
		{Name: "zero-sum", Offsets: [][3]float64{{1, 0, 1}, {0, 1, -1}}, Normalize: true},
	}
	for _, k := range cases {
		err := error(nil)
		if _, err = Dither(im, k, 1.0, 4); err == nil {
			t.Fatalf("核 %s 应报错", k.Name)
		}
		if !strings.Contains(err.Error(), "kernel") {
			t.Fatalf("错误信息应带参数名 kernel: %v", err)
		}
	}
}

func TestDitherNegativeStrength(t *testing.T) {
	im, _ := raster.New(2, 2)
	if _, err := Dither(im, FloydSteinberg(), -0.5, 4); err == nil {
		t.Fatal("strength<0 应报错")
	} else if !strings.Contains(err.Error(), "strength") {
		t.Fatalf("错误信息应带参数名 strength: %v", err)
	}
}

func TestDitherBadDepth(t *testing.T) {
	im, _ := raster.New(2, 2)
	for _, d := range []int{0, 9} {
		if _, err := Dither(im, FloydSteinberg(), 1.0, d); err == nil {
			t.Fatalf("bitDepth=%d 应报错", d)
		} else if !strings.Contains(err.Error(), "bitDepth") {
			t.Fatalf("错误信息应带参数名 bitDepth: %v", err)
		}
	}
}

// 未归一化核（权重和 16 不归一）应明显劣于归一化核。
func TestDitherUnnormalizedIsWorse(t *testing.T) {
	src := gradientImage(64, 16)
	norm := FloydSteinberg()
	raw := FloydSteinberg()
	raw.Normalize = false
	good, err := Dither(src, norm, 1.0, 4)
	if err != nil {
		t.Fatal(err)
	}
	bad, err := Dither(src, raw, 1.0, 4)
	if err != nil {
		t.Fatal(err)
	}
	eGood, _ := MeanAbsError(good, src)
	eBad, _ := MeanAbsError(bad, src)
	if eBad <= eGood {
		t.Fatalf("未归一化核应更差: 归一化 %.3f, 未归一化 %.3f", eGood, eBad)
	}
}
