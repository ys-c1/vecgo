package raster

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"vecgo/geom"
)

// ---- 场景构造：直边、斜边、细缝、大画幅、空场景 ----

func straightEdgeScene() [][]geom.Point {
	return [][]geom.Point{{
		{X: 1, Y: 1}, {X: 9, Y: 1}, {X: 9, Y: 9}, {X: 1, Y: 9},
	}}
}

func slantedEdgeScene() [][]geom.Point {
	// 菱形：四条边全部倾斜，覆盖块边界上的斜边反走样像素。
	return [][]geom.Point{{
		{X: 0, Y: 5}, {X: 5, Y: 0}, {X: 10, Y: 5}, {X: 5, Y: 10},
	}, {
		// 斜三角，制造多个多边形叠加。
		{X: 2, Y: 8}, {X: 8, Y: 8}, {X: 5, Y: 2},
	}}
}

func thinSlitScene() [][]geom.Point {
	// 两块高矩形之间留一条细缝，缝宽约占画幅的 2%。
	left := []geom.Point{{X: 0, Y: 0}, {X: 4.7, Y: 0}, {X: 4.7, Y: 10}, {X: 0, Y: 10}}
	right := []geom.Point{{X: 5.3, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 5.3, Y: 10}}
	return [][]geom.Point{left, right}
}

// renderPair 用同一画幅、同一场景分别走串行与并行路径，返回两张图。
func renderPair(t *testing.T, w, h, ss int, scene [][]geom.Point, pc ParallelConfig) (*Image, *Image) {
	t.Helper()
	serial, err := New(w, h)
	if err != nil {
		t.Fatal(err)
	}
	parallel, err := New(w, h)
	if err != nil {
		t.Fatal(err)
	}
	if err := serial.Fill(scene, ss); err != nil {
		t.Fatalf("串行 Fill 失败: %v", err)
	}
	if err := parallel.FillParallel(scene, ss, pc); err != nil {
		t.Fatalf("并行 FillParallel 失败: %v", err)
	}
	return serial, parallel
}

func assertBytesIdentical(t *testing.T, serial, parallel *Image) {
	t.Helper()
	if len(serial.Pix) != len(parallel.Pix) {
		t.Fatalf("像素缓冲长度不一致: %d vs %d", len(serial.Pix), len(parallel.Pix))
	}
	for i, v := range serial.Pix {
		if parallel.Pix[i] != v {
			row, col := i/serial.W, i%serial.W
			t.Fatalf("像素不一致于 (row=%d,col=%d) idx=%d: 串行=%d 并行=%d", row, col, i, v, parallel.Pix[i])
		}
	}
}

// seamMismatchCount 统计块边界两侧相邻行上，并行图与串行图不一致的像素数（接缝像素数）。
func seamMismatchCount(serial, parallel *Image, blocks int) int {
	rowsPerBlock := serial.H / blocks
	mismatch := 0
	for b := 1; b < blocks; b++ {
		boundary := b * rowsPerBlock
		for _, row := range []int{boundary - 1, boundary} {
			base := row * serial.W
			for col := 0; col < serial.W; col++ {
				if serial.Pix[base+col] != parallel.Pix[base+col] {
					mismatch++
				}
			}
		}
	}
	return mismatch
}

// 五种场景 × 多组块数/并发度，全部要求逐字节相同、接缝像素为零。
func TestParallelByteIdentical(t *testing.T) {
	cases := []struct {
		name  string
		scene [][]geom.Point
	}{
		{"直边", straightEdgeScene()},
		{"斜边", slantedEdgeScene()},
		{"细缝", thinSlitScene()},
		{"大画幅", slantedEdgeScene()},
		{"空场景", nil},
	}
	sizes := []struct {
		w, h int
	}{
		{64, 64},
		{256, 192},
		{1024, 1024}, // 大画幅
	}
	pcs := []ParallelConfig{
		{Blocks: 2, Workers: 2},
		{Blocks: 8, Workers: 4},   // 并发度小于块数
		{Blocks: 16, Workers: 32}, // 并发度大于块数
		{Blocks: 4, Workers: 1},   // 并发度为 1 仍应正确
	}
	for _, c := range cases {
		for _, sz := range sizes {
			for _, pc := range pcs {
				if sz.h%pc.Blocks != 0 {
					continue
				}
				name := fmt.Sprintf("%s_b%dw%d", c.name, pc.Blocks, pc.Workers)
				t.Run(name, func(t *testing.T) {
					serial, parallel := renderPair(t, sz.w, sz.h, 3, c.scene, pc)
					assertBytesIdentical(t, serial, parallel)
					if n := seamMismatchCount(serial, parallel, pc.Blocks); n != 0 {
						t.Fatalf("块边界接缝像素数=%d，必须为零", n)
					}
				})
			}
		}
	}
}

