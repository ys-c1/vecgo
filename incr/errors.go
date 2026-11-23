package incr

import (
	"fmt"

	"vecgo/raster"
)

// ErrorKind 标出增量输入属于哪一类错误。
type ErrorKind string

const (
	// ErrRectOutOfBounds: 脏矩形越界（含退化矩形与超出画幅）。
	ErrRectOutOfBounds ErrorKind = "rect-out-of-bounds"
	// ErrNoPreviousFrame: 还没有上一帧就要按增量路径渲染。
	ErrNoPreviousFrame ErrorKind = "no-previous-frame"
	// ErrOverlapConflict: 调用方给出的两个脏矩形有面积重叠，账目无法对齐。
	ErrOverlapConflict ErrorKind = "overlap-conflict"
	// ErrBadSource: 未知的脏矩形来源。
	ErrBadSource ErrorKind = "unknown-source"
	// ErrEmptyScene: 场景为空，无从采样。
	ErrEmptyScene ErrorKind = "empty-scene"
	// ErrBadGeometry: 场景中的多边形少于三个顶点。
	ErrBadGeometry ErrorKind = "bad-geometry"
)

// DirtyError 是三类脏矩形输入错误统一携带的信息。
// Index 即出问题的矩形号（0 基）；没有具体矩形时为 0，
// 但 RectNumber 仍可读取，便于按“带矩形号”处理。
type DirtyError struct {
	Kind      ErrorKind
	Index     int
	Other     int
	Rect      raster.Rect
	OtherRect raster.Rect
	W, H      int
}

// RectNumber 返回该错误指向的脏矩形编号（0 基）。
func (e *DirtyError) RectNumber() int { return e.Index }

// OtherRectNumber 返回冲突中的另一个矩形编号；非冲突错误返回 -1。
func (e *DirtyError) OtherRectNumber() int {
	if e.Kind == ErrOverlapConflict {
		return e.Other
	}
	return -1
}

func (e *DirtyError) Error() string {
	switch e.Kind {
	case ErrRectOutOfBounds:
		return fmt.Sprintf("脏矩形 #%d 越界或退化: [%d,%d,%d,%d) 画幅 %dx%d",
			e.Index, e.Rect.X0, e.Rect.Y0, e.Rect.X1, e.Rect.Y1, e.W, e.H)
	case ErrNoPreviousFrame:
		return fmt.Sprintf("上一帧缺失: 脏矩形 #%d 无法定位到上一帧", e.Index)
	case ErrEmptyScene:
		return "场景为空: 至少需要一个多边形"
	case ErrBadGeometry:
		return fmt.Sprintf("场景第 #%d 个多边形不足三个顶点", e.Index)
	case ErrOverlapConflict:
		return fmt.Sprintf("脏矩形 #%d 与 #%d 重叠: [%d,%d,%d,%d) ∩ [%d,%d,%d,%d)",
			e.Index, e.Other,
			e.Rect.X0, e.Rect.Y0, e.Rect.X1, e.Rect.Y1,
			e.OtherRect.X0, e.OtherRect.Y0, e.OtherRect.X1, e.OtherRect.Y1)
	default:
		return fmt.Sprintf("脏矩形错误 #%d: %s", e.Index, string(e.Kind))
	}
}
