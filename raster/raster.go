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
		return ErrSubZero
	}
	for _, p := range polys {
		if len(p) < 3 {
			return errors.New("多边形至少三个顶点")
		}
	}
	if im.W <= 0 || im.H <= 0 {
		return ErrEmptyImage
	}
	// 8 位路径：Coverage(子点覆盖率) + Quantize(256 级)，
	// Quantize 的 8 位分支与本函数历史整数公式逐字节一致。
	cov, err := Coverage(im.W, im.H, polys, supersample)
	if err != nil {
		return err
	}
	return Quantize(cov, im, 8, false)
}

func (im *Image) SavePGM(path string) error {
	if im.W == 0 || im.H == 0 {
		return ErrEmptyImage
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
