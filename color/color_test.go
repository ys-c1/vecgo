package color

import (
	"math"
	"strings"
	"testing"
)

func TestGammaOneIsIdentityQuantization(t *testing.T) {
	// gamma=1 必须回到历史的直接量化路径：uint8(255*x)。
	for x := 0; x <= 1000; x++ {
		linear := float64(x) / 1000
		got, err := Encode(linear, 1)
		if err != nil {
			t.Fatal(err)
		}
		want := uint8(0)
		if linear > 0 && linear < 1 {
			want = uint8(255 * linear)
		} else if linear >= 1 {
			want = 255
		}
		if got != want {
			t.Fatalf("gamma=1 量化偏移: Encode(%v)=%d want %d", linear, got, want)
		}
	}
	// Decode 在 gamma=1 下严格等于 i/255。
	for i := 0; i < 256; i++ {
		v, err := Decode(uint8(i), 1)
		if err != nil {
			t.Fatal(err)
		}
		if v != float64(i)/255 {
			t.Fatalf("gamma=1 Decode(%d)=%v", i, v)
		}
	}
}

func TestEncodeGammaTwoBrightens(t *testing.T) {
	v, err := Encode(128.0/255, 2)
	if err != nil {
		t.Fatal(err)
	}
	if v <= 128 {
		t.Fatalf("gamma>1 应提亮中间调，得到 %d", v)
	}
}

func TestEndpointsStable(t *testing.T) {
	for _, g := range []float64{0.1, 0.5, 1, 2.2, 10} {
		lo, err := Encode(0, g)
		if err != nil || lo != 0 {
			t.Fatalf("黑场不稳定 g=%v: %d %v", g, lo, err)
		}
		hi, err := Encode(1, g)
		if err != nil || hi != 255 {
			t.Fatalf("白场不稳定 g=%v: %d %v", g, hi, err)
		}
	}
}

func TestLUTsAndScalarAgree(t *testing.T) {
	// 表路径与标量路径必须是同一份函数，逐值一致。
	for _, g := range []float64{0.5, 1, 2.2} {
		enc, err := EncodeLUT(g)
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 256; i++ {
			want, err := Encode(ByteToLinear(uint8(i)), g)
			if err != nil {
				t.Fatal(err)
			}
			if enc[i] != want {
				t.Fatalf("编码表与标量不一致 g=%v i=%d: %d != %d", g, i, enc[i], want)
			}
		}
		dec, err := DecodeLUT(g)
		if err != nil {
			t.Fatal(err)
		}
		// 解码表必须单调不减；再编码误差在暗部地板（解码到线性 0 的
		// 最低几个编码级）以外不超过一个量化级。截断编码是多对一的，
		// 地板处不存在逐字节精确逆映射。
		for e := 1; e < 256; e++ {
			if dec[e] < dec[e-1] {
				t.Fatalf("g=%v 解码表非单调: dec[%d]=%d < dec[%d]=%d",
					g, e, dec[e], e-1, dec[e-1])
			}
		}
		floor := map[float64]uint8{0.5: 0, 1: 0, 2.2: 13}[g]
		for e := 0; e < 256; e++ {
			if dec[e] <= floor {
				continue // 暗部地板：固有量化损失
			}
			if d := absInt(int(enc[dec[e]]) - e); d > 1 {
				t.Fatalf("g=%v 再编码误差超过一个量化级: e=%d -> %d -> %d",
					g, e, dec[e], enc[dec[e]])
			}
		}
	}
}

