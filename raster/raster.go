package raster

import (
	"errors"
	"fmt"
	"os"
	"sync/atomic"

	"vecgo/geom"
)

// Image 是整条管线共享的唯一一块整图缓冲。除 New 之外，任何阶段都只能
// 通过 Lease 取得同一份 Pix 原地处理，不得再分配或拷贝第二张整图。
type Image struct {
	W, H int
	Pix  []uint8
}

// Stats 记录整图缓冲的使用情况，用于对照改动前后的内存与拷贝行为。
//
//   - FrameAllocs：整图缓冲分配次数。输入落地时分配一次，正常应为 1。
//   - FullCopies：整图像素被复制到另一块缓冲的次数。正常应为 0。
//   - StageLeases：各阶段向共享缓冲取图的次数。五个阶段各一次。
type Stats struct {
	FrameAllocs int64
	FullCopies  int64
	StageLeases int64
}

var (
	frameAllocs atomic.Int64
	fullCopies  atomic.Int64
	stageLeases atomic.Int64
)

func New(w, h int) (*Image, error) {
	if w <= 0 || h <= 0 {
		return nil, errors.New("画幅必须为正")
	}
	frameAllocs.Add(1)
	return &Image{W: w, H: h, Pix: make([]uint8, w*h)}, nil
}

// Lease 让一个处理阶段取得共享缓冲的像素平面。它从不复制，返回的就是
// im.Pix 本身；计数只用于证明各阶段确实共用同一块缓冲。
func (im *Image) Lease(stage string) []uint8 {
	stageLeases.Add(1)
	if Hook != nil && len(im.Pix) > 0 {
		Hook(stage, &im.Pix[0])
	}
	return im.Pix
}

// Hook 若不为 nil，会在每个阶段取图时被调用，供测试核对各阶段拿到的
// 是同一块底层缓冲。
var Hook func(stage string, firstPixel *uint8)

// copyFrame 是整图拷贝的诊断入口：它把一帧像素复制到另一块缓冲，并让
// FullCopies 加一。渲染管线不经过它，所以正常一次运行该计数恒为 0；
// 测试用 TestCopyFrameCounts 证明这个计数本身有效。
func copyFrame(dst, src *Image) error {
	if dst == nil || src == nil {
		return errors.New("拷贝的图不能为空")
	}
	if dst.W != src.W || dst.H != src.H {
		return errors.New("拷贝的两图尺寸不一致")
	}
	if len(dst.Pix) != dst.W*dst.H || len(src.Pix) != src.W*src.H {
		return errors.New("图像缓冲长度与尺寸不符")
	}
	copy(dst.Pix, src.Pix)
	fullCopies.Add(1)
	return nil
}

// Snapshot 返回当前计数。
func Snapshot() Stats {
	return Stats{
		FrameAllocs: frameAllocs.Load(),
		FullCopies:  fullCopies.Load(),
		StageLeases: stageLeases.Load(),
	}
}

// ResetStats 清零计数，主要供测试与测量工具使用。
func ResetStats() {
	frameAllocs.Store(0)
	fullCopies.Store(0)
	stageLeases.Store(0)
}

func (im *Image) Fill(polys [][]geom.Point, supersample int) error {
	if supersample < 1 {
		return errors.New("supersample 必须为正")
	}
	if len(polys) == 0 {
		return errors.New("至少需要一个多边形")
	}
	for _, p := range polys {
		if len(p) < 3 {
			return errors.New("多边形至少三个顶点")
		}
	}
	pix := im.Lease("raster")
	minx, miny := polys[0][0].X, polys[0][0].Y
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
	spanx, spany := maxx-minx, maxy-miny
	if spanx == 0 {
		spanx = 1
	}
	if spany == 0 {
		spany = 1
	}
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
			pix[row*im.W+col] = uint8(255 * hit / den)
		}
	}
	return nil
}

func (im *Image) SavePGM(path string) error {
	if im.W == 0 || im.H == 0 {
		return errors.New("空图不能导出")
	}
	pix := im.Lease("export")
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := fmt.Fprintf(f, "P5\n%d %d\n255\n", im.W, im.H); err != nil {
		return err
	}
	_, err = f.Write(pix)
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
