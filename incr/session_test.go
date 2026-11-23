package incr

import (
	"bytes"
	"errors"
	"log"
	"strings"
	"testing"

	"vecgo/config"
	"vecgo/geom"
	"vecgo/pipe"
	"vecgo/raster"
)

func testCfg() config.Config {
	c := config.Default()
	c.Width, c.Height = 48, 48
	c.Supersample = 2
	c.Gamma = 1.4
	c.Blur = 1
	return c
}

func square(x, y, s float64) []geom.Point {
	return []geom.Point{{X: x, Y: y}, {X: x + s, Y: y}, {X: x + s, Y: y + s}, {X: x, Y: y + s}}
}

// anchors 把世界包围盒钉在 [0,100]²，使其不随被移动的图形变化。
func anchors() [][]geom.Point {
	return [][]geom.Point{
		{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 0, Y: 1}},
		{{X: 100, Y: 100}, {X: 99, Y: 100}, {X: 100, Y: 99}},
	}
}

func frameWith(moved ...[]geom.Point) [][]geom.Point {
	f := anchors()
	return append(f, moved...)
}

// assertMatchesFull 是核心断言：增量产物必须与全量路径逐字节相同。
func assertMatchesFull(t *testing.T, got *raster.Image, scene [][]geom.Point, cfg config.Config, frame int) {
	t.Helper()
	want, err := pipe.Run(scene, cfg)
	if err != nil {
		t.Fatalf("帧 %d 全量渲染: %v", frame, err)
	}
	if !bytes.Equal(got.Bytes(), want.Bytes()) {
		for i := range got.Pix {
			if got.Pix[i] != want.Pix[i] {
				t.Fatalf("帧 %d 字节不一致，首个差异像素(%d,%d): 增量=%d 全量=%d",
					frame, i%got.W, i/got.W, got.Pix[i], want.Pix[i])
			}
		}
		t.Fatalf("帧 %d 头或尺寸不一致", frame)
	}
}

// 1. 单图形移动
func TestSingleMovingShape(t *testing.T) {
	cfg := testCfg()
	s, err := NewSession(cfg, SessionOptions{Source: SourceDiff})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		scene := frameWith(square(float64(i)*2, 5, 20), square(50, 20, 18), square(20, 60, 16))
		img, stats, err := s.Frame(scene, nil)
		if err != nil {
			t.Fatalf("帧 %d: %v", i, err)
		}
		assertMatchesFull(t, img, scene, cfg, i)
		if i > 0 {
			if stats.RectsMerged != 1 {
				t.Fatalf("帧 %d 合并后应只剩 1 个脏矩形，得到 %d", i, stats.RectsMerged)
			}
			if stats.WholeFrame {
				t.Fatalf("帧 %d 不应整帧重画", i)
			}
		}
	}
}

// 2. 多图形交替
func TestMultipleAlternatingShapes(t *testing.T) {
	cfg := testCfg()
	s, _ := NewSession(cfg, SessionOptions{Source: SourceDiff})
	for i := 0; i < 10; i++ {
		scene := frameWith(
			square(5, 20+float64(i)*2, 12),
			square(75, 70-float64(i)*2, 12),
			square(40, 40, 10),
		)
		img, stats, err := s.Frame(scene, nil)
		if err != nil {
			t.Fatalf("帧 %d: %v", i, err)
		}
		assertMatchesFull(t, img, scene, cfg, i)
		if i > 0 && stats.RectsMerged != 2 {
			t.Fatalf("帧 %d 应报告 2 个合并后脏矩形，得到 %d %v", i, stats.RectsMerged, stats.RectList)
		}
	}
}

