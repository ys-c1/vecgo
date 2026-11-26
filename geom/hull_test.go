package geom

import (
	"fmt"
	"math/rand"
	"testing"
)

// addBuggy 是修复前的增量维护：可见链查找正确，但重建凸包时漏拼链首切点 h[s]。
// 这正是“增量结果偶尔比全量少一个顶点”的 off-by-one 切片错误。
func addBuggy(h []Point, p Point) []Point {
	switch len(h) {
	case 0:
		return []Point{p}
	case 1:
		if h[0] == p {
			return h
		}
		return Hull([]Point{h[0], p})
	case 2:
		return Hull([]Point{h[0], h[1], p})
	}
	n := len(h)
	c := make([]float64, n)
	first := -1
	for i := 0; i < n; i++ {
		c[i] = Cross(h[i], h[(i+1)%n], p)
		if c[i] < 0 && first < 0 {
			first = i
		}
	}
	if first < 0 {
		return h
	}
	s, t := first, first
	for k := 0; k < n && c[(s-1+n)%n] <= 0; k++ {
		s = (s - 1 + n) % n
	}
	for k := 0; k < n && c[(t+1)%n] <= 0; k++ {
		t = (t + 1) % n
	}
	// BUG：只插入 p，忘了保留链首切点 h[s]，直接从 h[t+1] 接回。
	out := make([]Point, 0, n+1)
	out = append(out, p)
	for k := (t + 1) % n; k != s; k = (k + 1) % n {
		out = append(out, h[k])
	}
	return rotateToLexMin(out)
}

func sameCycle(a, b []Point) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// 三组点集：A 内部点不改变凸包；B 单个可见边且位于下标 0；C 可见链跨下标边界且含共线边。
func tableSets() (setA, setB, setC []Point) {
	setA = []Point{{0, 0}, {4, 0}, {0, 4}, {1, 1}, {2, 2}}
	square := []Point{{0, 0}, {2, 0}, {2, 2}, {0, 2}}
	setB = append(append([]Point{}, square...), Point{1, -1})
	pentagon := []Point{{0, 0}, {2, 0}, {5, 3}, {3, 5}, {1, 4}}
	setC = append(append([]Point{}, pentagon...), Point{-1, 3})
	return
}

func TestHullIncrementalMatchesFull(t *testing.T) {
	setA, setB, setC := tableSets()
	for idx, set := range [][]Point{setA, setB, setC} {
		full := Hull(set)
		ih := NewIncrementalHull(nil)
		for _, p := range set {
			ih.Add(p)
		}
		if !sameCycle(ih.Points(), full) {
			t.Fatalf("set %d: 增量 %v != 全量 %v", idx, ih.Points(), full)
		}
	}
}

func TestBuggyReproducesMissingVertex(t *testing.T) {
	_, setB, setC := tableSets()
	for idx, set := range [][]Point{setB, setC} {
		seed, p := set[:len(set)-1], set[len(set)-1]
		full := Hull(set)
		buggy := addBuggy(Hull(seed), p)
		if sameCycle(buggy, full) {
			t.Fatalf("set %d: 缺陷实现意外与全量一致，反例失效", idx)
		}
		t.Logf("set %c: 全量 %d 点 %v；缺陷增量 %d 点 %v",
			'B'+idx, len(full), full, len(buggy), buggy)
	}
}

// 完整插入序列复现：从空集开始逐个加入，记录缺陷首次出现的时刻与最终结果。
func TestFullInsertionSequenceRepro(t *testing.T) {
	seq := []Point{{0, 0}, {2, 0}, {2, 2}, {0, 2}, {1, -1}}
	var buggy []Point
	for i, p := range seq {
		buggy = addBuggy(buggy, p)
		full := Hull(seq[:i+1])
		t.Logf("第 %d 步加入 %v：缺陷增量=%v (%d 点)；全量=%v (%d 点)；一致=%v",
			i+1, p, buggy, len(buggy), full, len(full), sameCycle(buggy, full))
	}
	full := Hull(seq)
	if sameCycle(buggy, full) {
		t.Fatal("复现失败：缺陷实现未偏离全量结果")
	}
}

// 随机差分测试：含重复点、共线点，增量与全量必须逐点相同。
func TestRandomDifferential(t *testing.T) {
	rng := rand.New(rand.NewSource(20261126))
	for trial := 0; trial < 200; trial++ {
		m := 3 + rng.Intn(40)
		pts := make([]Point, m)
		for i := range pts {
			pts[i] = Point{float64(rng.Intn(21) - 10), float64(rng.Intn(21) - 10)}
		}
		ih := NewIncrementalHull(nil)
		for _, p := range pts {
			ih.Add(p)
		}
		full := Hull(pts)
		if !sameCycle(ih.Points(), full) {
			t.Fatalf("trial %d 点集 %v：增量 %v != 全量 %v", trial, pts, ih.Points(), full)
		}
		// 缺陷版本必须在至少一部分试验中偏离，证明测试确实有能力抓住该 bug。
		if trial == 0 {
			var bh []Point
			for _, p := range pts {
				bh = addBuggy(bh, p)
			}
			t.Logf("随机首trial：缺陷增量 %d 点 vs 全量 %d 点", len(bh), len(full))
		}
	}
}

// 打印三组点集的汇总表，供核对。
func TestPrintTable(t *testing.T) {
	setA, setB, setC := tableSets()
	labels := []string{"A", "B", "C"}
	for idx, set := range [][]Point{setA, setB, setC} {
		seed, p := set[:len(set)-1], set[len(set)-1]
		base := Hull(seed)
		full := Hull(set)

		ih := NewIncrementalHull(seed)
		ih.Add(p)
		fixed := ih.Points()
		buggy := addBuggy(base, p)

		// 可见边叉积符号
		signs := ""
		visible := 0
		for i := range base {
			c := Cross(base[i], base[(i+1)%len(base)], p)
			switch {
			case c < 0:
				signs += "-"
				visible++
			case c == 0:
				signs += "0"
			default:
				signs += "+"
			}
		}
		missing := []Point{}
		for _, q := range full {
			found := false
			for _, r := range buggy {
				if q == r {
					found = true
				}
			}
			if !found {
				missing = append(missing, q)
			}
		}
		t.Logf("组%s 新点%v 叉积序列%q 可见边数=%d 全量=%d点 修复增量=%d点 缺陷增量=%d点 漏点=%v 逐点相同=%v",
			labels[idx], p, signs, visible, len(full), len(fixed), len(buggy), missing, sameCycle(fixed, full))
	}
	fmt.Println("(汇总见 test log)")
}
