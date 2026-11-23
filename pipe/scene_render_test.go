package pipe

import (
	"path/filepath"
	"testing"

	"vecgo/scene"
)

// 场景文件解析结果能驱动完整渲染。
func TestRunFromSceneFile(t *testing.T) {
	for _, name := range []string{"single.scene", "multi.scene", "nested.scene"} {
		res := scene.Parse(filepath.Join("..", "scene", "testdata", name))
		if res.Err != nil {
			t.Fatalf("%s: %v", name, res.Err)
		}
		polys, cfg, err := res.Map()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		im, err := Run(polys, cfg)
		if err != nil {
			t.Fatalf("%s: 渲染失败: %v", name, err)
		}
		if im.W != cfg.Width || im.H != cfg.Height {
			t.Fatalf("%s: 画幅异常 %dx%d", name, im.W, im.H)
		}
		if im.CountOn(1) == 0 {
			t.Fatalf("%s: 应有像素被点亮", name)
		}
	}
}

// 坏文件的部分结果也能渲染, 且只含回收到的图形。
func TestRunPartialFromBadScene(t *testing.T) {
	res := scene.Parse(filepath.Join("..", "scene", "testdata", "bad.scene"))
	if res.Err == nil {
		t.Fatal("坏文件应报错")
	}
	if len(res.Scroll) != 1 {
		t.Fatalf("应回收 1 个图形, got %d", len(res.Scroll))
	}
	im, err := Run(res.Scroll, res.Config)
	if err != nil {
		t.Fatalf("部分结果应可渲染: %v", err)
	}
	if im.CountOn(1) == 0 {
		t.Fatal("部分结果渲染应有像素")
	}
}
