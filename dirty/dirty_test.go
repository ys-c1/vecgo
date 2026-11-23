package dirty

import (
	"errors"
	"testing"

	"vecgo/config"
	"vecgo/geom"
	"vecgo/pipe"
	"vecgo/raster"
)

func cfg16() config.Config {
	c := config.Default()
	c.Width, c.Height = 16, 16
	c.Blur = 1
	return c
}

func square(x0, y0, x1, y1 float64) []geom.Point {
	return []geom.Point{
		{X: x0, Y: y0}, {X: x1, Y: y0},
		{X: x1, Y: y1}, {X: x0, Y: y1},
	}
}

// bytesEqual 逐字节对照两张图。
func bytesEqual(t *testing.T, a, b *raster.Image) {
	t.Helper()
	if a.W != b.W || a.H != b.H || len(a.Pix) != len(b.Pix) {
		t.Fatalf("尺寸不一致 %dx%d vs %dx%d", a.W, a.H, b.W, b.H)
	}
	for i := range a.Pix {
		if a.Pix[i] != b.Pix[i] {
			t.Fatalf("字节不变量破坏于偏移 %d: %d != %d", i, a.Pix[i], b.Pix[i])
		}
	}
}

// assertAgainstFull 每帧都与 pipe.Run 全量路径逐字节对照。
func assertAgainstFull(t *testing.T, scene [][]geom.Point, cfg config.Config, got *raster.Image) {
	t.Helper()
	ref, err := pipe.Run(scene, cfg)
	if err != nil {
		t.Fatal(err)
	}
	bytesEqual(t, got, ref)
}

// TestFullPipelineMatchesPipeRun 字节不变量基础：参考管线必须与 pipe.Run 完全一致。
func TestFullPipelineMatchesPipeRun(t *testing.T) {
	cfg := cfg16()
	scene := [][]geom.Point{square(2, 2, 8, 8)}
	ref, err := FullReference(scene, cfg)
	if err != nil {
		t.Fatal(err)
	}
	want, err := pipe.Run(scene, cfg)
	if err != nil {
		t.Fatal(err)
	}
	bytesEqual(t, ref, want)
}

// TestSingleShapeMove 单图形移动：增量帧字节不变 + 有像素节省。
func TestSingleShapeMove(t *testing.T) {
	cfg := cfg16()
	r := New(cfg)
	// 两个角上的锚点小方块固定全局包围盒，移动发生在中间区域。
	anchorL := square(0, 0, 1, 1)
	anchorR := square(15, 15, 16, 16)
	f0 := [][]geom.Point{anchorL, anchorR, square(4, 4, 6, 6)}
	out0, st0, err := r.Render(f0)
	if err != nil {
		t.Fatal(err)
	}
	assertAgainstFull(t, f0, cfg, out0)
	if !st0.FullInvalid || st0.SavedPixels != 0 {
		t.Fatalf("首帧应整帧失效: %+v", st0)
	}

	f1 := [][]geom.Point{anchorL, anchorR, square(9, 9, 11, 11)}
	out1, st1, err := r.Render(f1)
	if err != nil {
		t.Fatal(err)
	}
	assertAgainstFull(t, f1, cfg, out1)
	if st1.FullInvalid {
		t.Fatal("单图形移动不应整帧失效")
	}
	if st1.RawRects != 1 || st1.MergedRects != 1 {
		t.Fatalf("应有 1 个原始/合并矩形: %+v", st1)
	}
	if st1.SavedPixels <= 0 || st1.RepaintPixels >= st1.TotalPixels {
		t.Fatalf("应有像素节省: %+v", st1)
	}
}

