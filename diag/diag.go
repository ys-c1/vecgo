// Package diag 为非法渲染参数生成诊断图。
//
// 背景：过去 NaN 能穿过所有区间比较（NaN < x 与 NaN > x 都为假），
// rotate 完全没有校验，于是 scale=NaN / rotate=Inf 会静默产出一张
// 全黑图。校验补上之后，被拒绝的参数仍需要一个可附在工单里的证据：
// 用安全值渲染一张参考图，再把该参数会影响的区域标红。
package diag

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"image"
	"image/png"
	"io"
	"math"
	"os"
	"strings"

	"vecgo/config"
	"vecgo/geom"
	"vecgo/pipe"
)

// Scope 是被拒绝参数影响的范围。
type Scope int

const (
	// Frame 表示整幅画面：画幅、后处理（gamma/blur）。
	Frame Scope = iota
	// Shape 表示几何覆盖区域：变换（scale/rotate）、采样与着色只
	// 作用在图形覆盖到的像素上。
	Shape
)

// Finding 是一条拒绝记录：被拒参数、原因，以及它影响的范围。
type Finding struct {
	config.Issue
	Scope Scope
}

// Analyze 把非法参数逐条列出，并给出各自的影响范围。
// 顺序与 config.Config.Issues 一致，便于工单里逐条对照。
func Analyze(cfg config.Config) []Finding {
	issues := cfg.Issues()
	out := make([]Finding, len(issues))
	for i, is := range issues {
		out[i] = Finding{Issue: is, Scope: scopeOf(is.Param)}
	}
	return out
}

func scopeOf(param string) Scope {
	switch param {
	case "width", "height", "gamma", "blur", "output":
		return Frame
	default: // scale, rotate, supersample, shading
		return Shape
	}
}

// Safe 把非法参数替换成默认值，其余参数原样保留，用于渲染参考图。
// 它的输出一定通过 config.Validate。
func Safe(c config.Config) config.Config {
	d := config.Default()
	if c.Width < 1 || c.Width > 4096 {
		c.Width = d.Width
	}
	if c.Height < 1 || c.Height > 4096 {
		c.Height = d.Height
	}
	if !finite(c.Scale) || c.Scale <= 0 || c.Scale > 100 {
		c.Scale = d.Scale
	}
	if !finite(c.Rotate) {
		c.Rotate = d.Rotate
	}
	if c.Supersample < 1 || c.Supersample > 8 {
		c.Supersample = d.Supersample
	}
	if c.Shading != "flat" && c.Shading != "gradient" {
		c.Shading = d.Shading
	}
	if !finite(c.Gamma) || c.Gamma < 0.1 || c.Gamma > 10 {
		c.Gamma = d.Gamma
	}
	if c.Blur < 0 || c.Blur > 16 {
		c.Blur = d.Blur
	}
	c.Output = d.Output
	return c
}

func finite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}

// Image 是诊断图：3 字节/像素的 RGB 位图。Marked 与像素一一对应，
// 记录哪些像素是被诊断标记染红的，避免用颜色猜标记（参考图本身
// 也可能有红色通道拉满的像素）。
type Image struct {
	W, H     int
	Pix      []uint8
	Marked   []bool
	Findings []Finding
}

// SavePNG 把诊断图写成 PNG。工单附件通常是 PNG，且诊断图带红色标记，
// 灰度的 PGM 承载不了，因此诊断图单独用一种彩色格式导出。
//
// 拒绝原因与影响范围写进 PNG 的 iTXt 文本块（关键字 vecgo-diag），
// 图片被单独转发、改名或贴进工单时说明仍然跟着图走。
func (im *Image) SavePNG(path string) error {
	if im.W <= 0 || im.H <= 0 || len(im.Pix) != im.W*im.H*3 {
		return fmt.Errorf("诊断图尺寸非法: %dx%d", im.W, im.H)
	}
	// 先建文件再编码：路径不可写时要立刻失败，而不是渲染完才发现。
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	rgba := &image.RGBA{
		Pix:    make([]uint8, im.W*im.H*4),
		Stride: im.W * 4,
		Rect:   image.Rect(0, 0, im.W, im.H),
	}
	for i := 0; i < im.W*im.H; i++ {
		rgba.Pix[4*i] = im.Pix[3*i]
		rgba.Pix[4*i+1] = im.Pix[3*i+1]
		rgba.Pix[4*i+2] = im.Pix[3*i+2]
		rgba.Pix[4*i+3] = 255
	}
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.DefaultCompression}
	if err := enc.Encode(&buf, rgba); err != nil {
		return err
	}
	out, err := addTextChunk(buf.Bytes(), "vecgo-diag", im.reportText())
	if err != nil {
		return err
	}
	_, err = f.Write(out)
	return err
}

func (im *Image) reportText() string {
	var b strings.Builder
	for _, f := range im.Findings {
		fmt.Fprintf(&b, "%s=%s: %s (scope: %s)\n", f.Param, f.Value, f.Reason, scopeName(f.Scope))
	}
	return b.String()
}

