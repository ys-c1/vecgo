// Package snap 把“五处成功输出”的基准固化下来，并提供带编码的比对。
//
// 基准按包分段，一包一套，互不借用：config / geom / raster / shading /
// post 五段各自有独立的 section 与独立的基准文件清单。任何一段的成功
// 输出发生变化，只会在它自己那一段报出来。
//
// 基准本身是一组 P5 PGM（无压缩），逐像素比对就是逐字节比对。
// 加载、解析、不一致都归 verr 的 snap 段（0x06xx），与五段渲染的错误编码
// 分开：快照失败不等于渲染失败，也不该去摘渲染段。
package snap

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"vecgo/verr"
)

// Section 是一段基准。Name 与 verr.Stage 的字符串一致，方便日志对齐。
type Section struct {
	Stage verr.Stage
	Name  string
	Files []string // 该段基准里的文件名（不含目录）
}

// Sections 返回五段的基准清单。顺序与 pipe 的执行顺序一致。
//
// 清单是显式列出的：新增某个输入矩阵时，这里也要同步加一行；
// 漏加会让新图悄悄没有基准，所以 TestSectionsCoverKnownImages 会兜底。
func Sections() []Section {
	return []Section{
		{Stage: verr.StageConfig, Name: "config", Files: []string{"config_matrix.pgm"}},
		{Stage: verr.StageGeom, Name: "geom", Files: []string{"geom_primitives.pgm"}},
		{Stage: verr.StageRaster, Name: "raster", Files: rasterFileList()},
		{Stage: verr.StageShading, Name: "shading", Files: shadingFileList()},
		{Stage: verr.StagePost, Name: "post", Files: postFileList()},
	}
}

func rasterFileList() []string {
	scenes := []string{"square", "two", "tri"}
	cfgs := []string{"default", "gradient", "gamma", "blur", "ss1", "ss3", "small", "rot", "combo"}
	out := make([]string, 0, len(scenes)*len(cfgs))
	for _, s := range scenes {
		for _, c := range cfgs {
			out = append(out, s+"_"+c+".pgm")
		}
	}
	return out
}

func shadingFileList() []string {
	out := []string{"base.pgm"}
	for _, mode := range []string{"flat", "gradient"} {
		for _, tp := range []string{"1.0_0.0", "0.6_0.2", "0.0_1.0"} {
			out = append(out, mode+"_"+tp+".pgm")
		}
	}
	return out
}

func postFileList() []string {
	out := []string{}
	for _, g := range []string{"0.5", "1.0", "1.7", "2.2", "3.0"} {
		out = append(out, "gamma_"+g+".pgm")
	}
	for _, r := range []string{"0", "1", "2", "3"} {
		out = append(out, "blur_"+r+".pgm")
	}
	return out
}

// PGM 是解析后的 P5 图。
type PGM struct {
	W, H int
	Pix  []uint8
}

// ParsePGM 解析 P5/P2 文本头 + 二进制体。只支持 maxval ≤ 255。
func ParsePGM(data []byte) (*PGM, error) {
	const magic = "P5"
	if len(data) < 2 || string(data[:2]) != magic {
		return nil, verr.Errorf(verr.KindSnapParse, "不是 P5 PGM: 魔数 %q", firstBytes(data, 2))
	}
	i := 2
	readInt := func() (int, error) {
		for i < len(data) && (data[i] == ' ' || data[i] == '\n' || data[i] == '\r' || data[i] == '\t' || data[i] == '#') {
			if data[i] == '#' {
				for i < len(data) && data[i] != '\n' {
					i++
				}
			} else {
				i++
			}
		}
		start := i
		for i < len(data) && data[i] >= '0' && data[i] <= '9' {
			i++
		}
		if start == i {
			return 0, verr.Errorf(verr.KindSnapParse, "PGM 头缺数字 @%d", start)
		}
		n, err := strconv.Atoi(string(data[start:i]))
		if err != nil {
			return 0, verr.Wrap(verr.KindSnapParse, err)
		}
		return n, nil
	}
	w, err := readInt()
	if err != nil {
		return nil, err
	}
	h, err := readInt()
	if err != nil {
		return nil, err
	}
	maxv, err := readInt()
	if err != nil {
		return nil, err
	}
	if w <= 0 || h <= 0 || maxv <= 0 || maxv > 255 {
		return nil, verr.Errorf(verr.KindSnapParse, "PGM 头非法: %dx%d max=%d", w, h, maxv)
	}
	// 头之后必须恰好一个空白字节，然后就是像素体。
	if i >= len(data) || (data[i] != '\n' && data[i] != ' ' && data[i] != '\r' && data[i] != '\t') {
		return nil, verr.Errorf(verr.KindSnapParse, "PGM 头与像素体之间缺少空白 @%d", i)
	}
	i++
	need := w * h
	if len(data)-i != need {
		return nil, verr.Errorf(verr.KindSnapParse, "像素体长度不符: 期望 %d 实际 %d", need, len(data)-i)
	}
	return &PGM{W: w, H: h, Pix: append([]uint8(nil), data[i:]...)}, nil
}

