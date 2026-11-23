package cull

import (
	"errors"
	"math"
	"math/big"
	"math/rand"
	"strings"
	"testing"
)

// boxFrustum describes an axis-aligned frustum whose inside half-space is
// [lo,hi] on each axis. The normal scaling is deliberately not unit length,
// so the batch path has to normalize each plane.
func boxFrustum(lo, hi Vec3, scale float64) Frustum {
	return Frustum{Planes: [6]Plane{
		{N: Vec3{X: scale}, D: -scale * lo.X}, // x >= lo.X
		{N: Vec3{X: -scale}, D: scale * hi.X}, // x <= hi.X
		{N: Vec3{Y: scale}, D: -scale * lo.Y}, // y >= lo.Y
		{N: Vec3{Y: -scale}, D: scale * hi.Y}, // y <= hi.Y
		{N: Vec3{Z: scale}, D: -scale * lo.Z}, // z >= lo.Z
		{N: Vec3{Z: -scale}, D: scale * hi.Z}, // z <= hi.Z
	}}
}

func unitBox() Frustum {
	return boxFrustum(Vec3{X: -1, Y: -1, Z: -1}, Vec3{X: 1, Y: 1, Z: 1}, 1)
}

// ---------------------------------------------------------------------------
// High-precision reference, independent of the float64 code path
// ---------------------------------------------------------------------------

const refPrec = 256

func bf(v float64) *big.Float { return new(big.Float).SetPrec(refPrec).SetFloat64(v) }

// exactPlane returns the plane's normal and offset normalized at 256 bits.
func exactPlane(p Plane) (nx, ny, nz, d *big.Float) {
	m := math.Max(math.Abs(p.N.X), math.Max(math.Abs(p.N.Y), math.Abs(p.N.Z)))
	ux := new(big.Float).Quo(bf(p.N.X), bf(m))
	uy := new(big.Float).Quo(bf(p.N.Y), bf(m))
	uz := new(big.Float).Quo(bf(p.N.Z), bf(m))
	l2 := new(big.Float).Add(
		new(big.Float).Add(new(big.Float).Mul(ux, ux), new(big.Float).Mul(uy, uy)),
		new(big.Float).Mul(uz, uz),
	)
	l := new(big.Float).Sqrt(l2)
	return new(big.Float).Quo(ux, l),
		new(big.Float).Quo(uy, l),
		new(big.Float).Quo(uz, l),
		new(big.Float).Quo(new(big.Float).Quo(bf(p.D), bf(m)), l)
}

// exactCenterDist is the true signed distance from the sphere center to p.
func exactCenterDist(p Plane, s Sphere) *big.Float {
	nx, ny, nz, d := exactPlane(p)
	dot := new(big.Float).Add(
		new(big.Float).Add(
			new(big.Float).Mul(nx, bf(s.Center.X)),
			new(big.Float).Mul(ny, bf(s.Center.Y)),
		),
		new(big.Float).Mul(nz, bf(s.Center.Z)),
	)
	return new(big.Float).Add(dot, d)
}

// exactMax is the largest signed distance of any point of s to p.
func exactMax(p Plane, s Sphere) *big.Float {
	return new(big.Float).Add(exactCenterDist(p, s), bf(s.Radius))
}

// exactState is the reference three-state classification. Outside is only
// reported when the true maximum signed distance is negative; Inside only
// when the true minimum (center distance minus radius) is positive.
func exactState(f Frustum, s Sphere) State {
	inside := true
	for _, p := range f.Planes {
		if exactMax(p, s).Sign() < 0 {
			return Outside
		}
		if exactCenterDist(p, s).Cmp(bf(s.Radius)) <= 0 {
			inside = false
		}
	}
	if inside {
		return Inside
	}
	return Intersect
}

// ---------------------------------------------------------------------------
// Batch result must equal the one-object-at-a-time result, exactly
// ---------------------------------------------------------------------------

