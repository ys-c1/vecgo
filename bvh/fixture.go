package bvh

import "vecgo/geom"

// 本文件提供可复现的三角形场景，供测试与基准共用。
// 随机源是固定种子的 splitmix64 派生流，不依赖 math/rand 的版本实现，
// 同一份代码在任意进程、任意机器上生成完全相同的三角形。

type rng struct{ state uint64 }

func newRNG(seed uint64) *rng { return &rng{state: seed} }

func (r *rng) next() uint64 {
	r.state += 0x9e3779b97f4a7c15
	z := r.state
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	return z ^ (z >> 31)
}

// float 返回 [0,1) 的均匀浮点数。
func (r *rng) float() float64 {
	return float64(r.next()>>11) / (1 << 53)
}

// between 返回 [lo,hi) 的均匀浮点数。
func (r *rng) between(lo, hi float64) float64 { return lo + (hi-lo)*r.float() }

// Scene 是一类三角形场景。
type Scene struct {
	Name  string
	build func(n int, r *rng) []Triangle
}

// Scenes 是任务要求覆盖的五类场景，顺序固定。
func Scenes() []Scene {
	return []Scene{
		{Name: "uniform", build: uniformScene},
		{Name: "clustered", build: clusteredScene},
		{Name: "elongated", build: elongatedScene},
		{Name: "overlapping", build: overlappingScene},
		{Name: "single", build: singleScene},
	}
}

// SceneByName 按名字取场景。
func SceneByName(name string) (Scene, bool) {
	for _, s := range Scenes() {
		if s.Name == name {
			return s, true
		}
	}
	return Scene{}, false
}

// Generate 生成 n 个三角形。单三角形场景恒为 1 个。
//
// 五类场景的构造见各 build 函数的注释；随机源固定种子，任意进程生成同一批三角形。
func (s Scene) Generate(n int) []Triangle {
	if s.Name == "single" {
		return s.build(1, newRNG(1))
	}
	if n < 1 {
		n = 1
	}
	return s.build(n, newRNG(0x5eed0000+uint64(len(s.Name))*7919))
}

// triWithPositiveBox 生成三个随机点，直到其包围盒宽高都大于 0。
// 建树要求每个三角形包围盒非退化，这里保证场景本身合法。
func triWithPositiveBox(r *rng, gen func() (float64, float64)) Triangle {
	for attempt := 0; attempt < 64; attempt++ {
		var pts [3]geom.Point
		for i := range pts {
			x, y := gen()
			pts[i] = geom.Point{X: x, Y: y}
		}
		a := Triangle{A: pts[0], B: pts[1], C: pts[2]}
		b := a.Bounds()
		if b.Width() > 0 && b.Height() > 0 {
			return a
		}
	}
	// 理论上不会走到这里；兜底给一个确定的小三角形。
	return Triangle{A: geom.Point{X: 0, Y: 0}, B: geom.Point{X: 1, Y: 0}, C: geom.Point{X: 0, Y: 1}}
}

func uniformScene(n int, r *rng) []Triangle {
	out := make([]Triangle, n)
	for i := range out {
		cx, cy := r.between(0, 1000), r.between(0, 1000)
		// 均匀分布的小三角形：位置均匀，边长在 2~20 之间。
		out[i] = triWithPositiveBox(r, func() (float64, float64) {
			return cx + r.between(-10, 10), cy + r.between(-10, 10)
		})
	}
	return out
}

func clusteredScene(n int, r *rng) []Triangle {
	const clusters = 12
	cx := make([]float64, clusters)
	cy := make([]float64, clusters)
	for i := 0; i < clusters; i++ {
		// 故意让簇心分布不均：一半挤在左下角，一半散开。
		if i < clusters/2 {
			cx[i], cy[i] = r.between(0, 200), r.between(0, 200)
		} else {
			cx[i], cy[i] = r.between(0, 1000), r.between(0, 1000)
		}
	}
	out := make([]Triangle, n)
	for i := range out {
		c := int(r.next() % clusters)
		out[i] = triWithPositiveBox(r, func() (float64, float64) {
			return cx[c] + r.between(-25, 25), cy[c] + r.between(-25, 25)
		})
	}
	return out
}

func elongatedScene(n int, r *rng) []Triangle {
	out := make([]Triangle, n)
	for i := range out {
		// 长条：x 方向几百，y 方向不到 1，整体沿 y 铺开。
		y := r.between(0, 1000)
		x := r.between(0, 600)
		out[i] = Triangle{
			A: geom.Point{X: x, Y: y},
			B: geom.Point{X: x + r.between(300, 400), Y: y + r.between(0, 0.4)},
			C: geom.Point{X: x + r.between(100, 200), Y: y + r.between(0.5, 1)},
		}
	}
	return out
}

func overlappingScene(n int, r *rng) []Triangle {
	out := make([]Triangle, n)
	// 所有三角形都围着一个公共区域，彼此大量重叠。
	for i := range out {
		out[i] = triWithPositiveBox(r, func() (float64, float64) {
			return 500 + r.between(-60, 60), 500 + r.between(-60, 60)
		})
	}
	return out
}

func singleScene(n int, r *rng) []Triangle {
	_ = n
	_ = r
	return []Triangle{{
		A: geom.Point{X: 10, Y: 10},
		B: geom.Point{X: 90, Y: 20},
		C: geom.Point{X: 30, Y: 80},
	}}
}

// QueryKind 标出采样查询的类型。
type QueryKind int

const (
	KindPoint QueryKind = iota
	KindSmallBox
	KindLargeBox
)

func (k QueryKind) String() string {
	switch k {
	case KindPoint:
		return "point"
	case KindSmallBox:
		return "small-box"
	default:
		return "large-box"
	}
}

// Query 是一次采样查询：KindPoint 用 Point，其余用 Box。
type Query struct {
	Kind  QueryKind
	Point geom.Point
	Box   Bounds
}

// SampleQueries 在三角形集合的包围盒范围内按固定种子采样 n 组查询，
// 供基准与演示共用。查询类型轮转：点、小矩形、大矩形。
func SampleQueries(tris []Triangle, n int, seed uint64) []Query {
	r := newRNG(seed)
	b := tris[0].Bounds()
	for _, tri := range tris[1:] {
		b = unionBounds(b, tri.Bounds())
	}
	out := make([]Query, n)
	for i := range out {
		switch i % 3 {
		case 0:
			out[i] = Query{Kind: KindPoint, Point: geom.Point{
				X: r.between(b.Min.X, b.Max.X),
				Y: r.between(b.Min.Y, b.Max.Y),
			}}
		case 1:
			w := r.between(20, 80)
			h := r.between(20, 80)
			x := r.between(b.Min.X-50, b.Max.X)
			y := r.between(b.Min.Y-50, b.Max.Y)
			out[i] = Query{Kind: KindSmallBox, Box: Bounds{
				Min: geom.Point{X: x, Y: y},
				Max: geom.Point{X: x + w, Y: y + h},
			}}
		default:
			w := r.between(200, 500)
			h := r.between(200, 500)
			x := r.between(b.Min.X-100, b.Max.X)
			y := r.between(b.Min.Y-100, b.Max.Y)
			out[i] = Query{Kind: KindLargeBox, Box: Bounds{
				Min: geom.Point{X: x, Y: y},
				Max: geom.Point{X: x + w, Y: y + h},
			}}
		}
	}
	return out
}
