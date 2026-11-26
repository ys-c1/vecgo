package matrixchain

import "math/big"

const bigPrecision = 512

// BigMat is a high-precision reference matrix.
type BigMat [3][3]*big.Float

func newBig(x float64) *big.Float {
	return new(big.Float).SetPrec(bigPrecision).SetFloat64(x)
}

func zeroBig() *big.Float {
	return new(big.Float).SetPrec(bigPrecision)
}

func floatToBig(a Mat3) BigMat {
	var out BigMat
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			out[i][j] = newBig(a[i][j])
		}
	}
	return out
}

func eyeBig() BigMat {
	var out BigMat
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			out[i][j] = zeroBig()
			if i == j {
				out[i][j].SetFloat64(1)
			}
		}
	}
	return out
}

func mulBig(a, b BigMat) BigMat {
	var c BigMat
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			c[i][j] = zeroBig()
			for k := 0; k < 3; k++ {
				t := zeroBig().Mul(a[i][k], b[k][j])
				c[i][j].Add(c[i][j], t)
			}
		}
	}
	return c
}

func absBig(a BigMat) BigMat {
	var out BigMat
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			out[i][j] = zeroBig().Abs(a[i][j])
		}
	}
	return out
}

func zeroBigMat() BigMat {
	var out BigMat
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			out[i][j] = zeroBig()
		}
	}
	return out
}

func addBigMats(as ...BigMat) BigMat {
	out := zeroBigMat()
	for _, a := range as {
		for i := 0; i < 3; i++ {
			for j := 0; j < 3; j++ {
				out[i][j].Add(out[i][j], a[i][j])
			}
		}
	}
	return out
}

func scaleBigMat(g *big.Float, a BigMat) BigMat {
	out := zeroBigMat()
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			out[i][j].Mul(g, a[i][j])
		}
	}
	return out
}

func maxAbsBigMat(a BigMat) *big.Float {
	m := zeroBig()
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			v := zeroBig().Abs(a[i][j])
			if v.Cmp(m) > 0 {
				m = v
			}
		}
	}
	return m
}

func exactIntervals(ms []Mat3) [6][6]BigMat {
	n := len(ms)
	var p [6][6]BigMat
	for i := 0; i <= n; i++ {
		p[i][i] = eyeBig()
	}
	for i := 0; i < n; i++ {
		p[i][i+1] = floatToBig(ms[i])
	}
	for length := 2; length <= n; length++ {
		for i := 0; i+length <= n; i++ {
			j := i + length
			p[i][j] = mulBig(p[i][j-1], p[j-1][j])
		}
	}
	return p
}

func nodeWeights(ms []Mat3, p [6][6]BigMat) map[nodeKey]float64 {
	n := len(ms)
	prefix := make([]BigMat, n+1)
	suffix := make([]BigMat, n+1)
	prefix[0] = eyeBig()
	for i := 0; i < n; i++ {
		prefix[i+1] = mulBig(prefix[i], absBig(floatToBig(ms[i])))
	}
	suffix[n] = eyeBig()
	for i := n - 1; i >= 0; i-- {
		suffix[i] = mulBig(absBig(floatToBig(ms[i])), suffix[i+1])
	}

	rootNorm := maxAbsBigMat(p[0][n])
	gammaOverEps := 3.0 / (1.0 - 3.0*Epsilon)
	w := make(map[nodeKey]float64)
	for i := 0; i < n; i++ {
		for k := i + 1; k < n; k++ {
			for j := k + 1; j <= n; j++ {
				m := mulBig(prefix[i], absBig(p[i][k]))
				m = mulBig(m, absBig(p[k][j]))
				m = mulBig(m, suffix[j])
				q, _ := zeroBig().Quo(maxAbsBigMat(m), rootNorm).Float64()
				w[nodeKey{i, k, j}] = gammaOverEps * q
			}
		}
	}
	return w
}

func maxElementError(got Mat3, exact BigMat) (float64, Mat3) {
	var deltas Mat3
	max := 0.0
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			d := zeroBig().Sub(newBig(got[i][j]), exact[i][j])
			deltas[i][j], _ = d.Float64()
			a, _ := zeroBig().Abs(d).Float64()
			if a > max {
				max = a
			}
		}
	}
	return max, deltas
}

// certifiedError evaluates the recursive Wilkinson error bound for the tree
// encoded by split.  Leaves are the stored binary64 input matrices.
func certifiedError(ms []Mat3, exact [6][6]BigMat, split [6][6]int, i, j int) (Mat3, BigMat) {
	if j-i == 1 {
		return ms[i], zeroBigMat()
	}
	k := split[i][j]
	leftHat, leftE := certifiedError(ms, exact, split, i, k)
	rightHat, rightE := certifiedError(ms, exact, split, k, j)
	hat := MulFloat(leftHat, rightHat)

	gamma := newBig(3 * Epsilon / (1 - 3*Epsilon))
	local := scaleBigMat(gamma, mulBig(absBig(floatToBig(leftHat)), absBig(floatToBig(rightHat))))
	propagateLeft := mulBig(leftE, absBig(exact[k][j]))
	propagateRight := mulBig(absBig(exact[i][k]), rightE)
	cross := mulBig(leftE, rightE)
	e := addBigMats(local, propagateLeft, propagateRight, cross)
	return hat, e
}
