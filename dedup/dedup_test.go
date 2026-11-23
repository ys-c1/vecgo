package dedup

import (
	"errors"
	"testing"

	"vecgo/geom"
)

func sq(x, y, s float64) []geom.Point {
	return []geom.Point{
		{X: x, Y: y},
		{X: x + s, Y: y},
		{X: x + s, Y: y + s},
		{X: x, Y: y + s},
	}
}

func rotatedSquare(x, y, s float64) []geom.Point {
	// Same vertex set as sq, listed from a different starting vertex.
	p := sq(x, y, s)
	return []geom.Point{p[2], p[3], p[0], p[1]}
}

func TestMergeAllSame(t *testing.T) {
	scene := [][]geom.Point{sq(0, 0, 10), sq(0, 0, 10), sq(0, 0, 10), sq(0, 0, 10)}
	res, err := MergeAuto(scene)
	if err != nil {
		t.Fatal(err)
	}
	if res.Stats.Unique != 1 || res.Stats.Duplicates != 3 {
		t.Fatalf("全同去重错误: %+v", res.Stats)
	}
	if got := res.Stats.FingerprintBits; got != 0 {
		t.Fatalf("单个不同图形应无需指纹位，得到 %d", got)
	}
	if res.Stats.FingerprintBits > res.Stats.RequiredBits {
		t.Fatalf("实际位 %d 超过反解值 %d", res.Stats.FingerprintBits, res.Stats.RequiredBits)
	}
	if len(res.Groups) != 1 || len(res.Groups[0]) != 4 {
		t.Fatalf("分组错误: %v", res.Groups)
	}
}

func TestMergePartial(t *testing.T) {
	scene := [][]geom.Point{
		sq(0, 0, 10),
		sq(20, 0, 5),
		sq(0, 0, 10),
		rotatedSquare(20, 0, 5),
		sq(40, 40, 3),
	}
	res, err := MergeAuto(scene)
	if err != nil {
		t.Fatal(err)
	}
	if res.Stats.Total != 5 || res.Stats.Unique != 3 || res.Stats.Duplicates != 2 {
		t.Fatalf("部分同去重错误: %+v", res.Stats)
	}
	if res.Stats.FingerprintBits > res.Stats.RequiredBits {
		t.Fatalf("实际位 %d 超过反解值 %d", res.Stats.FingerprintBits, res.Stats.RequiredBits)
	}
	wantGroups := [][]int{{0, 2}, {1, 3}, {4}}
	if len(res.Groups) != len(wantGroups) {
		t.Fatalf("分组数错误: %v", res.Groups)
	}
	for i := range wantGroups {
		if len(res.Groups[i]) != len(wantGroups[i]) {
			t.Fatalf("分组 %d 错误: %v", i, res.Groups[i])
		}
		for j := range wantGroups[i] {
			if res.Groups[i][j] != wantGroups[i][j] {
				t.Fatalf("分组 %d 错误: %v", i, res.Groups)
			}
		}
	}
}

func TestMergeAllDifferent(t *testing.T) {
	scene := [][]geom.Point{
		sq(0, 0, 10), sq(20, 0, 10), sq(40, 0, 10), sq(60, 0, 10),
	}
	res, err := MergeAuto(scene)
	if err != nil {
		t.Fatal(err)
	}
	if res.Stats.Unique != 4 || res.Stats.Duplicates != 0 {
		t.Fatalf("全不同去重错误: %+v", res.Stats)
	}
	if res.Stats.DuplicateRate != 0 {
		t.Fatalf("全不同重复率应为 0: %v", res.Stats.DuplicateRate)
	}
	// d = 4 -> C(4,2)/2^32 = 6/2^32 < 1, so 35 bits (34 would not suffice).
	if res.Stats.RequiredBits != 35 {
		t.Fatalf("反解位数错误: %d", res.Stats.RequiredBits)
	}
	if res.Stats.FingerprintBits != 35 {
		t.Fatalf("实际位数错误: %d", res.Stats.FingerprintBits)
	}
}

func TestMergeSingle(t *testing.T) {
	res, err := MergeAuto([][]geom.Point{sq(0, 0, 10)})
	if err != nil {
		t.Fatal(err)
	}
	if res.Stats.Unique != 1 || res.Stats.Duplicates != 0 {
		t.Fatalf("单图形错误: %+v", res.Stats)
	}
	if res.Stats.RequiredBits != 0 || res.Stats.FingerprintBits != 0 {
		t.Fatalf("单图形无需指纹位: %+v", res.Stats)
	}
}

func TestMergeDegenerate(t *testing.T) {
	// Collinear (zero-area) triangle, and one with a repeated vertex.  Both
	// have at least three vertices and must be handled, not rejected.
	collinear := []geom.Point{{X: 0, Y: 0}, {X: 5, Y: 0}, {X: 10, Y: 0}}
	repeat := []geom.Point{{X: 0, Y: 0}, {X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}}
	scene := [][]geom.Point{collinear, repeat, collinear}
	res, err := MergeAuto(scene)
	if err != nil {
		t.Fatal(err)
	}
	if res.Stats.Unique != 2 || res.Stats.Duplicates != 1 {
		t.Fatalf("退化图形去重错误: %+v", res.Stats)
	}
	if len(res.Unique[0]) != 3 {
		t.Fatalf("代表图形应保持原始坐标: %v", res.Unique[0])
	}
}

