// render.go 为五段各生成“成功输出”的图，落在 <dir>/<section>/ 下。
//
// 每段只调用该段自己的包和它的输入矩阵，不借用别段结果：
// config 段画配置矩阵板，geom 段画几何基元，raster 段画 Fill 覆盖率，
// shading 段画 Apply 后的灰度图，post 段画 Gamma/Blur 后的图。
//
// 这正是 snap_test 的比对来源：同一份生成逻辑在 baseline（改动前）树上
// 跑一遍得到 testdata/golden，在改动后的树上跑一遍得到实际结果，
// 两棵树各段逐像素对比。
package snap

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"

	"vecgo/config"
	"vecgo/geom"
	"vecgo/post"
	"vecgo/raster"
	"vecgo/shading"
)

// golden 的输入矩阵定义在这里，生成器与测试共用，保证两棵树输入一致。
type sceneDef struct {
	name  string
	polys [][]geom.Point
}

func scenes() []sceneDef {
	return []sceneDef{
		{"square", [][]geom.Point{{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}}},
		{"two", [][]geom.Point{
			{{X: 0, Y: 0}, {X: 6, Y: 0}, {X: 6, Y: 6}, {X: 0, Y: 6}},
			{{X: 12, Y: 4}, {X: 18, Y: 4}, {X: 18, Y: 10}, {X: 12, Y: 10}},
		}},
		{"tri", [][]geom.Point{
			{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 5, Y: 9}},
			{{X: 8, Y: 6}, {X: 9, Y: 6}, {X: 8.5, Y: 9}},
		}},
	}
}

type cfgDef struct {
	name string
	cfg  config.Config
}

func cfgs() []cfgDef {
	base := config.Default()
	out := make([]cfgDef, 0, 9)
	add := func(name string, mut func(*config.Config)) {
		c := base
		mut(&c)
		out = append(out, cfgDef{name: name, cfg: c})
	}
	add("default", func(c *config.Config) {})
	add("gradient", func(c *config.Config) { c.Shading = "gradient" })
	add("gamma", func(c *config.Config) { c.Gamma = 2.2 })
	add("blur", func(c *config.Config) { c.Blur = 2 })
	add("ss1", func(c *config.Config) { c.Supersample = 1 })
	add("ss3", func(c *config.Config) { c.Width = 40; c.Height = 30; c.Supersample = 3 })
	add("small", func(c *config.Config) { c.Width = 16; c.Height = 16; c.Scale = 1.5 })
	add("rot", func(c *config.Config) { c.Rotate = math.Pi / 6; c.Scale = 0.8 })
	add("combo", func(c *config.Config) {
		c.Width, c.Height = 48, 40
		c.Shading = "gradient"
		c.Gamma = 1.7
		c.Blur = 1
		c.Rotate = 0.3
		c.Supersample = 3
	})
	return out
}

func writePGM(dir, name string, w, h int, pix []uint8) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	head := make([]byte, 0, 32)
	head = append(head, "P5\n"...)
	head = append(head, fmt.Sprintf("%d %d\n255\n", w, h)...)
	out := append(head, pix...)
	return os.WriteFile(filepath.Join(dir, name), out, 0o644)
}

