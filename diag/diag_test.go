package diag

import (
	"bytes"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"

	"vecgo/config"
	"vecgo/geom"
)

func scene() [][]geom.Point {
	// 三角形只覆盖画面一部分，用来区分“几何覆盖区域”和“整幅画面”。
	return [][]geom.Point{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 0, Y: 10}}}
}

func TestAnalyzeScopes(t *testing.T) {
	c := config.Default()
	c.Width, c.Height = 16, 16
	c.Scale = math.NaN()
	c.Gamma = math.NaN()
	c.Rotate = math.Inf(1)
	fs := Analyze(c)
	if len(fs) != 3 {
		t.Fatalf("应有 3 条拒绝记录: %+v", fs)
	}
	want := map[string]Scope{"scale": Shape, "rotate": Shape, "gamma": Frame}
	for _, f := range fs {
		if want[f.Param] != f.Scope {
			t.Fatalf("%s 影响范围应为 %v，实际 %v", f.Param, want[f.Param], f.Scope)
		}
	}
}

func TestRenderMarksGeometryOnly(t *testing.T) {
	c := config.Default()
	c.Width, c.Height = 16, 16
	c.Scale = math.NaN()
	im, fs, err := Render(scene(), c)
	if err != nil {
		t.Fatal(err)
	}
	if len(fs) != 1 || fs[0].Param != "scale" {
		t.Fatalf("记录异常: %+v", fs)
	}
	// 参考图（scale 换成默认值）里三角形约占一半像素，标记数应与之相当，
	// 而不是整幅 256。
	marked := im.CountMarked()
	if marked == 0 || marked >= im.W*im.H {
		t.Fatalf("标记像素异常: %d / %d", marked, im.W*im.H)
	}
	// 标记必须落在原图有内容的像素上：中心 (4,4) 与左上角 (0,0) 在
	// 三角形内，应染红；右下角 (15,15) 与 (12,12) 在斜边外，不应染红。
	if !im.Marked[4*im.W+4] || !im.Marked[0] {
		t.Fatal("三角形内像素应被标记")
	}
	if im.Marked[15*im.W+15] || im.Marked[12*im.W+12] {
		t.Fatal("覆盖区域外不应被标记")
	}
}

func TestRenderMarksFrame(t *testing.T) {
	c := config.Default()
	c.Width, c.Height = 16, 16
	c.Gamma = math.NaN()
	im, _, err := Render(scene(), c)
	if err != nil {
		t.Fatal(err)
	}
	// 边框至少覆盖上下两整行。
	if marked := im.CountMarked(); marked < 2*im.W {
		t.Fatalf("整幅标记应有边框，实际仅 %d 像素", marked)
	}
	if im.Pix[0] != 255 || im.Pix[3*(im.W-1)] != 255 {
		t.Fatal("边框首行应被标记")
	}
	// 内部不应被染色：整幅范围只画边框，不把几何区域染红。
	if im.Marked[8*im.W+8] {
		t.Fatal("整幅范围不应把内部像素染红")
	}
}

func TestRenderCleanConfigHasNoMarks(t *testing.T) {
	c := config.Default()
	c.Width, c.Height = 16, 16
	im, fs, err := Render(scene(), c)
	if err != nil {
		t.Fatal(err)
	}
	if len(fs) != 0 {
		t.Fatalf("合法配置不应有记录: %+v", fs)
	}
	if n := im.CountMarked(); n != 0 {
		t.Fatalf("合法配置不应有标记，实际 %d", n)
	}
}

func TestSafeAlwaysValid(t *testing.T) {
	bad := config.Config{
		Width: -1, Height: 0, Scale: math.NaN(), Rotate: math.Inf(1),
		Supersample: 0, Shading: "bogus", Gamma: math.NaN(), Blur: -1,
	}
	s := Safe(bad)
	if err := s.Validate(); err != nil {
		t.Fatalf("Safe 结果必须通过校验: %v", err)
	}
	if s.Scale != 1 || s.Rotate != 0 || s.Supersample != 2 || s.Shading != "flat" || s.Gamma != 1 || s.Blur != 0 {
		t.Fatalf("Safe 应回落到默认值，实际 %+v", s)
	}
	// 合法参数必须原样保留，参考图才反映用户本意。
	good := config.Default()
	good.Scale, good.Rotate, good.Gamma, good.Blur = 2.5, 1.2, 3, 4
	s = Safe(good)
	if s.Scale != 2.5 || s.Rotate != 1.2 || s.Gamma != 3 || s.Blur != 4 {
		t.Fatalf("合法参数被改动: %+v", s)
	}
}

func TestSavePNG(t *testing.T) {
	c := config.Default()
	c.Width, c.Height = 4, 3
	c.Scale = math.NaN()
	im, _, err := Render(scene(), c)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "d.png")
	if err := im.SavePNG(path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("PNG 解码失败: %v", err)
	}
	if img.Bounds().Dx() != 4 || img.Bounds().Dy() != 3 {
		t.Fatalf("PNG 尺寸错误: %v", img.Bounds())
	}
	r, _, _, _ := img.At(0, 0).RGBA()
	if uint8(r>>8) != 255 {
		t.Fatalf("左上角应为红色，实际 R=%d", r>>8)
	}
	// 拒绝原因必须随图走：单独转发这张 PNG 时说明不能丢。
	if !bytes.Contains(data, []byte("vecgo-diag")) || !bytes.Contains(data, []byte("scale=NaN")) {
		t.Fatal("PNG 缺少 vecgo-diag 文本块")
	}
}

func TestReport(t *testing.T) {
	c := config.Default()
	c.Scale = math.NaN()
	var b bytes.Buffer
	Report(&b, Analyze(c))
	out := b.String()
	if !bytes.Contains(b.Bytes(), []byte("scale=NaN")) ||
		!bytes.Contains(b.Bytes(), []byte("几何覆盖区域")) {
		t.Fatalf("报告内容缺失: %s", out)
	}
}