// TestMultiAlternating 多图形交替：不同图形在不同帧轮流变化，矩形号随图形号走。
func TestMultiAlternating(t *testing.T) {
	cfg := cfg16()
	r := New(cfg)
	anchorL := square(0, 0, 1, 1)
	anchorR := square(15, 15, 16, 16)
	movA := square(3, 3, 4, 4)
	movB := square(11, 11, 12, 12)
	f0 := [][]geom.Point{anchorL, anchorR, movA, movB}
	if _, _, err := r.Render(f0); err != nil {
		t.Fatal(err)
	}
	// 帧 1：只动图形 2
	f1 := [][]geom.Point{anchorL, anchorR, square(5, 5, 6, 6), movB}
	out1, st1, err := r.Render(f1)
	if err != nil {
		t.Fatal(err)
	}
	assertAgainstFull(t, f1, cfg, out1)
	if st1.RawRects != 1 {
		t.Fatalf("只应有一个脏矩形: %+v", st1)
	}
	// 帧 2：只动图形 3
	f2 := [][]geom.Point{anchorL, anchorR, square(5, 5, 6, 6), square(12, 8, 13, 9)}
	out2, st2, err := r.Render(f2)
	if err != nil {
		t.Fatal(err)
	}
	assertAgainstFull(t, f2, cfg, out2)
	if st2.RawRects != 1 {
		t.Fatalf("只应有一个脏矩形: %+v", st2)
	}
	// 帧 3：两个同时动 -> 两个原始矩形；小画幅 + blur 余量外扩后二者
	// 面积相交，合并为 1 个重画区并按矩形号上报冲突。
	f3 := [][]geom.Point{anchorL, anchorR, square(3, 6, 4, 7), square(10, 10, 11, 11)}
	out3, st3, err := r.Render(f3)
	if err != nil {
		t.Fatal(err)
	}
	assertAgainstFull(t, f3, cfg, out3)
	if st3.RawRects != 2 {
		t.Fatalf("应有两个原始矩形: %+v", st3)
	}
	if st3.MergedRects != 1 || len(st3.Issues) != 1 || st3.Issues[0].Rect != 0 {
		t.Fatalf("两个相交矩形应合并为 1 个并上报 0/1 冲突: %+v", st3)
	}
}

// TestFullInvalidation 整帧失效：全局包围盒变化时上报并整帧重画。
func TestFullInvalidation(t *testing.T) {
	cfg := cfg16()
	r := New(cfg)
	f0 := [][]geom.Point{square(2, 2, 8, 8)}
	if _, _, err := r.Render(f0); err != nil {
		t.Fatal(err)
	}
	// 图形扩展到新的坐标极值 -> 全局 bbox 变化
	f1 := [][]geom.Point{square(2, 2, 12, 12)}
	out1, st, err := r.Render(f1)
	if err != nil {
		t.Fatal(err)
	}
	assertAgainstFull(t, f1, cfg, out1)
	if !st.FullInvalid {
		t.Fatalf("应整帧失效: %+v", st)
	}
	if len(st.Issues) != 1 || st.Issues[0].Rect != -1 {
		t.Fatalf("应上报矩形号 -1: %+v", st.Issues)
	}
}

// TestZeroChange 零变化：零矩形、零重画像素、输出与上一帧及全量都一致。
func TestZeroChange(t *testing.T) {
	cfg := cfg16()
	r := New(cfg)
	f0 := [][]geom.Point{square(2, 2, 8, 8)}
	out0, _, err := r.Render(f0)
	if err != nil {
		t.Fatal(err)
	}
	out1, st, err := r.Render(f0)
	if err != nil {
		t.Fatal(err)
	}
	if st.RawRects != 0 || st.MergedRects != 0 || st.RepaintPixels != 0 {
		t.Fatalf("零变化统计错误: %+v", st)
	}
	if st.SavedPixels != st.TotalPixels {
		t.Fatalf("零变化应省下 100%% 像素: %+v", st)
	}
	bytesEqual(t, out0, out1)
	assertAgainstFull(t, f0, cfg, out1)
}

