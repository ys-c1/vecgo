package raster

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"

	"vecgo/color"
	"vecgo/geom"
)

type Image struct {
	W, H int
	Pix  []uint8
}

func New(w, h int) (*Image, error) {
	if w <= 0 || h <= 0 {
		return nil, errors.New("raster: width/height 必须为正")
	}
	return &Image{W: w, H: h, Pix: make([]uint8, w*h)}, nil
}

func (im *Image) Fill(polys [][]geom.Point, supersample int) error {
	if supersample < 1 {
		return errors.New("raster: supersample 必须为正")
	}
	for _, p := range polys {
		if len(p) < 3 {
			return errors.New("raster: 多边形至少三个顶点")
		}
	}
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
			im.Pix[row*im.W+col] = uint8(255 * hit / den)
		}
	}
	return nil
}

// SavePGM 原样写出 Pix（调用方负责保证 Pix 已经是要落盘的编码字节）。
func (im *Image) SavePGM(path string) error {
	if im.W == 0 || im.H == 0 {
		return errors.New("raster: 空图不能导出 PGM")
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

// SaveLinearPGM 把线性光缓冲按 gamma 编码后写成 PGM。
// 编码表来自 color 包，与 post.Gamma 同一份转换函数。
func (im *Image) SaveLinearPGM(path string, gamma float64) error {
	if im.W == 0 || im.H == 0 {
		return errors.New("raster: 空图不能导出 PGM")
	}
	table, err := color.EncodeLUT(gamma)
	if err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := fmt.Fprintf(f, "P5\n%d %d\n255\n", im.W, im.H); err != nil {
		return err
	}
	bw := bufio.NewWriter(f)
	for _, v := range im.Pix {
		if err := bw.WriteByte(table[v]); err != nil {
			return err
		}
	}
	return bw.Flush()
}

// LoadLinearPGM 读取 P5 灰度 PGM，并按 gamma 把编码字节解码回线性光缓冲。
// 与 SaveLinearPGM 共用 color 包的正反映射。
func LoadLinearPGM(path string, gamma float64) (*Image, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	magic, rest, err := pgmToken(data)
	if err != nil {
		return nil, err
	}
	if magic != "P5" {
		return nil, errors.New("raster: PGM magic 非法，仅支持 P5")
	}
	w, rest, err := pgmToken(rest)
	if err != nil {
		return nil, err
	}
	h, rest, err := pgmToken(rest)
	if err != nil {
		return nil, err
	}
	maxv, rest, err := pgmToken(rest)
	if err != nil {
		return nil, err
	}
	var width, height, maxVal int
	if _, err := fmt.Sscanf(w+" "+h+" "+maxv, "%d %d %d", &width, &height, &maxVal); err != nil {
		return nil, fmt.Errorf("raster: PGM 头字段非法: %w", err)
	}
	if width <= 0 || height <= 0 {
		return nil, errors.New("raster: PGM width/height 必须为正")
	}
	if maxVal != 255 {
		return nil, fmt.Errorf("raster: PGM maxval 不支持: %d", maxVal)
	}
	// 头与数据之间恰好一个空白字节。
	if len(rest) != width*height+1 {
		return nil, fmt.Errorf("raster: PGM 数据长度非法: 期望 %d 实际 %d", width*height, len(rest)-1)
	}
	table, err := color.DecodeLUT(gamma)
	if err != nil {
		return nil, err
	}
	im := &Image{W: width, H: height, Pix: make([]byte, width*height)}
	for i, b := range rest[1:] {
		im.Pix[i] = table[b]
	}
	return im, nil
}

// pgmToken 取一个 PGM 头 token，跳过空白与 # 注释，返回 token 和剩余字节。
func pgmToken(data []byte) (string, []byte, error) {
	i := 0
	for i < len(data) {
		c := data[i]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			i++
			continue
		}
		if c == '#' {
			for i < len(data) && data[i] != '\n' {
				i++
			}
			continue
		}
		break
	}
	start := i
	for i < len(data) {
		c := data[i]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			break
		}
		i++
	}
	if start == i {
		return "", nil, errors.New("raster: PGM 头被截断")
	}
	return string(bytes.Clone(data[start:i])), data[i:], nil
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
