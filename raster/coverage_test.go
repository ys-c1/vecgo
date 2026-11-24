package raster

import (
	"bytes"
	"encoding/csv"
	"math"
	"strconv"
	"strings"
	"testing"

	"vecgo/geom"
)

func square(x, y, size float64) []geom.Point {
	return []geom.Point{
		{X: x, Y: y}, {X: x + size, Y: y}, {X: x + size, Y: y + size}, {X: x, Y: y + size},
	}
}

// TestFillStatsMatchesFill 验证统计路径与普通填充逐字节相同。
func TestFillStatsMatchesFill(t *testing.T) {
	scenes := map[string][][]geom.Point{
		"square":  {square(0, 0, 10)},
		"seam":    {{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}}, {{X: 0, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}},
		"overlap": {square(0, 0, 8), square(4, 4, 8)},
		"stack":   {square(0, 0, 8), square(3, 0, 8), square(0, 3, 8), square(3, 3, 8)},
	}
	for name, scene := range scenes {
		for _, ss := range []int{1, 2, 3, 4} {
			plain, err := New(32, 32)
			if err != nil {
				t.Fatal(err)
			}
			if err := plain.Fill(scene, ss); err != nil {
				t.Fatal(err)
			}
			measured, err := New(32, 32)
			if err != nil {
				t.Fatal(err)
			}
			st, err := measured.FillStats(scene, ss)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(plain.Pix, measured.Pix) {
				t.Fatalf("%s ss=%d: 统计路径改变了像素", name, ss)
			}
			if issues := st.Verify(); len(issues) != 0 {
				t.Fatalf("%s ss=%d: 账目不符: %v", name, ss, issues)
			}
		}
	}
}

// TestFillStatsSingleShape 单个正方形铺满画幅：每像素正好一次，差值为零。
func TestFillStatsSingleShape(t *testing.T) {
	im, _ := New(8, 8)
	st, err := im.FillStats([][]geom.Point{square(0, 0, 10)}, 2)
	if err != nil {
		t.Fatal(err)
	}
	for i, c := range st.Counts {
		if c != 1 {
			t.Fatalf("像素 %d 覆盖次数=%d，应为 1", i, c)
		}
	}
	if st.TotalWrites != 64 {
		t.Fatalf("覆盖总次数=%d，应为 64", st.TotalWrites)
	}
	if math.Abs(st.AreaSum-64) > 1e-9 {
		t.Fatalf("面积和=%v，应为 64", st.AreaSum)
	}
	if math.Abs(st.Diff()) > 1e-9 {
		t.Fatalf("差值=%v，应为 0", st.Diff())
	}
	buckets := st.Buckets()
	if len(buckets) != 3 {
		t.Fatalf("分桶数=%d，应为 3", len(buckets))
	}
	if buckets[0].Interval != "1" || buckets[0].Pixels != 64 ||
		buckets[1].Pixels != 0 || buckets[2].Pixels != 0 {
		t.Fatalf("分桶异常: %+v", buckets)
	}
	if st.CoveredPixels != 64 || len(st.Top) != 10 {
		t.Fatalf("覆盖像素=%d Top=%d", st.CoveredPixels, len(st.Top))
	}
}

// TestFillStatsSeamDoubleWrite 沿对角线拆分的两个三角形：接缝像素被
// 两个图形各写一次，分桶里应出现 2 次档，总次数大于面积和。
func TestFillStatsSeamDoubleWrite(t *testing.T) {
	im, _ := New(16, 16)
	tris := [][]geom.Point{
		{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}},
		{{X: 0, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}},
	}
	st, err := im.FillStats(tris, 4)
	if err != nil {
		t.Fatal(err)
	}
	if issues := st.Verify(); len(issues) != 0 {
		t.Fatalf("账目不符: %v", issues)
	}
	two := st.Buckets()[1]
	if two.Interval != "2" || two.Pixels == 0 {
		t.Fatalf("接缝应产生 2 次覆盖像素: %+v", two)
	}
	if st.Diff() <= 0 {
		t.Fatalf("接缝重复写入应使总次数大于面积和，差值=%v", st.Diff())
	}
	if len(st.Top) != 10 {
		t.Fatalf("Top=%d，应为 10", len(st.Top))
	}
	for _, p := range st.Top {
		if p.Count != 2 {
			t.Fatalf("Top 像素覆盖次数=%d，应为 2", p.Count)
		}
		if len(p.Shapes) != 2 || p.Shapes[0] != 0 || p.Shapes[1] != 1 {
			t.Fatalf("Top 像素图形明细异常: %v", p.Shapes)
		}
	}
}

