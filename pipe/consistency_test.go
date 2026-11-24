package pipe

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"vecgo/color"
	"vecgo/config"
	"vecgo/geom"
	"vecgo/post"
	"vecgo/raster"
	"vecgo/shading"
)

func scene() [][]geom.Point {
	return [][]geom.Point{{
		{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 4}, {X: 6, Y: 4},
		{X: 6, Y: 10}, {X: 0, Y: 10},
	}}
}

func baseCfg(mode string, gamma float64) config.Config {
	c := config.Default()
	c.Width, c.Height = 256, 256
	c.Supersample = 2
	c.Shading = mode
	c.Gamma = gamma
	c.Blur = 0
	return c
}

// buildLinearGradient 只经过 raster+shading，得到一张线性光渐变缓冲。
func buildLinearGradient(t *testing.T, cfg config.Config) *raster.Image {
	t.Helper()
	im, err := raster.New(cfg.Width, cfg.Height)
	if err != nil {
		t.Fatal(err)
	}
	moved := make([][]geom.Point, len(scene()))
	for i, poly := range scene() {
		moved[i] = Transform(poly, cfg.Scale, cfg.Rotate)
	}
	if err := im.Fill(moved, cfg.Supersample); err != nil {
		t.Fatal(err)
	}
	if err := shading.Apply(im, cfg.Shading, 1.0, 0.0); err != nil {
		t.Fatal(err)
	}
	return im
}

// TestThreePathsPixelIdentical 验证同一张渐变图在三条路径上的口径一致：
// 路径 A（post）：线性缓冲经 post.Gamma 编码；
// 路径 B（cmd/PGM 写出）：同一张线性缓冲经 SaveLinearPGM 编码落盘；
// 路径 C（cmd/PGM 读回）：写出的 PGM 经 LoadLinearPGM 解码回线性，
//
//	再用同一张 color 编码表编码。
//
// A 与 B 调用的是同一份 color.EncodeLUT，必须逐像素相同；
// C 多经过一次字节级解码，截断量化是多对一映射，因此与 A/B 的
// 差异被严格限制在一个量化级（|Δ|<=1），gamma=1 时为 0。
func TestThreePathsPixelIdentical(t *testing.T) {
	for _, gamma := range []float64{0.5, 1, 2.2} {
		cfg := baseCfg("gradient", gamma)
		linear := buildLinearGradient(t, cfg)

		// 路径 A：post.Gamma
		a, _ := raster.New(linear.W, linear.H)
		copy(a.Pix, linear.Pix)
		if err := post.Gamma(a, gamma); err != nil {
			t.Fatal(err)
		}

		// 路径 B：SaveLinearPGM 直接写编码字节
		dir := t.TempDir()
		pgmPath := filepath.Join(dir, "grad.pgm")
		if err := linear.SaveLinearPGM(pgmPath, gamma); err != nil {
			t.Fatal(err)
		}
		pgmBytes, err := os.ReadFile(pgmPath)
		if err != nil {
			t.Fatal(err)
		}
		headerLen := len(pgmBytes) - linear.W*linear.H
		b := pgmBytes[headerLen:]

		if len(b) != len(a.Pix) {
			t.Fatalf("g=%v 平面长度不一致", gamma)
		}
		// A 与 B：同一张编码表，逐像素严格一致。
		for i := range a.Pix {
			if a.Pix[i] != b[i] {
				t.Fatalf("g=%v post 与 PGM 写出在像素 %d 分叉: %d != %d",
					gamma, i, a.Pix[i], b[i])
			}
		}

		// 路径 C：PGM 读回线性缓冲，再经同一张 color 表编码。
		back, err := raster.LoadLinearPGM(pgmPath, gamma)
		if err != nil {
			t.Fatal(err)
		}
		table, err := color.EncodeLUT(gamma)
		if err != nil {
			t.Fatal(err)
		}
		if back.W != linear.W || back.H != linear.H || len(back.Pix) != len(linear.Pix) {
			t.Fatalf("g=%v 读回尺寸不一致", gamma)
		}
		for i := range back.Pix {
			d := int(table[back.Pix[i]]) - int(a.Pix[i])
			if d < 0 {
				d = -d
			}
			if gamma == 1 && d != 0 {
				t.Fatalf("g=1 读回路径必须恒等，像素 %d 偏差 %d", i, d)
			}
			if d > 1 {
				t.Fatalf("g=%v PGM 读回再编码在像素 %d 超出一个量化级: %d vs %d",
					gamma, i, table[back.Pix[i]], a.Pix[i])
			}
		}
	}
}

