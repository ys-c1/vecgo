// Command aastop measures how fast anti-aliasing coverage error falls as
// samples are added, and derives the sample budget where the marginal
// benefit drops below a relative-variance threshold.
//
// Method: a rotated square is rasterised on an n x n sample grid exactly as
// raster.Image.Fill does. For every pixel that the outline crosses, the true
// coverage is computed by exact polygon clipping (geom.Clip). V(N) is the
// mean squared coverage error over those edge pixels, N = n*n. V is recorded
// for every n in 1..nmax (continuous sampling, no endpoint shortcuts), and
// fitted to V = C*N^-p in log-log space.
//
// Marginal benefit per added sample: MB(N) = -dV/dN = p*V(N)/N.
// Relative marginal benefit:          b(N) = MB(N)/V(N) = p/N.
// Stop point with threshold eps:      N* = p/eps, n* = ceil(sqrt(N*)).
//
// Usage:
//
//	go run ./tools/aastop -sizes 64,128,256 -angle 30 -nmax 32 -csv out.csv
package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"

	"vecgo/geom"
	"vecgo/raster"
)

type edge struct {
	x, y  int
	truth float64 // exact area, 0 < truth < 1
}

type series struct {
	W, L  int
	Perim float64
	V     []float64 // float coverage variance, index n
	Vq    []float64 // 8-bit coverage variance (raster.Fill truncation)
}

// scene builds a square of side side(W), rotated by ang, centred in W x W.
// The rotation angle is measured so the axis-aligned bounding box fits W x W.
func scene(W int, ang float64) []geom.Point {
	c, s := math.Cos(ang), math.Sin(ang)
	side := float64(W) / (math.Abs(c) + math.Abs(s))
	h := side / 2
	cx, cy := float64(W)/2, float64(W)/2
	b := [][2]float64{{-h, -h}, {h, -h}, {h, h}, {-h, h}}
	out := make([]geom.Point, 4)
	for i, p := range b {
		out[i] = geom.Point{X: cx + p[0]*c - p[1]*s, Y: cy + p[0]*s + p[1]*c}
	}
	return out
}

func cell(x, y int) []geom.Point {
	return []geom.Point{
		{X: float64(x), Y: float64(y)},
		{X: float64(x + 1), Y: float64(y)},
		{X: float64(x + 1), Y: float64(y + 1)},
		{X: float64(x), Y: float64(y + 1)},
	}
}

func area(p []geom.Point) float64 {
	if len(p) < 3 {
		return 0
	}
	a := 0.0
	for i := range p {
		j := (i + 1) % len(p)
		a += p[i].X*p[j].Y - p[j].X*p[i].Y
	}
	return math.Abs(a) / 2
}

func measure(W int, angDeg float64, nmax int) (*series, error) {
	ang := angDeg * math.Pi / 180
	sc := scene(W, ang)
	var edges []edge
	for y := 0; y < W; y++ {
		for x := 0; x < W; x++ {
			cl, err := geom.Clip(cell(x, y), sc)
			if err != nil {
				return nil, err
			}
			a := area(cl)
			if a > 1e-9 && a < 1-1e-9 {
				edges = append(edges, edge{x, y, a})
			}
		}
	}
	s := &series{W: W, L: len(edges),
		Perim: 4 * float64(W) / (math.Abs(math.Cos(ang)) + math.Abs(math.Sin(ang))),
		V:     make([]float64, nmax+1), Vq: make([]float64, nmax+1)}
	if len(edges) == 0 {
		return s, nil // axis-aligned outline on pixel boundaries: no AA needed
	}
	counts := make([][]int, len(edges))
	for i := range counts {
		counts[i] = make([]int, nmax+1)
	}
	for ei, e := range edges {
		for n := 1; n <= nmax; n++ {
			hit := 0
			for sj := 0; sj < n; sj++ {
				py := float64(e.y) + (float64(sj)+0.5)/float64(n)
				for si := 0; si < n; si++ {
					px := float64(e.x) + (float64(si)+0.5)/float64(n)
					if geom.Inside(sc, px, py) {
						hit++
					}
				}
			}
			counts[ei][n] = hit
		}
	}
	for n := 1; n <= nmax; n++ {
		var sv, svq float64
		for ei, e := range edges {
			h := counts[ei][n]
			c := float64(h) / float64(n*n)
			d := c - e.truth
			sv += d * d
			// exact replica of raster.Fill's uint8 truncation
			q := float64(uint8(255*h/(n*n))) / 255
			dq := q - e.truth
			svq += dq * dq
		}
		s.V[n] = sv / float64(len(edges))
		s.Vq[n] = svq / float64(len(edges))
	}
	return s, nil
}

