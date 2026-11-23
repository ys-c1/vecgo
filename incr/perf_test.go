package incr

import (
	"bytes"
	"testing"
	"time"

	"vecgo/config"
	"vecgo/geom"
	"vecgo/pipe"
	"vecgo/raster"
)

func perfCfg() config.Config {
	c := config.Default()
	c.Width, c.Height = 96, 96
	c.Supersample = 2
	c.Gamma = 1.4
	c.Blur = 1
	return c
}

// timingScene 是一段可复现的动画与其配套的两种标记方式。
type timingScene struct {
	name  string
	frame func(i int) [][]geom.Point
	// marks 返回第 i 帧的调用方标记；nil 表示不提供（只跑 diff）。
	marks func(i int, prev, cur [][]geom.Point, m raster.Mapping) []raster.Rect
}

func timingScenes(cfg config.Config) []timingScene {
	return []timingScene{
		{
			name: "单图形移动",
			frame: func(i int) [][]geom.Point {
				return frameWith(square(5+float64(i%12)*1.5, 5, 20), square(60, 20, 15), square(25, 75, 12))
			},
			marks: func(i int, prev, cur [][]geom.Point, m raster.Mapping) []raster.Rect {
				if i == 0 {
					return nil
				}
				return []raster.Rect{BBoxOf(m, cfg, prev[2], cur[2])}
			},
		},
		{
			name: "多图形交替",
			frame: func(i int) [][]geom.Point {
				t := float64(i)
				return frameWith(square(10+8*sinf(t*0.5), 20, 14), square(55, 40+8*sinf(t*0.5+3.14), 14))
			},
			marks: func(i int, prev, cur [][]geom.Point, m raster.Mapping) []raster.Rect {
				if i == 0 {
					return nil
				}
				var out []raster.Rect
				for k := 2; k < len(cur); k++ {
					if samePoints(prev[k], cur[k]) {
						continue
					}
					out = append(out, BBoxOf(m, cfg, prev[k], cur[k]))
				}
				return out
			},
		},
		{
			name: "整帧失效",
			frame: func(i int) [][]geom.Point {
				f := frameWith(square(10+float64(i%8)*2, 10, 18))
				n := 0
				switch {
				case i >= 5:
					n = 1
				case i >= 17:
					n = 2
				}
				for k := 0; k < n; k++ {
					f = append(f, square(30+float64(k)*12, 40, 6))
				}
				return f
			},
			marks: func(i int, prev, cur [][]geom.Point, m raster.Mapping) []raster.Rect {
				if i == 0 {
					return nil
				}
				if len(prev) != len(cur) {
					return []raster.Rect{{X0: 0, Y0: 0, X1: 96, Y1: 96}}
				}
				return []raster.Rect{BBoxOf(m, cfg, prev[2], cur[2])}
			},
		},
		{
			name: "零变化",
			frame: func(i int) [][]geom.Point {
				return frameWith(square(10, 10, 25), square(55, 55, 20))
			},
			marks: func(i int, prev, cur [][]geom.Point, m raster.Mapping) []raster.Rect {
				return nil
			},
		},
		{
			name: "极端长条",
			frame: func(i int) [][]geom.Point {
				y := 10 + float64(i%10)*0.8
				return frameWith(
					[]geom.Point{{X: 3, Y: y}, {X: 97, Y: y}, {X: 97, Y: y + 1.5}, {X: 3, Y: y + 1.5}},
					square(40, 70, 14),
				)
			},
			marks: func(i int, prev, cur [][]geom.Point, m raster.Mapping) []raster.Rect {
				if i == 0 {
					return nil
				}
				return []raster.Rect{BBoxOf(m, cfg, prev[2], cur[2])}
			},
		},
	}
}

func sinf(x float64) float64 {
	// 固定实现，避免平台 libm 差异影响帧序列。
	const tau = 6.283185307179586
	x = x - tau*float64(int(x/tau))
	x2 := x * x
	return x * (1 - x2/6 + x2*x2/120 - x2*x2*x2/5040)
}