func firstBytes(b []byte, n int) string {
	if len(b) < n {
		return string(b)
	}
	return string(b[:n])
}

// Equal 报告两张图是否逐像素相等。
func (p *PGM) Equal(q *PGM) bool {
	if p.W != q.W || p.H != q.H {
		return false
	}
	// bytes.Equal 在长度相等时与逐像素比较等价。
	return bytes.Equal(p.Pix, q.Pix)
}

// Diff 返回第一个不同像素的坐标与两边的值；相等时返回 ok=true。
func (p *PGM) Diff(q *PGM) (x, y int, a, b uint8, ok bool) {
	if p.W != q.W || p.H != q.H {
		return 0, 0, 0, 0, false
	}
	for i := 0; i < len(p.Pix); i++ {
		if p.Pix[i] != q.Pix[i] {
			return i % p.W, i / p.W, p.Pix[i], q.Pix[i], false
		}
	}
	return 0, 0, 0, 0, true
}

// Mismatch 是一处基准不一致。
type Mismatch struct {
	Stage verr.Stage
	File  string
	What  string
}

// Compare 把 actualDir 里的每个 section 与 goldenDir 里的基准逐像素比对。
//
// 返回的不一致已经全部归一到 verr 的 snap 段（KindSnapMismatch），
// 并通过 Multi 汇总；没有不一致时返回 nil。
func Compare(goldenDir, actualDir string) error {
	var errs []error
	for _, sec := range Sections() {
		for _, f := range sec.Files {
			gp := filepath.Join(goldenDir, sec.Name, f)
			ap := filepath.Join(actualDir, sec.Name, f)
			gdata, err := os.ReadFile(gp)
			if err != nil {
				errs = append(errs, verr.Wrap(verr.KindSnapRead, err))
				continue
			}
			g, err := ParsePGM(gdata)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			adata, err := os.ReadFile(ap)
			if err != nil {
				errs = append(errs, verr.Wrap(verr.KindSnapRead, err))
				continue
			}
			a, err := ParsePGM(adata)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			if g.W != a.W || g.H != a.H {
				errs = append(errs, verr.Errorf(verr.KindSnapMismatch,
					"%s/%s 尺寸不一致: 基准 %dx%d 实际 %dx%d", sec.Name, f, g.W, g.H, a.W, a.H))
				continue
			}
			if !g.Equal(a) {
				x, y, gv, av, _ := g.Diff(a)
				n := countDiff(g, a)
				errs = append(errs, verr.Errorf(verr.KindSnapMismatch,
					"%s/%s 像素不一致: %d 处，首处 (%d,%d) 基准=%d 实际=%d", sec.Name, f, n, x, y, gv, av))
			}
		}
	}
	return verr.NewMulti(errs...)
}

func countDiff(a, b *PGM) int {
	n := 0
	for i := 0; i < len(a.Pix) && i < len(b.Pix); i++ {
		if a.Pix[i] != b.Pix[i] {
			n++
		}
	}
	return n
}

// Summary 是某段基准的摘要行，用于测试失败时打印“这一段”的结论。
func Summary(goldenDir, actualDir string, stage verr.Stage) (string, error) {
	sec, ok := sectionFor(stage)
	if !ok {
		return "", verr.Errorf(verr.KindSnapParse, "未知段: %s", stage.String())
	}
	var bad []string
	for _, f := range sec.Files {
		gdata, err := os.ReadFile(filepath.Join(goldenDir, sec.Name, f))
		if err != nil {
			return "", verr.Wrap(verr.KindSnapRead, err)
		}
		adata, err := os.ReadFile(filepath.Join(actualDir, sec.Name, f))
		if err != nil {
			return "", verr.Wrap(verr.KindSnapRead, err)
		}
		g, err := ParsePGM(gdata)
		if err != nil {
			return "", err
		}
		a, err := ParsePGM(adata)
		if err != nil {
			return "", err
		}
		if !g.Equal(a) {
			bad = append(bad, f)
		}
	}
	if len(bad) == 0 {
		return fmt.Sprintf("%s: %d/%d 一致", sec.Name, len(sec.Files), len(sec.Files)), nil
	}
	sort.Strings(bad)
	return fmt.Sprintf("%s: %d/%d 不一致 -> %s", sec.Name, len(bad), len(sec.Files), strings.Join(bad, ",")), nil
}

func sectionFor(stage verr.Stage) (Section, bool) {
	for _, s := range Sections() {
		if s.Stage == stage {
			return s, true
		}
	}
	return Section{}, false
}
