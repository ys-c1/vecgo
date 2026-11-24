package format

import (
	"bytes"
	"errors"
	"testing"

	"vecgo/config"
	"vecgo/raster"
)

func sampleImage(t *testing.T) *raster.Image {
	t.Helper()
	im, err := raster.New(4, 3)
	if err != nil {
		t.Fatal(err)
	}
	for i := range im.Pix {
		im.Pix[i] = uint8(i * 17)
	}
	return im
}

func sampleMeta() Meta {
	return MetaFromConfig(config.Default())
}

func TestLegacyMatchesSavePGM(t *testing.T) {
	im := sampleImage(t)
	data, err := Encode(im, sampleMeta(), VersionLegacy)
	if err != nil {
		t.Fatal(err)
	}
	legacy := encodeLegacy(im)
	if !bytes.Equal(data, legacy) {
		t.Fatal("v0 必须与旧版 SavePGM 逐字节相同")
	}
	if bytes.Contains(data, []byte(magic)) {
		t.Fatal("v0 不得写入版本字段")
	}
}

// 三组版本组合：文件版本 × 解码器字段集。
func TestVersionMatrix(t *testing.T) {
	im := sampleImage(t)
	ref := im.Pix

	cases := []struct {
		version    int
		decoder    Decoder
		fieldset   int
		downgraded bool
		metaKnown  bool
	}{
		{VersionLegacy, DecoderV1, 1, false, false},
		{VersionLegacy, DecoderV2, 1, false, false},
		{Version1, DecoderV1, 1, false, false},
		{Version1, DecoderV2, 1, false, false},
		{Version2, DecoderV1, 1, true, false},
		{Version2, DecoderV2, 2, false, true},
	}
	for _, c := range cases {
		data, err := Encode(im, sampleMeta(), c.version)
		if err != nil {
			t.Fatalf("v%d 编码失败: %v", c.version, err)
		}
		dec, err := Decode(data, c.decoder)
		if err != nil {
			t.Fatalf("v%d + 字段集%d 应可读: %v", c.version, c.decoder.FieldSet, err)
		}
		if !bytes.Equal(dec.Image.Pix, ref) {
			t.Fatalf("v%d + 字段集%d 像素不一致", c.version, c.decoder.FieldSet)
		}
		if dec.FileVersion != c.version {
			t.Fatalf("v%d 读出的文件版本为 %d", c.version, dec.FileVersion)
		}
		if dec.SelectedField != c.fieldset {
			t.Fatalf("v%d + 字段集%d 选中字段集为 %d", c.version, c.decoder.FieldSet, dec.SelectedField)
		}
		if dec.Downgraded != c.downgraded {
			t.Fatalf("v%d + 字段集%d 降级标志应为 %v", c.version, c.decoder.FieldSet, c.downgraded)
		}
		if dec.MetadataKnown != c.metaKnown {
			t.Fatalf("v%d + 字段集%d 元数据可见性应为 %v", c.version, c.decoder.FieldSet, c.metaKnown)
		}
		if dec.MetadataKnown {
			m := sampleMeta()
			if dec.Meta.Shading != m.Shading || dec.Meta.Supersample != m.Supersample || dec.Meta.Blur != m.Blur {
				t.Fatalf("v2 元数据读出不一致: %+v", dec.Meta)
			}
			if dec.Meta.Gamma < 0.999 || dec.Meta.Gamma > 1.001 {
				t.Fatalf("gamma 读出 %v，应为 1 附近（毫值量化）", dec.Meta.Gamma)
			}
		}
	}
}

func TestDowngradeIsPixelIdentical(t *testing.T) {
	im := sampleImage(t)
	v2, err := Encode(im, sampleMeta(), Version2)
	if err != nil {
		t.Fatal(err)
	}
	d2, err := Decode(v2, DecoderV2)
	if err != nil {
		t.Fatal(err)
	}
	d1, err := Decode(v2, DecoderV1)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(d1.Image.Pix, d2.Image.Pix) {
		t.Fatal("v2 降级与完整解码像素必须一致")
	}
	v1, err := Encode(im, sampleMeta(), Version1)
	if err != nil {
		t.Fatal(err)
	}
	dv1, err := Decode(v1, DecoderV1)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(d1.Image.Pix, dv1.Image.Pix) {
		t.Fatal("v2 降级像素必须与 v1 逐像素一致")
	}
}

