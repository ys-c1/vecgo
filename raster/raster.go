package raster

import (
	"errors"
	"fmt"
	"os"

	"vecgo/geom"
)

type Image struct {
	W, H int
	Pix  []uint8
}

// Frame is the sampling grid a Fill uses. The polygon bounding box is mapped
// onto the image so that the box fills the image exactly; Sample returns the
// world-space point for one supersample. Exposing the frame lets analysis code
// reproduce the renderer's samples bit for bit.
type Frame struct {
	MinX, MinY   float64
	SpanX, SpanY float64
	W, H         int
}

// Bounds builds the sampling frame for polys. Degenerate extents are widened
// to 1 so the frame stays invertible, matching Fill's historical behaviour.
func Bounds(polys [][]geom.Point) Frame {
	f := Frame{}
	first := true
	minx, miny, maxx, maxy := 0.0, 0.0, 0.0, 0.0
	for _, p := range polys {
		for _, q := range p {
			if first {
				minx, miny, maxx, maxy = q.X, q.Y, q.X, q.Y
				first = false
				continue
			}
			if q.X < minx {
				minx = q.X
			}
			if q.X > maxx {
				maxx = q.X
			}
			if q.Y < miny {
				miny = q.Y
			}
			if q.Y > maxy {
				maxy = q.Y
			}
		}
	}
	f.MinX, f.MinY = minx, miny
	f.SpanX, f.SpanY = maxx-minx, maxy-miny
	if f.SpanX == 0 {
		f.SpanX = 1
	}
	if f.SpanY == 0 {
		f.SpanY = 1
	}
	return f
}

// Sample returns the world-space sample at supersample (si,sj) of pixel
// (col,row), with ss samples per axis.
func (f Frame) Sample(col, row, si, sj, ss int) geom.Point {
	return geom.Point{
		X: f.MinX + (float64(col)+(float64(si)+0.5)/float64(ss))*f.SpanX/float64(f.W),
		Y: f.MinY + (float64(row)+(float64(sj)+0.5)/float64(ss))*f.SpanY/float64(f.H),
	}
}

func New(w, h int) (*Image, error) {
	if w <= 0 || h <= 0 {
		return nil, errors.New("画幅必须为正")
	}
	return &Image{W: w, H: h, Pix: make([]uint8, w*h)}, nil
}

// Fill paints polys with the default fill rule, NonZero.
func (im *Image) Fill(polys [][]geom.Point, supersample int) error {
	return im.FillRule(polys, supersample, geom.NonZero)
}

// FillRule paints polys with an explicit fill rule. The rule is applied inside
// each polygon; polygons are unioned, matching the historical behaviour.
func (im *Image) FillRule(polys [][]geom.Point, supersample int, rule geom.FillRule) error {
	if supersample < 1 {
		return errors.New("supersample 必须为正")
	}
	if rule != geom.NonZero && rule != geom.EvenOdd {
		return fmt.Errorf("未知填充规则: %v", rule)
	}
	for _, p := range polys {
		if len(p) < 3 {
			return errors.New("多边形至少三个顶点")
		}
	}
	if len(polys) == 0 {
		return nil
	}
	f := Bounds(polys)
	f.W, f.H = im.W, im.H
	den := supersample * supersample
	for row := 0; row < im.H; row++ {
		for col := 0; col < im.W; col++ {
			hit := 0
			for sj := 0; sj < supersample; sj++ {
				for si := 0; si < supersample; si++ {
					p := f.Sample(col, row, si, sj, supersample)
					for _, poly := range polys {
						if geom.InsideRule(poly, p.X, p.Y, rule) {
							hit++
							break
						}
					}
				}
			}
			im.Pix[row*im.W+col] = uint8(255 * hit / den)
		}
	}
	return nil
}

func (im *Image) SavePGM(path string) error {
	if im.W == 0 || im.H == 0 {
		return errors.New("空图不能导出")
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := fmt.Fprintf(f, "P5\n%d %d\n255\n", im.W, im.H); err != nil {
		return err
	}
	_, err = f.Write(im.Pix)
	return err
}

func (im *Image) CountOn(threshold uint8) int {
	n := 0
	for _, v := range im.Pix {
		if v >= threshold {
			n++
		}
	}
	return n
}

// CountCovered returns the number of pixels with any coverage.
func (im *Image) CountCovered() int {
	n := 0
	for _, v := range im.Pix {
		if v > 0 {
			n++
		}
	}
	return n
}
