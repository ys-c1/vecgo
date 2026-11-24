// Package color 是 vecgo 唯一的灰度线性光/编码值转换口径。
//
// 约定：管线内部的 Image.Pix 一律表示线性光强度（0..1 量化为 uint8），
// PGM 文件与 post.Gamma 的输出才是按 gamma 编码后的字节。
// 其他包不得自行实现幂函数或重复转换逻辑，只能调用本包。
package color

import (
	"errors"
	"math"
)

// validGamma 与 config.Config.Validate 的 gamma 区间保持一致。
func validGamma(gamma float64) bool {
	return !math.IsNaN(gamma) && gamma >= 0.1 && gamma <= 10
}

// EncodeFloat 是不做字节量化的纯浮点编码，字节版 Encode 在它之上截断。
func EncodeFloat(linear, gamma float64) (float64, error) {
	if !validGamma(gamma) {
		return 0, errors.New("color: gamma 超出范围 [0.1,10]")
	}
	if linear <= 0 {
		return 0, nil
	}
	if linear >= 1 {
		return 1, nil
	}
	if gamma == 1 {
		return linear, nil
	}
	return math.Pow(linear, 1/gamma), nil
}

// DecodeFloat 是 Decode 的纯浮点形态，二者共用同一个幂函数核心。
func DecodeFloat(encoded, gamma float64) (float64, error) {
	if !validGamma(gamma) {
		return 0, errors.New("color: gamma 超出范围 [0.1,10]")
	}
	if encoded <= 0 {
		return 0, nil
	}
	if encoded >= 1 {
		return 1, nil
	}
	if gamma == 1 {
		return encoded, nil
	}
	return math.Pow(encoded, gamma), nil
}

// Encode 把线性光强度 linear（0..1）按 gamma 编码，并截断量化为一个字节。
// gamma=1 时结果恒等于 uint8(255*linear)，即历史的直接量化路径。
func Encode(linear, gamma float64) (uint8, error) {
	v, err := EncodeFloat(linear, gamma)
	if err != nil {
		return 0, err
	}
	return uint8(255 * v), nil
}

// Decode 把一个编码字节还原为线性光强度（0..1 的浮点）。
// gamma=1 时结果恒等于 encoded/255，与历史直接量化路径严格互逆。
func Decode(encoded uint8, gamma float64) (float64, error) {
	return DecodeFloat(float64(encoded)/255, gamma)
}

// LinearToByte 按线性光口径把 0..1 的强度量化为字节（gamma=1 的 Encode）。
// shading 阶段写入线性缓冲时统一走这里，保证三处只有一份转换代码。
func LinearToByte(linear float64) uint8 {
	if linear <= 0 {
		return 0
	}
	if linear >= 1 {
		return 255
	}
	return uint8(255 * linear)
}

// ByteToLinear 把线性缓冲中的一个字节还原为 0..1 的线性强度。
func ByteToLinear(linear uint8) float64 {
	return float64(linear) / 255
}

// EncodeLUT 返回 256 项编码查找表：索引为线性字节，值为编码字节。
func EncodeLUT(gamma float64) ([256]uint8, error) {
	var table [256]uint8
	for i := 0; i < 256; i++ {
		v, err := Encode(ByteToLinear(uint8(i)), gamma)
		if err != nil {
			return table, err
		}
		table[i] = v
	}
	return table, nil
}

// DecodeLUT 返回 256 项解码查找表：索引为编码字节，值为最接近的线性字节。
// 编码表是多对一的截断量化，不存在逐字节精确逆映射；本函数按浮点逆函数
// 取最近线性字节，保证单调不减，且再编码的误差不超过一个量化级。
// PGM 读回时用它把编码文件还原成线性缓冲。
func DecodeLUT(gamma float64) ([256]uint8, error) {
	var table [256]uint8
	for i := 0; i < 256; i++ {
		v, err := Decode(uint8(i), gamma)
		if err != nil {
			return table, err
		}
		if v <= 0 {
			table[i] = 0
			continue
		}
		if v >= 1 {
			table[i] = 255
			continue
		}
		r := int(255*v + 0.5)
		if r > 255 {
			r = 255
		}
		table[i] = uint8(r)
	}
	return table, nil
}
