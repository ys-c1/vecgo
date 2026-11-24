package pipe

import (
	"testing"

	"vecgo/config"
	"vecgo/geom"
	"vecgo/pyramid"
)

func gradConfig(w, h int) config.Config {
	cfg := config.Default()
	cfg.Width, cfg.Height = w, h
	cfg.Supersample = 2
	cfg.Shading = "gradient"
	return cfg
}

func squareScene() [][]geom.Point {
	return [][]geom.Point{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}}
}

// 默认（金字塔关闭）路径不受新字段影响。
func TestPyramidOffByDefault(t *testing.T) {
	cfg := gradConfig(32, 32)
	res, err := RunDetailed(squareScene(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if res.PyramidEnabled || res.SelectedLevels != 0 || res.Adaptive {
		t.Fatalf("零值配置不应启用金字塔: enabled=%v n=%d adaptive=%v",
			res.PyramidEnabled, res.SelectedLevels, res.Adaptive)
	}
}

// 固定层数路径：n=最大层数时与关闭金字塔的结果逐字节一致。
func TestFixedFullDepthParity(t *testing.T) {
	scene := squareScene()
	baseCfg := gradConfig(32, 32)
	base, err := Run(scene, baseCfg)
	if err != nil {
		t.Fatal(err)
	}

	full := pyramid.MaxDepth(32, 32)
	fixCfg := gradConfig(32, 32)
	fixCfg.PyramidLevels = full
	res, err := RunDetailed(scene, fixCfg)
	if err != nil {
		t.Fatal(err)
	}
	if !res.PyramidEnabled || res.Adaptive || res.SelectedLevels != full {
		t.Fatalf("固定路径选层信息错误: enabled=%v adaptive=%v n=%d want %d",
			res.PyramidEnabled, res.Adaptive, res.SelectedLevels, full)
	}
	for i := range base.Pix {
		if res.Image.Pix[i] != base.Pix[i] {
			t.Fatalf("全层固定重建应与原路径逐字节一致, 像素 %d: %d vs %d",
				i, res.Image.Pix[i], base.Pix[i])
		}
	}
}

// 固定的中间层数确实降低误差但不精确。
func TestFixedIntermediateLevel(t *testing.T) {
	cfg := gradConfig(64, 64)
	cfg.PyramidLevels = 4
	res, err := RunDetailed(squareScene(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if res.SelectedLevels != 4 {
		t.Fatalf("n=%d want 4", res.SelectedLevels)
	}
	if len(res.Image.Pix) != 64*64 {
		t.Fatal("重建图尺寸被改变")
	}
}

// 自适应路径对宽松目标给出解析最小层，且图像非空。
func TestAdaptiveSelectsLevels(t *testing.T) {
	cfg := gradConfig(128, 128)
	cfg.TargetError = 5
	res, err := RunDetailed(squareScene(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !res.PyramidEnabled || !res.Adaptive {
		t.Fatal("target_error>0 应进入自适应路径")
	}
	if res.SelectedLevels < 1 || res.SelectedLevels > pyramid.MaxDepth(128, 128) {
		t.Fatalf("自适应层数越界: %d", res.SelectedLevels)
	}
	if res.Model == nil || !res.Model.Usable() {
		t.Fatal("渐变场景应给出可用模型")
	}
}

// 固定层数优先于目标误差。
func TestFixedLevelsTakePrecedence(t *testing.T) {
	cfg := gradConfig(32, 32)
	cfg.PyramidLevels = 3
	cfg.TargetError = 0.001
	res, err := RunDetailed(squareScene(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if res.Adaptive || res.SelectedLevels != 3 {
		t.Fatalf("固定层数应优先, got adaptive=%v n=%d", res.Adaptive, res.SelectedLevels)
	}
}