// TestExtremeLongStrip 极端长条：1 像素高的长条横跨画幅，仍字节一致且只重画细条。
func TestExtremeLongStrip(t *testing.T) {
	c := config.Default()
	c.Width, c.Height = 64, 64
	c.Blur = 0 // 纯细条：不加 blur 外扩，展示极端长宽比下的节省
	cfg := c
	r := New(cfg)
	anchorL := square(0, 0, 1, 1)
	anchorR := square(63, 63, 64, 64)
	f0 := [][]geom.Point{anchorL, anchorR, square(1, 30, 63, 31)}
	if _, _, err := r.Render(f0); err != nil {
		t.Fatal(err)
	}
	f1 := [][]geom.Point{anchorL, anchorR, square(1, 38, 63, 39)}
	out1, st, err := r.Render(f1)
	if err != nil {
		t.Fatal(err)
	}
	assertAgainstFull(t, f1, cfg, out1)
	if st.MergedRects != 1 {
		t.Fatalf("长条应合并为 1 个矩形: %+v", st)
	}
	// 细条只覆盖少数行（含 2px 余量与 blur 外扩），重画面积远小于整帧。
	if st.RepaintPixels >= st.TotalPixels/3 {
		t.Fatalf("长条重画面积过大: %+v", st)
	}
	if st.SavedRatio() < 0.66 {
		t.Fatalf("长条节省比例应 >= 0.66: %+v", st)
	}
}

// TestOutOfBoundsRectReportsIndex 越界矩形必须带矩形号报错。
func TestOutOfBoundsRectReportsIndex(t *testing.T) {
	cfg := cfg16()
	r := New(cfg)
	f0 := [][]geom.Point{square(2, 2, 8, 8)}
	if _, _, err := r.Render(f0); err != nil {
		t.Fatal(err)
	}
	bad := []Rect{
		{X0: 0, Y0: 0, X1: 4, Y1: 4},
		{X0: 0, Y0: 0, X1: 17, Y1: 4}, // 矩形号 1 越界
	}
	_, _, err := r.Apply(f0, bad)
	var re *RectError
	if !errors.As(err, &re) || re.Kind != IssueOutOfBounds {
		t.Fatalf("应报越界 RectError: %v", err)
	}
	if len(re.Rects) != 1 || re.Rects[0] != 1 {
		t.Fatalf("应报矩形号 1: %v", re.Rects)
	}
}

// TestMissingPrevFrameReportsIndex 上一帧缺失必须带矩形号 -1 报错（显式入口）。
func TestMissingPrevFrameReportsIndex(t *testing.T) {
	cfg := cfg16()
	r := New(cfg)
	f0 := [][]geom.Point{square(2, 2, 8, 8)}
	_, _, err := r.Apply(f0, []Rect{{X0: 0, Y0: 0, X1: 4, Y1: 4}})
	var re *RectError
	if !errors.As(err, &re) || re.Kind != IssueNoPrevFrame {
		t.Fatalf("应报上一帧缺失: %v", err)
	}
	if len(re.Rects) != 1 || re.Rects[0] != -1 {
		t.Fatalf("应报矩形号 -1: %v", re.Rects)
	}
}

