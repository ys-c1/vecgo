package matrixchain

import (
	"fmt"
	"math"
	"strings"
)

// Mat3 is a homogeneous 3x3 transform matrix.
type Mat3 [3][3]float64

// Epsilon is the IEEE-754 binary64 machine epsilon reported by Go.
// The tighter unit roundoff is Epsilon / 2.  Using Epsilon in the bound is
// deliberately conservative.
const Epsilon = 2.220446049250313080847263336181640625e-16

// MulFloat multiplies two binary64 matrices with an explicit k-loop.
// Keeping the loop explicit makes the association order observable.
func MulFloat(a, b Mat3) Mat3 {
	var c Mat3
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			s := 0.0
			for k := 0; k < 3; k++ {
				p := a[i][k] * b[k][j]
				s += p
			}
			c[i][j] = s
		}
	}
	return c
}

// LeftAssoc evaluates ((((A0 A1) A2) A3) A4).
func LeftAssoc(ms [5]Mat3) Mat3 {
	c := ms[0]
	for i := 1; i < 5; i++ {
		c = MulFloat(c, ms[i])
	}
	return c
}

// Chain is one five-layer matrix chain.
type Chain struct {
	Name string
	Mats [5]Mat3
}

// ElementReport keeps the two elementwise error matrices and the direct
// elementwise difference between the two floating-point orders.
type ElementReport struct {
	LeftError  Mat3
	OptError   Mat3
	OrderDelta Mat3
	MaxDiff    float64
}

// Report contains the numbers needed beside one chain.
type Report struct {
	Name          string
	Order         string
	Left          Mat3
	Optimal       Mat3
	Exact         Mat3
	Norm          float64
	LeftDev       float64
	OptDev        float64
	LeftKappa     float64
	OptKappa      float64
	LeftBound     float64
	OptBound      float64
	LeftCertKappa float64
	OptCertKappa  float64
	LeftCertBound float64
	OptCertBound  float64
	Elements      ElementReport
}

// DefaultChains returns the three reproducible affine transform stacks.
// Their decimal literals are rounded to binary64 exactly as Go float64
// constants normally are; the high-precision reference uses those stored
// binary64 values, not the ideal decimal numbers.
func DefaultChains() []Chain {
	return []Chain{
		{
			Name: "chain-1",
			Mats: [5]Mat3{
				{{0, -0.1, -24}, {-0.2, 0, 60}, {0, 0, 1}},
				{{0, -0.3, -85}, {-0.2, -1, -67}, {0, 0, 1}},
				{{1, 0.2, -113}, {0, 1, -4}, {0, 0, 1}},
				{{1, 0, 26}, {-0.2, 1, -98}, {0, 0, 1}},
				{{1, -0.2, 74}, {-0.3, -1, -52}, {0, 0, 1}},
			},
		},
		{
			Name: "chain-2",
			Mats: [5]Mat3{
				{{-1, 0, 94}, {0.1, 0, -65}, {0, 0, 1}},
				{{0, -0.2, -77}, {0.2, -1, -97}, {0, 0, 1}},
				{{-1, 0.3, -82}, {0.3, -1, -48}, {0, 0, 1}},
				{{-1, 0.3, -117}, {0, 1, 101}, {0, 0, 1}},
				{{-1, 0.2, -91}, {-0.3, -1, -77}, {0, 0, 1}},
			},
		},
		{
			Name: "chain-3",
			Mats: [5]Mat3{
				{{-1, -0.3, -114}, {0.2, 1, -58}, {0, 0, 1}},
				{{0, -0.2, 68}, {-0.3, -1, 69}, {0, 0, 1}},
				{{-1, -0.3, 31}, {0, 0, -113}, {0, 0, 1}},
				{{0, -0.2, -61}, {0.1, -1, -114}, {0, 0, 1}},
				{{-1, -0.2, 63}, {0.2, -1, 19}, {0, 0, 1}},
			},
		},
	}
}