func TestCullMatchesPerObjectFixed(t *testing.T) {
	f := boxFrustum(Vec3{X: -1, Y: -2, Z: -3}, Vec3{X: 4, Y: 5, Z: 6}, 3.5)
	spheres := []Sphere{
		{Center: Vec3{X: 1, Y: 1, Z: 1}, Radius: 0.5},   // inside
		{Center: Vec3{X: 100, Y: 1, Z: 1}, Radius: 1},   // outside +x
		{Center: Vec3{X: 1, Y: 1, Z: 1}, Radius: 10},    // intersects
		{Center: Vec3{X: -1, Y: -2, Z: -3}, Radius: 2},  // over a corner
		{Center: Vec3{X: 4, Y: 5, Z: 6}, Radius: 0.001}, // over the far corner
		{Center: Vec3{X: 1.5, Y: 1.5, Z: 1.5}, Radius: 1e-9},
	}
	res, err := Cull(f, spheres)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.States) != len(spheres) {
		t.Fatalf("结果数错误: %d", len(res.States))
	}
	for i, s := range spheres {
		if want := ClassifySphere(f, s); res.States[i] != want {
			t.Fatalf("物体 %d: 批量 %v != 逐个 %v", i, res.States[i], want)
		}
	}
	if res.Stats.Inside+res.Stats.Intersect+res.Stats.Outside != len(spheres) {
		t.Fatalf("三态计数不自洽: %+v", res.Stats)
	}
}

