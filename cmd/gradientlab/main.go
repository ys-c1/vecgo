package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"

	"vecgo/geom"
	"vecgo/shading"
)

type caseSpec struct {
	ID       string
	Label    string
	Poly     []geom.Point
	Gradient shading.RadialGradient
}

func regularPolygon(n int, cx, cy, r float64) []geom.Point {
	const start = -1.5707963267948966 // -pi/2，顶点朝上
	out := make([]geom.Point, n)
	for i := 0; i < n; i++ {
		a := start + 6.283185307179586*float64(i)/float64(n)
		out[i] = geom.Point{X: cx + r*math.Cos(a), Y: cy + r*math.Sin(a)}
	}
	return out
}

func main() {
	out := flag.String("out", "artifacts/gradient", "输出文件前缀")
	size := flag.Int("size", 192, "画布边长")
	ss := flag.Int("ss", 4, "超采样倍数")
	flag.Parse()

	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		fatal(err)
	}

	cx, cy := float64(*size)/2, float64(*size)/2
	cases := []caseSpec{
		{
			ID:    "case-1-hexagon",
			Label: "hexagon warm",
			Poly:  regularPolygon(6, cx, cy, 70),
			Gradient: shading.RadialGradient{
				Center: geom.Point{X: cx, Y: cy}, Radius: 42,
				Inner: shading.RGB{R: 28.0 / 255, G: 10.0 / 255, B: 0},
				Outer: shading.RGB{R: 255.0 / 255, G: 214.0 / 255, B: 178.0 / 255},
			},
		},
		{
			ID:    "case-2-pentagon",
			Label: "pentagon ice",
			Poly:  regularPolygon(5, cx, cy+2, 68),
			Gradient: shading.RadialGradient{
				Center: geom.Point{X: cx, Y: cy + 2}, Radius: 39,
				Inner: shading.RGB{R: 0, G: 42.0 / 255, B: 58.0 / 255},
				Outer: shading.RGB{R: 186.0 / 255, G: 226.0 / 255, B: 238.0 / 255},
			},
		},
		{
			ID:    "case-3-dodecagon",
			Label: "dodecagon sage",
			Poly:  regularPolygon(12, cx, cy, 72),
			Gradient: shading.RadialGradient{
				Center: geom.Point{X: cx, Y: cy}, Radius: 50,
				Inner: shading.RGB{R: 20.0 / 255, G: 28.0 / 255, B: 8.0 / 255},
				Outer: shading.RGB{R: 224.0 / 255, G: 238.0 / 255, B: 196.0 / 255},
			},
		},
	}

	metrics := make([]shading.Metrics, 0, len(cases))
	for _, tc := range cases {
		before := shading.RenderRadial(tc.Poly, tc.Gradient, *size, *size, *ss, false)
		after := shading.RenderRadial(tc.Poly, tc.Gradient, *size, *size, *ss, true)
		overflow := shading.RenderOverflow(tc.Poly, tc.Gradient, after)
		diff := shading.DiffHeatmap(before, after)
		m := shading.MeasureRadial(tc.ID, tc.Label, tc.Poly, tc.Gradient, before, after, *ss)
		metrics = append(metrics, m)

		writePNG(*out+"-"+tc.ID+"-before.png", before)
		writePNG(*out+"-"+tc.ID+"-after.png", after)
		writePNG(*out+"-"+tc.ID+"-overflow.png", overflow)
		writePNG(*out+"-"+tc.ID+"-diff.png", diff)
		fmt.Printf("%s: overflow=%d max_delta=%d white %d->%d bound<=%d\n",
			tc.ID, m.ParameterOverflowPixels, m.MaxPixelDelta,
			m.WhitePixelsBefore, m.WhitePixelsAfter, m.Bound8Bit)
	}

	payload, err := json.MarshalIndent(map[string]any{
		"width": *size, "height": *size, "supersample": *ss, "cases": metrics,
	}, "", "  ")
	if err != nil {
		fatal(err)
	}
	if err := os.WriteFile(*out+"-metrics.json", payload, 0o644); err != nil {
		fatal(err)
	}
}

func writePNG(path string, im image.Image) {
	f, err := os.Create(path)
	if err != nil {
		fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, im); err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "失败:", err)
	os.Exit(1)
}
