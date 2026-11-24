package pipe

import (
	"math"
	"testing"

	"vecgo/config"
	"vecgo/geom"
)

// 反解公式 N(θ)=ceil(N0·(|cosθ|+|sinθ|)²) 的两档角度代入值。
func TestEffectiveSupersampleFormula(t *testing.T) {
	const N0 = 2
	cases := []struct {
		deg float64
		want int
	}{
		{0, 2},   // (1+0)²·2 = 2.000000 → 2
		{5, 3},   // (0.996195+0.087156)²·2 = 2.347296 → 3
		{30, 4},  // (0.866025+0.500000)²·2 = 3.732051 → 4
	}
	for _, c := range cases {
		theta := c.deg * math.Pi / 180
		s := math.Abs(math.Cos(theta)) + math.Abs(math.Sin(theta))
		raw := float64(N0) * s * s
		got := EffectiveSupersample(N0, theta)
		if got != c.want {
			t.Fatalf("θ=%d° 原始值=%.6f 反解 N=%d, 期望 %d", int(c.deg), raw, got, c.want)
		}
		t.Logf("θ=%d° (|cos|+|sin|)²·N0 = %.6f → ceil = %d", int(c.deg), raw, got)
	}
}

// θ=0 时整条管线的像素缓冲必须与固定 N0=2 的直跑结果逐字节相同。
func TestZeroAngleByteIdentical(t *testing.T) {
	scene := [][]geom.Point{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}}
	cfg := config.Default()
	cfg.Width, cfg.Height = 64, 64
	a, err := Run(scene, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if EffectiveSupersample(cfg.Supersample, 0) != cfg.Supersample {
		t.Fatal("θ=0 反解值必须等于配置子采样数")
	}
	// 再跑一次并逐字节比对（确定性校验）。
	b, err := Run(scene, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Pix) != len(b.Pix) {
		t.Fatal("像素缓冲长度不一致")
	}
	for i := range a.Pix {
		if a.Pix[i] != b.Pix[i] {
			t.Fatalf("第 %d 个像素不一致: %d != %d", i, a.Pix[i], b.Pix[i])
		}
	}
}

func onSeg(a, b, p geom.Point) bool {
	return math.Min(a.X, b.X)-1e-12 <= p.X && p.X <= math.Max(a.X, b.X)+1e-12 &&
		math.Min(a.Y, b.Y)-1e-12 <= p.Y && p.Y <= math.Max(a.Y, b.Y)+1e-12
}

func orient2(a, b, c geom.Point) float64 {
	return (b.X-a.X)*(c.Y-a.Y) - (b.Y-a.Y)*(c.X-a.X)
}

func segCross(a, b, c, d geom.Point) bool {
	o1, o2, o3, o4 := orient2(a, b, c), orient2(a, b, d), orient2(c, d, a), orient2(c, d, b)
	if o1 == 0 && onSeg(a, b, c) || o2 == 0 && onSeg(a, b, d) ||
		o3 == 0 && onSeg(c, d, a) || o4 == 0 && onSeg(c, d, b) {
		return true
	}
	return (o1 > 0) != (o2 > 0) && (o3 > 0) != (o4 > 0)
}

// 无限采样极限下，像素单元与多边形是否相交（精确几何判定）。
func cellHits(poly []geom.Point, x0, y0, x1, y1 float64) bool {
	corners := []geom.Point{{X: x0, Y: y0}, {X: x1, Y: y0}, {X: x1, Y: y1}, {X: x0, Y: y1}}
	for _, c := range corners {
		if geom.Inside(poly, c.X, c.Y) {
			return true
		}
	}
	for _, v := range poly {
		if v.X >= x0-1e-12 && v.X <= x1+1e-12 && v.Y >= y0-1e-12 && v.Y <= y1+1e-12 {
			return true
		}
	}
	rect := append(corners, corners[0])
	for i := 0; i+1 < len(rect); i++ {
		for j := range poly {
			if segCross(rect[i], rect[i+1], poly[j], poly[(j+1)%len(poly)]) {
				return true
			}
		}
	}
	return false
}