func TestCullMatchesPerObjectRandom(t *testing.T) {
	rng := rand.New(rand.NewSource(20261124))
	for trial := 0; trial < 200; trial++ {
		scale := math.Pow(10, float64(rng.Intn(9)-4)) // 1e-4 .. 1e4
		f := boxFrustum(
			Vec3{X: -1 - rng.Float64()*3, Y: -1 - rng.Float64()*3, Z: -1 - rng.Float64()*3},
			Vec3{X: 1 + rng.Float64()*3, Y: 1 + rng.Float64()*3, Z: 1 + rng.Float64()*3},
			scale,
		)
		spheres := make([]Sphere, 80)
		for i := range spheres {
			spheres[i] = Sphere{
				Center: Vec3{
					X: (rng.Float64()*2 - 1) * 30,
					Y: (rng.Float64()*2 - 1) * 30,
					Z: (rng.Float64()*2 - 1) * 30,
				},
				Radius: math.Pow(10, rng.Float64()*4-2),
			}
		}
		// Exact boundary ties and one-ulp neighbours exercise the tolerance.
		spheres[0] = Sphere{Center: Vec3{X: 1, Y: 0, Z: 0}, Radius: 1}
		spheres[1] = Sphere{Center: Vec3{X: math.Nextafter(1, 2), Y: 0, Z: 0}, Radius: 1}

		res, err := Cull(f, spheres)
		if err != nil {
			t.Fatalf("trial %d: %v", trial, err)
		}
		for i, s := range spheres {
			if got, want := res.States[i], ClassifySphere(f, s); got != want {
				t.Fatalf("trial %d 物体 %d: 批量 %v != 逐个 %v (球 %+v)", trial, i, got, want, s)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Conservativeness, checked separately from exact equality
// ---------------------------------------------------------------------------

// Intersect means "may be visible" and must never be dropped.
func TestIntersectNeverDropped(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	f := boxFrustum(Vec3{X: -1, Y: -1, Z: -1}, Vec3{X: 1, Y: 1, Z: 1}, 7)
	for i := 0; i < 20000; i++ {
		s := Sphere{
			Center: Vec3{X: rng.Float64()*4 - 2, Y: rng.Float64()*4 - 2, Z: rng.Float64()*4 - 2},
			Radius: math.Pow(10, rng.Float64()*4-3),
		}
		got := ClassifySphere(f, s)
		exact := exactState(f, s)
		// The only acceptable disagreement is a conservative one: the float
		// path may keep a sphere that really misses, but must never drop one
		// that intersects. It must also agree on every sphere whose true
		// distance is clear of the tolerance band, not just on the sign.
		if got == Outside && exact != Outside {
			t.Fatalf("相交球被误剔除: 球 %+v 精确 %v 实际 %v", s, exact, got)
		}
		if exact != Outside && !Visible(f, s) {
			t.Fatalf("可见球被剔除: 球 %+v 精确 %v", s, exact)
		}
		if got != exact {
			if !nearBoundary(f, s) {
				t.Fatalf("远离容差带却判定不一致: 球 %+v 精确 %v 实际 %v", s, exact, got)
			}
		}
	}
}

// nearBoundary reports whether any plane of f puts the sphere's true extreme
// distance within 1e-9 of zero, i.e. inside the tolerance band where a
// conservative disagreement is expected.
func nearBoundary(f Frustum, s Sphere) bool {
	const band = 1e-9
	for _, p := range f.Planes {
		e, _ := exactMax(p, s).Float64()
		if math.Abs(e) <= band {
			return true
		}
		d, _ := exactCenterDist(p, s).Float64()
		if math.Abs(d-s.Radius) <= band || math.Abs(d+s.Radius) <= band {
			return true
		}
	}
	return false
}

// Outside must only be reported for spheres that really are outside.
func TestOutsideIsSound(t *testing.T) {
	rng := rand.New(rand.NewSource(99))
	f := boxFrustum(Vec3{X: -1, Y: -1, Z: -1}, Vec3{X: 1, Y: 1, Z: 1}, 1.7)
	spheres := make([]Sphere, 5000)
	for i := range spheres {
		spheres[i] = Sphere{
			Center: Vec3{X: rng.Float64()*8 - 4, Y: rng.Float64()*8 - 4, Z: rng.Float64()*8 - 4},
			Radius: math.Pow(10, rng.Float64()*3-2),
		}
	}
	res, err := Cull(f, spheres)
	if err != nil {
		t.Fatal(err)
	}
	for i, s := range spheres {
		if res.States[i] != Outside {
			continue
		}
		if exactState(f, s) != Outside {
			t.Fatalf("物体 %d 判为 Outside 但精确判定并非 Outside: %+v", i, s)
		}
	}
}

// ---------------------------------------------------------------------------
// Measured error must stay within the derived tolerance bound
// ---------------------------------------------------------------------------

func TestToleranceBoundHolds(t *testing.T) {
	rng := rand.New(rand.NewSource(4242))
	trials := 0
	maxRatio := 0.0
	for trial := 0; trial < 5000; trial++ {
		scale := math.Pow(10, float64(rng.Intn(21)-10)) // 1e-10 .. 1e10
		p := Plane{
			N: Vec3{
				X: (rng.Float64()*2 - 1) * scale,
				Y: (rng.Float64()*2 - 1) * scale,
				Z: (rng.Float64()*2 - 1) * scale,
			},
			D: (rng.Float64()*2 - 1) * scale * 10,
		}
		if p.N.X == 0 && p.N.Y == 0 && p.N.Z == 0 {
			continue
		}
		cs := math.Pow(10, float64(rng.Intn(11)-5))
		s := Sphere{
			Center: Vec3{
				X: (rng.Float64()*2 - 1) * cs,
				Y: (rng.Float64()*2 - 1) * cs,
				Z: (rng.Float64()*2 - 1) * cs,
			},
			Radius: math.Pow(10, rng.Float64()*6-3),
		}
		got, tol, err := MaxSignedDistance(p, s)
		if err != nil {
			t.Fatalf("trial %d: %v (p=%+v s=%+v)", trial, err, p, s)
		}
		errBF := new(big.Float).Sub(bf(got), exactMax(p, s))
		errBF.Abs(errBF)
		if errBF.Cmp(bf(tol)) > 0 {
			t.Fatalf("trial %d: 实测误差 %s > 推导上界 %g (p=%+v s=%+v)",
				trial, errBF.Text('g', 25), tol, p, s)
		}
		if tol > 0 {
			ratio, _ := new(big.Float).Quo(errBF, bf(tol)).Float64()
			if ratio > maxRatio {
				maxRatio = ratio
			}
		}
		trials++
	}
	if trials == 0 {
		t.Fatal("没有有效样本")
	}
	t.Logf("实测误差 / 推导上界 最大比值 %.6f（%d 组样本）", maxRatio, trials)
}

// ---------------------------------------------------------------------------
// Required scenes: all in, all out, boundary, degenerate sphere, large scene
// ---------------------------------------------------------------------------

func TestSceneAllInside(t *testing.T) {
	f := boxFrustum(Vec3{X: -10, Y: -10, Z: -10}, Vec3{X: 10, Y: 10, Z: 10}, 2)
	spheres := []Sphere{
		{Center: Vec3{X: 0, Y: 0, Z: 0}, Radius: 1},
		{Center: Vec3{X: 5, Y: -5, Z: 5}, Radius: 4.9},
		{Center: Vec3{X: -9, Y: 9, Z: 0}, Radius: 0.5},
	}
	res, err := Cull(f, spheres)
	if err != nil {
		t.Fatal(err)
	}
	if res.Stats.Inside != len(spheres) || res.Stats.Intersect != 0 || res.Stats.Outside != 0 {
		t.Fatalf("三态计数错误: %+v", res.Stats)
	}
	// Nothing is pruned when everything is inside.
	if res.Stats.ObjectVisits != len(spheres) || res.Stats.PlaneTests != 6*len(spheres) {
		t.Fatalf("访问计数错误: %+v", res.Stats)
	}
}

func TestSceneAllOutside(t *testing.T) {
	f := unitBox()
	spheres := []Sphere{
		{Center: Vec3{X: 10, Y: 0, Z: 0}, Radius: 1},
		{Center: Vec3{X: 0, Y: -10, Z: 0}, Radius: 2},
		{Center: Vec3{X: 0, Y: 0, Z: 10}, Radius: 3},
		{Center: Vec3{X: -5, Y: -5, Z: -5}, Radius: 0.5},
	}
	res, err := Cull(f, spheres)
	if err != nil {
		t.Fatal(err)
	}
	if res.Stats.Outside != len(spheres) || res.Stats.Inside != 0 || res.Stats.Intersect != 0 {
		t.Fatalf("三态计数错误: %+v", res.Stats)
	}
	// Every sphere is culled by exactly one failing plane test; the other
	// planes are never visited, so an all-outside scene costs less than the
	// worst case of 6 per sphere. The scene is laid out so four culls take
	// twelve tests total.
	if res.Stats.Culled != len(spheres) || res.Stats.PlaneTests >= 6*len(spheres) {
		t.Fatalf("早剔除计数错误: %+v", res.Stats)
	}
	for i, st := range res.States {
		if st != Outside {
			t.Fatalf("物体 %d 应在外: %v", i, st)
		}
	}
}

func TestSceneBoundaryIntersects(t *testing.T) {
	f := unitBox()
	spheres := []Sphere{
		{Center: Vec3{X: 1, Y: 0, Z: 0}, Radius: 1},                     // straddles +x
		{Center: Vec3{X: 2, Y: 0, Z: 0}, Radius: 1},                     // externally tangent at +x
		{Center: Vec3{X: 0, Y: 0, Z: 0}, Radius: 1},                     // internally tangent on every face
		{Center: Vec3{X: 1, Y: 1, Z: 1}, Radius: math.Sqrt(3)},          // circumscribes the corner
		{Center: Vec3{X: 1.5, Y: 0, Z: 0}, Radius: 0.5000000000000002},  // last-ulp overlap
		{Center: Vec3{X: 0.5, Y: 0.5, Z: 0.5}, Radius: 0.49},            // comfortably inside
		{Center: Vec3{X: 1.5 + 1e-12, Y: 0, Z: 0}, Radius: 0.5 - 1e-12}, // genuine 2e-12 gap
	}
	res, err := Cull(f, spheres)
	if err != nil {
		t.Fatal(err)
	}
	for i, s := range spheres {
		if want := ClassifySphere(f, s); res.States[i] != want {
			t.Fatalf("物体 %d 批量 %v != 逐个 %v", i, res.States[i], want)
		}
	}
	// The first five touch or cross a face and must be Intersect. The sixth
	// is strictly inside. The seventh is really outside by 2e-12 and is only
	// allowed to be Outside because that gap is far beyond the tolerance.
	for i := 0; i < 5; i++ {
		if res.States[i] != Intersect {
			t.Fatalf("物体 %d 应为 Intersect: %v", i, res.States[i])
		}
	}
	if res.States[5] != Inside {
		t.Fatalf("物体 5 应为 Inside: %v", res.States[5])
	}
	if res.States[6] != Outside {
		t.Fatalf("物体 6 应在外: %v", res.States[6])
	}
	if res.Stats.Intersect != 5 || res.Stats.Inside != 1 || res.Stats.Outside != 1 {
		t.Fatalf("三态计数错误: %+v", res.Stats)
	}
}

func TestDegenerateSphereRejected(t *testing.T) {
	f := unitBox()
	cases := []struct {
		name   string
		index  int
		mutate func(*Sphere)
	}{
		{"零半径-首", 0, func(s *Sphere) { s.Radius = 0 }},
		{"零半径-中", 1, func(s *Sphere) { s.Radius = 0 }},
		{"零半径-末", 2, func(s *Sphere) { s.Radius = 0 }},
		{"负半径", 1, func(s *Sphere) { s.Radius = -1 }},
		{"NaN半径", 1, func(s *Sphere) { s.Radius = math.NaN() }},
		{"Inf半径", 1, func(s *Sphere) { s.Radius = math.Inf(1) }},
		{"NaN球心", 2, func(s *Sphere) { s.Center.Y = math.NaN() }},
		{"Inf球心", 0, func(s *Sphere) { s.Center.Z = math.Inf(-1) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spheres := []Sphere{
				{Center: Vec3{X: 0.1, Y: 0.1, Z: 0.1}, Radius: 0.2},
				{Center: Vec3{X: 0, Y: 0, Z: 0}, Radius: 0.3},
				{Center: Vec3{X: -0.2, Y: 0.2, Z: 0}, Radius: 0.1},
			}
			tc.mutate(&spheres[tc.index])
			_, err := Cull(f, spheres)
			if err == nil {
				t.Fatal("非法包围球应报错")
			}
			var ce *CullError
			if !errors.As(err, &ce) {
				t.Fatalf("错误类型错误: %T", err)
			}
			if ce.Object != tc.index {
				t.Fatalf("应报出物体号 %d，实际 %d: %v", tc.index, ce.Object, err)
			}
			if !strings.Contains(err.Error(), "物体 "+itoa(tc.index)) {
				t.Fatalf("错误信息未包含物体号: %v", err)
			}
			if tc.name[:3] == "零半径" && !errors.Is(err, ErrZeroRadius) {
				t.Fatalf("应为 ErrZeroRadius: %v", err)
			}
		})
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func TestLargeScene(t *testing.T) {
	const n = 50000
	rng := rand.New(rand.NewSource(5))
	f := boxFrustum(Vec3{X: -5, Y: -5, Z: -5}, Vec3{X: 5, Y: 5, Z: 5}, 1)
	spheres := make([]Sphere, n)
	for i := range spheres {
		spheres[i] = Sphere{
			Center: Vec3{
				X: rng.Float64()*40 - 20,
				Y: rng.Float64()*40 - 20,
				Z: rng.Float64()*40 - 20,
			},
			Radius: 0.1 + rng.Float64()*3,
		}
	}
	res, err := Cull(f, spheres)
	if err != nil {
		t.Fatal(err)
	}
	if res.Stats.Total != n || res.Stats.Inside+res.Stats.Intersect+res.Stats.Outside != n {
		t.Fatalf("三态计数不自洽: %+v", res.Stats)
	}
	if res.Stats.ObjectVisits != n {
		t.Fatalf("访问物体数错误: %d", res.Stats.ObjectVisits)
	}
	if res.Stats.PlaneTests <= 0 || res.Stats.PlaneTests > 6*n {
		t.Fatalf("平面测试数越界: %d", res.Stats.PlaneTests)
	}
	// A sparse scene must trigger early rejection on the batch path.
	if res.Stats.PlaneTests >= 6*n {
		t.Fatalf("大规模场景未触发早剔除: %+v", res.Stats)
	}
	for i := 0; i < n; i++ {
		if got, want := res.States[i], ClassifySphere(f, spheres[i]); got != want {
			t.Fatalf("物体 %d: 批量 %v != 逐个 %v", i, got, want)
		}
	}
	t.Logf("大场景: %s", res.Stats.Summary())
}

// ---------------------------------------------------------------------------
// Rejections: empty scene and degenerate planes
// ---------------------------------------------------------------------------

func TestEmptySceneRejected(t *testing.T) {
	_, err := Cull(unitBox(), nil)
	if err == nil {
		t.Fatal("空场景应报错")
	}
	if !errors.Is(err, ErrEmptyScene) {
		t.Fatalf("错误种类错误: %v", err)
	}
	var ce *CullError
	if !errors.As(err, &ce) {
		t.Fatalf("错误类型错误: %T", err)
	}
	if ce.Object != -1 {
		t.Fatalf("空场景不应给出物体号: %d", ce.Object)
	}
	if !strings.Contains(err.Error(), "物体号") {
		t.Fatalf("错误信息应说明物体号: %v", err)
	}
}

func TestDegeneratePlanesRejected(t *testing.T) {
	spheres := []Sphere{{Center: Vec3{}, Radius: 1}}
	cases := []struct {
		name   string
		plane  int
		modify func(*Frustum)
	}{
		{"零法向-首", 0, func(f *Frustum) { f.Planes[0] = Plane{N: Vec3{}, D: 1} }},
		{"零法向-中", 3, func(f *Frustum) { f.Planes[3] = Plane{N: Vec3{}, D: -1} }},
		{"NaN法向", 5, func(f *Frustum) { f.Planes[5] = Plane{N: Vec3{X: math.NaN()}, D: 0} }},
		{"Inf法向", 2, func(f *Frustum) { f.Planes[2] = Plane{N: Vec3{Y: math.Inf(1)}, D: 0} }},
		{"NaN偏移", 1, func(f *Frustum) { f.Planes[1].D = math.NaN() }},
		{"Inf偏移", 4, func(f *Frustum) { f.Planes[4].D = math.Inf(-1) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := unitBox()
			tc.modify(&f)
			_, err := Cull(f, spheres)
			if err == nil {
				t.Fatal("退化平面应报错")
			}
			if !errors.Is(err, ErrDegeneratePlane) {
				t.Fatalf("错误种类错误: %v", err)
			}
			var ce *CullError
			if !errors.As(err, &ce) {
				t.Fatalf("错误类型错误: %T", err)
			}
			if ce.Plane != tc.plane {
				t.Fatalf("应报出平面号 %d，实际 %d: %v", tc.plane, ce.Plane, err)
			}
			if !strings.Contains(err.Error(), "平面 "+itoa(tc.plane)) {
				t.Fatalf("错误信息未包含平面号: %v", err)
			}
		})
	}
}

func TestNormalizePlaneScaling(t *testing.T) {
	// Tiny and huge normals must normalize without squaring overflow or
	// underflow; all describe the plane x = 1.
	for _, scale := range []float64{1e-300, 1e-200, 1e200, 1e300} {
		p := Plane{N: Vec3{X: scale}, D: -scale}
		np, err := NormalizePlane(p)
		if err != nil {
			t.Fatalf("scale %g: %v", scale, err)
		}
		if math.Abs(np.N.X-1) > 1e-15 || math.Abs(np.D+1) > 1e-15 {
			t.Fatalf("scale %g: 归一化错误 %+v", scale, np)
		}
	}
	if _, err := NormalizePlane(Plane{N: Vec3{}, D: 0}); !errors.Is(err, ErrDegeneratePlane) {
		t.Fatalf("零法向应报退化: %v", err)
	}
}
