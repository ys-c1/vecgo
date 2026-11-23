// matdemo 打印矩阵分解表与两档场景的分解误差、耗时、顺序核对结果。
package main

import (
	"fmt"
	"os"
	"text/tabwriter"

	"vecgo/mat"
)

func main() {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	fmt.Println("== 第一档：典型用例分解表 ==")
	fmt.Fprintln(w, "用例\t状态\t平移\t旋转(rad)\t缩放(sx,sy)\t错切h\t乘回最大误差\t耗时\t顺序核对")
	for _, r := range mat.RunTier1() {
		status := "OK"
		detail := "-"
		if r.Err != nil {
			status = "报错"
			if r.Pos != nil {
				detail = fmt.Sprintf("元素(%d,%d)=%g %s", r.Pos.Row, r.Pos.Col, r.Pos.Val, r.Pos.Msg)
			} else {
				detail = r.Err.Error()
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t-\t-\t-\t-\t%s\t-\n", r.Name, status, detail, r.Elapsed)
			continue
		}
		p := r.Parts
		order := "一致"
		if r.OrderErr != nil {
			order = "不一致:" + r.OrderErr.Error()
		}
		fmt.Fprintf(w, "%s\t%s\t(%.4g,%.4g)\t%.4g\t(%.4g,%.4g)\t%.4g\t%.2e\t%s\t%s\n",
			r.Name, status, p.TX, p.TY, p.Angle, p.SX, p.SY, p.SH,
			r.MaxErr, r.Elapsed, order)
	}
	w.Flush()

	fmt.Println()
	fmt.Println("== 第一档：乘回矩阵逐元素核对（全复合用例） ==")
	for _, s := range mat.Tier1Scenarios() {
		if s.ExpectErr {
			continue
		}
		r := mat.RunScenario(s)
		fmt.Printf("【%s】\n原矩阵:\n%s\n乘回矩阵:\n%s\n逐元素最大绝对差: %.3e\n\n",
			r.Name, fmtMat(s.M), fmtMat(r.Recomposed), r.MaxErr)
	}

	fmt.Println("== 第二档：随机非退化矩阵压力用例 ==")
	for _, n := range []int{100, 10000} {
		res := mat.RunStress(n, 20261124)
		fmt.Fprintf(w, "n=%d\t最大乘回误差=%.3e\t分解失败=%d\t顺序不匹配=%d\t总耗时=%s\t单次平均=%s\n",
			res.N, res.MaxRecomposeErr, res.RecomposeFailures, res.OrderMismatches,
			res.TotalElapsed, res.AvgElapsed)
	}
	w.Flush()
}

func fmtMat(m mat.Mat3) string {
	v := &m.V
	return fmt.Sprintf("  [%10.6g %10.6g %10.6g]\n  [%10.6g %10.6g %10.6g]\n  [%10.6g %10.6g %10.6g]",
		v[0], v[1], v[2], v[3], v[4], v[5], v[6], v[7], v[8])
}
