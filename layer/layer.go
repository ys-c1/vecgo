// Package layer 提供运行期图层开关：每一层对应掩码中的一个位，
// 位为 1 表示该层可见，为 0 表示该层被隐藏。掩码放在配置里，
// 改掩码即可生效，不需要重新编译。
package layer

import (
	"fmt"
	"strings"
)

// MaxLayers 是掩码能寻址的最大图层数（每位一层，uint64 共 64 位）。
const MaxLayers = 64

// Mask 是图层可见性位掩码，位 i 对应场景中的第 i 层（scene[i]）。
type Mask uint64

// All 返回低 n 位全部置 1 的掩码（前 n 层全部可见）。
func All(n int) (Mask, error) {
	if n < 0 {
		return 0, fmt.Errorf("图层数不能为负: %d", n)
	}
	if n > MaxLayers {
		return 0, fmt.Errorf("位数超限: 图层数 %d 超过 %d，位 %s 无法表示",
			n, MaxLayers, joinBits(overflowBits(n)))
	}
	if n == MaxLayers {
		return Mask(^uint64(0)), nil
	}
	return Mask(uint64(1)<<uint(n) - 1), nil
}

// IssueKind 是掩码诊断的类别。
type IssueKind int

const (
	// KindOverflow 位数超限：图层数超过掩码位数，高位图层无法寻址。
	KindOverflow IssueKind = iota
	// KindUnknown 未知图层位：掩码中置位的位超出了实际图层数。
	KindUnknown
	// KindAllOff 掩码全为零：显式给出的掩码没有任何位置 1。
	KindAllOff
)

// Issue 是一条掩码诊断，Bits 列出涉及的具体位号（升序）。
type Issue struct {
	Kind IssueKind
	Bits []int
}

func (is Issue) String() string {
	switch is.Kind {
	case KindOverflow:
		return fmt.Sprintf("位数超限: 位 %s 超出掩码位数 %d，对应图层将保持可见且无法开关",
			joinBits(is.Bits), MaxLayers)
	case KindUnknown:
		return fmt.Sprintf("未知图层位: 位 %s 没有对应图层，已忽略，其余图层不受影响",
			joinBits(is.Bits))
	case KindAllOff:
		return fmt.Sprintf("掩码全为零: 位 %s 全部关闭，渲染结果为空图",
			joinBits(is.Bits))
	}
	return fmt.Sprintf("未知诊断: 位 %s", joinBits(is.Bits))
}

// Check 对照图层数检查掩码，返回全部诊断（可能为空）。
// 三类问题都会给出具体位号；诊断不影响其余图层的渲染。
func Check(m Mask, layers int) []Issue {
	var out []Issue
	if layers < 0 {
		layers = 0
	}
	if layers > MaxLayers {
		out = append(out, Issue{Kind: KindOverflow, Bits: overflowBits(layers)})
	}
	known := layers
	if known > MaxLayers {
		known = MaxLayers
	}
	// 未知图层位：置位但 >= layers 的位。
	var unknown []int
	for b := known; b < MaxLayers; b++ {
		if m&Mask(uint64(1)<<uint(b)) != 0 {
			unknown = append(unknown, b)
		}
	}
	if len(unknown) > 0 {
		out = append(out, Issue{Kind: KindUnknown, Bits: unknown})
	}
	// 掩码全为零：已知位没有任何一个置 1。
	if m == 0 {
		bits := make([]int, known)
		for i := range bits {
			bits[i] = i
		}
		out = append(out, Issue{Kind: KindAllOff, Bits: bits})
	}
	return out
}

