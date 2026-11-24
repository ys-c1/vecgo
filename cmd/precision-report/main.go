// precision-report 输出三步坐标变换的条件数、位数分配表与实测总误差。
//
// 用法: go run ./cmd/precision-report [E]
package main

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"strconv"

	"vecgo/geom"
	"vecgo/pipe/precision"
)

func main() {
	E := 0.01
	if len(os.Args) > 1 {
		v, err := strconv.ParseFloat(os.Args[1], 64)
		if err != nil || v <= 0 {
			fmt.Fprintln(os.Stderr, "E 必须为正数")
			os.Exit(1)
		}
		E = v
	}
	const k, tx, ty = 2.0, 32.0, 32.0
	angles := []struct {
		deg float64
		rad float64
	}{{0, 0}, {30, math.Pi / 6}, {45, math.Pi / 4}}

	fmt.Printf("误差上限 E = %g，缩放 k=%g，平移 (%g,%g)\n\n", E, k, tx, ty)

	for _, a := range angles {
		st := precision.Conditions(k, a.rad, tx, ty)
		fmt.Printf("=== θ=%g°（cos=%g sin=%g）===\n", a.deg, math.Cos(a.rad), math.Sin(a.rad))
		fmt.Println("阶段        雅可比              κ₂   ‖J‖∞   到输出放大 a_i")
		for _, s := range st {
			j := s.Jacobian
			fmt.Printf("%-9s  [%5.3f %5.3f]      %4.2f %6.4f  %8.5f\n",
				s.Name, j[0][0], j[0][1], s.Cond2, s.InfGain, s.AmplifyTo)
			fmt.Printf("%-9s  [%5.3f %5.3f]\n", "", j[1][0], j[1][1])
		}

		pa := precision.PlanAdaptive(k, a.rad, tx, ty, E)
		pe := precision.PlanEqual(k, a.rad, tx, ty, E)
		fmt.Println()
		fmt.Println("位数表（逐行）：")
		fmt.Println("阶段        a_i      连续预算 ε_i   理想位 x_i | 自适应 d_i | 等精度 d_i")
		for i := range pa.Stages {
			sa, se := pa.Stages[i], pe.Stages[i]
			fmt.Printf("%-9s  %7.4f  %12.3e  %9.4f  | %9d  | %9d\n",
				sa.Name, sa.Amplify, sa.Budget, sa.Target, sa.Digits, se.Digits)
		}
		fmt.Printf("合计位数: 自适应 %d  等精度 %d\n", pa.TotalDigits(), pe.TotalDigits())
		fmt.Printf("保证上界: 自适应 %.6g  等精度 %.6g  (E=%g)\n\n", pa.Bound(), pe.Bound(), E)
	}

	// 实测总误差：每角度 100 万个随机点，取分量误差最坏值。
	fmt.Println("实测总误差（100 万随机点/角度，点坐标 ∈[0,100]）：")
	fmt.Println("角度   自适应实测最坏   自适应保证上界 | 等精度实测最坏   等精度保证上界")
	rng := rand.New(rand.NewSource(42))
	for _, a := range angles {
		pa := precision.PlanAdaptive(k, a.rad, tx, ty, E)
		pe := precision.PlanEqual(k, a.rad, tx, ty, E)
		var wa, we float64
		for n := 0; n < 1_000_000; n++ {
			p := geom.Point{X: rng.Float64() * 100, Y: rng.Float64() * 100}
			ga := precision.Apply(p, k, a.rad, tx, ty, pa.DigitsOf())
			ge := precision.Apply(p, k, a.rad, tx, ty, pe.DigitsOf())
			ref := precision.Exact(p, k, a.rad, tx, ty)
			if v := math.Max(math.Abs(ga.X-ref.X), math.Abs(ga.Y-ref.Y)); v > wa {
				wa = v
			}
			if v := math.Max(math.Abs(ge.X-ref.X), math.Abs(ge.Y-ref.Y)); v > we {
				we = v
			}
		}
		fmt.Printf("%3g°  %13.6e  %13.3e | %13.6e  %13.3e\n",
			a.deg, wa, pa.Bound(), we, pe.Bound())
	}
}