// TestFillStatsStackThreePlus 四个错位方块叠加，3+ 档应非空且覆盖次数可达 4。
func TestFillStatsStackThreePlus(t *testing.T) {
	im, _ := New(32, 32)
	scene := [][]geom.Point{square(0, 0, 8), square(3, 0, 8), square(0, 3, 8), square(3, 3, 8)}
	st, err := im.FillStats(scene, 2)
	if err != nil {
		t.Fatal(err)
	}
	b := st.Buckets()
	if b[2].Interval != "3+" || b[2].Pixels == 0 {
		t.Fatalf("3+ 档应为空: %+v", b)
	}
	if len(st.Top) == 0 || st.Top[0].Count < 3 || len(st.Top[0].Shapes) != st.Top[0].Count {
		t.Fatalf("最高覆盖像素异常: %+v", st.Top)
	}
	if issues := st.Verify(); len(issues) != 0 {
		t.Fatalf("账目不符: %v", issues)
	}
}

// TestBucketsReconcileArea 分桶 Write 之和与总面积和的差，应等于
// Diff()（相差仅浮点舍入）。
func TestBucketsReconcileArea(t *testing.T) {
	im, _ := New(20, 20)
	scene := [][]geom.Point{square(0, 0, 8), square(4, 4, 8)}
	st, err := im.FillStats(scene, 3)
	if err != nil {
		t.Fatal(err)
	}
	var writes uint64
	for _, b := range st.Buckets() {
		writes += b.Writes
	}
	if writes != st.TotalWrites {
		t.Fatalf("分桶次数之和=%d 与总次数=%d 不符", writes, st.TotalWrites)
	}
	if got := float64(writes) - st.AreaSum; math.Abs(got-st.Diff()) > 1e-9 {
		t.Fatalf("分桶差值=%v 与 Diff=%v 不符", got, st.Diff())
	}
	if math.Abs(st.TouchGap()-st.Diff()) > 1e-9 {
		t.Fatalf("差值分解=%v 与 Diff=%v 不符", st.TouchGap(), st.Diff())
	}
}

// TestBucketCSV 验证导出恰好两列、表头与三档数据行。
func TestBucketCSV(t *testing.T) {
	im, _ := New(12, 12)
	scene := [][]geom.Point{square(0, 0, 8), square(4, 4, 8)}
	st, err := im.FillStats(scene, 2)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := st.WriteBucketCSV(&buf); err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(&buf).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 4 {
		t.Fatalf("行数=%d，应为 4（表头+3 档）", len(rows))
	}
	if rows[0][0] != "次数区间" || rows[0][1] != "像素数" || len(rows[0]) != 2 {
		t.Fatalf("表头异常: %v", rows[0])
	}
	wantIntervals := []string{"1", "2", "3+"}
	var sum int
	var sumWrites uint64
	for i, row := range rows[1:] {
		if len(row) != 2 {
			t.Fatalf("第 %d 行不是两列: %v", i+1, row)
		}
		if row[0] != wantIntervals[i] {
			t.Fatalf("区间异常: %v", row[0])
		}
		var n int
		n, err = strconv.Atoi(row[1])
		if err != nil {
			t.Fatalf("像素数不是整数: %v", row[1])
		}
		sum += n
		if i == 0 {
			sumWrites += uint64(n)
		} else if i == 1 {
			sumWrites += 2 * uint64(n)
		}
	}
	if sum != st.CoveredPixels {
		t.Fatalf("分桶像素和=%d 与 CoveredPixels=%d 不符", sum, st.CoveredPixels)
	}
	if sumWrites > st.TotalWrites {
		t.Fatalf("分桶下界次数和=%d 超过总次数=%d", sumWrites, st.TotalWrites)
	}
}

// TestReportMentionsConcreteDiff 报告必须给出具体差值数字与分桶表。
func TestReportMentionsConcreteDiff(t *testing.T) {
	im, _ := New(16, 16)
	scene := [][]geom.Point{square(0, 0, 8), square(4, 4, 8)}
	st, err := im.FillStats(scene, 2)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := st.Report(&buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"覆盖总次数", "面积和", "差值", "分桶统计", "次数区间", "像素数", "覆盖次数最高的 10 个像素"} {
		if !strings.Contains(out, want) {
			t.Fatalf("报告缺少 %q:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "1") || !strings.Contains(out, "2") {
		t.Fatalf("报告缺少分桶档位:\n%s", out)
	}
}

// TestVerifyCatchesMismatch 人为破坏账目时 Verify 必须报出具体不符项。
func TestVerifyCatchesMismatch(t *testing.T) {
	im, _ := New(8, 8)
	st, err := im.FillStats([][]geom.Point{square(0, 0, 10)}, 1)
	if err != nil {
		t.Fatal(err)
	}
	st.TotalWrites++
	issues := st.Verify()
	if len(issues) == 0 {
		t.Fatal("总次数被篡改后 Verify 应报错")
	}
	found := false
	for _, s := range issues {
		if strings.Contains(s, "Σ Counts") && strings.Contains(s, "TotalWrites") {
			found = true
		}
	}
	if !found {
		t.Fatalf("应报出 Σ Counts 与 TotalWrites 不符: %v", issues)
	}
}