// addTextChunk 在 PNG 的 IEND 之前插入一个 iTXt 块。标准库编码器
// 不支持写文本块，这里手工拼块并算 CRC。
func addTextChunk(pngData []byte, keyword, text string) ([]byte, error) {
	if len(pngData) < 8 || !bytes.Equal(pngData[:8], []byte("\x89PNG\r\n\x1a\n")) {
		return nil, fmt.Errorf("不是 PNG 数据")
	}
	// iTXt: keyword\0 compressionFlag(0) compressionMethod(0) languageTag\0 translatedKeyword\0 text
	var data []byte
	data = append(data, keyword...)
	data = append(data, 0, 0, 0)
	data = append(data, 0) // 空 languageTag
	data = append(data, 0) // 空 translatedKeyword
	data = append(data, text...)

	chunk := make([]byte, 8+len(data)+4)
	binary.BigEndian.PutUint32(chunk[0:4], uint32(len(data)))
	copy(chunk[4:8], "iTXt")
	copy(chunk[8:], data)
	crc := crc32.ChecksumIEEE(chunk[4 : 8+len(data)])
	binary.BigEndian.PutUint32(chunk[8+len(data):], crc)

	// IEND 是固定 12 字节的最后一个块。
	if len(pngData) < 12 || string(pngData[len(pngData)-8:len(pngData)-4]) != "IEND" {
		return nil, fmt.Errorf("PNG 缺少 IEND")
	}
	out := make([]byte, 0, len(pngData)+len(chunk))
	out = append(out, pngData[:len(pngData)-12]...)
	out = append(out, chunk...)
	out = append(out, pngData[len(pngData)-12:]...)
	return out, nil
}

// Render 生成诊断图：用 Safe(cfg) 渲染参考图，再把每条拒绝记录
// 对应的区域标红。
//
//   - Shape 范围：参考图中被图形覆盖的像素（值 > 0）染红；
//     这正是该参数出错时会变黑的那一块。
//   - Frame 范围：沿画面四周画红色边框，表示整幅都受影响。
//
// 没有任何拒绝记录时，返回的是一张无标记的参考渲染图。
// 返回的 findings 与 Analyze(cfg) 相同，便于调用方打印。
func Render(scene [][]geom.Point, cfg config.Config) (*Image, []Finding, error) {
	findings := Analyze(cfg)
	if len(scene) == 0 {
		return nil, findings, fmt.Errorf("场景为空")
	}
	ref, err := pipe.Run(scene, Safe(cfg))
	if err != nil {
		return nil, findings, err
	}
	out := &Image{W: ref.W, H: ref.H, Pix: make([]uint8, ref.W*ref.H*3), Marked: make([]bool, ref.W*ref.H)}
	for i, v := range ref.Pix {
		out.Pix[3*i], out.Pix[3*i+1], out.Pix[3*i+2] = v, v, v
	}
	markShape, markFrame := false, false
	for _, f := range findings {
		if f.Scope == Shape {
			markShape = true
		} else {
			markFrame = true
		}
	}
	if markShape {
		// 用红色斜纹覆盖几何区域，而不是整块涂红：底图的明暗仍然可见，
		// 出问题时能看出“哪一块本来该有内容”。
		for y := 0; y < ref.H; y++ {
			for x := 0; x < ref.W; x++ {
				i := y*ref.W + x
				if ref.Pix[i] == 0 {
					continue
				}
				out.Marked[i] = true
				if (x+y)%6 >= 3 {
					continue
				}
				out.Pix[3*i] = 255
				out.Pix[3*i+1] = ref.Pix[i] / 2
				out.Pix[3*i+2] = ref.Pix[i] / 2
			}
		}
	}
	if markFrame {
		b := 2
		if ref.W < 8 || ref.H < 8 {
			b = 1
		}
		markBorder(out, b)
	}
	out.Findings = findings
	return out, findings, nil
}

func markBorder(im *Image, b int) {
	for y := 0; y < im.H; y++ {
		for x := 0; x < im.W; x++ {
			if x >= b && x < im.W-b && y >= b && y < im.H-b {
				continue
			}
			i := 3 * (y*im.W + x)
			im.Pix[i], im.Pix[i+1], im.Pix[i+2] = 255, 0, 0
			im.Marked[y*im.W+x] = true
		}
	}
}

// Report 把拒绝记录写到 w（通常为 stderr），每条一行，可直接贴进工单。
func Report(w io.Writer, findings []Finding) {
	for _, f := range findings {
		fmt.Fprintf(w, "[diag] 拒绝 %s=%s: %s（影响范围: %s）\n",
			f.Param, f.Value, f.Reason, scopeName(f.Scope))
	}
}

func scopeName(s Scope) string {
	if s == Frame {
		return "整幅画面"
	}
	return "几何覆盖区域"
}

// CountMarked 统计被诊断标记的像素数，供测试与验收使用。
func (im *Image) CountMarked() int {
	n := 0
	for _, m := range im.Marked {
		if m {
			n++
		}
	}
	return n
}
