package pipe

import (
	"testing"

	"vecgo/config"
	"vecgo/geom"
	"vecgo/raster"
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

// TestRunSharesOneFrame 证明整条管线只分配一块整图、不复制整图，
// 且五个处理阶段（光栅化、着色、gamma、模糊、命令行导出）取图用的
// 是同一份。
func TestRunSharesOneFrame(t *testing.T) {
	scene := [][]geom.Point{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}}
	cfg := config.Default()
	cfg.Width, cfg.Height = 32, 32
	cfg.Blur = 2
	cfg.Gamma = 2.2

	raster.ResetStats()
	before := raster.Snapshot()
	im, err := Run(scene, cfg)
	if err != nil {
		t.Fatal(err)
	}
	after := raster.Snapshot()

	if got := after.FrameAllocs - before.FrameAllocs; got != 1 {
		t.Fatalf("整图分配次数应为 1，实际 %d", got)
	}
	if got := after.FullCopies - before.FullCopies; got != 0 {
		t.Fatalf("整图拷贝次数应为 0，实际 %d", got)
	}
	// 光栅化、着色、gamma、模糊各取图一次。
	if got := after.StageLeases - before.StageLeases; got != 4 {
		t.Fatalf("管线阶段取图次数应为 4，实际 %d", got)
	}
	if im.W != cfg.Width || im.H != cfg.Height {
		t.Fatalf("返回图尺寸错误: %dx%d", im.W, im.H)
	}

	// 第五处：命令行导出也向同一块缓冲取图，而不是另拷一份。
	if err := im.SavePGM(t.TempDir() + "/out.pgm"); err != nil {
		t.Fatal(err)
	}
	afterExport := raster.Snapshot()
	if got := afterExport.StageLeases - before.StageLeases; got != 5 {
		t.Fatalf("含导出的取图次数应为 5，实际 %d", got)
	}
	if got := afterExport.FrameAllocs - before.FrameAllocs; got != 1 {
		t.Fatalf("导出后整图分配次数应仍为 1，实际 %d", got)
	}
	if got := afterExport.FullCopies - before.FullCopies; got != 0 {
		t.Fatalf("导出后整图拷贝次数应仍为 0，实际 %d", got)
	}
}

// TestRunRejectsBadConfig 保持不变。
func TestRunRejectsBadConfig(t *testing.T) {
	scene := [][]geom.Point{{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 1, Y: 1}}}
	cfg := config.Default()
	cfg.Gamma = 0
	if _, err := Run(scene, cfg); err == nil {
		t.Fatal("gamma=0 应报错")
	}
}