// fitPower fits V = C*N^-p by least squares in log-log space.
func fitPower(V []float64, n0, n1 int) (p, C, r2, se float64) {
	var xs, ys []float64
	for n := n0; n <= n1; n++ {
		if V[n] <= 0 {
			continue
		}
		xs = append(xs, math.Log(float64(n*n)))
		ys = append(ys, math.Log(V[n]))
	}
	if len(xs) < 3 {
		return math.NaN(), math.NaN(), math.NaN(), math.NaN()
	}
	m := float64(len(xs))
	var sx, sy, sxx, sxy float64
	for i := range xs {
		sx += xs[i]
		sy += ys[i]
		sxx += xs[i] * xs[i]
		sxy += xs[i] * ys[i]
	}
	slope := (m*sxy - sx*sy) / (m*sxx - sx*sx)
	inter := (sy - slope*sx) / m
	var ssr, sst float64
	for i := range xs {
		pred := inter + slope*xs[i]
		ssr += (ys[i] - pred) * (ys[i] - pred)
		sst += (ys[i] - sy/m) * (ys[i] - sy/m)
	}
	se = math.Sqrt((ssr / (m - 2)) / (m*sxx - sx*sx))
	return -slope, math.Exp(inter), 1 - ssr/sst, se
}

// smooth3 is the 3-point geometric mean of V; it de-noises the lattice
// resonance in V(n) without dropping any recorded n.
func smooth3(V []float64, n, nmax int) float64 {
	lo, hi := n-1, n+1
	if lo < 1 {
		lo = 1
	}
	if hi > nmax {
		hi = nmax
	}
	s, k := 0.0, 0
	for i := lo; i <= hi; i++ {
		s += math.Log(V[i])
		k++
	}
	return math.Exp(s / float64(k))
}

type row struct {
	W      int
	L      int
	P, C   float64
	R2, SE float64
	NStar  float64
	N      int // practical grid n*
	V, Vq  float64
}