// 3. 整帧失效：图形数量变化
func TestWholeFrameInvalidation(t *testing.T) {
	cfg := testCfg()
	s, _ := NewSession(cfg, SessionOptions{Source: SourceDiff})
	base := frameWith(square(10, 10, 20))
	img, _, err := s.Frame(base, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertMatchesFull(t, img, base, cfg, 0)

	grown := append(append([][]geom.Point{}, base...), square(60, 60, 15))
	img, stats, err := s.Frame(grown, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertMatchesFull(t, img, grown, cfg, 1)
	if !stats.WholeFrame {
		t.Fatal("图形数量变化应整帧失效")
	}
	if stats.RectsMerged != 1 || stats.DirtyPixels != cfg.Width*cfg.Height {
		t.Fatalf("整帧失效账目错误: %+v", stats)
	}
}

// 3b. 包围盒变化导致整帧失效（图形移出原包围盒）
func TestBBoxChangeInvalidatesWholeFrame(t *testing.T) {
	cfg := testCfg()
	s, _ := NewSession(cfg, SessionOptions{Source: SourceDiff})
	a := [][]geom.Point{square(10, 10, 20)}
	if _, _, err := s.Frame(a, nil); err != nil {
		t.Fatal(err)
	}
	b := [][]geom.Point{square(80, 80, 20)} // 包围盒整体改变
	img, stats, err := s.Frame(b, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertMatchesFull(t, img, b, cfg, 1)
	if !stats.WholeFrame {
		t.Fatal("映射变化应整帧重画")
	}
}

// 4. 零变化
func TestZeroChange(t *testing.T) {
	cfg := testCfg()
	s, _ := NewSession(cfg, SessionOptions{Source: SourceDiff})
	scene := frameWith(square(10, 10, 20), square(60, 60, 15))
	if _, _, err := s.Frame(scene, nil); err != nil {
		t.Fatal(err)
	}
	for i := 1; i < 4; i++ {
		img, stats, err := s.Frame(scene, nil)
		if err != nil {
			t.Fatal(err)
		}
		assertMatchesFull(t, img, scene, cfg, i)
		if stats.RectsMerged != 0 || stats.DirtyPixels != 0 || stats.TouchedPixels != 0 {
			t.Fatalf("零变化应无脏矩形: %+v", stats)
		}
	}
}

// 5. 极端长条
func TestExtremeStrip(t *testing.T) {
	cfg := testCfg()
	s, _ := NewSession(cfg, SessionOptions{Source: SourceDiff})
	for i := 0; i < 8; i++ {
		y := 10 + float64(i)*0.7
		scene := frameWith(
			[]geom.Point{{X: 2, Y: y}, {X: 98, Y: y}, {X: 98, Y: y + 1.2}, {X: 2, Y: y + 1.2}},
			square(40, 70, 14),
		)
		img, stats, err := s.Frame(scene, nil)
		if err != nil {
			t.Fatalf("帧 %d: %v", i, err)
		}
		assertMatchesFull(t, img, scene, cfg, i)
		if i > 0 {
			if stats.RectsMerged != 1 {
				t.Fatalf("长条帧应只有 1 个脏矩形，得到 %d", stats.RectsMerged)
			}
			// 重填面积应远小于整幅。
			if stats.DirtyPixels > cfg.Width*cfg.Height/4 {
				t.Fatalf("长条脏区过大: %d/%d", stats.DirtyPixels, cfg.Width*cfg.Height)
			}
		}
	}
}

func TestMarksSourceMatchesFull(t *testing.T) {
	cfg := testCfg()
	s, _ := NewSession(cfg, SessionOptions{Source: SourceMarks})
	prev := frameWith(square(5, 5, 20), square(60, 60, 15))
	if _, _, err := s.Frame(prev, nil); err != nil {
		t.Fatal(err)
	}
	for i := 1; i < 6; i++ {
		cur := frameWith(square(5+float64(i)*2, 5, 20), square(60, 60, 15))
		marks := []raster.Rect{BBoxOf(s.Mapping(), cfg, prev[2], cur[2])}
		img, stats, err := s.Frame(cur, marks)
		if err != nil {
			t.Fatalf("帧 %d: %v", i, err)
		}
		assertMatchesFull(t, img, cur, cfg, i)
		if stats.FellBack {
			t.Fatalf("帧 %d 不应回退", i)
		}
		if stats.RectsMerged != 1 {
			t.Fatalf("帧 %d 应有 1 个脏矩形，得到 %d", i, stats.RectsMerged)
		}
		prev = cur
	}
}

func TestMergeRects(t *testing.T) {
	in := []raster.Rect{
		{X0: 0, Y0: 0, X1: 4, Y1: 4},
		{X0: 4, Y0: 0, X1: 8, Y1: 4}, // 共边，可合并
		{X0: 20, Y0: 20, X1: 22, Y1: 22},
	}
	out := mergeRects(in)
	if len(out) != 2 {
		t.Fatalf("应合并成 2 个，得到 %d: %+v", len(out), out)
	}
	found := false
	for _, r := range out {
		if r == (raster.Rect{X0: 0, Y0: 0, X1: 8, Y1: 4}) {
			found = true
		}
	}
	if !found {
		t.Fatalf("合并结果错误: %+v", out)
	}
	// 对角相接不应合并（会引入大块空白）。
	diag := []raster.Rect{
		{X0: 0, Y0: 0, X1: 4, Y1: 4},
		{X0: 4, Y0: 4, X1: 8, Y1: 8},
	}
	if got := mergeRects(diag); len(got) != 2 {
		t.Fatalf("对角相接不应合并: %+v", got)
	}
}

func TestErrorRectOutOfBounds(t *testing.T) {
	cfg := testCfg()
	s, _ := NewSession(cfg, SessionOptions{Source: SourceMarks})
	if _, _, err := s.Frame(frameWith(square(5, 5, 20)), nil); err != nil {
		t.Fatal(err)
	}
	// 第 1 个矩形越界（画幅 48x48）。
	marks := []raster.Rect{
		{X0: 0, Y0: 0, X1: 4, Y1: 4},
		{X0: 40, Y0: 40, X1: 60, Y1: 60},
	}
	_, _, err := s.Frame(frameWith(square(6, 5, 20)), marks)
	var de *DirtyError
	if !errors.As(err, &de) {
		t.Fatalf("应返回 DirtyError，得到 %v", err)
	}
	if de.Kind != ErrRectOutOfBounds || de.RectNumber() != 1 {
		t.Fatalf("错误类型或矩形号不对: %+v", de)
	}
}

func TestErrorNoPreviousFrame(t *testing.T) {
	cfg := testCfg()
	s, _ := NewSession(cfg, SessionOptions{Source: SourceMarks})
	marks := []raster.Rect{{X0: 0, Y0: 0, X1: 4, Y1: 4}}
	_, _, err := s.Frame(frameWith(square(5, 5, 20)), marks)
	var de *DirtyError
	if !errors.As(err, &de) || de.Kind != ErrNoPreviousFrame {
		t.Fatalf("首帧带标记应返回上一帧缺失，得到 %v", err)
	}
	if de.RectNumber() != 0 {
		t.Fatalf("矩形号应为 0，得到 %d", de.RectNumber())
	}
	// Incremental 在任意来源下都必须拒绝无上一帧。
	s2, _ := NewSession(cfg, SessionOptions{Source: SourceDiff})
	_, _, err = s2.Incremental(frameWith(square(5, 5, 20)), nil)
	if !errors.As(err, &de) || de.Kind != ErrNoPreviousFrame {
		t.Fatalf("Incremental 无上一帧应报错，得到 %v", err)
	}
}

func TestErrorOverlapConflict(t *testing.T) {
	cfg := testCfg()
	s, _ := NewSession(cfg, SessionOptions{Source: SourceMarks})
	if _, _, err := s.Frame(frameWith(square(5, 5, 20)), nil); err != nil {
		t.Fatal(err)
	}
	marks := []raster.Rect{
		{X0: 0, Y0: 0, X1: 10, Y1: 10},
		{X0: 5, Y0: 5, X1: 15, Y1: 15},
	}
	_, _, err := s.Frame(frameWith(square(6, 5, 20)), marks)
	var de *DirtyError
	if !errors.As(err, &de) || de.Kind != ErrOverlapConflict {
		t.Fatalf("应返回重叠冲突，得到 %v", err)
	}
	if de.RectNumber() != 1 || de.OtherRectNumber() != 0 {
		t.Fatalf("重叠双方矩形号不对: #%d vs #%d", de.RectNumber(), de.OtherRectNumber())
	}
}

// 回退路径：调用方漏标，必须回退全量、结果仍正确、并且留下日志。
func TestFallbackOnIncompleteMarks(t *testing.T) {
	cfg := testCfg()
	var buf strings.Builder
	s, _ := NewSession(cfg, SessionOptions{Source: SourceMarks, FallbackLog: log.New(&buf, "", 0)})

	prev := frameWith(square(5, 5, 20), square(70, 70, 15))
	if _, _, err := s.Frame(prev, nil); err != nil {
		t.Fatal(err)
	}
	// 只标静止的第二个图形，漏掉真正移动的第一个。
	cur := frameWith(square(20, 5, 20), square(70, 70, 15))
	marks := []raster.Rect{BBoxOf(s.Mapping(), cfg, cur[3])}
	img, stats, err := s.Frame(cur, marks)
	if err != nil {
		t.Fatalf("回退不应返回错误: %v", err)
	}
	assertMatchesFull(t, img, cur, cfg, 1)
	if !stats.FellBack {
		t.Fatal("漏标必须触发回退")
	}
	if stats.FallbackWhy == "" {
		t.Fatal("回退需记录原因")
	}
	if !strings.Contains(buf.String(), "回退全量") {
		t.Fatalf("回退必须留日志，实际日志: %q", buf.String())
	}
}

// 回退之后，会话必须恢复正常增量，而不是一直退化。
func TestFallbackRecovers(t *testing.T) {
	cfg := testCfg()
	var buf strings.Builder
	s, _ := NewSession(cfg, SessionOptions{Source: SourceMarks, FallbackLog: log.New(&buf, "", 0)})
	cur := frameWith(square(5, 5, 20), square(70, 70, 15))
	if _, _, err := s.Frame(cur, nil); err != nil {
		t.Fatal(err)
	}
	// 一帧漏标 → 回退
	bad := frameWith(square(20, 5, 20), square(70, 70, 15))
	_, st, err := s.Frame(bad, []raster.Rect{BBoxOf(s.Mapping(), cfg, bad[3])})
	if err != nil || !st.FellBack {
		t.Fatalf("应先回退，err=%v stats=%+v", err, st)
	}
	// 下一帧标记正确 → 应回到增量
	good := frameWith(square(22, 5, 20), square(70, 70, 15))
	marks := []raster.Rect{BBoxOf(s.Mapping(), cfg, bad[2], good[2])}
	img, st, err := s.Frame(good, marks)
	if err != nil {
		t.Fatal(err)
	}
	assertMatchesFull(t, img, good, cfg, 2)
	if st.FellBack || st.WholeFrame {
		t.Fatalf("回退后应恢复正常增量: %+v", st)
	}
	// 回退后第二帧的基准必须正确：再来一帧验证仍逐字节一致
	next := frameWith(square(24, 5, 20), square(70, 70, 15))
	marks = []raster.Rect{BBoxOf(s.Mapping(), cfg, good[2], next[2])}
	img, _, err = s.Frame(next, marks)
	if err != nil {
		t.Fatal(err)
	}
	assertMatchesFull(t, img, next, cfg, 3)
}

// 回退路径在无日志器时也必须安全回退。
func TestFallbackWithoutLogger(t *testing.T) {
	cfg := testCfg()
	s, _ := NewSession(cfg, SessionOptions{Source: SourceMarks})
	if _, _, err := s.Frame(frameWith(square(5, 5, 20)), nil); err != nil {
		t.Fatal(err)
	}
	cur := frameWith(square(20, 5, 20))
	img, stats, err := s.Frame(cur, nil) // 完全没有标记
	if err != nil {
		t.Fatal(err)
	}
	assertMatchesFull(t, img, cur, cfg, 1)
	if !stats.FellBack {
		t.Fatal("空标记且确有变化时应回退")
	}
}

func TestBlurExpandsTouchedArea(t *testing.T) {
	cfg := testCfg()
	cfg.Blur = 3
	s, _ := NewSession(cfg, SessionOptions{Source: SourceDiff})
	if _, _, err := s.Frame(frameWith(square(5, 5, 20)), nil); err != nil {
		t.Fatal(err)
	}
	cur := frameWith(square(7, 5, 20))
	img, stats, err := s.Frame(cur, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertMatchesFull(t, img, cur, cfg, 1)
	if stats.TouchedPixels <= stats.DirtyPixels {
		t.Fatalf("模糊应扩大重写范围: dirty=%d touched=%d", stats.DirtyPixels, stats.TouchedPixels)
	}
}

func TestNoBlurPath(t *testing.T) {
	cfg := testCfg()
	cfg.Blur = 0
	s, _ := NewSession(cfg, SessionOptions{Source: SourceDiff})
	if _, _, err := s.Frame(frameWith(square(5, 5, 20)), nil); err != nil {
		t.Fatal(err)
	}
	cur := frameWith(square(9, 5, 20))
	img, stats, err := s.Frame(cur, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertMatchesFull(t, img, cur, cfg, 1)
	if stats.TouchedPixels != stats.DirtyPixels {
		t.Fatal("blur=0 时重写范围应等于脏区")
	}
}

// 越界/重叠属于调用方输入的账目错误，即使本帧几何变化、可以走整帧重画，
// 也必须先报错，而不是悄悄吞掉。
func TestInputErrorsEvenWhenGeometryChanged(t *testing.T) {
	cfg := testCfg()
	cases := []struct {
		name  string
		marks []raster.Rect
		kind  ErrorKind
	}{
		{"越界", []raster.Rect{{X0: 0, Y0: 0, X1: 100, Y1: 5}}, ErrRectOutOfBounds},
		{"重叠", []raster.Rect{{X0: 0, Y0: 0, X1: 10, Y1: 10}, {X0: 5, Y0: 5, X1: 20, Y1: 20}}, ErrOverlapConflict},
	}
	for _, tc := range cases {
		s, _ := NewSession(cfg, SessionOptions{Source: SourceMarks})
		if _, _, err := s.Frame(frameWith(square(5, 5, 20)), nil); err != nil {
			t.Fatal(err)
		}
		// 场景外包盒改变：按旧逻辑会先整帧重画，吞掉输入错误。
		cur := [][]geom.Point{square(80, 80, 15)}
		_, _, err := s.Frame(cur, tc.marks)
		var de *DirtyError
		if !errors.As(err, &de) || de.Kind != tc.kind {
			t.Fatalf("%s: 应返回 %s，得到 %v", tc.name, tc.kind, err)
		}
	}
}

// 错误必须带矩形号，且矩形号指向真正有问题的那一个。
func TestErrorCarriesRectNumber(t *testing.T) {
	cfg := testCfg()
	s, _ := NewSession(cfg, SessionOptions{Source: SourceMarks})
	if _, _, err := s.Frame(frameWith(square(5, 5, 20)), nil); err != nil {
		t.Fatal(err)
	}
	marks := []raster.Rect{
		{X0: 0, Y0: 0, X1: 5, Y1: 5},
		{X0: 6, Y0: 6, X1: 9, Y1: 9},
		{X0: 10, Y0: 10, X1: 9, Y1: 14}, // 退化矩形
	}
	_, _, err := s.Frame(frameWith(square(6, 5, 20)), marks)
	var de *DirtyError
	if !errors.As(err, &de) {
		t.Fatalf("应返回 DirtyError: %v", err)
	}
	if de.RectNumber() != 2 {
		t.Fatalf("应指向 #2，得到 #%d", de.RectNumber())
	}
}

// 标记恰好覆盖变化量时应走增量；覆盖不足应回退。两条分支都测。
func TestMarksExactVsInsufficient(t *testing.T) {
	cfg := testCfg()
	s, _ := NewSession(cfg, SessionOptions{Source: SourceMarks})
	prev := frameWith(square(5, 5, 20))
	if _, _, err := s.Frame(prev, nil); err != nil {
		t.Fatal(err)
	}
	cur := frameWith(square(9, 5, 20))
	need := BBoxOf(s.Mapping(), cfg, prev[2], cur[2])
	// 恰好覆盖
	_, st, err := s.Frame(cur, []raster.Rect{need})
	if err != nil || st.FellBack {
		t.Fatalf("恰好覆盖不应回退: %v %+v", err, st)
	}
	// 少一列
	s2, _ := NewSession(cfg, SessionOptions{Source: SourceMarks})
	if _, _, err := s2.Frame(prev, nil); err != nil {
		t.Fatal(err)
	}
	short := need
	short.X1--
	_, st, err = s2.Frame(cur, []raster.Rect{short})
	if err != nil {
		t.Fatal(err)
	}
	if !st.FellBack {
		t.Fatal("覆盖不足必须回退")
	}
}

// 一次走查：两种来源在整个动画里每一帧都与全量一致，且增量帧占比合规。
func TestBothSourcesAcrossAnimation(t *testing.T) {
	cfg := testCfg()
	for _, src := range []Source{SourceDiff, SourceMarks} {
		s, _ := NewSession(cfg, SessionOptions{Source: src})
		var prev [][]geom.Point
		for i := 0; i < 12; i++ {
			cur := frameWith(square(5+float64(i)*3, 5, 18), square(60, 60, 12))
			var marks []raster.Rect
			if src == SourceMarks && i > 0 {
				marks = []raster.Rect{BBoxOf(s.Mapping(), cfg, prev[2], cur[2])}
			}
			img, _, err := s.Frame(cur, marks)
			if err != nil {
				t.Fatalf("%s 帧 %d: %v", src, i, err)
			}
			assertMatchesFull(t, img, cur, cfg, i)
			prev = cur
		}
	}
}

// 随机对拍：随机图形、随机移动、两种来源、随机小画幅与参数，
// 每一帧都必须与全量路径逐字节相同。
func TestRandomizedByteIdentical(t *testing.T) {
	seed := uint64(0x5eed1234)
	rnd := func(n int) int {
		seed ^= seed << 13
		seed ^= seed >> 7
		seed ^= seed << 17
		return int(seed % uint64(n))
	}
	rndf := func(lo, hi float64) float64 {
		return lo + (hi-lo)*float64(rnd(10000))/10000
	}
	for trial := 0; trial < 40; trial++ {
		cfg := config.Default()
		cfg.Width = 8 + rnd(40)
		cfg.Height = 8 + rnd(40)
		cfg.Supersample = 1 + rnd(3)
		cfg.Blur = rnd(3)
		cfg.Gamma = 0.5 + float64(rnd(20))/10
		if rnd(2) == 0 {
			cfg.Shading = "gradient"
		}
		for _, src := range []Source{SourceDiff, SourceMarks} {
			s, err := NewSession(cfg, SessionOptions{Source: src})
			if err != nil {
				t.Fatal(err)
			}
			// 随机不变的锚点，钉住世界范围。
			anchor := [][]geom.Point{
				{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 0, Y: 1}},
				{{X: 200, Y: 200}, {X: 199, Y: 200}, {X: 200, Y: 199}},
			}
			nShapes := 1 + rnd(3)
			pos := make([][2]float64, nShapes)
			size := make([]float64, nShapes)
			for k := range pos {
				pos[k] = [2]float64{rndf(5, 150), rndf(5, 150)}
				size[k] = rndf(4, 20)
			}
			var prev [][]geom.Point
			for f := 0; f < 8; f++ {
				for k := range pos {
					if rnd(3) == 0 {
						pos[k][0] = clampf(pos[k][0]+rndf(-15, 15), 2, 180)
						pos[k][1] = clampf(pos[k][1]+rndf(-15, 15), 2, 180)
					}
				}
				scene := append([][]geom.Point{}, anchor...)
				for k := range pos {
					scene = append(scene, square(pos[k][0], pos[k][1], size[k]))
				}
				var marks []raster.Rect
				if src == SourceMarks && f > 0 && len(prev) == len(scene) {
					for k := 2; k < len(scene); k++ {
						if !samePoints(prev[k], scene[k]) {
							marks = append(marks, BBoxOf(s.Mapping(), cfg, prev[k], scene[k]))
						}
					}
					// 自动生成的矩形可能重叠，调用方需自行合并。
					marks = MergeRects(marks)
					if rnd(10) == 0 {
						marks = nil // 偶发漏标，走回退
					}
				}
				img, _, err := s.Frame(scene, marks)
				if err != nil {
					t.Fatalf("试次 %d/%s 帧 %d: %v", trial, src, f, err)
				}
				want, err := pipe.Run(scene, cfg)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(img.Bytes(), want.Bytes()) {
					for i := range img.Pix {
						if img.Pix[i] != want.Pix[i] {
							t.Fatalf("试次 %d/%s 帧 %d 像素(%d,%d) 增量=%d 全量=%d",
								trial, src, f, i%img.W, i/img.W, img.Pix[i], want.Pix[i])
						}
					}
				}
				prev = scene
			}
		}
	}
}

func clampf(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func TestMergeRectsResolvesOverlap(t *testing.T) {
	in := []raster.Rect{
		{X0: 0, Y0: 0, X1: 5, Y1: 5},
		{X0: 2, Y0: 0, X1: 7, Y1: 5},
		{X0: 20, Y0: 20, X1: 22, Y1: 22},
	}
	out := MergeRects(in)
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[i].Overlaps(out[j]) {
				t.Fatalf("MergeRects 后仍重叠: %+v 与 %+v", out[i], out[j])
			}
		}
	}
	// 合并结果必须覆盖原矩形。
	for _, r := range in {
		ok := false
		for _, m := range out {
			if m.X0 <= r.X0 && m.Y0 <= r.Y0 && m.X1 >= r.X1 && m.Y1 >= r.Y1 {
				ok = true
			}
		}
		if !ok {
			t.Fatalf("合并后丢失区域 %+v: %+v", r, out)
		}
	}
}
