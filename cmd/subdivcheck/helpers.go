package main

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"

	"vecgo/raster"
	"vecgo/subdiv"
)

// sweepRow 是固定 n=2 时某个坐标分辨率下三组的实测误差与上界。
type sweepRow struct {
	Res    float64
	Delta  [3]float64
	Bound2 [3]float64
}

// coarseRow 记录“每级中点立即按整数像素取整”的复现结果。
type coarseRow struct {
	ID      string
	DeltaA1 float64
	DeltaA2 float64
	On128N1 int
	On128N2 int
	Diff128 int
	BitmapD int
}

func resolutionSweep(gs []groupDef, ress []float64) []sweepRow {
	rows := make([]sweepRow, 0, len(ress))
	for _, rv := range ress {
		r := sweepRow{Res: rv}
		for i, g := range gs {
			a0 := subdiv.Area(g.T)
			lcap := subdiv.EdgeCap(subdiv.LongestEdge(g.T), rv)
			snapped := subdiv.SnapAll(subdiv.Subdivide(g.T, 2), rv)
			r.Delta[i] = subdiv.TotalArea(snapped) - a0
			r.Bound2[i] = subdiv.TotalBound(2, rv, lcap)
		}
		rows = append(rows, r)
	}
	return rows
}

func coarseRecursiveDemo(gs []groupDef) []coarseRow {
	const coarseRes = 1.0
	rows := make([]coarseRow, 0, len(gs))
	for _, g := range gs {
		a0 := subdiv.Area(g.T)
		t1 := subdiv.SubdivideRounded(g.T, 1, coarseRes)
		t2 := subdiv.SubdivideRounded(g.T, 2, coarseRes)
		im1, err := subdiv.Coverage(t1, width, height, ss)
		if err != nil {
			fatal(err)
		}
		im2, err := subdiv.Coverage(t2, width, height, ss)
		if err != nil {
			fatal(err)
		}
		rows = append(rows, coarseRow{
			ID:      g.ID,
			DeltaA1: subdiv.TotalArea(t1) - a0,
			DeltaA2: subdiv.TotalArea(t2) - a0,
			On128N1: im1.CountOn(128),
			On128N2: im2.CountOn(128),
			Diff128: im2.CountOn(128) - im1.CountOn(128),
			BitmapD: subdiv.BitmapDifferentPixels(im1, im2),
		})
	}
	return rows
}

// overlayMesh 在灰度覆盖图上用红线画出所有三角形边。
func overlayMesh(im *raster.Image, tris []subdiv.Triangle) *image.RGBA {
	rgba := image.NewRGBA(image.Rect(0, 0, im.W, im.H))
	for y := 0; y < im.H; y++ {
		for x := 0; x < im.W; x++ {
			g := im.Pix[y*im.W+x]
			rgba.SetRGBA(x, y, color.RGBA{R: g, G: g, B: g, A: 255})
		}
	}
	edge := color.RGBA{R: 220, G: 40, B: 40, A: 255}
	for _, t := range tris {
		for i := 0; i < 3; i++ {
			p := t[i]
			q := t[(i+1)%3]
			drawLine(rgba, int(math.Round(p.X)), int(math.Round(p.Y)),
				int(math.Round(q.X)), int(math.Round(q.Y)), edge)
		}
	}
	return rgba
}

func drawLine(img *image.RGBA, x0, y0, x1, y1 int, c color.Color) {
	dx := absInt(x1 - x0)
	dy := -absInt(y1 - y0)
	sx := -1
	if x0 < x1 {
		sx = 1
	}
	sy := -1
	if y0 < y1 {
		sy = 1
	}
	err := dx + dy
	for {
		if 0 <= x0 && x0 < img.Bounds().Dx() && 0 <= y0 && y0 < img.Bounds().Dy() {
			img.Set(x0, y0, c)
		}
		if x0 == x1 && y0 == y1 {
			break
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func saveRGBA(img *image.RGBA, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

// writeStrip 把三组在同一 n 下的覆盖图横向拼成一张总览 PNG。
func writeStrip(outDir, name string, gs []groupDef, n int) error {
	const gap = 6
	strip := image.NewGray(image.Rect(0, 0, 3*width+2*gap, height))
	// 间隔填白，便于区分三组。
	for i := range strip.Pix {
		strip.Pix[i] = 255
	}
	for k, g := range gs {
		tris := subdiv.SnapAll(subdiv.Subdivide(g.T, n), res)
		im, err := subdiv.Coverage(tris, width, height, ss)
		if err != nil {
			return err
		}
		x0 := k * (width + gap)
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				strip.Pix[y*strip.Stride+x0+x] = im.Pix[y*width+x]
			}
		}
	}
	f, err := os.Create(filepath.Join(outDir, name))
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, strip)
}
