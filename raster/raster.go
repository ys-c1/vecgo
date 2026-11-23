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

func New(w, h int) (*Image, error) {
	if w <= 0 || h <= 0 {
		return nil, errors.New("画幅必须为正")
	}
	return &Image{W: w, H: h, Pix: make([]uint8, w*h)}, nil
}

func (im *Image) Fill(polys [][]geom.Point, supersample int) error {
	if supersample < 1 {
		return errors.New("supersample 必须为正")
	}
	for _, p := range polys {
		if len(p) < 3 {
			return errors.New("多边形至少三个顶点")
		}
	}
	minx, miny, spanx, spany := sceneBounds(polys)
	den := supersample * supersample
	for row := 0; row < im.H; row++ {
		for col := 0; col < im.W; col++ {
			hit := 0
			for sj := 0; sj < supersample; sj++ {
				for si := 0; si < supersample; si++ {
					px := minx + (float64(col)+(float64(si)+0.5)/float64(supersample))*spanx/float64(im.W)
					py := miny + (float64(row)+(float64(sj)+0.5)/float64(supersample))*spany/float64(im.H)
					for _, p := range polys {
						if geom.Inside(p, px, py) {
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

// sceneBounds is the scene-wide bounding box, shared by Fill and FillCached.
// Because it is scene-wide, a subsample's world position depends only on the
// grid, not on which triangles are present — the property the coverage cache
// relies on.
func sceneBounds(polys [][]geom.Point) (minx, miny, spanx, spany float64) {
	minx, miny = polys[0][0].X, polys[0][0].Y
	maxx, maxy := minx, miny
	for _, p := range polys {
		for _, q := range p {
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
	spanx, spany = maxx-minx, maxy-miny
	if spanx == 0 {
		spanx = 1
	}
	if spany == 0 {
		spany = 1
	}
	return minx, miny, spanx, spany
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

var errCacheTriangle = errors.New("覆盖率缓存只支持三角形")

// samplePos is the world position of subsample (si,sj) of pixel (col,row),
// computed exactly as Fill does.
func samplePos(col, row, si, sj, ss, w, h int, minx, miny, spanx, spany float64) (float64, float64) {
	px := minx + (float64(col)+(float64(si)+0.5)/float64(ss))*spanx/float64(w)
	py := miny + (float64(row)+(float64(sj)+0.5)/float64(ss))*spany/float64(h)
	return px, py
}

// triangleMask records, at subsample resolution, which samples of the grid lie
// inside the triangle. Bit index is (row*ss+sj)*W*ss + col*ss+si, matching the
// loops in Fill, so OR of masks counts distinct containing triangles and stays
// identical to Fill's "any polygon contains this sample".
func triangleMaskInto(mask []uint64, t []geom.Point, ss, w, h int, minx, miny, spanx, spany float64) {
	for i := range mask {
		mask[i] = 0
	}
	bit := 0
	for row := 0; row < h; row++ {
		for sj := 0; sj < ss; sj++ {
			for col := 0; col < w; col++ {
				for si := 0; si < ss; si++ {
					px, py := samplePos(col, row, si, sj, ss, w, h, minx, miny, spanx, spany)
					if geom.Inside(t, px, py) {
						mask[bit>>6] |= 1 << uint(bit&63)
					}
					bit++
				}
			}
		}
	}
}

// FillCached renders triangles through the coverage cache. Polys must all be
// triangles. With cache nil it is exactly Fill (the no-cache control). Pixel
// output is byte-identical either way: a hit reuses a mask, a miss computes it,
// and both are OR-combined and quantised by the same code. Entry numbers used
// in the stats are 0-based occurrences in polys.
func (im *Image) FillCached(polys [][]geom.Point, supersample int, cache *Cache) error {
	if supersample < 1 {
		return errors.New("supersample 必须为正")
	}
	for _, p := range polys {
		if len(p) < 3 {
			return errors.New("多边形至少三个顶点")
		}
	}
	if cache == nil {
		return im.Fill(polys, supersample)
	}
	for _, p := range polys {
		if len(p) != 3 {
			return errCacheTriangle
		}
	}
	minx, miny, spanx, spany := sceneBounds(polys)
	cache.configure(im.W, im.H, supersample, minx, miny, spanx, spany)
	words := cache.maskWords()
	// The accumulator and the mask being computed are reused across runs, so
	// a run allocates only when it stores a mask (store takes a copy).
	total := cache.accumulate(words)
	for i := range total {
		total[i] = 0
	}
	for seq, t := range polys {
		cache.noteDegenerate(seq, t)
		k, err := TriangleKey(t)
		if err != nil {
			return err
		}
		_, hit := cache.lookup(seq, k)
		if hit {
			// Reuse the stored mask; it is read-only here.
			mask := cache.planes[k]
			for i := range total {
				total[i] |= mask[i]
			}
			continue
		}
		mask := cache.scratch(words)
		triangleMaskInto(mask, t, supersample, im.W, im.H, minx, miny, spanx, spany)
		cache.store(seq, k, mask)
		// The mask is OR-combined whatever store decided (stored, evicted,
		// rejected), so this run's output never depends on the policy.
		for i := range total {
			total[i] |= mask[i]
		}
	}
	cache.notePeak()
	den := supersample * supersample
	for row := 0; row < im.H; row++ {
		for col := 0; col < im.W; col++ {
			hit := 0
			for sj := 0; sj < supersample; sj++ {
				for si := 0; si < supersample; si++ {
					bit := (row*supersample+sj)*im.W*supersample + col*supersample + si
					if total[bit>>6]&(1<<uint(bit&63)) != 0 {
						hit++
					}
				}
			}
			im.Pix[row*im.W+col] = uint8(255 * hit / den)
		}
	}
	return nil
}