func TestThinSlitPreserved(t *testing.T) {
	serial, parallel := renderPair(t, 512, 512, 4, thinSlitScene(), ParallelConfig{Blocks: 16, Workers: 16})
	assertBytesIdentical(t, serial, parallel)
	// 细缝列（约在画幅中央 4.7~5.3 的映射区间）必须存在暗像素。
	mid := serial.W / 2
	dark := 0
	for row := 0; row < serial.H; row++ {
		if serial.Pix[row*serial.W+mid] == 0 {
			dark++
		}
	}
	if dark == 0 {
		t.Fatal("细缝在中央列没有留下暗像素，场景构造失效")
	}
}

// 块数不能整除画幅：必须返回带块号的错误，不允许静默降级。
func TestBlocksNotDivisible(t *testing.T) {
	im, err := New(64, 10)
	if err != nil {
		t.Fatal(err)
	}
	err = im.FillParallel(straightEdgeScene(), 2, ParallelConfig{Blocks: 3, Workers: 2})
	if err == nil {
		t.Fatal("块数不能整除画幅高度时必须报错")
	}
	msg := err.Error()
	if !strings.Contains(msg, "blocks") || !strings.Contains(msg, "height") {
		t.Fatalf("错误必须带字段名: %v", err)
	}
	if !strings.Contains(msg, "块号 2") {
		t.Fatalf("错误必须带块号: %v", err)
	}
}

// 并发创建失败：必须返回带块号与 workers 字段名的错误，不允许静默降级。
func TestWorkerSpawnFailure(t *testing.T) {
	orig := newTaskGroup
	t.Cleanup(func() { newTaskGroup = orig })
	newTaskGroup = func(workers int) taskGroup {
		return &failGroup{failAfter: 2} // 前两个任务成功，第三个任务创建失败
	}

	im, err := New(64, 64)
	if err != nil {
		t.Fatal(err)
	}
	err = im.FillParallel(straightEdgeScene(), 2, ParallelConfig{Blocks: 8, Workers: 4})
	if err == nil {
		t.Fatal("并发创建失败必须上抛错误")
	}
	msg := err.Error()
	if !strings.Contains(msg, "块号 2") {
		t.Fatalf("错误必须带块号: %v", err)
	}
	if !strings.Contains(msg, "workers") {
		t.Fatalf("错误必须带 workers 字段名: %v", err)
	}
}

// 画幅为零：串行与并行都必须返回带字段名的错误。
func TestZeroFrameRejected(t *testing.T) {
	zero := &Image{W: 0, H: 0}
	if err := zero.Fill(straightEdgeScene(), 1); err == nil ||
		!strings.Contains(err.Error(), "width") || !strings.Contains(err.Error(), "height") {
		t.Fatalf("串行路径零画幅错误必须带字段名: %v", err)
	}
	if err := zero.FillParallel(straightEdgeScene(), 1, ParallelConfig{Blocks: 2, Workers: 2}); err == nil ||
		!strings.Contains(err.Error(), "width") || !strings.Contains(err.Error(), "height") {
		t.Fatalf("并行路径零画幅错误必须带字段名: %v", err)
	}
	if _, err := New(0, 8); err == nil ||
		!strings.Contains(err.Error(), "width") || !strings.Contains(err.Error(), "height") {
		t.Fatalf("New 零画幅错误必须带字段名: %v", err)
	}
}

func TestBadParallelConfig(t *testing.T) {
	im, _ := New(32, 32)
	if err := im.FillParallel(straightEdgeScene(), 1, ParallelConfig{Blocks: 0, Workers: 2}); err == nil ||
		!strings.Contains(err.Error(), "blocks") {
		t.Fatalf("blocks=0 必须报带字段名的错误: %v", err)
	}
	if err := im.FillParallel(straightEdgeScene(), 1, ParallelConfig{Blocks: 2, Workers: 0}); err == nil ||
		!strings.Contains(err.Error(), "workers") {
		t.Fatalf("workers=0 必须报带字段名的错误: %v", err)
	}
}