// TestOverlapConflictReportsIndices 区域重叠冲突报矩形号；普通模式合并，严格模式报错。
func TestOverlapConflictReportsIndices(t *testing.T) {
	cfg := cfg16()
	r := New(cfg)
	f0 := [][]geom.Point{square(2, 2, 8, 8)}
	out0, _, err := r.Render(f0)
	if err != nil {
		t.Fatal(err)
	}
	rects := []Rect{
		{X0: 0, Y0: 0, X1: 6, Y1: 6},   // 矩形号 0
		{X0: 4, Y0: 4, X1: 10, Y1: 10}, // 矩形号 1，与 0 面积相交
		{X0: 12, Y0: 12, X1: 15, Y1: 15},
	}
	out, st, err := r.Apply(f0, rects)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Issues) != 1 || st.Issues[0].Rect != 0 {
		t.Fatalf("应上报 0/1 冲突: %+v", st.Issues)
	}
	if st.RawRects != 3 || st.MergedRects != 2 {
		t.Fatalf("3 个原始矩形应合并为 2 个: %+v", st)
	}
	assertAgainstFull(t, f0, cfg, out)

	// 严格模式：同样重叠直接报错，矩形号 0、1
	r2 := New(cfg)
	r2.Strict = true
	if _, _, err := r2.Render(f0); err != nil {
		t.Fatal(err)
	}
	_, _, err = r2.Apply(f0, rects)
	var re *RectError
	if !errors.As(err, &re) || re.Kind != IssueOverlap {
		t.Fatalf("严格模式应报重叠错误: %v", err)
	}
	if len(re.Rects) != 2 || re.Rects[0] != 0 || re.Rects[1] != 1 {
		t.Fatalf("应报矩形号 0,1: %v", re.Rects)
	}
	// 严格模式报错后状态不得被污染
	if _, st, err := r2.Render(f0); err != nil || st.RawRects != 0 {
		t.Fatalf("报错后零变化帧应仍可用: %v %+v", err, st)
	}
	_ = out0
}

// TestEscalationOnByteMismatch 验证安全网：显式矩形没覆盖真实变化时，
// 字节对照发现差异，Apply 直接报错（不静默产出错误像素）。
func TestEscalationOnByteMismatch(t *testing.T) {
	cfg := cfg16()
	r := New(cfg)
	anchorL := square(0, 0, 1, 1)
	anchorR := square(15, 15, 16, 16)
	f0 := [][]geom.Point{anchorL, anchorR, square(4, 4, 6, 6)}
	if _, _, err := r.Render(f0); err != nil {
		t.Fatal(err)
	}
	// 图形移到 (11,11)，但调用方只声明了左上角一个不相关的小矩形
	f1 := [][]geom.Point{anchorL, anchorR, square(11, 11, 13, 13)}
	_, _, err := r.Apply(f1, []Rect{{X0: 0, Y0: 0, X1: 2, Y1: 2}})
	if err == nil {
		t.Fatal("字节对照应捕获未覆盖的变化并报错")
	}
}

// TestMergeReportsCountAndSavedPixels 合并后个数与省下像素数都要正确。
func TestMergeReportsCountAndSavedPixels(t *testing.T) {
	rects := []Rect{
		{X0: 0, Y0: 0, X1: 4, Y1: 4},
		{X0: 2, Y0: 2, X1: 6, Y1: 6}, // 与 0 重叠 -> 合并
		{X0: 10, Y0: 10, X1: 14, Y1: 14},
	}
	merged, conflicts := Merge(rects)
	if len(merged) != 2 || len(conflicts) != 1 {
		t.Fatalf("合并结果错误: %d 矩形 %d 冲突", len(merged), len(conflicts))
	}
	got := unionArea(merged)
	// 合并后并集 = 6x6 + 4x4 = 52
	if got != 52 {
		t.Fatalf("并集像素数应为 52, 实际 %d", got)
	}
}

// TestGradientAndGammaPaths 非默认 shading/gamma 下增量与全量仍逐字节一致。
func TestGradientAndGammaPaths(t *testing.T) {
	cfg := cfg16()
	cfg.Shading = "gradient"
	cfg.Gamma = 2.2
	cfg.Blur = 2
	r := New(cfg)
	anchorL := square(0, 0, 1, 1)
	anchorR := square(15, 15, 16, 16)
	f0 := [][]geom.Point{anchorL, anchorR, square(4, 4, 6, 6)}
	if _, _, err := r.Render(f0); err != nil {
		t.Fatal(err)
	}
	f1 := [][]geom.Point{anchorL, anchorR, square(9, 4, 11, 6)}
	out1, _, err := r.Render(f1)
	if err != nil {
		t.Fatal(err)
	}
	assertAgainstFull(t, f1, cfg, out1)
}