func TestEmptyFigureReportsIndex(t *testing.T) {
	scene := [][]geom.Point{sq(0, 0, 10), {}, sq(5, 5, 1)}
	_, err := MergeAuto(scene)
	var fe *FigureError
	if !errors.As(err, &fe) {
		t.Fatalf("应返回 FigureError: %v", err)
	}
	if fe.Kind != ErrEmptyFigure || len(fe.Indices) != 1 || fe.Indices[0] != 1 {
		t.Fatalf("图形号错误: %+v", fe)
	}
	if fe.Error() != "图形 1 为空" {
		t.Fatalf("错误信息错误: %s", fe.Error())
	}
}

func TestTooFewVerticesReportsIndex(t *testing.T) {
	scene := [][]geom.Point{sq(0, 0, 10), {{X: 1, Y: 1}, {X: 2, Y: 2}}}
	_, err := MergeAuto(scene)
	var fe *FigureError
	if !errors.As(err, &fe) {
		t.Fatalf("应返回 FigureError: %v", err)
	}
	if fe.Kind != ErrTooFewVertices || fe.Indices[0] != 1 {
		t.Fatalf("图形号错误: %+v", fe)
	}
}

func TestFingerprintCollisionReportsIndices(t *testing.T) {
	scene := [][]geom.Point{sq(0, 0, 10), sq(20, 0, 10)}
	_, err := Merge(scene, 0) // zero-bit fingerprint forces a collision
	var fe *FigureError
	if !errors.As(err, &fe) {
		t.Fatalf("应返回 FigureError: %v", err)
	}
	if fe.Kind != ErrFingerprintCollision || fe.Indices[0] != 0 || fe.Indices[1] != 1 {
		t.Fatalf("图形号错误: %+v", fe)
	}
	if fe.FingerprintBits != 0 {
		t.Fatalf("应报出指纹长度: %+v", fe)
	}
}

func TestSameVertexSetDifferentRingCannotMerge(t *testing.T) {
	// Same four points, but the second ring is not a rotation or reversal of
	// the first; even-odd filling differs, so it must not be merged.
	a := []geom.Point{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}
	b := []geom.Point{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 0, Y: 10}, {X: 10, Y: 10}}
	_, err := Merge([][]geom.Point{a, b}, 64)
	var fe *FigureError
	if !errors.As(err, &fe) {
		t.Fatalf("应返回 FigureError: %v", err)
	}
	if fe.Kind != ErrFingerprintCollision || fe.Indices[0] != 0 || fe.Indices[1] != 1 {
		t.Fatalf("图形号错误: %+v", fe)
	}
}

func TestFingerprintSetBased(t *testing.T) {
	a := sq(0, 0, 10)
	b := rotatedSquare(0, 0, 10)
	if Fingerprint(a, 64) != Fingerprint(b, 64) {
		t.Fatal("顶点顺序不同的同一图形应有相同指纹")
	}
	c := []geom.Point{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}, {X: 0, Y: 0}}
	if Fingerprint(a, 64) != Fingerprint(c, 64) {
		t.Fatal("重复顶点不应改变顶点集合指纹")
	}
}

func TestClosedRingMerges(t *testing.T) {
	// The same square, one submission repeating its first vertex as a closing
	// point.  A zero-length edge does not change the filled area, so it merges.
	a := sq(0, 0, 10)
	b := append(sq(0, 0, 10), a[0])
	res, err := MergeAuto([][]geom.Point{a, b})
	if err != nil {
		t.Fatal(err)
	}
	if res.Stats.Unique != 1 {
		t.Fatalf("闭合重复点应合并: %+v", res.Stats)
	}
}

func TestRequiredBitsBacksolve(t *testing.T) {
	// The duplicate rate determines the distinct count, which determines the
	// birthday-bounded length.
	cases := []struct{ total, unique int }{
		{10, 10}, {10, 5}, {10, 2}, {10, 1}, {30, 6}, {1, 1},
	}
	for _, c := range cases {
		rate := float64(c.total-c.unique) / float64(c.total)
		got := RequiredBits(c.total, rate, TargetCollision)
		want := BirthdayBits(c.unique, TargetCollision)
		if got != want {
			t.Fatalf("total=%d unique=%d 反解 %d, 期望 %d", c.total, c.unique, got, want)
		}
	}
	if RequiredBits(10, 0, TargetCollision) <= RequiredBits(10, 0.9, TargetCollision) {
		t.Fatal("重复率越高（不同图形越少）所需位数应不增")
	}
}

func TestMergeAutoBitsNeverExceedReverseSolve(t *testing.T) {
	scene := make([][]geom.Point, 0, 40)
	for i := 0; i < 40; i++ {
		scene = append(scene, sq(float64(i%7)*20, float64(i%5)*20, 10))
	}
	res, err := MergeAuto(scene)
	if err != nil {
		t.Fatal(err)
	}
	if res.Stats.FingerprintBits > res.Stats.RequiredBits {
		t.Fatalf("实际位 %d 超过反解值 %d", res.Stats.FingerprintBits, res.Stats.RequiredBits)
	}
	if res.Stats.Duplicates != 40-res.Stats.Unique {
		t.Fatalf("计数不自洽: %+v", res.Stats)
	}
}

func TestEmptyScene(t *testing.T) {
	res, err := MergeAuto(nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Stats.Total != 0 || res.Stats.Unique != 0 || res.Stats.Duplicates != 0 {
		t.Fatalf("空场景错误: %+v", res.Stats)
	}
}
