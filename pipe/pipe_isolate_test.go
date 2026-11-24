package pipe

import (
	"math"
	"reflect"
	"testing"

	"vecgo/config"
	"vecgo/geom"
)

func mixedScene() [][]geom.Point {
	return [][]geom.Point{
		{{X: 0, Y: 0}, {X: 4, Y: 0}, {X: 4, Y: 4}, {X: 0, Y: 4}},
		{{X: 6, Y: 6}, {X: 9, Y: 6}, {X: 9, Y: 9}, {X: 6, Y: 9}},
		{{X: 1, Y: 7}, {X: 3, Y: 7}, {X: 3, Y: 9}, {X: 1, Y: 9}},
	}
}

// 管线级：正常场景两条路径输出逐像素相同。
func TestRunIsolatedPixelIdenticalWhenHealthy(t *testing.T) {
	cfg := config.Default()
	cfg.Width, cfg.Height = 32, 32

	a, err := Run(mixedScene(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	b, skipped, err := RunIsolated(mixedScene(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(skipped) != 0 {
		t.Fatalf("正常场景不应跳过图形: %v", skipped)
	}
	if !reflect.DeepEqual(a.Pix, b.Pix) {
		t.Fatal("正常场景下隔离路径与传统路径输出不一致")
	}
}

// 管线级：异常场景隔离后照常出图，跳过清单升序且编号正确。
func TestRunIsolatedContainsFailure(t *testing.T) {
	scene := mixedScene()
	scene[1][0].X = math.Inf(-1)                                    // #1 坏坐标
	scene = append(scene, []geom.Point{{X: 0, Y: 0}, {X: 5, Y: 5}}) // #3 顶点不足

	cfg := config.Default()
	cfg.Width, cfg.Height = 32, 32

	im, skipped, err := RunIsolated(scene, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(skipped.IDs(), []int{1, 3}) {
		t.Fatalf("跳过清单应为 [1 3]，实际 %v", skipped.IDs())
	}
	if im.CountOn(1) == 0 {
		t.Fatal("隔离路径异常场景必须出图，不许黑帧")
	}

	// 传统路径对照：同一输入直接整体失败（顶点不足报错）。
	if _, err := Run(scene, cfg); err == nil {
		t.Fatal("传统路径对同一异常输入应整体报错")
	}
}

// 仅含 Inf 坏图形的场景：传统路径静默黑帧，隔离路径出图。
func TestRunInfBlackFrameVsIsolation(t *testing.T) {
	scene := mixedScene()
	scene[1][0].Y = math.Inf(1)

	cfg := config.Default()
	cfg.Width, cfg.Height = 32, 32

	black, err := Run(scene, cfg)
	if err != nil {
		t.Fatalf("Inf 输入传统路径不报错: %v", err)
	}
	if black.CountOn(1) != 0 {
		t.Fatal("传统路径应为静默黑帧")
	}

	im, skipped, err := RunIsolated(scene, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(skipped.IDs(), []int{1}) {
		t.Fatalf("应只跳过 #1，实际 %v", skipped.IDs())
	}
	if im.CountOn(1) == 0 {
		t.Fatal("隔离路径应保留其余图形，不能黑帧")
	}
}
