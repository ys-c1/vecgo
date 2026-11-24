package fx

import "testing"

func TestFromFloatRoundTrip(t *testing.T) {
	// q=0 网格只含整数，1.25 取整为 1。
	if got := FromFloat(1.25, 0).Float(0); got != 1 {
		t.Fatalf("q=0 取整错误: %v", got)
	}
	for _, q := range []uint{4, 16, 24} {
		x := FromFloat(1.25, q)
		if got := x.Float(q); got != 1.25 {
			t.Fatalf("q=%d 往返失败: %v", q, got)
		}
	}
}

func TestFromFloatRounding(t *testing.T) {
	// Q=1 网格步长 0.5：0.6 四舍五入到 0.5，-0.6 到 -0.5
	if got := FromFloat(0.6, 1).Float(1); got != 0.5 {
		t.Fatalf("正数四舍五入错误: %v", got)
	}
	if got := FromFloat(-0.6, 1).Float(1); got != -0.5 {
		t.Fatalf("负数四舍五入错误: %v", got)
	}
}

func TestMulShr(t *testing.T) {
	const q = uint(16)
	one := int64(1) << q
	// 0.5 * 0.5 = 0.25
	got := MulShr(T(one/2), T(one/2), q)
	want := T(one / 4)
	if got != want {
		t.Fatalf("MulShr 0.5*0.5: got %d want %d", got, want)
	}
	// 与浮点截断结果对照：1.1 * 1.2
	a, b := FromFloat(1.1, q), FromFloat(1.2, q)
	got = MulShr(a, b, q)
	ref := FromFloat(1.1*1.2, q)
	// 定点向零截断，允许差 1 个 ULP
	d := int64(got) - int64(ref)
	if d < -1 || d > 1 {
		t.Fatalf("MulShr 与浮点参考偏差过大: got %d ref %d", got, ref)
	}
}

func TestMulShrLarge(t *testing.T) {
	// 乘积超出 int64 的场景，验证 128 位中间结果不溢出。
	const q = uint(24)
	big := T(1) << 30 // 约 64.0（Q24）
	got := MulShr(big, big, q)
	want := T(1) << 36 // 64^2 = 4096（Q24）
	if got != want {
		t.Fatalf("大整数乘法错误: got %d want %d", got, want)
	}
}

func TestDiv128(t *testing.T) {
	// 7 / 2 = 3（向零），-7 / 2 = -3（向零）。
	// -7 的 128 位补码为 (hi=-1, lo=-7)。
	if got := Div128(0, 7, 2); got != 3 {
		t.Fatalf("正除法错误: %d", got)
	}
	if got := Div128(-1, -7, 2); got != -3 {
		t.Fatalf("负除法错误: %d", got)
	}
}

func TestRoundDiv(t *testing.T) {
	if got := RoundDiv(5, 2); got != 3 {
		t.Fatalf("5/2 应四舍五入为 3: %d", got)
	}
	if got := RoundDiv(-5, 2); got != -3 {
		t.Fatalf("-5/2 应四舍五入为 -3: %d", got)
	}
}
