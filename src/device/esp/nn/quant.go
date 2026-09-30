package nn

import "math"

// SatRoundDoublingHighMul returns the high 32 bits of 2*a*b with rounding
// and saturation, as esp-nn esp_nn_sat_round_doubling_high_mul.
func SatRoundDoublingHighMul(a, b int32) int32 {
	if a == b && a == math.MinInt32 {
		return math.MaxInt32
	}
	nudge := int64(1 << 30)
	if (a < 0) != (b < 0) {
		nudge = 1 - nudge
	}
	m := int64(a)*int64(b) + nudge
	if m < 0 {
		m += 1<<31 - 1
	}
	return int32(m >> 31)
}

// DivByPowerOfTwo divides v by 2 to the power exp and rounds half away from
// zero, as esp-nn esp_nn_div_by_power_of_two.
func DivByPowerOfTwo(v, exp int32) int32 {
	mask := int32(1)<<uint(exp) - 1
	rem := v & mask
	r := v >> uint(exp)
	threshold := mask >> 1
	if r < 0 {
		threshold++
	}
	if rem > threshold {
		r++
	}
	return r
}

// MultiplyByQuantizedMult scales x by the fixed point multiplier mult and by
// 2 to the power shift, as esp-nn esp_nn_multiply_by_quantized_mult.
func MultiplyByQuantizedMult(x, mult, shift int32) int32 {
	left, right := int32(0), -shift
	if shift > 0 {
		left, right = shift, 0
	}
	return DivByPowerOfTwo(SatRoundDoublingHighMul(x<<uint(left), mult), right)
}
