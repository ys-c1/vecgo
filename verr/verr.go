// Package verr 把 config / geom / raster / shading / post 五处的失败错误
// 归到同一套编码里，供上层按包分段做故障隔离。
//
// 编码规则（Kind 是一个 32 位值）：
//
//	Kind = 0x0000_0000        未归一
//	Kind = Stage<<8 | point   已归一，Stage 是包分段（1..6），point 是段内失败点
//
//	Stage = 1 config   编码落在 0x0101 .. 0x01FF
//	Stage = 2 geom              0x0201 .. 0x02FF
//	Stage = 3 raster            0x0301 .. 0x03FF
//	Stage = 4 shading           0x0401 .. 0x04FF
//	Stage = 5 post              0x0501 .. 0x05FF
//	Stage = 6 snap              0x0601 .. 0x06FF（快照比对自身，不参与渲染隔离）
//
// 已有的编码一旦发出就不要改；新增失败点只能占用段内空闲槽位并追加常量，
// 不要在中间插入，否则日志与历史快照会错位。
//
// Kind 与 Stage 两个维度刻意分开：Stage 回答“哪一段渲染要摘掉”，
// Kind 回答“这一处具体是什么失败”。调用方通常只关心 Stage；
// Stage 不单独存一份，永远由 Kind 的段位推导，所以两者不会对不上。
package verr

import (
	"errors"
	"fmt"
)

// Stage 是渲染阶段的包分段，顺序与 pipe 的执行顺序一致。
type Stage uint32

const (
	StageNone Stage = iota
	StageConfig
	StageGeom
	StageRaster
	StageShading
	StagePost
	StageSnap
)

// Base 返回该段的编码下界，例如 StageRaster.Base() == 0x0300。
func (s Stage) Base() Kind {
	if s == StageNone || s > StageSnap {
		return 0
	}
	return Kind(uint32(s) << 8)
}

// String 让日志可读。
func (s Stage) String() string {
	switch s {
	case StageConfig:
		return "config"
	case StageGeom:
		return "geom"
	case StageRaster:
		return "raster"
	case StageShading:
		return "shading"
	case StagePost:
		return "post"
	case StageSnap:
		return "snap"
	}
	return "none"
}

// Kind 是归一后的失败编码。
type Kind uint32

const (
	// config 段 0x01xx
	KindConfigWidth       Kind = 0x0101
	KindConfigHeight      Kind = 0x0102
	KindConfigScale       Kind = 0x0103
	KindConfigRotate      Kind = 0x0104
	KindConfigSupersample Kind = 0x0105
	KindConfigGamma       Kind = 0x0106
	KindConfigBlur        Kind = 0x0107
	KindConfigShading     Kind = 0x0108
	KindConfigOutput      Kind = 0x0109

	// geom 段 0x02xx
	KindGeomSegments  Kind = 0x0201
	KindGeomClipVerts Kind = 0x0202
	KindGeomStrokeW   Kind = 0x0203
	KindGeomDupPoint  Kind = 0x0204
	KindGeomFewVerts  Kind = 0x0205
	KindGeomNonFinite Kind = 0x0206
	KindGeomEmptyPts  Kind = 0x0207
	KindGeomTooFewPts Kind = 0x0208

	// raster 段 0x03xx
	KindRasterSize     Kind = 0x0301
	KindRasterSuper    Kind = 0x0302
	KindRasterPolyWide Kind = 0x0303
	KindRasterEmptyImg Kind = 0x0304
	KindRasterCreate   Kind = 0x0305
	KindRasterHeader   Kind = 0x0306
	KindRasterWrite    Kind = 0x0307
	KindRasterClose    Kind = 0x0308
	KindRasterNilImage Kind = 0x0309
	KindRasterBadImage Kind = 0x030A
	KindRasterTooBig   Kind = 0x030B

	// shading 段 0x04xx
	KindShadingMode     Kind = 0x0401
	KindShadingStrength Kind = 0x0402
	KindShadingNilImage Kind = 0x0403
	KindShadingBadImage Kind = 0x0404

	// post 段 0x05xx
	KindPostGamma      Kind = 0x0501
	KindPostBlurRadius Kind = 0x0502
	KindPostNilImage   Kind = 0x0503
	KindPostBlurImage  Kind = 0x0504
	KindPostBadImage   Kind = 0x0505

	// snap 段 0x06xx：基准快照文件的读写/解析/比对，不属于五段渲染
	KindSnapRead     Kind = 0x0601
	KindSnapParse    Kind = 0x0602
	KindSnapMismatch Kind = 0x0603
	KindSnapWrite    Kind = 0x0604
)

