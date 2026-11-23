package dirty

import (
	"testing"

	"vecgo/config"
	"vecgo/geom"
)

// benchScene 构造 n 个静止锚点 + 一个每帧平移的小方块，
// 锚点把全局包围盒固定在画幅四角，保证增量帧不会整帧失效。
func benchScene(n int, shift float64) [][]geom.Point {
	scene := make([][]geom.Point, 0, n+2)
	// 四角锚点固定全局 bbox（坐标空间 0..100）
	scene = append(scene,
		square(0, 0, 2, 2),
		square(98, 98, 100, 100),
	)
	// n 个图形铺满中部网格
	cols := n
	for i := 0; i < n; i++ {
		x := 4 + float64(i%cols)*0.4
		y := 4 + float64(i/cols)*4
		scene = append(scene, square(x, y, x+1.5, y+1.5))
	}
	// 最后一个图形随帧平移
	scene[len(scene)-1] = square(4+shift, 50, 5.5+shift, 51.5)
	return scene
}

func benchCfg() config.Config {
	c := config.Default()
	c.Width, c.Height = 256, 256
	c.Blur = 1
	return c
}

func BenchmarkFull8(b *testing.B)  { benchmarkFull(b, 8) }
func BenchmarkFull64(b *testing.B) { benchmarkFull(b, 64) }

// benchmarkFull 直接走全量管线（与 pipe.Run 同数学），代表改造前的整帧重画成本。
func benchmarkFull(b *testing.B, n int) {
	cfg := benchCfg()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		out, err := FullReference(benchScene(n, float64(i%4)*0.5), cfg)
		if err != nil {
			b.Fatal(err)
		}
		if out == nil {
			b.Fatal("空输出")
		}
	}
}

func BenchmarkIncremental8(b *testing.B)  { benchmarkIncremental(b, 8) }
func BenchmarkIncremental64(b *testing.B) { benchmarkIncremental(b, 64) }

func benchmarkIncremental(b *testing.B, n int) {
	cfg := benchCfg()
	r := New(cfg)
	r.Verify = false // 纯增量耗时；字节不变量由功能测试逐帧验证
	if _, _, err := r.Render(benchScene(n, 0)); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, s, err := r.Render(benchScene(n, float64(i%4)*0.5+0.5))
		if err != nil {
			b.Fatal(err)
		}
		if s.FullInvalid {
			b.Fatalf("基准场景不应整帧失效: %+v", s)
		}
	}
}

// BenchmarkIncrementalVerify8 开启逐帧字节对照的增量路径，
// 展示带安全网时的真实开销。
func BenchmarkIncrementalVerify8(b *testing.B) {
	cfg := benchCfg()
	r := New(cfg)
	r.Verify = true
	if _, _, err := r.Render(benchScene(8, 0)); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := r.Render(benchScene(8, float64(i%4)*0.5+0.5)); err != nil {
			b.Fatal(err)
		}
	}
}