// BuildReport computes the DP-optimal bound order and all reported errors.
func BuildReport(ch Chain) Report {
	const n = 5
	ms := ch.Mats[:]
	exact := exactIntervals(ms)
	weights := nodeWeights(ms, exact)
	optKappa, split := solveDP(weights)
	optimal := evalDP(ms, split, 0, n)
	order := orderString(split, 0, n)
	left := LeftAssoc(ch.Mats)

	exactRoot := exact[0][n]
	norm := maxAbsBigMat(exactRoot)
	normF, _ := norm.Float64()

	leftKappa := leftKappa(weights)
	leftDev, leftErr := maxElementError(left, exactRoot)
	optDev, optErr := maxElementError(optimal, exactRoot)

	var leftSplit [6][6]int
	for j := 2; j <= n; j++ {
		leftSplit[0][j] = j - 1
	}
	_, leftCertE := certifiedError(ms, exact, leftSplit, 0, n)
	_, optCertE := certifiedError(ms, exact, split, 0, n)
	leftCertBound, _ := maxAbsBigMat(leftCertE).Float64()
	optCertBound, _ := maxAbsBigMat(optCertE).Float64()

	var exactApprox Mat3
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			exactApprox[i][j], _ = exactRoot[i][j].Float64()
		}
	}

	var orderDelta Mat3
	maxDiff := 0.0
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			orderDelta[i][j] = left[i][j] - optimal[i][j]
			maxDiff = math.Max(maxDiff, math.Abs(orderDelta[i][j]))
		}
	}

	return Report{
		Name:          ch.Name,
		Order:         order,
		Left:          left,
		Optimal:       optimal,
		Exact:         exactApprox,
		Norm:          normF,
		LeftDev:       leftDev,
		OptDev:        optDev,
		LeftKappa:     leftKappa,
		OptKappa:      optKappa,
		LeftBound:     leftKappa * Epsilon * normF,
		OptBound:      optKappa * Epsilon * normF,
		LeftCertKappa: leftCertBound / (Epsilon * normF),
		OptCertKappa:  optCertBound / (Epsilon * normF),
		LeftCertBound: leftCertBound,
		OptCertBound:  optCertBound,
		Elements: ElementReport{
			LeftError:  leftErr,
			OptError:   optErr,
			OrderDelta: orderDelta,
			MaxDiff:    maxDiff,
		},
	}
}

type nodeKey struct{ i, k, j int }

func solveDP(w map[nodeKey]float64) (float64, [6][6]int) {
	const n = 5
	var dp [6][6]float64
	var split [6][6]int
	for i := 0; i < n; i++ {
		dp[i][i+1] = 0
	}
	for length := 2; length <= n; length++ {
		for i := 0; i+length <= n; i++ {
			j := i + length
			dp[i][j] = math.Inf(1)
			for k := i + 1; k < j; k++ {
				cost := dp[i][k] + dp[k][j] + w[nodeKey{i, k, j}]
				if cost < dp[i][j] {
					dp[i][j] = cost
					split[i][j] = k
				}
			}
		}
	}
	return dp[0][n], split
}

func evalDP(ms []Mat3, split [6][6]int, i, j int) Mat3 {
	if j-i == 1 {
		return ms[i]
	}
	k := split[i][j]
	return MulFloat(evalDP(ms, split, i, k), evalDP(ms, split, k, j))
}

func orderString(split [6][6]int, i, j int) string {
	if j-i == 1 {
		return fmt.Sprintf("A%d", i+1)
	}
	k := split[i][j]
	return "(" + orderString(split, i, k) + "·" + orderString(split, k, j) + ")"
}

func leftKappa(w map[nodeKey]float64) float64 {
	// Left-deep tree nodes are [0,2], [0,3], [0,4], [0,5].
	return w[nodeKey{0, 1, 2}] +
		w[nodeKey{0, 2, 3}] +
		w[nodeKey{0, 3, 4}] +
		w[nodeKey{0, 4, 5}]
}

// FormatTable prints the side-by-side deviation/bound table.
func FormatTable(rs []Report) string {
	var b strings.Builder
	fmt.Fprintln(&b, "| 链 | DP 最小上界顺序 | κ左 | κ* | 左偏差 | 左偏差上界 | 最优偏差 | 最优偏差上界 |")
	fmt.Fprintln(&b, "|---|---|---:|---:|---:|---:|---:|---:|")
	for _, r := range rs {
		fmt.Fprintf(&b,
			"| %s | `%s` | %.6f | %.6f | %.6e | %.6e | %.6e | %.6e |\n",
			r.Name, r.Order, r.LeftKappa, r.OptKappa,
			r.LeftDev, r.LeftBound, r.OptDev, r.OptBound,
		)
	}
	return b.String()
}

// FormatElementComparison prints one elementwise comparison.
func FormatElementComparison(r Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## %s 逐元素比较\n\n", r.Name)
	fmt.Fprintln(&b, "左结合偏差 ΔL = fl_left(A)-A：")
	fmt.Fprintln(&b, formatMat(r.Elements.LeftError, "%.3e"))
	fmt.Fprintln(&b, "\n最优顺序偏差 Δ* = fl_opt(A)-A：")
	fmt.Fprintln(&b, formatMat(r.Elements.OptError, "%.3e"))
	fmt.Fprintln(&b, "\n两种浮点顺序之差 fl_left(A)-fl_opt(A)：")
	fmt.Fprintln(&b, formatMat(r.Elements.OrderDelta, "%.3e"))
	fmt.Fprintf(&b, "\n最大顺序差：%.17g\n", r.Elements.MaxDiff)
	return b.String()
}

func formatMat(m Mat3, verb string) string {
	var b strings.Builder
	fmt.Fprintln(&b, "```text")
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			fmt.Fprintf(&b, verb+"  ", m[i][j])
		}
		fmt.Fprintln(&b)
	}
	fmt.Fprint(&b, "```")
	return b.String()
}
