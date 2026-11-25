// 命令 cliparea 跑三档分块裁剪的面积误差实验，并列打印每档的实测面积差、
// 实测上界、与顶点数成正比的闭式上界 C·V，以及逐档判定。
package main

import (
	"fmt"

	"vecgo/cliparea"
	"vecgo/geom"
)

func main() {
	// 六边形完全位于 [0,10]² 内，且 (3,10)、(0,6)、(7,0) 等顶点
	// 落在 2×2 / 4×4 网格的裁剪线上，用来制造可统计的线上二义情形。
	poly := []geom.Point{
		{X: 0, Y: 0}, {X: 7, Y: 0}, {X: 10, Y: 4},
		{X: 6, Y: 10}, {X: 3, Y: 10}, {X: 0, Y: 6},
	}
	reports, err := cliparea.Run(poly)
	if err != nil {
		panic(err)
	}
	coef := cliparea.ComputeCoefficient(poly)
	fmt.Print(cliparea.FormatTable(reports, coef))
	fmt.Println()
	fmt.Println("二义判据（一句）: 当 |e×(p-a)| ≤ 8u·|e|·|p-a| 时该点在线上，内/外分类不可信，")
	fmt.Println("统一按“在内”处理并计入二义次数；精确有理路径以叉积符号 c=0 为准，不受二义影响。")
}
