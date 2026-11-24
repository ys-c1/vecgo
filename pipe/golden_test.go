package pipe

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"unsafe"

	"vecgo/config"
	"vecgo/geom"
	"vecgo/post"
	"vecgo/raster"
	"vecgo/shading"
)

// TestGoldenAllFiveStages 直接把改动前的基线实现生成的五份输出当作金样，
// 逐字节比对五个阶段（光栅化、着色、gamma、模糊、命令行导出）的结果，
// 保证共享缓冲与原地模糊没有改变任何一个像素。
func TestGoldenAllFiveStages(t *testing.T) {
	cfg := config.Default()
	cfg.Width, cfg.Height = 48, 40
	cfg.Supersample = 2
	cfg.Shading = "gradient"
	cfg.Gamma = 2.2
	cfg.Blur = 2
	cfg.Rotate = 0.3
	cfg.Scale = 1

	scene := [][]geom.Point{{
		{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 4}, {X: 6, Y: 4},
		{X: 6, Y: 10}, {X: 0, Y: 10},
	}}

	im, err := raster.New(cfg.Width, cfg.Height)
	if err != nil {
		t.Fatal(err)
	}
	moved := make([][]geom.Point, len(scene))
	for i, poly := range scene {
		moved[i] = Transform(poly, cfg.Scale, cfg.Rotate)
	}

	check := func(name string, got []byte) {
		want, err := os.ReadFile(filepath.Join("testdata", "golden", name))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("%s 与基线金样不一致", name)
		}
	}

	if err := im.Fill(moved, cfg.Supersample); err != nil {
		t.Fatal(err)
	}
	check("01_raster.raw", im.Pix)

	if err := shading.Apply(im, cfg.Shading, 1.0, 0.0); err != nil {
		t.Fatal(err)
	}
	check("02_shading.raw", im.Pix)

	if err := post.Gamma(im, cfg.Gamma); err != nil {
		t.Fatal(err)
	}
	check("03_post_gamma.raw", im.Pix)

	out, err := post.Blur(im, cfg.Blur)
	if err != nil {
		t.Fatal(err)
	}
	check("04_post_blur.raw", out.Pix)

	path := filepath.Join(t.TempDir(), "out.pgm")
	if err := out.SavePGM(path); err != nil {
		t.Fatal(err)
	}
	pgm, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	check("05_cmd_output.raw", pgm)
}

// TestAllStagesLeaseSameBuffer 用钩子记录每次取图，确认五个阶段取的
// 是同一块底层缓冲（首元素地址相同），且顺序就是管线顺序。
func TestAllStagesLeaseSameBuffer(t *testing.T) {
	scene := [][]geom.Point{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}}
	cfg := config.Default()
	cfg.Width, cfg.Height = 16, 16
	cfg.Blur = 1

	var stages []string
	var firsts []uintptr
	raster.Hook = func(stage string, first *uint8) {
		stages = append(stages, stage)
		firsts = append(firsts, uintptr(unsafe.Pointer(first)))
	}
	defer func() { raster.Hook = nil }()

	im, err := Run(scene, cfg)
	if err != nil {
		t.Fatal(err)
	}
	// 导出阶段也取同一块。
	if err := im.SavePGM(filepath.Join(t.TempDir(), "o.pgm")); err != nil {
		t.Fatal(err)
	}

	want := []string{"raster", "shading", "post.gamma", "post.blur", "export"}
	if len(stages) != len(want) {
		t.Fatalf("阶段序列长度 %d，期望 %d: %v", len(stages), len(want), stages)
	}
	for i := range want {
		if stages[i] != want[i] {
			t.Fatalf("阶段 %d = %q，期望 %q", i, stages[i], want[i])
		}
		if firsts[i] != firsts[0] {
			t.Fatalf("阶段 %q 用的不是共享缓冲（首像素地址不同）", stages[i])
		}
	}
}

func TestImageInputsUntouched(t *testing.T) {
	scene := [][]geom.Point{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}}
	orig := append([]geom.Point(nil), scene[0]...)
	cfg := config.Default()
	cfg.Width, cfg.Height = 8, 8
	cfg.Blur = 1
	if _, err := Run(scene, cfg); err != nil {
		t.Fatal(err)
	}
	for i := range orig {
		if scene[0][i] != orig[i] {
			t.Fatalf("输入几何被改动: %v", scene[0])
		}
	}
}
