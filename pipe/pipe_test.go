package pipe

import (
	"runtime"
	"testing"

	"vecgo/config"
	"vecgo/geom"
	"vecgo/post"
	"vecgo/raster"
	"vecgo/shading"
)

func TestRunProducesPixels(t *testing.T) {
	scene := [][]geom.Point{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}}
	cfg := config.Default()
	cfg.Width, cfg.Height = 16, 16
	im, err := Run(scene, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if im.CountOn(1) == 0 {
		t.Fatal("应有像素被点亮")
	}
}

func TestRunRejectsBadConfig(t *testing.T) {
	scene := [][]geom.Point{{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 1, Y: 1}}}
	cfg := config.Default()
	cfg.Gamma = 0
	if _, err := Run(scene, cfg); err == nil {
		t.Fatal("gamma=0 应报错")
	}
}

// TestTransformIntoMatchesTransform 固定“只有一份变换实现”的契约：
// Transform 与 TransformInto 对同一输入给出逐点相同的坐标。
func TestTransformIntoMatchesTransform(t *testing.T) {
	pts := []geom.Point{{X: 1, Y: 2}, {X: -3, Y: 4.5}, {X: 0, Y: 0}, {X: 7, Y: -2}}
	want := Transform(pts, 1.7, 0.63)
	got := make([]geom.Point, len(pts))
	TransformInto(got, pts, 1.7, 0.63)
	for i := range pts {
		if got[i] != want[i] {
			t.Fatalf("第 %d 点不一致: %v vs %v", i, got[i], want[i])
		}
	}
}

// TestRunAllocatesOneFrame 固定内存契约：一次 Run 只应分配一张整帧像素缓冲。
// 用“总分配字节 < 2 帧”来判定——改造前是两张整帧（严格等于 2 帧及以上）。
func TestRunAllocatesOneFrame(t *testing.T) {
	const n = 96
	scene := [][]geom.Point{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}}
	cfg := config.Default()
	cfg.Width, cfg.Height = n, n
	cfg.Blur = 2
	cfg.Supersample = 1
	// 先热一次，排除只发生一次的运行时初始化分配。
	if _, err := Run(scene, cfg); err != nil {
		t.Fatal(err)
	}
	runtime.GC()
	var a, b runtime.MemStats
	runtime.ReadMemStats(&a)
	if _, err := Run(scene, cfg); err != nil {
		t.Fatal(err)
	}
	runtime.ReadMemStats(&b)
	frame := uint64(n * n)
	got := b.TotalAlloc - a.TotalAlloc
	if got >= 2*frame {
		t.Fatalf("一次 Run 分配了 %d 字节，达到或超过两帧 (%d)；应只分配一张整帧", got, 2*frame)
	}
}

// TestRunLeavesOnlyWorkingImage 固定“中间图不再另留”的契约：
// Run 返回的像素就是逐阶段就地推进后的结果，模糊没有返回第二张。
func TestRunLeavesOnlyWorkingImage(t *testing.T) {
	scene := [][]geom.Point{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}}
	cfg := config.Default()
	cfg.Width, cfg.Height = 24, 24
	cfg.Blur = 2
	if _, err := Run(scene, cfg); err != nil {
		t.Fatal(err)
	}
	// 与分阶段（含 Blur 返回新图）路径对齐，验证就地结果完全相同。
	im, _ := raster.New(cfg.Width, cfg.Height)
	moved := make([][]geom.Point, len(scene))
	for i, p := range scene {
		moved[i] = Transform(p, cfg.Scale, cfg.Rotate)
	}
	if err := im.Fill(moved, cfg.Supersample); err != nil {
		t.Fatal(err)
	}
	if err := shading.Apply(im, cfg.Shading, 1.0, 0.0); err != nil {
		t.Fatal(err)
	}
	if err := post.Gamma(im, cfg.Gamma); err != nil {
		t.Fatal(err)
	}
	out, err := post.Blur(im, cfg.Blur)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Run(scene, cfg)
	if err != nil {
		t.Fatal(err)
	}
	for i := range out.Pix {
		if got.Pix[i] != out.Pix[i] {
			t.Fatalf("Run 与分阶段结果不一致: 第 %d 像素 %d vs %d", i, got.Pix[i], out.Pix[i])
		}
	}
}