func main() {
	sizes := flag.String("sizes", "64,128,256", "frame side lengths in pixels")
	angDeg := flag.Float64("angle", 30, "outline rotation in degrees")
	nmax := flag.Int("nmax", 32, "largest n (n x n grid)")
	csvPath := flag.String("csv", "", "write the full continuous record here")
	flag.Parse()
	eps := 0.01 // stop when relative variance decrease per added sample < 1%

	var ws []int
	for _, f := range strings.Split(*sizes, ",") {
		w, err := strconv.Atoi(strings.TrimSpace(f))
		if err != nil {
			panic(err)
		}
		ws = append(ws, w)
	}
	ser := make([]*series, 0, len(ws))
	var rows []row
	var rec [][7]float64
	for _, W := range ws {
		s, err := measure(W, *angDeg, *nmax)
		if err != nil {
			fmt.Fprintln(os.Stderr, "测量失败:", err)
			os.Exit(1)
		}
		ser = append(ser, s)
		if s.L == 0 {
			fmt.Printf("W=%d: 轮廓与像素边界重合，L=0，无需抗锯齿\n", W)
			rows = append(rows, row{W: W})
			continue
		}
		p, C, r2, se := fitPower(s.V, 4, 20)
		ns := p / eps
		n := int(math.Ceil(math.Sqrt(ns)))
		rows = append(rows, row{W, s.L, p, C, r2, se, ns, n, s.V[n], s.Vq[n]})
		for k := 1; k <= *nmax; k++ {
			var bStep, bFit float64 = math.NaN(), math.NaN()
			if k >= 2 && s.V[k-1] > 0 {
				bStep = (s.V[k-1] - s.V[k]) / float64(k*k-(k-1)*(k-1)) / s.V[k-1]
			}
			if !math.IsNaN(p) {
				bFit = p / float64(k*k)
			}
			rec = append(rec, [7]float64{float64(W), float64(k), float64(k * k), float64(s.L), s.V[k], s.Vq[k], bStep})
			_ = bFit
		}
	}

	// Cross-check the simulator against the real rasteriser, so the numbers
	// describe raster.Image.Fill and not a look-alike.
	if len(ser) > 0 && ser[0].L > 0 {
		W := ser[0].W
		sc := scene(W, *angDeg*math.Pi/180)
		im, err := raster.New(W, W)
		if err != nil {
			panic(err)
		}
		if err := im.Fill([][]geom.Point{sc}, 14); err != nil {
			panic(err)
		}
		bad := 0
		for y := 0; y < W; y++ {
			for x := 0; x < W; x++ {
				hit := 0
				for sj := 0; sj < 14; sj++ {
					py := float64(y) + (float64(sj)+0.5)/14
					for si := 0; si < 14; si++ {
						px := float64(x) + (float64(si)+0.5)/14
						if geom.Inside(sc, px, py) {
							hit++
						}
					}
				}
				if im.Pix[y*W+x] != uint8(255*hit/(14*14)) {
					bad++
				}
			}
		}
		fmt.Printf("交叉校验 raster.Fill(W=%d,n=14): 不一致像素 %d\n", W, bad)
	}

	fmt.Printf("\n几何: 正方形旋转 %.0f°，包围盒 W×W；真值=精确裁剪面积；连续记录 n=1..%d\n", *angDeg, *nmax)
	for i, s := range ser {
		if s.L == 0 {
			continue
		}
		r := rows[i]
		fmt.Printf("\n=== W=%d L=%d perimeter=%.1f ===\n", s.W, s.L, s.Perim)
		fmt.Printf("%3s %7s %12s %12s %12s %12s\n", "n", "N", "V", "Vq", "b_step", "b_fit")
		for k := 1; k <= *nmax; k++ {
			var bs, bf float64 = math.NaN(), math.NaN()
			if k >= 2 {
				bs = (s.V[k-1] - s.V[k]) / float64(k*k-(k-1)*(k-1)) / s.V[k-1]
			}
			bf = r.P / float64(k*k)
			fmt.Printf("%3d %7d %12.5e %12.5e %11.3f%% %11.3f%%\n", k, k*k, s.V[k], s.Vq[k], 100*bs, 100*bf)
		}
		fmt.Printf("拟合 n=4..20: p=%.4f±%.4f C=%.4g R2=%.5f\n", r.P, r.SE, r.C, r.R2)
		fmt.Printf("N*=p/ε=%.1f → n*=ceil(sqrt(N*))=%d, N=%d\n", r.NStar, r.N, r.N*r.N)
		fmt.Printf("收益（从 n*=14, N=196 继续加样）:\n")
		n0 := r.N
		for _, nn := range []int{16, 20, 24, 28, 32} {
			if nn <= n0 || nn > *nmax {
				continue
			}
			dN := nn*nn - n0*n0
			fmt.Printf("  n=%2d N=%4d 样本 +%3d/像素 (+%4.0f%%, 全图 +%7d): ΔV=%+7.1f%%  ΔVq=%+7.1f%%  MB_fit=%.3f%%/样本  MB_abs=%.2e/样本\n",
				nn, nn*nn, dN, 100*float64(dN)/float64(n0*n0), dN*s.L,
				100*(s.V[nn]-s.V[n0])/s.V[n0], 100*(s.Vq[nn]-s.Vq[n0])/s.Vq[n0],
				100*r.P/float64(nn*nn), r.P*r.C*math.Pow(float64(nn*nn), -r.P-1))
		}
		half := r.NStar * math.Pow(2, 1/r.P)
		fmt.Printf("方差减半所需: N=%.0f→%.0f (×%.2f 样本, 每边缘像素; 全图 ×%.2f), 即 n 从 %d 到 %.1f\n",
			r.NStar, half, math.Pow(2, 1/r.P), math.Pow(2, 1/r.P), r.N, math.Sqrt(half))
	}

	fmt.Printf("\n=== 停止点汇总（每档一行）===\n")
	fmt.Printf("%4s %5s %7s %7s %5s %6s %11s %11s %11s %11s %9s %8s %8s %8s\n",
		"W", "L", "p", "R²", "n*", "N*", "V_float", "RMSE", "Vq_8bit", "RMSEq", "S*", "S*/L", "S*/W²", "S*/P")
	for _, r := range rows {
		if r.L == 0 {
			continue
		}
		s := serOf(ser, r.W)
		fmt.Printf("%4d %5d %7.3f %7.4f %5d %6d %11.3e %11.3e %11.3e %11.3e %9.0f %8.1f %8.2f %8.1f\n",
			r.W, r.L, r.P, r.R2, r.N, r.N*r.N, r.V, math.Sqrt(r.V), r.Vq, math.Sqrt(r.Vq),
			float64(r.N*r.N*r.L), float64(r.N*r.N), float64(r.N*r.N*r.L)/float64(r.W*r.W), float64(r.N*r.N*r.L)/s.Perim)
	}

	if *csvPath != "" {
		f, err := os.Create(*csvPath)
		if err != nil {
			panic(err)
		}
		w := csv.NewWriter(f)
		w.Write([]string{"W", "n", "N", "L", "V_float", "Vq_8bit", "b_step"})
		for _, r := range rec {
			vals := make([]string, len(r))
			for i, v := range r {
				vals[i] = strconv.FormatFloat(v, 'g', -1, 64)
			}
			w.Write(vals)
		}
		w.Flush()
		f.Close()
		fmt.Printf("\n连续记录已写入 %s (%d 行)\n", *csvPath, len(rec))
	}
}

func serOf(all []*series, W int) *series {
	for _, s := range all {
		if s.W == W {
			return s
		}
	}
	return nil
}
