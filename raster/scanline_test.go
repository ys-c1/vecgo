package raster

import (
	"fmt"
	"math/rand"
	"testing"

	"vecgo/geom"
)

type vertexClass struct {
	name    string
	verdict string // 旧闭区间规则在该类顶点处的奇偶一致性判定
	polys   [][]geom.Point
}

// 三类顶点位置：顶点恰好落在扫描线上的三种形态。
// 画幅 10x10、supersample=1，多边形包围盒取 [0,10]x[0,10]，
// 采样点即 (col+0.5, row+0.5)；把关键顶点放在半整数高度上，
// 使其恰好落在扫描线上。
func vertexClasses() []vertexClass {
	out := []vertexClass{
		{
			// 凹点 (5,4.5) 为局部极小：两条邻边在扫描线同侧。
			// 闭区间计 2 次 ≡ 0，与射线法同奇偶，侥幸一致。
			name:    "极值顶点(邻边同侧)",
			verdict: "成立(2≡0)",
			polys: [][]geom.Point{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 5, Y: 4.5}, {X: 0, Y: 10}}},
		},
		{
			// 顶点 (4,4.5) 的两条邻边分居扫描线两侧：射线穿过边界。
			// 闭区间计 2 次 ≡ 0，射线法计 1 次 ≡ 1，矛盾 → 豁口。
			name:    "通过顶点(邻边异侧)",
			verdict: "不成立(2≢1)",
			polys: [][]geom.Point{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 4, Y: 4.5}, {X: 10, Y: 10}, {X: 0, Y: 10}}},
		},
		{
			// 阶梯形：水平边 (10,4.5)-(6,4.5) 是台阶顶，端点 (10,4.5)
			// 为其斜邻边（竖边 (10,0)-(10,4.5)）的上端点。
			// 闭区间把该端点多计一次 → 交点 {0,6,10} 共 3 个（奇数），
			// 整行内外全部反转：该亮的变豁口、该灭的变亮。
			name:    "水平边(两端点共线)",
			verdict: "不成立(端点归属不明)",
			polys: [][]geom.Point{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 4.5}, {X: 6, Y: 4.5}, {X: 6, Y: 10}, {X: 0, Y: 10}}},
		},
	}
	return out
}

func diffCount(a, b *Image) int {
	n := 0
	for i := range a.Pix {
		if a.Pix[i] != b.Pix[i] {
			n++
		}
	}
	return n
}

func fillBoth(t *testing.T, polys [][]geom.Point, w, h, ss int) (ref, naive, fixed *Image) {
	t.Helper()
	var err error
	if ref, err = New(w, h); err != nil {
		t.Fatal(err)
	}
	if err = ref.Fill(polys, ss); err != nil {
		t.Fatal(err)
	}
	if naive, err = New(w, h); err != nil {
		t.Fatal(err)
	}
	if err = naive.fillScanline(polys, ss, ruleClosed); err != nil {
		t.Fatal(err)
	}
	if fixed, err = New(w, h); err != nil {
		t.Fatal(err)
	}
	if err = fixed.FillScanline(polys, ss); err != nil {
		t.Fatal(err)
	}
	return ref, naive, fixed
}

// 修复后的扫描线填充必须与逐点判定逐像素相同。
func TestScanlineMatchesPerPixel(t *testing.T) {
	for _, tc := range vertexClasses() {
		ref, _, fixed := fillBoth(t, tc.polys, 10, 10, 1)
		if d := diffCount(ref, fixed); d != 0 {
			t.Errorf("%s: 修复后仍有 %d 个差异像素", tc.name, d)
		}
	}
}

// 旧闭区间规则在通过顶点与水平边处必须露出豁口，极值顶点处侥幸一致。
func TestScanlineNaiveGaps(t *testing.T) {
	wantGap := []bool{false, true, true}
	for i, tc := range vertexClasses() {
		ref, naive, _ := fillBoth(t, tc.polys, 10, 10, 1)
		gap := diffCount(ref, naive)
		if (gap > 0) != wantGap[i] {
			t.Errorf("%s: 豁口像素数=%d，与预期不符", tc.name, gap)
		}
	}
}

// 打印三类顶点位置的对照表（go test -v 查看）。
func TestScanlineVertexClassTable(t *testing.T) {
	fmt.Println("顶点位置类别 | 旧规则豁口像素数 | 奇偶一致性 | 修复后差异像素数")
	for _, tc := range vertexClasses() {
		ref, naive, fixed := fillBoth(t, tc.polys, 10, 10, 1)
		fmt.Printf("%s | %d | %s | %d\n",
			tc.name, diffCount(ref, naive), tc.verdict, diffCount(ref, fixed))
	}
}

// 随机多边形：顶点吸附到半整数网格，频繁恰好落在扫描线上，
// 修复后的扫描线填充仍须与逐点判定逐像素相同。
func TestScanlineRandomSnapped(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for trial := 0; trial < 500; trial++ {
		n := 3 + rng.Intn(6)
		poly := make([]geom.Point, n)
		for i := range poly {
			poly[i] = geom.Point{X: float64(rng.Intn(10)) + 0.5, Y: float64(rng.Intn(10)) + 0.5}
		}
		polys := [][]geom.Point{poly}
		ref, _, fixed := fillBoth(t, polys, 10, 10, 1)
		if d := diffCount(ref, fixed); d != 0 {
			t.Fatalf("trial %d: %d 个差异像素, poly=%v", trial, d, poly)
		}
	}
}

// 多多边形取并集、带超采样，同样逐像素一致。
func TestScanlineMultiPolySupersample(t *testing.T) {
	polys := [][]geom.Point{
		{{X: 1, Y: 1}, {X: 6, Y: 1}, {X: 6, Y: 6}, {X: 1, Y: 6}},
		{{X: 4, Y: 4}, {X: 9, Y: 4}, {X: 9, Y: 9}, {X: 4, Y: 9}},
	}
	for ss := 1; ss <= 3; ss++ {
		ref, _, fixed := fillBoth(t, polys, 12, 12, ss)
		if d := diffCount(ref, fixed); d != 0 {
			t.Fatalf("supersample=%d: %d 个差异像素", ss, d)
		}
	}
}