// RenderAll 生成五段的全部基准图到 dir。各段独立：单段失败不阻止其余段，
// 全部失败用 errors.Join 汇总。基准生成本身是工具行为，编码归 snap 段的
// 判定放在 Compare 侧（见 snap.go）。
func RenderAll(dir string) error {
	var errs []error
	if err := RenderConfig(dir); err != nil {
		errs = append(errs, err)
	}
	if err := RenderGeom(dir); err != nil {
		errs = append(errs, err)
	}
	if err := RenderRaster(dir); err != nil {
		errs = append(errs, err)
	}
	if err := RenderShading(dir); err != nil {
		errs = append(errs, err)
	}
	if err := RenderPost(dir); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// RenderConfig 把每个配置的合法性与字段值规范化成一行像素。
func RenderConfig(dir string) error {
	cs := cfgs()
	const cols = 12
	w, h := cols, len(cs)
	pix := make([]uint8, w*h)
	for r, cd := range cs {
		v := cd.cfg
		ok := uint8(255)
		if err := v.Validate(); err != nil {
			ok = 0
		}
		pix[r*w+0] = ok
		pix[r*w+1] = uint8(clampByte(float64(v.Width) / 4096))
		pix[r*w+2] = uint8(clampByte(float64(v.Height) / 4096))
		pix[r*w+3] = uint8(clampByte(v.Scale / 100))
		pix[r*w+4] = uint8(clampByte(v.Rotate/math.Pi + 0.5))
		pix[r*w+5] = uint8(v.Supersample * 32)
		pix[r*w+6] = uint8(clampByte(v.Gamma / 10))
		pix[r*w+7] = uint8(v.Blur * 16)
		switch v.Shading {
		case "flat":
			pix[r*w+8] = 32
		case "gradient":
			pix[r*w+8] = 224
		default:
			pix[r*w+8] = 0
		}
		pix[r*w+9] = uint8(len(v.Output))
		pix[r*w+10] = 17
		pix[r*w+11] = 43
	}
	return writePGM(filepath.Join(dir, "config"), "config_matrix.pgm", w, h, pix)
}

func clampByte(f float64) float64 {
	if f < 0 {
		return 0
	}
	if f > 255 {
		return 255
	}
	return f
}

// RenderGeom 只用 geom 包的结果做显式绘制。
func RenderGeom(dir string) error {
	const W, H = 192, 128
	img := make([]uint8, W*H)
	dm := func(x, y float64) (int, int) {
		return int(x / 24 * W), int(y / 16 * H)
	}
	dot := func(x, y float64, v uint8) {
		ix, iy := dm(x, y)
		if ix >= 0 && ix < W && iy >= 0 && iy < H {
			img[iy*W+ix] = v
		}
	}
	line := func(pts []geom.Point, v uint8) {
		for i := 0; i+1 < len(pts); i++ {
			x0, y0 := pts[i].X, pts[i].Y
			x1, y1 := pts[i+1].X, pts[i+1].Y
			n := int(math.Hypot(x1-x0, y1-y0) * 8)
			if n < 1 {
				n = 1
			}
			for s := 0; s <= n; s++ {
				t := float64(s) / float64(n)
				dot(x0+(x1-x0)*t, y0+(y1-y0)*t, v)
			}
		}
	}
	for si, sd := range scenes() {
		for pi, poly := range sd.polys {
			c0 := poly[0]
			c1 := poly[1]
			c2 := poly[2]
			c3 := poly[len(poly)-1]
			pts, err := geom.Flatten(c0, c1, c2, c3, 7)
			if err != nil {
				return err
			}
			v := uint8(40 + si*30 + pi*20)
			line(pts, v)
			if st, err := geom.Stroke(pts, 0.15); err == nil {
				line(st, uint8(255-int(v)/3))
			}
			win := []geom.Point{{X: 2, Y: 1}, {X: 16, Y: 1}, {X: 16, Y: 8}, {X: 2, Y: 8}}
			cl, err := geom.Clip(poly, win)
			if err != nil {
				return err
			}
			if len(cl) >= 3 {
				for y := 0; y < 16; y++ {
					for x := 0; x < 24; x++ {
						px := (float64(x) + 0.5) / 24 * 24
						py := (float64(y) + 0.5) / 16 * 16
						if geom.Inside(cl, px, py) {
							dot(px, py, 200)
						}
					}
				}
			}
		}
	}
	return writePGM(filepath.Join(dir, "geom"), "geom_primitives.pgm", W, H, img)
}

// RenderRaster 画 Fill 覆盖率图。
func RenderRaster(dir string) error {
	for _, sd := range scenes() {
		for _, cd := range cfgs() {
			c, s := math.Cos(cd.cfg.Rotate), math.Sin(cd.cfg.Rotate)
			moved := make([][]geom.Point, len(sd.polys))
			for i := range sd.polys {
				moved[i] = make([]geom.Point, len(sd.polys[i]))
				for j, p := range sd.polys[i] {
					moved[i][j] = geom.Point{
						X: cd.cfg.Scale * (p.X*c - p.Y*s),
						Y: cd.cfg.Scale * (p.X*s + p.Y*c),
					}
				}
			}
			im, err := raster.New(cd.cfg.Width, cd.cfg.Height)
			if err != nil {
				return err
			}
			if err := im.Fill(moved, cd.cfg.Supersample); err != nil {
				return err
			}
			if err := writePGM(filepath.Join(dir, "raster"), sd.name+"_"+cd.name+".pgm", im.W, im.H, im.Pix); err != nil {
				return err
			}
		}
	}
	return nil
}

// RenderShading 画 Apply 前后的灰度图。
func RenderShading(dir string) error {
	base, err := raster.New(48, 32)
	if err != nil {
		return err
	}
	for i := range base.Pix {
		base.Pix[i] = uint8((i * 37) % 256)
	}
	for i := 0; i < len(base.Pix); i += 5 {
		base.Pix[i] = 0
	}
	if err := writePGM(filepath.Join(dir, "shading"), "base.pgm", base.W, base.H, base.Pix); err != nil {
		return err
	}
	for _, mode := range []string{"flat", "gradient"} {
		for _, tp := range [][2]float64{{1.0, 0.0}, {0.6, 0.2}, {0.0, 1.0}} {
			im := &raster.Image{W: base.W, H: base.H, Pix: append([]uint8(nil), base.Pix...)}
			if err := shading.Apply(im, mode, tp[0], tp[1]); err != nil {
				return err
			}
			name := fmt.Sprintf("%s_%.1f_%.1f.pgm", mode, tp[0], tp[1])
			if err := writePGM(filepath.Join(dir, "shading"), name, im.W, im.H, im.Pix); err != nil {
				return err
			}
		}
	}
	return nil
}

// RenderPost 画 Gamma / Blur 后的图。
func RenderPost(dir string) error {
	base, err := raster.New(48, 32)
	if err != nil {
		return err
	}
	for i := range base.Pix {
		if i%7 == 0 {
			base.Pix[i] = 0
		} else {
			base.Pix[i] = uint8((i*37 + 11) % 256)
		}
	}
	for _, g := range []float64{0.5, 1.0, 1.7, 2.2, 3.0} {
		im := &raster.Image{W: base.W, H: base.H, Pix: append([]uint8(nil), base.Pix...)}
		if err := post.Gamma(im, g); err != nil {
			return err
		}
		if err := writePGM(filepath.Join(dir, "post"), fmt.Sprintf("gamma_%.1f.pgm", g), im.W, im.H, im.Pix); err != nil {
			return err
		}
	}
	src, err := raster.New(64, 48)
	if err != nil {
		return err
	}
	for y := 8; y < 40; y++ {
		for x := 10; x < 54; x++ {
			src.Pix[y*src.W+x] = 255
		}
	}
	for y := 16; y < 32; y++ {
		src.Pix[y*src.W+32] = 128
	}
	for _, r := range []int{0, 1, 2, 3} {
		out, err := post.Blur(src, r)
		if err != nil {
			return err
		}
		if err := writePGM(filepath.Join(dir, "post"), fmt.Sprintf("blur_%d.pgm", r), out.W, out.H, out.Pix); err != nil {
			return err
		}
	}
	return nil
}
