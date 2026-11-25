package composite

import (
	"fmt"
	"testing"
)

func BenchmarkComposite3Layers256(b *testing.B) {
	const w, h = 256, 256
	mk := func(alpha uint8) *Layer {
		l, _ := NewLayer(w, h)
		for i := range l.Pix {
			l.Pix[i] = RGBA{R: uint8(i), G: uint8(i >> 1), B: alpha, A: alpha}
		}
		return l
	}
	layers := []*Layer{mk(80), mk(160), mk(120)}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Composite(layers); err != nil {
			b.Fatal(err)
		}
	}
}

// ExampleComposite 演示三层半透明图在两种括号下得到同一像素。
func ExampleComposite() {
	mk := func(px ...RGBA) *Layer {
		l, _ := NewLayer(1, 1)
		l.Pix[0] = px[0]
		return l
	}
	// alpha=(80,160,120)，反例颜色 (0,0,3)
	A := mk(RGBA{R: 0, A: 80})
	B := mk(RGBA{R: 0, A: 160})
	C := mk(RGBA{R: 3, A: 120})

	out, err := Composite([]*Layer{A, B, C})
	if err != nil {
		fmt.Println(err)
		return
	}
	// 精确累加器：左右括号恒等，颜色为 0，alpha 为 220。
	fmt.Printf("R=%d A=%d\n", out.Pix[0].R, out.Pix[0].A)
	// Output: R=0 A=220
}