// Visible 把掩码展开成每层的可见性表。
// m 为 nil 表示未设置掩码，全部可见（默认行为）。
// 图层数超过 MaxLayers 时返回带具体位号的错误。
func Visible(m *Mask, layers int) ([]bool, error) {
	if layers < 0 {
		return nil, fmt.Errorf("图层数不能为负: %d", layers)
	}
	if m == nil {
		vis := make([]bool, layers)
		for i := range vis {
			vis[i] = true
		}
		return vis, nil
	}
	if layers > MaxLayers {
		return nil, fmt.Errorf("位数超限: 图层数 %d 超过掩码位数 %d，位 %s 无法寻址",
			layers, MaxLayers, joinBits(overflowBits(layers)))
	}
	vis := make([]bool, layers)
	for i := 0; i < layers; i++ {
		vis[i] = *m&Mask(uint64(1)<<uint(i)) != 0
	}
	return vis, nil
}

// Mapping 描述迁移时一个旧位的去向。
// NewBit 为 -1 表示该位在新掩码中被丢弃（新图层数没有这一层）。
type Mapping struct {
	OldBit int
	NewBit int
	Note   string
}

func (mp Mapping) String() string {
	if mp.NewBit < 0 {
		return fmt.Sprintf("位 %d -> 丢弃（%s）", mp.OldBit, mp.Note)
	}
	return fmt.Sprintf("位 %d -> 位 %d（%s）", mp.OldBit, mp.NewBit, mp.Note)
}

// Migrate 把旧掩码映射到新的图层数。
// 规则：位 i 在新图层数内则留在位 i（同一层）；超出新图层数的位被丢弃；
// 新增的层（位 >= 旧图层数）默认置 1（可见）。
// 迁移失败（图层数为负或超过 MaxLayers）时返回原掩码和已算出的迁移表，
// 调用方可以保留旧状态并逐位排查。
func Migrate(old Mask, oldLayers, newLayers int) (Mask, []Mapping, error) {
	if oldLayers < 0 || newLayers < 0 {
		return old, nil, fmt.Errorf("图层数不能为负: old=%d new=%d", oldLayers, newLayers)
	}
	limit := oldLayers
	if newLayers > limit {
		limit = newLayers
	}
	if limit > MaxLayers {
		// 失败：保留旧状态，同时报出能算出的迁移表。
		table := buildTable(oldLayers, newLayers, MaxLayers)
		return old, table, fmt.Errorf("位数超限: 位 %s 超出掩码位数 %d，迁移失败，旧掩码保持不变",
			joinBits(overflowBits(limit)), MaxLayers)
	}
	table := buildTable(oldLayers, newLayers, MaxLayers)
	out := old
	// 丢弃超出新图层数的旧位。
	if newLayers < MaxLayers {
		keep := Mask(uint64(1)<<uint(newLayers)) - 1
		out &= keep
	}
	// 新增层默认可见。
	for b := oldLayers; b < newLayers; b++ {
		out |= Mask(uint64(1) << uint(b))
	}
	return out, table, nil
}

func buildTable(oldLayers, newLayers, max int) []Mapping {
	n := oldLayers
	if newLayers > n {
		n = newLayers
	}
	if n > max {
		n = max
	}
	table := make([]Mapping, 0, n)
	for b := 0; b < n; b++ {
		switch {
		case b < oldLayers && b < newLayers:
			table = append(table, Mapping{OldBit: b, NewBit: b, Note: "同一层，保留"})
		case b < oldLayers:
			table = append(table, Mapping{OldBit: b, NewBit: -1, Note: "新图层数不含该层"})
		default:
			table = append(table, Mapping{OldBit: b, NewBit: b, Note: "新增图层，默认可见"})
		}
	}
	return table
}

func overflowBits(layers int) []int {
	if layers <= MaxLayers {
		return nil
	}
	bits := make([]int, 0, layers-MaxLayers)
	for b := MaxLayers; b < layers; b++ {
		bits = append(bits, b)
	}
	return bits
}

func joinBits(bits []int) string {
	if len(bits) == 0 {
		return "(无)"
	}
	parts := make([]string, len(bits))
	for i, b := range bits {
		parts[i] = fmt.Sprintf("%d", b)
	}
	if len(parts) > 8 {
		return strings.Join(parts[:4], ",") + ",…," + strings.Join(parts[len(parts)-2:], ",")
	}
	return strings.Join(parts, ",")
}
