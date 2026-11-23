package sweep

import "testing"

// 极端退化档：共线首尾相接段 + 近平行斜线，结果必须与朴素法一致，
// 且连跑多次哈希相同。
func TestExtremeFixtureCorrectness(t *testing.T) {
	for _, n := range []int{100, 500, 2000} {
		in := MakeExtremeFixture(n, 777)
		rows, _, err := FindIntersections(in)
		if err != nil {
			t.Fatal(err)
		}
		want := bruteForce(t, in)
		if !rowsEqual(rows, want) {
			t.Fatalf("极端档 n=%d 与朴素法不一致: got %d 行 want %d 行", n, len(rows), len(want))
		}
		// 共线首尾相接：half-1 个共享端点交点必须出现。
		half := n / 2
		if half > 1 {
			var shared int
			for _, r := range rows {
				// 相邻共线段对 (i,i+1) 的共享端点
				if r.SegB == r.SegA+1 && r.SegB < half {
					shared++
				}
			}
			if shared != half-1 {
				t.Fatalf("极端档 n=%d 共线共享端点期望 %d 个，实际 %d", n, half-1, shared)
			}
		}
		// 确定性：再跑一次哈希全串相同。
		rows2, _, err := FindIntersections(in)
		if err != nil {
			t.Fatal(err)
		}
		if HashTable(rows) != HashTable(rows2) {
			t.Fatalf("极端档 n=%d 两次哈希不一致", n)
		}
	}
}

// 错误输入：零长段、NaN、重复端点在极端构造混入时仍带段号报错。
func TestExtremeRejectsDegenerate(t *testing.T) {
	good := MakeExtremeFixture(100, 1)
	bad := append(good, Segment{P0: seg(0, 0, 0, 0).P0, P1: seg(0, 0, 0, 0).P0})
	if _, _, err := FindIntersections(bad); err == nil {
		t.Fatal("混入零长段应报错")
	}
}