// 块数为一：FillParallel 必须委托串行基线，且输出逐字节相同。
func TestBlocksOneDelegatesToSerial(t *testing.T) {
	serial, parallel := renderPair(t, 128, 128, 2, slantedEdgeScene(), ParallelConfig{Blocks: 1, Workers: 8})
	assertBytesIdentical(t, serial, parallel)

	// 块数为一时即便画幅不能被任何分块整除也不会报整除错误，因为根本不进入并行分支。
	odd, _ := New(64, 13)
	if err := odd.FillParallel(straightEdgeScene(), 2, ParallelConfig{Blocks: 1, Workers: 8}); err != nil {
		t.Fatalf("blocks=1 应走串行基线，不应触发整除校验: %v", err)
	}
}

// 块数为一时并行任务工厂一次都不能被调用，证明不会创建任何 goroutine。
func TestBlocksOneSpawnsNoGoroutines(t *testing.T) {
	orig := newTaskGroup
	t.Cleanup(func() { newTaskGroup = orig })
	calls := 0
	newTaskGroup = func(workers int) taskGroup {
		calls++
		return orig(workers)
	}
	im, _ := New(64, 64)
	if err := im.FillParallel(straightEdgeScene(), 2, ParallelConfig{Blocks: 1, Workers: 16}); err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatalf("blocks=1 不应创建并行任务组，实际调用 %d 次", calls)
	}
}

func TestEmptySceneBlack(t *testing.T) {
	serial, parallel := renderPair(t, 64, 64, 2, nil, ParallelConfig{Blocks: 4, Workers: 4})
	assertBytesIdentical(t, serial, parallel)
	for i, v := range serial.Pix {
		if v != 0 {
			t.Fatalf("空场景必须全黑，位置 %d 出现 %d", i, v)
		}
	}
}

// ---- 加速比：两档画幅 ----

func benchScene() [][]geom.Point {
	return slantedEdgeScene()
}

func benchmarkSize(b *testing.B, w, h, ss int, parallel bool) {
	scene := benchScene()
	im, _ := New(w, h)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var err error
		if parallel {
			err = im.FillParallel(scene, ss, ParallelConfig{Blocks: 16, Workers: 16})
		} else {
			err = im.Fill(scene, ss)
		}
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFill_512(b *testing.B) {
	b.Run("serial", func(b *testing.B) { benchmarkSize(b, 512, 512, 4, false) })
	b.Run("parallel_b16w16", func(b *testing.B) { benchmarkSize(b, 512, 512, 4, true) })
}

func BenchmarkFill_2048(b *testing.B) {
	b.Run("serial", func(b *testing.B) { benchmarkSize(b, 2048, 2048, 4, false) })
	b.Run("parallel_b16w16", func(b *testing.B) { benchmarkSize(b, 2048, 2048, 4, true) })
}

// TestSpeedup 为默认跳过的硬门槛测试：VECGO_SPEEDUP=1 时运行，
// 两档画幅加速比低于 1.8 即失败，使"没做到"可以被客观判定。
func TestSpeedup(t *testing.T) {
	if os.Getenv("VECGO_SPEEDUP") == "" {
		t.Skip("设置 VECGO_SPEEDUP=1 运行加速比硬门槛测试")
	}
	scene := benchScene()
	for _, sz := range [][2]int{{512, 512}, {2048, 2048}} {
		w, h := sz[0], sz[1]
		measure := func(parallel bool) time.Duration {
			im, _ := New(w, h)
			const rounds = 3
			start := time.Now()
			for i := 0; i < rounds; i++ {
				var err error
				if parallel {
					err = im.FillParallel(scene, 4, ParallelConfig{Blocks: 16, Workers: 16})
				} else {
					err = im.Fill(scene, 4)
				}
				if err != nil {
					t.Fatal(err)
				}
			}
			return time.Since(start) / rounds
		}
		serial := measure(false)
		parallel := measure(true)
		speedup := float64(serial) / float64(parallel)
		t.Logf("画幅 %dx%d 串行=%v 并行=%v 加速比=%.2fx", w, h, serial, parallel, speedup)
		if speedup < 1.8 {
			t.Fatalf("画幅 %dx%d 加速比 %.2fx 低于 1.8x 门槛", w, h, speedup)
		}
	}
}