// Stage 由编码的段位推导，单一真相来源。
func (k Kind) Stage() Stage {
	if s := uint32(k) >> 8; s >= 1 && s <= 6 {
		return Stage(s)
	}
	return StageNone
}

// Hex 是日志与快照里使用的稳定表示，例如 "0x0302"。
func (k Kind) Hex() string { return fmt.Sprintf("0x%04X", uint32(k)) }

// Error 是一次归一失败。Cause 保留原始错误（若有）。
type Error struct {
	Kind  Kind
	Cause error
}

func (e *Error) Error() string {
	if e.Cause == nil {
		return fmt.Sprintf("%s[%s]", e.Kind.Stage().String(), e.Kind.Hex())
	}
	return fmt.Sprintf("%s[%s]: %v", e.Kind.Stage().String(), e.Kind.Hex(), e.Cause)
}

// Unwrap 让 errors.Is/As 能穿透到底层原因（例如 fs.PathError）。
func (e *Error) Unwrap() error { return e.Cause }

// Errorf 构造一条归一错误。参数形式同 fmt.Errorf。
func Errorf(k Kind, format string, args ...any) error {
	if format == "" && len(args) == 0 {
		return &Error{Kind: k}
	}
	return &Error{Kind: k, Cause: fmt.Errorf(format, args...)}
}

// Wrap 把外部错误（例如 os.Create / f.Write 的返回值）包进指定编码。
// nil 直接返回 nil；已经是 *Error 的保留原编码，避免重复包裹。
func Wrap(k Kind, err error) error {
	if err == nil {
		return nil
	}
	var ve *Error
	if errors.As(err, &ve) {
		return err
	}
	return &Error{Kind: k, Cause: err}
}

// Of 从错误链里取出编码。不是 *Error 的返回 0, false。
func Of(err error) (Kind, bool) {
	var ve *Error
	if errors.As(err, &ve) {
		return ve.Kind, true
	}
	return 0, false
}

// StageOf 从错误链里取出阶段；未归一的返回 StageNone。
func StageOf(err error) Stage {
	var ve *Error
	if errors.As(err, &ve) {
		return ve.Kind.Stage()
	}
	return StageNone
}

// IsKind 报告错误链里是否有指定编码。
func IsKind(err error, k Kind) bool {
	got, ok := Of(err)
	return ok && got == k
}

// Multi 把多条归一错误合成一条，Unwrap 支持 []error 以便 errors.Is/As
// 能同时找到其中任意一条。
type Multi struct {
	Errs []error
}

// NewMulti 合并多条错误；nil 项被忽略。没有有效项时返回 nil。
func NewMulti(errs ...error) error {
	kept := make([]error, 0, len(errs))
	for _, e := range errs {
		if e != nil {
			kept = append(kept, e)
		}
	}
	switch len(kept) {
	case 0:
		return nil
	case 1:
		return kept[0]
	}
	return &Multi{Errs: kept}
}

func (m *Multi) Error() string {
	b := make([]byte, 0, 24*len(m.Errs))
	b = append(b, "多段失败:"...)
	for _, e := range m.Errs {
		b = append(b, ' ')
		if k, ok := Of(e); ok {
			b = append(b, k.Stage().String()...)
			b = append(b, '[')
			b = append(b, k.Hex()...)
			b = append(b, ']')
		} else {
			b = append(b, "unclassified"...)
		}
	}
	return string(b)
}

// Unwrap 返回全部底层错误（Go 1.20+ 的 []error 语义）。
func (m *Multi) Unwrap() []error { return m.Errs }
