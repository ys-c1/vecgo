package main

import (
	"fmt"

	"vecgo/matrixchain"
)

func main() {
	chains := matrixchain.DefaultChains()
	reports := make([]matrixchain.Report, 0, len(chains))
	for _, ch := range chains {
		reports = append(reports, matrixchain.BuildReport(ch))
	}

	fmt.Println("# 五层矩阵链：结合顺序误差实验")
	fmt.Printf("machine epsilon = %.17g\n\n", matrixchain.Epsilon)
	fmt.Print(matrixchain.FormatTable(reports))
	fmt.Println()
	// chain-3 has the largest direct order difference and gives the
	// elementwise comparison requested in the analysis.
	fmt.Print(matrixchain.FormatElementComparison(reports[2]))
}
