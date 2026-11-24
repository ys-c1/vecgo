package dual

import (
	"fmt"
	"os"
	"sort"
)

// MaxDiffCoords 是差异坐标列表的默认上限。
const MaxDiffCoords = 20

// Diff 是单个差异像素的记录。
type Diff struct {
	X, Y  int
	A, B  uint8  // 两条路径在该像素的值
	Delta int    // 绝对差值
	Stage string // 首个出现分歧的阶段
}

// Report 是两张图的逐像素比对结果。
type Report struct {
	EngineA, EngineB string
	W, H             int
	DiffCount        int
	TotalDelta       int // 全量差异像素的绝对差值之和（不受坐标列表上限影响）
	MaxDelta         int
	FirstStage       string // 最早出现分歧的阶段（空表示全阶段一致）
	Diffs            []Diff
}

// Total 返回全量绝对差值之和，供报告直接引用。
func (r Report) Total() int { return r.TotalDelta }

// Compare 逐像素比对两个渲染结果，并按阶段快照定位首个分歧步骤。
// 差异先按 Delta 降序，再按行、列升序排列；坐标列表最多保留前 MaxDiffCoords 个。
func Compare(a, b *Result) Report {
	r := Report{EngineA: a.Engine, EngineB: b.Engine, W: a.Final.W, H: a.Final.H}
	r.FirstStage = firstDivergentStage(a, b)
	for y := 0; y < r.H; y++ {
		for x := 0; x < r.W; x++ {
			idx := y*r.W + x
			va, vb := a.Final.V[idx], b.Final.V[idx]
			if va == vb {
				continue
			}
			r.DiffCount++
			d := int(va) - int(vb)
			if d < 0 {
				d = -d
			}
			r.TotalDelta += d
			if d > r.MaxDelta {
				r.MaxDelta = d
			}
			r.Diffs = append(r.Diffs, Diff{
				X: x, Y: y, A: va, B: vb, Delta: d, Stage: r.FirstStage,
			})
		}
	}
	sort.Slice(r.Diffs, func(i, j int) bool {
		if r.Diffs[i].Delta != r.Diffs[j].Delta {
			return r.Diffs[i].Delta > r.Diffs[j].Delta
		}
		if r.Diffs[i].Y != r.Diffs[j].Y {
			return r.Diffs[i].Y < r.Diffs[j].Y
		}
		return r.Diffs[i].X < r.Diffs[j].X
	})
	if len(r.Diffs) > MaxDiffCoords {
		r.Diffs = r.Diffs[:MaxDiffCoords]
	}
	return r
}

// firstDivergentStage 对齐两个结果的阶段快照，返回首个不一致的阶段名。
func firstDivergentStage(a, b *Result) string {
	n := len(a.Stages)
	if len(b.Stages) < n {
		n = len(b.Stages)
	}
	for i := 0; i < n; i++ {
		pa, pb := a.Stages[i].Pix, b.Stages[i].Pix
		if len(pa.V) != len(pb.V) {
			return a.Stages[i].Name
		}
		for k := range pa.V {
			if pa.V[k] != pb.V[k] {
				return a.Stages[i].Name
			}
		}
	}
	return ""
}

// Identical 报告两条路径是否逐像素相同。
func (r Report) Identical() bool { return r.DiffCount == 0 }

// SavePGM 把像素写成二进制 PGM（P5）。
func SavePGM(p Pix, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := fmt.Fprintf(f, "P5\n%d %d\n255\n", p.W, p.H); err != nil {
		return err
	}
	_, err = f.Write(p.V)
	return err
}

// SideBySide 把两张同高图横向拼接，中间插一列灰色分隔线。
func SideBySide(a, b Pix) Pix {
	gap := 1
	out := Pix{W: a.W + gap + b.W, H: a.H, V: make([]uint8, (a.W+gap+b.W)*a.H)}
	for y := 0; y < a.H; y++ {
		copy(out.V[y*out.W:y*out.W+a.W], a.V[y*a.W:(y+1)*a.W])
		out.V[y*out.W+a.W] = 128
		copy(out.V[y*out.W+a.W+gap:], b.V[y*b.W:(y+1)*b.W])
	}
	return out
}

// DiffMap 生成差异可视化：背景黑，差异像素亮度与 Delta 成正比。
func DiffMap(a, b Pix) Pix {
	out := Pix{W: a.W, H: a.H, V: make([]uint8, len(a.V))}
	for i := range a.V {
		d := int(a.V[i]) - int(b.V[i])
		if d < 0 {
			d = -d
		}
		if d > 0 {
			v := d * 16
			if v > 255 {
				v = 255
			}
			out.V[i] = uint8(v)
		}
	}
	return out
}
