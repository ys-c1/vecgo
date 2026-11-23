package pipe

import (
	"strings"
	"testing"
	"time"

	"vecgo/config"
	"vecgo/geom"
	"vecgo/layer"
	"vecgo/raster"
)

// buildScene 生成 n 层场景：第 i 层是 [2i,2i] 到 [2i+8,2i+8] 的正方形，
// 层层交叠，每层都有独占区域，关掉任何一层都会改变画面。
func buildScene(n int) [][]geom.Point {
	scene := make([][]geom.Point, n)
	for i := 0; i < n; i++ {
		o := float64(2 * i)
		scene[i] = []geom.Point{
			{X: o, Y: o}, {X: o + 8, Y: o}, {X: o + 8, Y: o + 8}, {X: o, Y: o + 8},
		}
	}
	return scene
}

// hideInScene 手动隐藏图层：把该层多边形替换成张成原包围盒的退化
// 四边形 [A,B,A,B]（A、B 是包围盒对角）。它的覆盖为零（每条边成对
// 出现，奇偶抵消），但包围盒不变——这正是“在工具里隐藏图层”的语义。
func hideInScene(scene [][]geom.Point, hidden ...int) [][]geom.Point {
	out := make([][]geom.Point, len(scene))
	for i, poly := range scene {
		out[i] = poly
	}
	for _, h := range hidden {
		minx, miny := scene[h][0].X, scene[h][0].Y
		maxx, maxy := minx, miny
		for _, p := range scene[h] {
			if p.X < minx {
				minx = p.X
			}
			if p.Y < miny {
				miny = p.Y
			}
			if p.X > maxx {
				maxx = p.X
			}
			if p.Y > maxy {
				maxy = p.Y
			}
		}
		a := geom.Point{X: minx, Y: miny}
		b := geom.Point{X: maxx, Y: maxy}
		out[h] = []geom.Point{a, b, a, b}
	}
	return out
}

func render(t *testing.T, scene [][]geom.Point, mask *uint64) *raster.Image {
	t.Helper()
	cfg := config.Default()
	cfg.Width, cfg.Height = 32, 32
	cfg.LayerMask = mask
	im, err := Run(scene, cfg)
	if err != nil {
		t.Fatal(err)
	}
	return im
}

// diffPix 统计两张图差异像素数。
func diffPix(a, b *raster.Image) int {
	if a.W != b.W || a.H != b.H {
		return -1
	}
	n := 0
	for i := range a.Pix {
		if a.Pix[i] != b.Pix[i] {
			n++
		}
	}
	return n
}

func maskPtr(v uint64) *uint64 { return &v }

// 组合一：单层关闭。掩码关闭位 1，与手动隐藏第 1 层的参照图逐像素一致。
func TestSingleLayerHidden(t *testing.T) {
	scene := buildScene(4)
	got := render(t, scene, maskPtr(0b1101))
	ref := render(t, hideInScene(scene, 1), nil)
	if d := diffPix(got, ref); d != 0 {
		t.Fatalf("关闭位 1 与手动隐藏参照差异像素 = %d，应为 0", d)
	}
}

// 组合二：多层关闭。同时关闭位 0 和位 2。
func TestMultipleLayersHidden(t *testing.T) {
	scene := buildScene(4)
	got := render(t, scene, maskPtr(0b1010))
	ref := render(t, hideInScene(scene, 0, 2), nil)
	if d := diffPix(got, ref); d != 0 {
		t.Fatalf("关闭位 0,2 与手动隐藏参照差异像素 = %d，应为 0", d)
	}
}

// 组合三：全部关闭。显式掩码 0，画面全黑，诊断列出全部位号。
func TestAllLayersHidden(t *testing.T) {
	scene := buildScene(4)
	im := render(t, scene, maskPtr(0))
	for i, v := range im.Pix {
		if v != 0 {
			t.Fatalf("掩码全零应渲染空图，像素 %d = %d", i, v)
		}
	}
	issues := layer.Check(0, len(scene))
	if len(issues) != 1 || issues[0].Kind != layer.KindAllOff {
		t.Fatalf("应报掩码全为零: %v", issues)
	}
	if len(issues[0].Bits) != 4 {
		t.Fatalf("全零诊断应列出位 0..3，得到 %v", issues[0].Bits)
	}
}

// 组合四：反复开合。位 1 关→开→关→开，每步都与对应参照逐像素一致。
func TestRepeatedToggle(t *testing.T) {
	scene := buildScene(4)
	refOff := render(t, hideInScene(scene, 1), nil)
	refOn := render(t, scene, nil)
	for round := 0; round < 3; round++ {
		if d := diffPix(render(t, scene, maskPtr(0b1101)), refOff); d != 0 {
			t.Fatalf("第 %d 轮关闭位 1 差异像素 = %d，应为 0", round, d)
		}
		if d := diffPix(render(t, scene, maskPtr(0b1111)), refOn); d != 0 {
			t.Fatalf("第 %d 轮打开位 1 差异像素 = %d，应为 0", round, d)
		}
	}
}

