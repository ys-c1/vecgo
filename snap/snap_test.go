package snap

import (
	"os"
	"path/filepath"
	"testing"

	"vecgo/verr"
)

const goldenDir = "testdata/golden"

// 每一段单独比对、单独报结论：五处的成功输出分别出图，不能拿一张代表五处。
// 这里是“改动后”对“改动前（baseline HEAD）”的逐像素比对。
func TestGoldenPerStage(t *testing.T) {
	actual := t.TempDir()
	if err := RenderAll(actual); err != nil {
		t.Fatalf("生成当前基准图失败: %v", err)
	}
	for _, sec := range Sections() {
		sec := sec
		t.Run(sec.Name, func(t *testing.T) {
			if _, err := os.Stat(filepath.Join(goldenDir, sec.Name)); err != nil {
				t.Skipf("缺少 %s 的基准目录（需要在 baseline 树上生成）: %v", sec.Name, err)
			}
			gsum := summariseStage(t, goldenDir, actual, sec.Stage)
			t.Log(gsum)
			// 该段内逐文件比对，任一不一致都失败，并报出首处像素。
			for _, f := range sec.Files {
				gp := filepath.Join(goldenDir, sec.Name, f)
				ap := filepath.Join(actual, sec.Name, f)
				g, err := loadPGM(gp)
				if err != nil {
					t.Fatalf("%s/%s 基准读取失败: %v", sec.Name, f, err)
				}
				a, err := loadPGM(ap)
				if err != nil {
					t.Fatalf("%s/%s 实际读取失败: %v", sec.Name, f, err)
				}
				if !g.Equal(a) {
					x, y, gv, av, _ := g.Diff(a)
					n := countDiffPix(g, a)
					t.Errorf("%s/%s 像素不一致: %d 处，首处 (%d,%d) 基准=%d 实际=%d",
						sec.Name, f, n, x, y, gv, av)
				}
			}
		})
	}
}

func loadPGM(path string) (*PGM, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, verr.Wrap(verr.KindSnapRead, err)
	}
	return ParsePGM(data)
}

func countDiffPix(a, b *PGM) int {
	n := 0
	for i := 0; i < len(a.Pix) && i < len(b.Pix); i++ {
		if a.Pix[i] != b.Pix[i] {
			n++
		}
	}
	return n
}

// summariseStage 只比较某一段，返回一句人读的结论。
func summariseStage(t *testing.T, golden, actual string, stage verr.Stage) string {
	t.Helper()
	sec, ok := sectionFor(stage)
	if !ok {
		t.Fatalf("未知段: %v", stage)
	}
	consistent := 0
	var bad []string
	for _, f := range sec.Files {
		g, err := loadPGM(filepath.Join(golden, sec.Name, f))
		if err != nil {
			t.Fatalf("基准读取失败: %v", err)
		}
		a, err := loadPGM(filepath.Join(actual, sec.Name, f))
		if err != nil {
			t.Fatalf("实际读取失败: %v", err)
		}
		if g.Equal(a) {
			consistent++
		} else {
			bad = append(bad, f)
		}
	}
	if len(bad) == 0 {
		return sec.Name + ": " + itoa(consistent) + "/" + itoa(len(sec.Files)) + " 一致"
	}
	return sec.Name + ": " + itoa(consistent) + "/" + itoa(len(sec.Files)) + " 一致，不一致: " + joinBad(bad)
}

func joinBad(bad []string) string {
	out := ""
	for i, b := range bad {
		if i > 0 {
			out += ","
		}
		out += b
	}
	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

// 五段的基准清单必须覆盖磁盘上的全部文件，不多不少：
// 新增矩阵忘了登记，会让新图悄悄没有基准；删除文件忘了登记，会让比对空跑。
func TestSectionsCoverKnownImages(t *testing.T) {
	if _, err := os.Stat(goldenDir); err != nil {
		t.Skipf("缺少基准目录: %v", err)
	}
	for _, sec := range Sections() {
		dir := filepath.Join(goldenDir, sec.Name)
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("%s 基准目录缺失: %v", sec.Name, err)
		}
		onDisk := map[string]bool{}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			onDisk[e.Name()] = true
		}
		if len(onDisk) != len(sec.Files) {
			t.Fatalf("%s 清单 %d 个文件，磁盘 %d 个", sec.Name, len(sec.Files), len(onDisk))
		}
		for _, f := range sec.Files {
			if !onDisk[f] {
				t.Fatalf("%s 清单里的 %s 在磁盘上不存在", sec.Name, f)
			}
			delete(onDisk, f)
		}
		for f := range onDisk {
			t.Fatalf("%s 磁盘上有清单之外的文件: %s", sec.Name, f)
		}
	}
}

// 基准文件本身必须是可解析的 P5，且像素缓冲长度与画幅一致。
func TestGoldenFilesParse(t *testing.T) {
	if _, err := os.Stat(goldenDir); err != nil {
		t.Skipf("缺少基准目录: %v", err)
	}
	for _, sec := range Sections() {
		for _, f := range sec.Files {
			p, err := loadPGM(filepath.Join(goldenDir, sec.Name, f))
			if err != nil {
				t.Fatalf("%s/%s: %v", sec.Name, f, err)
			}
			if len(p.Pix) != p.W*p.H {
				t.Fatalf("%s/%s 像素数 %d != %dx%d", sec.Name, f, len(p.Pix), p.W, p.H)
			}
		}
	}
}

// 基准比对失败必须带 snap 段编码，且不能误报成渲染段。
func TestCompareFailureCarriesSnapKind(t *testing.T) {
	actual := t.TempDir()
	if err := RenderAll(actual); err != nil {
		t.Fatalf("生成失败: %v", err)
	}
	// 篡改一个像素，制造不一致。
	p := filepath.Join(actual, "post", "gamma_1.0.pgm")
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	raw[len(raw)-1] ^= 0xFF
	if err := os.WriteFile(p, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	err = Compare(goldenDir, actual)
	if err == nil {
		t.Fatal("篡改后应报不一致")
	}
	if got := verr.StageOf(err); got != verr.StageSnap {
		t.Fatalf("快照失败应归 snap 段，实际 %v", got)
	}
	if !verr.IsKind(err, verr.KindSnapMismatch) {
		t.Fatalf("应为 KindSnapMismatch: %v", err)
	}
}

// 缺文件应报 snap 段读取错误，而不是 panic。
func TestCompareMissingFile(t *testing.T) {
	actual := t.TempDir()
	err := Compare(goldenDir, actual)
	if err == nil {
		t.Fatal("空目录应报缺文件")
	}
	if got := verr.StageOf(err); got != verr.StageSnap {
		t.Fatalf("缺失文件应归 snap 段，实际 %v", got)
	}
}
