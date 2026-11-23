package raster

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"math/bits"

	"vecgo/cache"
	"vecgo/geom"
)

// maskVersion is bumped whenever the cached mask layout or the sampling
// formula changes, so stale entries can never be misinterpreted.
const maskVersion = 2

// maskHeaderWords is the number of uint64 words in a cached mask before the
// bit data: x0, y0, w, h, wordsPerPixel.
const maskHeaderWords = 5

// grid describes the sampling grid. Mask values are only valid for the exact
// grid they were built for, so its parameters are part of the cache key.
type grid struct {
	w, h         int
	ss           int
	minx, miny   float64
	spanx, spany float64
}

// wordsPerPixel returns how many uint64 words hold one pixel's samples.
func (g grid) wordsPerPixel() int {
	return (g.ss*g.ss + 63) / 64
}

// fillCached is the cache-backed equivalent of Fill. It must produce pixels
// byte-identical to Fill for every input.
//
// Each polygon is rasterized once into a per-pixel sample bitmask (one uint64
// per 64 samples). Masks are combined with bitwise OR, which reproduces
// Fill's "any polygon containing the sample counts once" rule exactly: the
// union of covered samples is counted once regardless of how many polygons
// cover it.
func (im *Image) fillCached(polys [][]geom.Point, supersample int, c *cache.Cache) error {
	if supersample < 1 {
		return errors.New("supersample 必须为正")
	}
	for _, p := range polys {
		if len(p) < 3 {
			return errors.New("多边形至少三个顶点")
		}
	}
	if c == nil {
		return &cache.EntryError{Op: "FillCached", Err: cache.ErrZeroCapacity}
	}
	if len(polys) == 0 {
		return errors.New("多边形列表不能为空")
	}
	minx, miny, maxx, maxy := polys[0][0].X, polys[0][0].Y, polys[0][0].X, polys[0][0].Y
	for _, p := range polys {
		for _, q := range p {
			minx = math.Min(minx, q.X)
			maxx = math.Max(maxx, q.X)
			miny = math.Min(miny, q.Y)
			maxy = math.Max(maxy, q.Y)
		}
	}
	spanx, spany := maxx-minx, maxy-miny
	if spanx == 0 {
		spanx = 1
	}
	if spany == 0 {
		spany = 1
	}
	g := grid{w: im.W, h: im.H, ss: supersample, minx: minx, miny: miny, spanx: spanx, spany: spany}

	acc := make([]uint64, im.W*im.H*g.wordsPerPixel())
	for _, poly := range polys {
		key := g.maskKey(poly)
		val, ok, err := c.Lookup(key)
		if err != nil {
			return err
		}
		if !ok {
			val = g.buildMask(poly)
			if err := c.Put(key, val); err != nil {
				return err
			}
		}
		if err := g.orMask(acc, val); err != nil {
			return err
		}
	}

	den := supersample * supersample
	wpp := g.wordsPerPixel()
	for i := 0; i < im.W*im.H; i++ {
		count := 0
		for w := 0; w < wpp; w++ {
			count += bits.OnesCount64(acc[i*wpp+w])
		}
		im.Pix[i] = uint8(255 * count / den)
	}
	return nil
}

// maskKey canonicalizes the polygon vertices and grid into a stable,
// printable key. Float bits are used verbatim so distinct coordinates never
// alias, and hex encoding keeps the key safe in reports and CSV.
func (g grid) maskKey(poly []geom.Point) string {
	var buf [8]byte
	b := make([]byte, 0, 48+16*len(poly))
	put := func(u uint64) {
		binary.LittleEndian.PutUint64(buf[:], u)
		b = append(b, buf[:]...)
	}
	put(maskVersion)
	put(uint64(uint32(g.w)))
	put(uint64(uint32(g.h)))
	put(uint64(uint32(g.ss)))
	for _, f := range []float64{g.minx, g.miny, g.spanx, g.spany} {
		put(math.Float64bits(f))
	}
	put(uint64(uint32(len(poly))))
	for _, p := range poly {
		put(math.Float64bits(p.X))
		put(math.Float64bits(p.Y))
	}
	return "v1:" + hex.EncodeToString(b)
}

