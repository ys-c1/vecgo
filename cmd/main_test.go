package main

import (
	"bytes"
	"encoding/csv"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// runArgs 用给定的参数调用 run，并截获标准输出。
func runArgs(t *testing.T, args ...string) (string, error) {
	t.Helper()
	oldArgs, oldStdout := os.Args, os.Stdout
	defer func() { os.Args, os.Stdout = oldArgs, oldStdout }()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	os.Args = append([]string{"vecgo"}, args...)
	runErr := run()
	w.Close()
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatal(err)
	}
	return buf.String(), runErr
}

// TestStatsToggleProducesIdenticalArtifact 统计关闭与开启时导出的 PGM
// 必须逐字节相同。
func TestStatsToggleProducesIdenticalArtifact(t *testing.T) {
	dir := t.TempDir()
	for _, scene := range []string{"square", "seam", "overlap", "stack"} {
		off := filepath.Join(dir, scene+"_off.pgm")
		on := filepath.Join(dir, scene+"_on.pgm")
		if _, err := runArgs(t, "-scene", scene, "-o", off); err != nil {
			t.Fatalf("%s off: %v", scene, err)
		}
		if _, err := runArgs(t, "-scene", scene, "-coverage", "-o", on); err != nil {
			t.Fatalf("%s on: %v", scene, err)
		}
		a, err := os.ReadFile(off)
		if err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(on)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(a, b) {
			t.Fatalf("%s: 统计开启改变了产物", scene)
		}
	}
}

// TestCoverageReportsConcreteDiffAndTop10 统计输出必须含具体差值数字、
// 分桶两列与最高 10 个像素的图形明细。
func TestCoverageReportsConcreteDiffAndTop10(t *testing.T) {
	dir := t.TempDir()
	out, err := runArgs(t, "-scene", "seam", "-coverage", "-o", filepath.Join(dir, "s.pgm"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"覆盖总次数", "面积和", "差值", "次数区间", "像素数", "覆盖次数最高的 10 个像素"} {
		if !strings.Contains(out, want) {
			t.Fatalf("输出缺少 %q:\n%s", want, out)
		}
	}
	// 接缝场景：覆盖总次数 4160、面积和 4096、差值 +64.000。
	for _, want := range []string{"覆盖总次数 4160", "差值（覆盖总次数 - 面积和）+64.000"} {
		if !strings.Contains(out, want) {
			t.Fatalf("输出缺少具体数字 %q:\n%s", want, out)
		}
	}
	if got := strings.Count(out, "由 2 个图形叠加"); got != 10 {
		t.Fatalf("应逐条列出 10 个叠加像素，实际 %d 条:\n%s", got, out)
	}
}

// TestCoverageCSVTwoColumns 分桶 CSV 恰好两列，区间 1/2/3+ 各一行。
func TestCoverageCSVTwoColumns(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "buckets.csv")
	if _, err := runArgs(t, "-scene", "stack", "-coverage-csv", csvPath,
		"-o", filepath.Join(dir, "s.pgm")); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(csvPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 4 {
		t.Fatalf("行数=%d，应为表头+3 行", len(rows))
	}
	if rows[0][0] != "次数区间" || rows[0][1] != "像素数" {
		t.Fatalf("表头异常: %v", rows[0])
	}
	for i, want := range []string{"1", "2", "3+"} {
		if len(rows[i+1]) != 2 || rows[i+1][0] != want {
			t.Fatalf("第 %d 行异常: %v", i+1, rows[i+1])
		}
	}
}

// TestDefaultArtifactUnchanged 默认调用的产物必须与未加统计的版本一致
// （回归哈希 fbda3e…）。
func TestDefaultArtifactUnchanged(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "default.pgm")
	if _, err := runArgs(t, path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 13+64*64 {
		t.Fatalf("字节数异常: %d", len(data))
	}
	if !bytes.Equal(data[13:], bytes.Repeat([]byte{255}, 64*64)) {
		t.Fatal("默认场景应每像素 255")
	}
}