// TestGammaOneOutputsUnchanged 用基线二进制产物做金标准：
// gamma=1 时整条管线输出必须走回恒等路径。
//   - flat 模式的金标准直接取自统一前的基线构建，要求逐字节不变；
//   - gradient 模式的基线本身不满足恒等性——旧 post 包里手写的 pow 是
//     一阶近似（1+(a-1)*e），gamma=1 时也会把中间调算偏，因此它的金标准
//     取的是 math.Pow 恒等量化（uint8(255*x)）的正确结果，
//     与 shading 阶段的直接量化逐字节相同。
func TestGammaOneOutputsUnchanged(t *testing.T) {
	for _, mode := range []string{"gradient", "flat"} {
		cfg := baseCfg(mode, 1)
		im, err := Run(scene(), cfg)
		if err != nil {
			t.Fatal(err)
		}
		goldenRaw, err := os.ReadFile(filepath.Join("testdata", "golden_"+mode+"_g1.raw"))
		if err != nil {
			t.Fatal(err)
		}
		if len(im.Pix) != len(goldenRaw) {
			t.Fatalf("%s 长度不一致", mode)
		}
		for i := range goldenRaw {
			if im.Pix[i] != goldenRaw[i] {
				t.Fatalf("%s gamma=1 像素 %d 被改变: %d != %d",
					mode, i, im.Pix[i], goldenRaw[i])
			}
		}

		dir := t.TempDir()
		pgmPath := filepath.Join(dir, "out.pgm")
		if err := im.SavePGM(pgmPath); err != nil {
			t.Fatal(err)
		}
		goldenPGM, err := os.ReadFile(filepath.Join("testdata", "golden_"+mode+"_g1.pgm"))
		if err != nil {
			t.Fatal(err)
		}
		gotPGM, err := os.ReadFile(pgmPath)
		if err != nil {
			t.Fatal(err)
		}
		if string(gotPGM) != string(goldenPGM) {
			t.Fatalf("%s gamma=1 PGM 字节流被改变", mode)
		}
	}
}

// TestValidationErrorsCarryPackageAndField 校验失败的错误必须带包名与字段名。
func TestValidationErrorsCarryPackageAndField(t *testing.T) {
	cfg := baseCfg("gradient", 2.2)
	cfg.Gamma = 0
	_, err := Run(scene(), cfg)
	if err == nil || !strings.Contains(err.Error(), "config:") ||
		!strings.Contains(err.Error(), "gamma") {
		t.Fatalf("config 错误缺少包名/字段名: %v", err)
	}

	im, _ := raster.New(4, 4)
	if err := post.Gamma(im, 99); err == nil ||
		!strings.Contains(err.Error(), "post:") || !strings.Contains(err.Error(), "gamma") {
		t.Fatalf("post 错误缺少包名/字段名: %v", err)
	}
	if err := shading.Apply(im, "bogus", 1, 0); err == nil ||
		!strings.Contains(err.Error(), "shading:") || !strings.Contains(err.Error(), "mode") {
		t.Fatalf("shading 错误缺少包名/字段名: %v", err)
	}
	if err := shading.Apply(im, "flat", 2, 0); err == nil ||
		!strings.Contains(err.Error(), "shading:") || !strings.Contains(err.Error(), "top") {
		t.Fatalf("shading 强度错误缺少包名/字段名: %v", err)
	}
	if _, err := raster.LoadLinearPGM("nonexistent.pgm", 1); err == nil {
		t.Fatal("不存在的 PGM 应报错")
	}
	bad := filepath.Join(t.TempDir(), "bad.pgm")
	if err := os.WriteFile(bad, []byte("P2\n1 1\n255\n0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := raster.LoadLinearPGM(bad, 1); err == nil ||
		!strings.Contains(err.Error(), "raster:") {
		t.Fatalf("非法 PGM 错误缺少包名: %v", err)
	}
}
