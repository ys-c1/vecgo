//go:build !vecgotiming

// 计时关闭时的空实现：Start/Done 均为空函数，编译器内联后零开销，
// pipe.Run 的可观测行为与输出字节与加入计时点之前完全一致。
package timing

const (
	StageTransform = "transform"
	StageRasterize = "rasterize"
	StageShading   = "shading"
	StageGamma     = "gamma"
	StageBlur      = "blur"
)

func Start(stage string) {}

func Done(stage string) {}

func Reset() {}