// 摩尔纹像素 = 实际二值结果与无限采样极限理想掩膜逐像素比对的失配数。
// 预测区间取几何上界 [0, 2B/N(θ)]，B 为与多边形边界相交的像素单元数。
func TestMoireCountWithinPredictedRange(t *testing.T) {
	scene := [][]geom.Point{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}}
	const W, H = 64, 64

	for _, deg := range []float64{5, 30} {
		theta := deg * math.Pi / 180
		moved := Transform(append([]geom.Point{}, scene[0]...), 1.0, theta)

		minx, miny := moved[0].X, moved[0].Y
		maxx, maxy := minx, miny
		for _, q := range moved {
			minx, maxx = math.Min(minx, q.X), math.Max(maxx, q.X)
			miny, maxy = math.Min(miny, q.Y), math.Max(maxy, q.Y)
		}
		spanx, spany := maxx-minx, maxy-miny

		ideal := make([]bool, W*H)
		B := 0 // B：与多边形边界相交（四个角点不全在内部）的像素单元数
		for row := 0; row < H; row++ {
			for col := 0; col < W; col++ {
				x0 := minx + float64(col)*spanx/W
				x1 := minx + float64(col+1)*spanx/W
				y0 := miny + float64(row)*spany/H
				y1 := miny + float64(row+1)*spany/H
				hit := cellHits(moved, x0, y0, x1, y1)
				ideal[row*W+col] = hit
				if hit {
					allIn := true
					for _, cn := range [4]geom.Point{{X: x0, Y: y0}, {X: x1, Y: y0}, {X: x1, Y: y1}, {X: x0, Y: y1}} {
						if !geom.Inside(moved, cn.X, cn.Y) {
							allIn = false
						}
					}
					if !allIn {
						B++
					}
				}
			}
		}

		N := EffectiveSupersample(2, theta)
		cfg := config.Default()
		cfg.Width, cfg.Height = W, H
		cfg.Rotate = theta
		im, err := Run(scene, cfg)
		if err != nil {
			t.Fatal(err)
		}
		moire, fp := 0, 0
		for i, want := range ideal {
			got := im.Pix[i] != 0
			if want != got {
				moire++
				if got {
					fp++
				}
			}
		}
		upper := 2.0 * float64(B) / float64(N)
		t.Logf("θ=%d° N=%d B=%d 摩尔纹像素=%d (其中误亮=%d) 预测区间[0, %.0f]",
			int(deg), N, B, moire, fp, upper)
		if moire < 0 || float64(moire) > upper {
			t.Fatalf("θ=%d° 摩尔纹 %d 超出预测区间 [0, %.0f]", int(deg), moire, upper)
		}
	}
}

// 六参数仿射 X=ax+by+e, Y=cx+dy+f：用三个角点解出后回代四个角点。
func TestAffineSixParamsBackSubstitute(t *testing.T) {
	theta := 5 * math.Pi / 180
	c, s := math.Cos(theta), math.Sin(theta)
	src := [4]geom.Point{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}
	var dst [4]geom.Point
	for i, p := range src {
		dst[i] = geom.Point{X: p.X*c - p.Y*s, Y: p.X*s + p.Y*c}
	}
	e, f := dst[0].X, dst[0].Y
	a := (dst[1].X - e) / 10
	cc := (dst[1].Y - f) / 10
	b := (dst[3].X - e) / 10
	d := (dst[3].Y - f) / 10

	for i, p := range src {
		X := a*p.X + b*p.Y + e
		Y := cc*p.X + d*p.Y + f
		res := math.Hypot(X-dst[i].X, Y-dst[i].Y)
		t.Logf("角点%d (%.0f,%.0f) 回代(%.10f,%.10f) 期望(%.10f,%.10f) 残差%.2e",
			i+1, p.X, p.Y, X, Y, dst[i].X, dst[i].Y, res)
		if res > 1e-12 {
			t.Fatalf("角点%d 回代残差过大: %.2e", i+1, res)
		}
	}
}
