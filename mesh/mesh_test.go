package mesh

import (
	"math"
	"math/rand"
	"strings"
	"testing"

	"vecgo/config"
	"vecgo/geom"
	"vecgo/pipe"
)

// ---- 五类网格形态：条带、扇形、随机、网格状、单三角形 ----

func stripMesh(quads int) Mesh {
	m := Mesh{}
	for i := 0; i <= quads; i++ {
		m.V = append(m.V, geom.Point{X: float64(i), Y: 0}, geom.Point{X: float64(i), Y: 1})
	}
	for i := 0; i < quads; i++ {
		a, b, c, d := 2*i, 2*i+1, 2*i+2, 2*i+3
		m.T = append(m.T, [3]int{a, b, c}, [3]int{b, d, c})
	}
	return m
}

func fanMesh(n int) Mesh {
	m := Mesh{V: []geom.Point{{X: 0, Y: 0}}}
	for k := 0; k <= n; k++ {
		a := 2 * math.Pi * float64(k) / float64(n)
		m.V = append(m.V, geom.Point{X: 10 * math.Cos(a), Y: 10 * math.Sin(a)})
	}
	for k := 1; k <= n; k++ {
		m.T = append(m.T, [3]int{0, k, k + 1})
	}
	return m
}

func randomMesh(nv, nt int, seed int64) Mesh {
	r := rand.New(rand.NewSource(seed))
	m := Mesh{}
	for i := 0; i < nv; i++ {
		m.V = append(m.V, geom.Point{X: r.Float64() * 100, Y: r.Float64() * 100})
	}
	for len(m.T) < nt {
		a, b, c := r.Intn(nv), r.Intn(nv), r.Intn(nv)
		if a == b || b == c || a == c {
			continue
		}
		m.T = append(m.T, [3]int{a, b, c})
	}
	return m
}

func gridMesh(w, h int) Mesh {
	m := Mesh{}
	for j := 0; j <= h; j++ {
		for i := 0; i <= w; i++ {
			m.V = append(m.V, geom.Point{X: float64(i), Y: float64(j)})
		}
	}
	at := func(i, j int) int { return j*(w+1) + i }
	for j := 0; j < h; j++ {
		for i := 0; i < w; i++ {
			m.T = append(m.T,
				[3]int{at(i, j), at(i+1, j), at(i, j+1)},
				[3]int{at(i+1, j), at(i+1, j+1), at(i, j+1)})
		}
	}
	return m
}

func singleMesh() Mesh {
	return Mesh{
		V: []geom.Point{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 0, Y: 10}},
		T: [][3]int{{0, 1, 2}},
	}
}

// shuffled 打乱顶点编号，模拟“顶点被反复提交、缓存不命中”的原始顺序。
func shuffled(m Mesh, seed int64) Mesh {
	r := rand.New(rand.NewSource(seed))
	perm := r.Perm(len(m.V))
	out := Mesh{V: make([]geom.Point, len(m.V)), T: append([][3]int{}, m.T...)}
	for old, p := range perm {
		out.V[p] = m.V[old]
	}
	inv := make([]int, len(m.V))
	for old, p := range perm {
		inv[p] = old
	}
	_ = inv
	remap := make([]int, len(m.V))
	for old, p := range perm {
		remap[old] = p
	}
	for i, tri := range out.T {
		out.T[i] = [3]int{remap[tri[0]], remap[tri[1]], remap[tri[2]]}
	}
	return out
}

func topologies() map[string]Mesh {
	return map[string]Mesh{
		"条带":   shuffled(stripMesh(24), 1),
		"扇形":   shuffled(fanMesh(40), 2),
		"随机":   shuffled(randomMesh(60, 80, 3), 4),
		"网格状":  shuffled(gridMesh(8, 8), 5),
		"单三角形": singleMesh(),
	}
}

func testCache(t *testing.T) Cache {
	c, err := NewCache(4, 4)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// ---- 三种非法输入必须拦下并点出出错索引 ----

func TestValidateOutOfBounds(t *testing.T) {
	m := singleMesh()
	m.T = append(m.T, [3]int{0, 1, 7})
	err := Validate(m)
	if err == nil || !strings.Contains(err.Error(), "越界") ||
		!strings.Contains(err.Error(), "7") || !strings.Contains(err.Error(), "三角形 1") {
		t.Fatalf("应报越界并指出索引: %v", err)
	}
}

func TestValidateDuplicate(t *testing.T) {
	m := singleMesh()
	m.T = append(m.T, [3]int{0, 2, 2})
	err := Validate(m)
	if err == nil || !strings.Contains(err.Error(), "重复") || !strings.Contains(err.Error(), "三角形 1") {
		t.Fatalf("应报顶点重复并指出索引: %v", err)
	}
}

func TestValidateEmpty(t *testing.T) {
	if err := Validate(Mesh{}); err == nil || !strings.Contains(err.Error(), "空网格") {
		t.Fatalf("应报空网格: %v", err)
	}
	m := singleMesh()
	m.T = nil
	if err := Validate(m); err == nil {
		t.Fatal("无三角形应报空网格")
	}
}

// ---- 要求一：重排后绘制结果与原顺序逐字节一致（独立测试） ----

func TestReorderByteIdentical(t *testing.T) {
	cfg := config.Default()
	cfg.Width, cfg.Height, cfg.Supersample = 24, 24, 1
	for name, m := range topologies() {
		re, err := Reorder(m)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		a, err := pipe.Run(Polygons(m), cfg)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		b, err := pipe.Run(Polygons(re), cfg)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(a.Pix) != len(b.Pix) {
			t.Fatalf("%s: 像素数不同", name)
		}
		for i := range a.Pix {
			if a.Pix[i] != b.Pix[i] {
				t.Fatalf("%s: 像素 %d 不同: %d != %d", name, i, a.Pix[i], b.Pix[i])
			}
		}
	}
}

// ---- 要求二：缓存未命中次数下降到规定比例以下（独立测试，不与上面合并） ----

func TestReorderMissRatio(t *testing.T) {
	c := testCache(t)
	ratios := map[string]float64{
		"条带": 0.5, "扇形": 0.5, "随机": 0.5, "网格状": 0.5,
		"单三角形": 1.0, // 只有一个三角形，不可能再降，要求不变差
	}
	for name, m := range topologies() {
		re, err := Reorder(m)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		before := c.Misses(m.T)
		after := c.Misses(re.T)
		limit := ratios[name]
		if float64(after) > limit*float64(before) {
			t.Fatalf("%s: 未命中 %d -> %d, 未降到比例 %.2f 以下", name, before, after, limit)
		}
		t.Logf("%s: 未命中 %d -> %d (比例上限 %.2f)", name, before, after, limit)
	}
}

// ---- 估计式与偏差上界：五类网格报实测值与估计值，跑通即算覆盖 ----

func TestTopologiesMissEstimate(t *testing.T) {
	c := testCache(t)
	for name, m := range topologies() {
		re, err := Reorder(m)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		est := c.Estimate(re)
		got := c.Misses(re.T)
		bound := c.MissBound(re.T)
		if est > got {
			t.Fatalf("%s: 估计 %d 超过实测 %d", name, est, got)
		}
		if got > bound {
			t.Fatalf("%s: 实测 %d 超过上界 %d", name, got, bound)
		}
		if dev := got - est; dev > bound-est {
			t.Fatalf("%s: 偏差 %d 超过上界 %d", name, dev, bound-est)
		}
		t.Logf("%s: 估计 %d, 实测 %d, 偏差 %d, 偏差上界 %d",
			name, est, got, got-est, bound-est)
	}
}
