package nn

import "math"

// LogisticTable makes the lookup table of LogisticInt8 for the input
// quantization, as esp-nn esp_nn_logistic_s8_prepare. The output scale is
// 1/256 and the output zero point is -128.
func LogisticTable(lut *[256]int8, inZeroPoint int32, inScale float32) {
	for i := range lut {
		x := float32(int32(int8(i))-inZeroPoint) * inScale
		s := 1 / (1 + float32(math.Exp(float64(-x))))
		lut[i] = sat8(int32(math.Round(float64(s*256))) - 128)
	}
}

// LogisticInt8 sets out[i] to the sigmoid of in[i] from lut, as esp-nn
// esp_nn_logistic_s8.
func LogisticInt8(out, in []int8, lut *[256]int8) {
	checkLen(len(in), len(out))
	for i, v := range in {
		out[i] = lut[uint8(v)]
	}
}
