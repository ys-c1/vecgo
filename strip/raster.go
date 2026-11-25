package strip

import (
	"image"
	"image/color"
	"image/png"
	"os"
)

// RasterConfig controls the exhaustive output-grid and subpixel grid.
type RasterConfig struct {
	Width       int
	Height      int
	SuperSample int
}

// RasterResult contains pixels plus full-grid equality evidence.
type RasterResult struct {
	Image       *image.Gray
	PixelCount  int
	SampleCount int
	Covered     int
}

func edgeSign(a, b, p Point) float64 {
	return (b.X-a.X)*(p.Y-a.Y) - (b.Y-a.Y)*(p.X-a.X)
}

// InsideSample uses a strict interior convention. A zero-area triangle can
// never satisfy "all three edge signs are strictly positive or all negative".
func InsideSample(t Triangle, p Point) bool {
	s1 := edgeSign(t.A, t.B, p)
	s2 := edgeSign(t.B, t.C, p)
	s3 := edgeSign(t.C, t.A, p)
	if s1 == 0 || s2 == 0 || s3 == 0 {
		return false
	}
	return (s1 > 0 && s2 > 0 && s3 > 0) || (s1 < 0 && s2 < 0 && s3 < 0)
}

// Render exhaustively evaluates every output pixel and every regular
// supersample point. There is no random or sparse sampling.
func Render(tris []Triangle, box Box, cfg RasterConfig) RasterResult {
	ss := cfg.SuperSample
	if ss < 1 {
		ss = 1
	}
	img := image.NewGray(image.Rect(0, 0, cfg.Width, cfg.Height))
	for i := range img.Pix {
		img.Pix[i] = 255
	}
	den := ss * ss
	coveredPixels := 0
	sampleCount := cfg.Width * cfg.Height * den

	for row := 0; row < cfg.Height; row++ {
		for col := 0; col < cfg.Width; col++ {
			hits := 0
			for sj := 0; sj < ss; sj++ {
				for si := 0; si < ss; si++ {
					fx := (float64(col) + (float64(si)+0.5)/float64(ss)) / float64(cfg.Width)
					fy := (float64(row) + (float64(sj)+0.5)/float64(ss)) / float64(cfg.Height)
					p := Point{
						X: box.MinX + fx*(box.MaxX-box.MinX),
						// Image rows run downward; grid Y runs upward.
						Y: box.MaxY - fy*(box.MaxY-box.MinY),
					}
					for _, t := range tris {
						if InsideSample(t, p) {
							hits++
							break
						}
					}
				}
			}
			v := uint8(255 - 255*hits/den)
			img.SetGray(col, row, color.Gray{Y: v})
			if hits > 0 {
				coveredPixels++
			}
		}
	}
	return RasterResult{
		Image:       img,
		PixelCount:  cfg.Width * cfg.Height,
		SampleCount: sampleCount,
		Covered:     coveredPixels,
	}
}

// PixelDiff returns the number of unequal gray pixels and a black/white diff.
func PixelDiff(a, b *image.Gray) (int, *image.Gray) {
	r := a.Bounds()
	diff := image.NewGray(r)
	n := 0
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			av := a.GrayAt(x, y).Y
			bv := b.GrayAt(x, y).Y
			if av != bv {
				n++
				diff.SetGray(x, y, color.Gray{Y: 0})
			} else {
				diff.SetGray(x, y, color.Gray{Y: 255})
			}
		}
	}
	return n, diff
}

// SavePNG writes an image as PNG.
func SavePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
