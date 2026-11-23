package mat

import (
	"errors"
	"math"
)

// OpKind 标识构造变换的原语类型。
type OpKind int

const (
	OpTranslate OpKind = iota
	OpRotate
	OpScale
	OpShear
)

// String 返回原语的中文名。
func (k OpKind) String() string {
	switch k {
	case OpTranslate:
		return "平移"
	case OpRotate:
		return "旋转"
	case OpScale:
		return "缩放"
	case OpShear:
		return "错切"
	default:
		return "未知"
	}
}

// Op 记录一次构造步骤。Param 分别为：平移 tx、旋转角 theta、
// 缩放 sx（sy 见 Param2）、错切 h。
type Op struct {
	Kind   OpKind
	Param  float64
	Param2 float64
}

// Builder 按实际构造顺序累积原语，并维护乘积矩阵。
// 每次追加原语 P 后当前矩阵变为 M·P，即点先经受新追加的 P。
type Builder struct {
	cur Mat3
	ops []Op
}

// NewBuilder 返回单位矩阵起步的构造器。
func NewBuilder() *Builder {
	return &Builder{cur: Identity()}
}

func (b *Builder) add(op Op, m Mat3) {
	b.cur = b.cur.Mul(m)
	b.ops = append(b.ops, op)
}

// Translate 追加一次平移。
func (b *Builder) Translate(tx, ty float64) *Builder {
	b.add(Op{Kind: OpTranslate, Param: tx, Param2: ty}, Translate(tx, ty))
	return b
}

// Rotate 追加一次旋转（弧度）。
func (b *Builder) Rotate(theta float64) *Builder {
	b.add(Op{Kind: OpRotate, Param: theta}, Rotate(theta))
	return b
}

// Scale 追加一次（可非均匀）缩放。
func (b *Builder) Scale(sx, sy float64) *Builder {
	b.add(Op{Kind: OpScale, Param: sx, Param2: sy}, Scale(sx, sy))
	return b
}

// Shear 追加一次错切。
func (b *Builder) Shear(h float64) *Builder {
	b.add(Op{Kind: OpShear, Param: h}, Shear(h))
	return b
}

// Matrix 返回当前乘积矩阵。
func (b *Builder) Matrix() Mat3 { return b.cur }

// Ops 返回实际构造顺序的拷贝。
func (b *Builder) Ops() []Op {
	out := make([]Op, len(b.ops))
	copy(out, b.ops)
	return out
}

// angleEqual 归一化比较两个角度（相差 2π 视为相同）。
func angleEqual(a, b float64) bool {
	d := math.Mod(a-b, 2*math.Pi)
	return math.Abs(d) < identityEps
}

// InferredOrder 由分解结果反解非单位分量的原始构造顺序，
// 规范化为 [平移, 旋转, 缩放, 错切] 中实际出现过的部分。
func InferredOrder(p Parts) []OpKind {
	var order []OpKind
	if math.Abs(p.TX) > identityEps || math.Abs(p.TY) > identityEps {
		order = append(order, OpTranslate)
	}
	if math.Abs(p.Angle) > identityEps {
		order = append(order, OpRotate)
	}
	if math.Abs(p.SX-1) > identityEps || math.Abs(p.SY-1) > identityEps {
		order = append(order, OpScale)
	}
	if math.Abs(p.SH) > identityEps {
		order = append(order, OpShear)
	}
	return order
}

// CheckOrder 把分解反解出的顺序与实际记录的构造顺序核对。
// 比较前先把记录顺序规范化（去重、按 T→R→S→H 排序，跳过单位原语），
// 与分解反解的规范化顺序逐位比对；同时抽查原语参数是否与分解参数一致。
func CheckOrder(recorded []Op, p Parts) error {
	want := InferredOrder(p)

	seen := map[OpKind]bool{}
	var got []OpKind
	for _, op := range recorded {
		if isIdentityOp(op) || seen[op.Kind] {
			continue
		}
		seen[op.Kind] = true
		got = append(got, op.Kind)
	}
	// 记录顺序未必按规范排列，这里按固定优先级排序后再比对。
	rank := map[OpKind]int{OpTranslate: 0, OpRotate: 1, OpScale: 2, OpShear: 3}
	for i := 1; i < len(got); i++ {
		for j := i; j > 0 && rank[got[j-1]] > rank[got[j]]; j-- {
			got[j-1], got[j] = got[j], got[j-1]
		}
	}
	if len(got) != len(want) {
		return errors.New("顺序核对失败：反解分量数与记录的非单位原语数不一致")
	}
	for i := range got {
		if got[i] != want[i] {
			return errors.New("顺序核对失败：实际顺序与分解反解顺序不一致")
		}
	}

	// 参数抽查：记录值必须能在分解结果中对上。
	for _, op := range recorded {
		switch op.Kind {
		case OpTranslate:
			if math.Abs(op.Param-p.TX) > identityEps || math.Abs(op.Param2-p.TY) > identityEps {
				return errors.New("顺序核对失败：平移参数与分解结果不一致")
			}
		case OpRotate:
			if !angleEqual(op.Param, p.Angle) {
				return errors.New("顺序核对失败：旋转角与分解结果不一致")
			}
		case OpScale:
			if math.Abs(op.Param-p.SX) > identityEps || math.Abs(op.Param2-p.SY) > identityEps {
				return errors.New("顺序核对失败：缩放参数与分解结果不一致")
			}
		case OpShear:
			if math.Abs(op.Param-p.SH) > identityEps {
				return errors.New("顺序核对失败：错切参数与分解结果不一致")
			}
		}
	}
	return nil
}

// isIdentityOp 判断一次原语是否为单位变换（单位原语不影响矩阵）。
func isIdentityOp(op Op) bool {
	switch op.Kind {
	case OpTranslate:
		return math.Abs(op.Param) <= identityEps && math.Abs(op.Param2) <= identityEps
	case OpRotate:
		return math.Abs(op.Param) <= identityEps
	case OpScale:
		return math.Abs(op.Param-1) <= identityEps && math.Abs(op.Param2-1) <= identityEps
	case OpShear:
		return math.Abs(op.Param) <= identityEps
	default:
		return true
	}
}