// buildMask returns a mask value laid out as [x0, y0, w, h, wordsPerPixel,
// words...], where words covers the clipped rectangle row-major, pixel by
// pixel, and each pixel occupies wordsPerPixel words. Bit i of a pixel's words
// is the sample with si = i%ss, sj = i/ss within that pixel.
func (g grid) buildMask(poly []geom.Point) []uint64 {
	// Pixel window that can possibly contain the polygon.
	px0 := int(math.Floor((minX(poly)-g.minx)*float64(g.w)/g.spanx)) - 1
	px1 := int(math.Ceil((maxX(poly)-g.minx)*float64(g.w)/g.spanx)) + 1
	py0 := int(math.Floor((minY(poly)-g.miny)*float64(g.h)/g.spany)) - 1
	py1 := int(math.Ceil((maxY(poly)-g.miny)*float64(g.h)/g.spany)) + 1
	px0 = clamp(px0, 0, g.w)
	px1 = clamp(px1, 0, g.w)
	py0 = clamp(py0, 0, g.h)
	py1 = clamp(py1, 0, g.h)
	mw, mh := px1-px0, py1-py0
	wpp := g.wordsPerPixel()
	if mw <= 0 || mh <= 0 {
		return []uint64{uint64(px0), uint64(py0), 0, 0, uint64(wpp)}
	}
	out := make([]uint64, maskHeaderWords+mw*mh*wpp)
	out[0], out[1] = uint64(px0), uint64(py0)
	out[2], out[3] = uint64(mw), uint64(mh)
	out[4] = uint64(wpp)
	for row := py0; row < py1; row++ {
		for col := px0; col < px1; col++ {
			base := maskHeaderWords + ((row-py0)*mw+(col-px0))*wpp
			for sj := 0; sj < g.ss; sj++ {
				for si := 0; si < g.ss; si++ {
					px := g.minx + (float64(col)+(float64(si)+0.5)/float64(g.ss))*g.spanx/float64(g.w)
					py := g.miny + (float64(row)+(float64(sj)+0.5)/float64(g.ss))*g.spany/float64(g.h)
					if geom.Inside(poly, px, py) {
						bit := sj*g.ss + si
						out[base+bit/64] |= 1 << uint(bit%64)
					}
				}
			}
		}
	}
	return out
}

func (g grid) orMask(acc, val []uint64) error {
	if len(val) < maskHeaderWords {
		return errors.New("缓存条目损坏：缺少掩码头")
	}
	x0, y0 := int(val[0]), int(val[1])
	mw, mh := int(val[2]), int(val[3])
	wpp := int(val[4])
	if wpp != g.wordsPerPixel() {
		return fmt.Errorf("缓存条目损坏：每像素字数 %d, 期望 %d", wpp, g.wordsPerPixel())
	}
	if mw < 0 || mh < 0 || x0 < 0 || y0 < 0 || x0+mw > g.w || y0+mh > g.h {
		return fmt.Errorf("缓存条目损坏：区域 %dx%d@%d,%d 越界", mw, mh, x0, y0)
	}
	if len(val) < maskHeaderWords+mw*mh*wpp {
		return errors.New("缓存条目损坏：位图长度不足")
	}
	accWpp := g.wordsPerPixel()
	for row := 0; row < mh; row++ {
		dst := acc[((y0+row)*g.w+x0)*accWpp:]
		src := val[maskHeaderWords+row*mw*wpp:]
		for col := 0; col < mw*wpp; col++ {
			dst[col] |= src[col]
		}
	}
	return nil
}

func minX(poly []geom.Point) float64 {
	m := poly[0].X
	for _, p := range poly[1:] {
		m = math.Min(m, p.X)
	}
	return m
}

func maxX(poly []geom.Point) float64 {
	m := poly[0].X
	for _, p := range poly[1:] {
		m = math.Max(m, p.X)
	}
	return m
}

func minY(poly []geom.Point) float64 {
	m := poly[0].Y
	for _, p := range poly[1:] {
		m = math.Min(m, p.Y)
	}
	return m
}

func maxY(poly []geom.Point) float64 {
	m := poly[0].Y
	for _, p := range poly[1:] {
		m = math.Max(m, p.Y)
	}
	return m
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// FillWithCache is Fill with per-polygon coverage masks memoized in c.
// The resulting image is byte-identical to the one Fill produces for the same
// inputs; see FillCached. Capacity is governed by the cache, and statistics
// are available through c.
func (im *Image) FillWithCache(polys [][]geom.Point, supersample int, c *cache.Cache) error {
	return im.fillCached(polys, supersample, c)
}

// FillCached is Fill with a cache. A nil cache is rejected with an error
// carrying entry information; use Fill when caching is disabled.
func (im *Image) FillCached(polys [][]geom.Point, supersample int, c *cache.Cache) error {
	return im.fillCached(polys, supersample, c)
}