func TestDeterministic(t *testing.T) {
	im := sampleImage(t)
	for _, v := range []int{VersionLegacy, Version1, Version2} {
		a, _ := Encode(im, sampleMeta(), v)
		b, _ := Encode(im, sampleMeta(), v)
		if !bytes.Equal(a, b) {
			t.Fatalf("v%d 编码不确定", v)
		}
	}
}

func TestV2HeaderStillValidPGM(t *testing.T) {
	im := sampleImage(t)
	data, err := Encode(im, sampleMeta(), Version2)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(data, []byte("P5\n")) {
		t.Fatal("v2 仍是 P5")
	}
	// 注释行之后必须紧跟画幅行，且像素区位置与老 PGM 一致。
	if !bytes.HasPrefix(data, []byte("P5\n#VCFM ")) {
		t.Fatal("v2 版本字段应位于 P5 后的注释行")
	}
	idx := bytes.Index(data, []byte("\n4 3\n255\n"))
	if idx < 0 {
		t.Fatal("v2 画幅行缺失")
	}
	if len(data)-(idx+len("\n4 3\n255\n")) != len(im.Pix) {
		t.Fatal("v2 像素区长度异常")
	}
}

func TestRejectsBadEncodes(t *testing.T) {
	im := sampleImage(t)
	meta := sampleMeta()
	if _, err := Encode(im, meta, 3); !errors.Is(err, ErrUnsupportedVersion) {
		t.Fatalf("写入 v3 应拒绝: %v", err)
	}
	meta.Shading = "bogus"
	if _, err := Encode(im, meta, Version2); err == nil {
		t.Fatal("未知着色应拒绝")
	}
	meta = sampleMeta()
	meta.Gamma = 0
	if _, err := Encode(im, meta, Version2); err == nil {
		t.Fatal("gamma 越界应拒绝")
	}
}

func TestRejectsBadDecodes(t *testing.T) {
	im := sampleImage(t)
	good, err := Encode(im, sampleMeta(), Version2)
	if err != nil {
		t.Fatal(err)
	}

	// 未登记版本 3：把版本字段改成 0003。
	badVer := bytes.Replace(good, []byte("VCFM 5643464d0200"), []byte("VCFM 5643464d0300"), 1)
	if _, err := Decode(badVer, DecoderV2); !errors.Is(err, ErrUnsupportedVersion) {
		t.Fatalf("v3 应明确拒绝: %v", err)
	}

	// 未知布局标志：把 v2 的 flags 从 0001 改成 0003。
	badFlag := bytes.Replace(good, []byte("02000100"), []byte("02000300"), 1)
	if _, err := Decode(badFlag, DecoderV2); !errors.Is(err, ErrUnsupportedLayout) {
		t.Fatalf("未知标志应明确拒绝: %v", err)
	}

	// 校验和不符：翻转像素末字节。
	badPix := append([]byte{}, good...)
	badPix[len(badPix)-1] ^= 0xff
	if _, err := Decode(badPix, DecoderV2); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("CRC 不符应拒绝: %v", err)
	}

	// 降级解码器不校验 v2 元数据，但仍必须解出像素。
	dec, err := Decode(badPix, DecoderV1)
	if err != nil {
		t.Fatalf("旧解码器应能降级读取（不校验 v2 元数据）: %v", err)
	}
	if dec.Downgraded != true {
		t.Fatal("应标记为降级")
	}

	// 画幅声明与 PGM 不符：把字段块里的宽 0400 改成 0500。
	badDim := bytes.Replace(good, []byte("5643464d0200010004000300"), []byte("5643464d0200010005000300"), 1)
	if _, err := Decode(badDim, DecoderV2); !errors.Is(err, ErrBadHeader) {
		t.Fatalf("画幅不一致应拒绝: %v", err)
	}

	// 截断像素区。
	trunc := good[:len(good)-1]
	if _, err := Decode(trunc, DecoderV2); !errors.Is(err, ErrBadHeader) {
		t.Fatalf("截断应拒绝: %v", err)
	}

	// 非十六进制版本块。
	badHex := bytes.Replace(good, []byte("VCFM 5643"), []byte("VCFM 5x43"), 1)
	if _, err := Decode(badHex, DecoderV2); !errors.Is(err, ErrBadHeader) {
		t.Fatalf("非法十六进制应拒绝: %v", err)
	}
}

func TestRejectsUnknownDecoder(t *testing.T) {
	im := sampleImage(t)
	data, _ := Encode(im, sampleMeta(), Version2)
	if _, err := Decode(data, Decoder{FieldSet: 3}); err == nil {
		t.Fatal("字段集 3 应拒绝")
	}
}