// 组合五：空掩码（未设置）。与默认渲染、显式全开掩码三者逐像素一致。
func TestEmptyMask(t *testing.T) {
	scene := buildScene(4)
	def := render(t, scene, nil)
	all := render(t, scene, maskPtr(0b1111))
	if d := diffPix(def, all); d != 0 {
		t.Fatalf("空掩码与全开掩码差异像素 = %d，应为 0", d)
	}
}

// 未知图层位：置了位 5 但场景只有 4 层。诊断指出位 5，
// 渲染结果与不含该位的掩码一致（其余图层不受影响）。
func TestUnknownLayerBit(t *testing.T) {
	scene := buildScene(4)
	issues := layer.Check(layer.Mask(0b100111), len(scene))
	if len(issues) != 1 || issues[0].Kind != layer.KindUnknown {
		t.Fatalf("应报未知图层位: %v", issues)
	}
	if len(issues[0].Bits) != 1 || issues[0].Bits[0] != 5 {
		t.Fatalf("未知位应为位 5，得到 %v", issues[0].Bits)
	}
	got := render(t, scene, maskPtr(0b100111))
	ref := render(t, scene, maskPtr(0b0111))
	if d := diffPix(got, ref); d != 0 {
		t.Fatalf("未知位不应影响其余图层，差异像素 = %d，应为 0", d)
	}
}

// 位数超限：65 层场景无法被 64 位掩码寻址，Run 报错并指出位 64。
func TestOverflowLayerCount(t *testing.T) {
	scene := buildScene(65)
	cfg := config.Default()
	cfg.LayerMask = maskPtr(1)
	_, err := Run(scene, cfg)
	if err == nil || !strings.Contains(err.Error(), "64") {
		t.Fatalf("65 层应报位数超限并指出位 64: %v", err)
	}
}

// 两档图层数：各报掩码位数与渲染毫秒。
func TestLayerTiers(t *testing.T) {
	for _, n := range []int{8, 64} {
		scene := buildScene(n)
		m, err := layer.All(n)
		if err != nil {
			t.Fatal(err)
		}
		cfg := config.Default()
		cfg.LayerMask = maskPtr(uint64(m))
		start := time.Now()
		im, err := Run(scene, cfg)
		if err != nil {
			t.Fatal(err)
		}
		ms := float64(time.Since(start).Microseconds()) / 1000.0
		if im.CountOn(1) == 0 {
			t.Fatalf("%d 层场景应有像素点亮", n)
		}
		t.Logf("档位 layers=%d maskBits=%d renderMs=%.3f", n, n, ms)
	}
}

// 迁移（扩容 4→6）：旧掩码逐位映射，新增层几何与已有层重合，
// 迁移前后各渲一张，差异像素数必须为零。
func TestMigrationGrowRenderParity(t *testing.T) {
	base := buildScene(4)
	oldMask := layer.Mask(0b1011)
	before := render(t, base, maskPtr(uint64(oldMask)))

	newMask, table, err := layer.Migrate(oldMask, 4, 6)
	if err != nil {
		t.Fatal(err)
	}
	for _, mp := range table {
		t.Logf("扩容迁移: %s", mp)
	}
	// 新场景：层 0..3 不变，层 4、5 与层 0 几何重合（可见也不改画面）。
	grown := append(append([][]geom.Point{}, base...), base[0], base[0])
	after := render(t, grown, maskPtr(uint64(newMask)))
	if d := diffPix(before, after); d != 0 {
		t.Fatalf("扩容迁移前后差异像素 = %d，应为 0", d)
	}
}

// 迁移（收缩 4→2）：被丢弃的层与保留层几何重合，
// 迁移前后各渲一张，差异像素数必须为零。
func TestMigrationShrinkRenderParity(t *testing.T) {
	base := buildScene(2)
	full := append(append([][]geom.Point{}, base...), base[0], base[1])
	oldMask := layer.Mask(0b1111)
	before := render(t, full, maskPtr(uint64(oldMask)))

	newMask, table, err := layer.Migrate(oldMask, 4, 2)
	if err != nil {
		t.Fatal(err)
	}
	for _, mp := range table {
		t.Logf("收缩迁移: %s", mp)
	}
	after := render(t, base, maskPtr(uint64(newMask)))
	if d := diffPix(before, after); d != 0 {
		t.Fatalf("收缩迁移前后差异像素 = %d，应为 0", d)
	}
}

// 迁移失败：保留旧状态，报出迁移表，旧掩码渲染结果不变。
func TestMigrationFailureKeepsState(t *testing.T) {
	scene := buildScene(4)
	oldMask := layer.Mask(0b1011)
	got, table, err := layer.Migrate(oldMask, 4, 65)
	if err == nil {
		t.Fatal("迁移到 65 层应失败")
	}
	if got != oldMask {
		t.Fatalf("迁移失败应保留旧掩码 %b，得到 %b", uint64(oldMask), uint64(got))
	}
	if len(table) == 0 {
		t.Fatal("迁移失败应报出迁移表")
	}
	for _, mp := range table {
		t.Logf("失败迁移表: %s", mp)
	}
	before := render(t, scene, maskPtr(uint64(oldMask)))
	after := render(t, scene, maskPtr(uint64(got)))
	if d := diffPix(before, after); d != 0 {
		t.Fatalf("迁移失败后旧状态渲染差异像素 = %d，应为 0", d)
	}
}