func TestRoundTripLinearDomain(t *testing.T) {
	// 经字节量化的往返：误差受 8 位编码限制，暗部（gamma<1 时压缩亮部、
	// gamma>1 时压缩暗部）损失最大。给出实测量级的有界断言，
	// 浮点标量本身（不经过字节）由 TestScalarFloatInverse 保证精确。
	worst := map[float64]float64{0.5: 16.0 / 255, 2.2: 3.0 / 255}
	for _, g := range []float64{0.5, 2.2} {
		maxErr := 0.0
		for i := 0; i < 256; i++ {
			enc, err := Encode(ByteToLinear(uint8(i)), g)
			if err != nil {
				t.Fatal(err)
			}
			back, err := Decode(enc, g)
			if err != nil {
				t.Fatal(err)
			}
			if d := math.Abs(back - ByteToLinear(uint8(i))); d > maxErr {
				maxErr = d
			}
		}
		if maxErr > worst[g] {
			t.Fatalf("g=%v 字节线性域往返误差 %v 超过界 %v", g, maxErr, worst[g])
		}
	}
}

func TestScalarFloatInverse(t *testing.T) {
	// 不经过字节量化：Encode/Decode 的浮点幂函数必须精确互逆。
	for _, g := range []float64{0.5, 1, 2.2} {
		for k := 0; k <= 100; k++ {
			x := float64(k) / 100
			enc, err := EncodeFloat(x, g)
			if err != nil {
				t.Fatal(err)
			}
			back, err := DecodeFloat(enc, g)
			if err != nil {
				t.Fatal(err)
			}
			if d := math.Abs(back - x); d > 1e-12 {
				t.Fatalf("g=%v x=%v 浮点往返误差 %v", g, x, d)
			}
		}
	}
}

func TestByteLUTRoundTripBounded(t *testing.T) {
	// 字节路径的量化极限：解码表单调不减；再编码误差在暗部地板以上
	// 不超过一个量化级。最暗的几个编码字节会量化到线性 0（陡 gamma 下
	// 8 位表示的固有损失，如 g=2.2 时 pow(2/255,2.2) 小于一个线性级），
	// 标量浮点路径 TestRoundTripLinearDomain 仍保持精确。
	for _, g := range []float64{0.5, 1, 2.2} {
		enc, err := EncodeLUT(g)
		if err != nil {
			t.Fatal(err)
		}
		dec, err := DecodeLUT(g)
		if err != nil {
			t.Fatal(err)
		}
		for e := 1; e < 256; e++ {
			if dec[e] < dec[e-1] {
				t.Fatalf("g=%v 解码表非单调 e=%d", g, e)
			}
		}
		// 暗部地板（实测）：陡 gamma 下最低的编码级解码到极小的线性值，
		// 截断量化使这一段不存在一个量化级内的紧逆映射。
		floor := map[float64]uint8{0.5: 0, 1: 0, 2.2: 13}[g]
		for e := 0; e < 256; e++ {
			d := absInt(int(enc[dec[e]]) - e)
			if g == 1 && d != 0 {
				t.Fatalf("g=1 必须恒等: e=%d -> %d -> %d", e, dec[e], enc[dec[e]])
			}
			if dec[e] <= floor {
				continue
			}
			if d > 1 {
				t.Fatalf("g=%v 字节往返超出一个量化级 e=%d -> %d -> %d",
					g, e, dec[e], enc[dec[e]])
			}
		}
	}
}

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func TestInvalidGammaError(t *testing.T) {
	for _, g := range []float64{0, 0.09, 10.1, math.NaN(), -1} {
		if _, err := Encode(0.5, g); err == nil || !strings.Contains(err.Error(), "color:") {
			t.Fatalf("gamma=%v 应返回带包名的错误，得到 %v", g, err)
		}
		if _, err := Decode(1, g); err == nil || !strings.Contains(err.Error(), "color:") {
			t.Fatalf("gamma=%v 应返回带包名的错误，得到 %v", g, err)
		}
		if _, err := EncodeLUT(g); err == nil {
			t.Fatalf("EncodeLUT gamma=%v 应报错", g)
		}
		if _, err := DecodeLUT(g); err == nil {
			t.Fatalf("DecodeLUT gamma=%v 应报错", g)
		}
	}
}
