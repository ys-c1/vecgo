package main

import (
	"fmt"
	"math"

	"vecgo/geom"
	"vecgo/precision"
)

type level struct {
	name string
	eps  float64
}

func main() {
	chains := []precision.Chain{
		{Name: "轻载 L (s=0.5)", Scale: 0.5, Rotate: 15 * math.Pi / 180, Shift: geom.Point{X: 3.2, Y: -1.4}},
		{Name: "中载 M (s=8)", Scale: 8, Rotate: 37 * math.Pi / 180, Shift: geom.Point{X: -12.5, Y: 7.75}},
		{Name: "重载 H (s=25)", Scale: 25, Rotate: 120 * math.Pi / 180, Shift: geom.Point{X: 40.125, Y: -33.5}},
	}
	levels := []level{
		{"ε₀=1e-2", 1e-2},
		{"ε₁=1e-3", 1e-3},
		{"ε₂=1e-4", 1e-4},
	}
	pts := precision.Grid(41, 10) // 1681 个覆盖 [0,10]² 的样本点

	for _, lv := range levels {
		fmt.Printf("\n### 误差上界 %s\n\n", lv.name)
		fmt.Println("| 变换链 | 权重 (平移, 旋转, 缩放) | 分配位数 (平移/旋转/缩放) | 分配总位数 | 理论界 | 实测最大偏差 | 等精度位数 | 等精度总位数 | 位数节省 |")
		fmt.Println("|---|---|---|---|---|---|---|---|---|")
		for _, ch := range chains {
			w := ch.Weights()
			dg, err := precision.Allocate(w, lv.eps)
			if err != nil {
				panic(err)
			}
			if g, err := precision.AllocateGreedy(w, lv.eps); err != nil ||
				g[0]+g[1]+g[2] != dg[0]+dg[1]+dg[2] ||
				math.Abs(precision.Bound(w, g)-precision.Bound(w, dg)) > 1e-15 {
				panic(fmt.Sprintf("贪心与穷举不等价: %v vs %v (%v)", g, dg, err))
			}
			eq, err := precision.EqualDigits(w, lv.eps)
			if err != nil {
				panic(err)
			}
			b := precision.Bound(w, dg)
			dev, _ := ch.MaxDeviation(pts, dg)
			allocTotal := dg[0] + dg[1] + dg[2]
			fmt.Printf("| %s | (%.3g, %.3g, %.3g) | %d/%d/%d | %d | %.3e | %.3e | %d/%d/%d | %d | %d |\n",
				ch.Name, w[0], w[1], w[2], dg[0], dg[1], dg[2], allocTotal, b, dev, eq, eq, eq, 3*eq, 3*eq-allocTotal)
		}
	}
}