// TestThreeFiveScenesByteIdentical 是收尾对拍：五类场景 × 两种来源，
// 每一帧的增量产物与 pipe.Run 逐字节比对。
func TestThreeFiveScenesByteIdentical(t *testing.T) {
	cfg := perfCfg()
	for _, sc := range timingScenes(cfg) {
		for _, src := range []Source{SourceDiff, SourceMarks} {
			if src == SourceMarks && sc.marks == nil {
				continue
			}
			s, err := NewSession(cfg, SessionOptions{Source: src})
			if err != nil {
				t.Fatal(err)
			}
			var prev [][]geom.Point
			for i := 0; i < 20; i++ {
				cur := sc.frame(i)
				var marks []raster.Rect
				if sc.marks != nil {
					marks = sc.marks(i, prev, cur, s.Mapping())
				}
				got, _, err := s.Frame(cur, marks)
				if err != nil {
					t.Fatalf("%s/%s 帧 %d: %v", sc.name, src, i, err)
				}
				want, err := pipe.Run(cur, cfg)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(got.Bytes(), want.Bytes()) {
					t.Fatalf("%s/%s 帧 %d 字节不一致", sc.name, src, i)
				}
				prev = cur
			}
		}
	}
}

// TestIncrementalWithinBudget 只统计真正的增量帧：它们的中位耗时
// 不得超过同一帧全量渲染的四成。整帧失效与回退帧按设计就是全量，不计入。
func TestIncrementalWithinBudget(t *testing.T) {
	cfg := perfCfg()
	const rounds = 5
	for _, sc := range timingScenes(cfg) {
		for _, src := range []Source{SourceDiff, SourceMarks} {
			if src == SourceMarks && sc.marks == nil {
				continue
			}
			var incSamples, fullSamples []time.Duration
			var prev [][]geom.Point
			for i := 0; i < 16; i++ {
				cur := sc.frame(i)
				incBest := time.Duration(1 << 62)
				fullBest := time.Duration(1 << 62)
				var st Stats
				for r := 0; r < rounds; r++ {
					s, err := NewSession(cfg, SessionOptions{Source: src})
					if err != nil {
						t.Fatal(err)
					}
					var p [][]geom.Point
					for j := 0; j < i; j++ {
						f := sc.frame(j)
						var mk []raster.Rect
						if sc.marks != nil {
							mk = sc.marks(j, p, f, s.Mapping())
						}
						if _, _, err := s.Frame(f, mk); err != nil {
							t.Fatal(err)
						}
						p = f
					}
					var marks []raster.Rect
					if sc.marks != nil {
						marks = sc.marks(i, prev, cur, s.Mapping())
					}
					t0 := time.Now()
					_, sti, err := s.Frame(cur, marks)
					if err != nil {
						t.Fatalf("%s/%s 帧 %d: %v", sc.name, src, i, err)
					}
					st = sti
					if d := time.Since(t0); d < incBest {
						incBest = d
					}
					t1 := time.Now()
					if _, err := pipe.Run(cur, cfg); err != nil {
						t.Fatal(err)
					}
					if d := time.Since(t1); d < fullBest {
						fullBest = d
					}
				}
				if !st.WholeFrame && !st.FellBack {
					incSamples = append(incSamples, incBest)
					fullSamples = append(fullSamples, fullBest)
				}
				prev = cur
			}
			if len(incSamples) == 0 {
				continue
			}
			medInc := median(incSamples)
			medFull := median(fullSamples)
			if medInc*10 > medFull*4 {
				t.Fatalf("%s/%s 增量中位耗时 %v 超过全量 %v 的四成", sc.name, src, medInc, medFull)
			}
			t.Logf("%s/%s 增量帧 %d 个，中位 增量=%v 全量=%v (%.1f%%)",
				sc.name, src, len(incSamples), medInc, medFull,
				100*float64(medInc)/float64(medFull))
		}
	}
}

func median(ds []time.Duration) time.Duration {
	c := append([]time.Duration{}, ds...)
	for i := 1; i < len(c); i++ {
		for j := i; j > 0 && c[j] < c[j-1]; j-- {
			c[j], c[j-1] = c[j-1], c[j]
		}
	}
	return c[len(c)/2]
}
