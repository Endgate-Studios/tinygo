package nn

import (
	"math"
	"math/bits"
)

// SoftmaxScratchSize returns the scratch size in bytes for SoftmaxInt8.
func SoftmaxScratchSize(width, height int) int {
	return softmaxScratchSize(width, height)
}

// SoftmaxInt8 is the softmax of each row of width values, as esp-nn
// esp_nn_softmax_s8. The mult, shift and diffMin values are from the
// TensorFlow Lite softmax prepare step.
func SoftmaxInt8(out, in []int8, width int, mult, shift, diffMin int32, scratch []byte) {
	if width <= 0 || len(in)%width != 0 {
		panic("nn: bad softmax width")
	}
	checkLen(len(in), len(out))
	softmaxInt8(out[:len(in)], in, width, mult, shift, diffMin, scratch)
}

const softmaxAccumBits = 12

func softmaxInt8Go(out, in []int8, width int, mult, shift, diffMin int32) {
	mask := int32(1) << uint(shift)
	for row := 0; row+width <= len(in); row += width {
		x, y := in[row:row+width], out[row:row+width]
		m := x[0]
		for _, v := range x[1:] {
			m = max(m, v)
		}
		var sum int32
		for _, v := range x {
			if d := int32(v) - int32(m); d >= diffMin {
				e := expOnNegativeValues(SatRoundDoublingHighMul(d*mask, mult))
				sum += DivByPowerOfTwo(e, softmaxAccumBits)
			}
		}
		scale, bitsOver := softmaxScale(sum)
		for i, v := range x {
			if d := int32(v) - int32(m); d >= diffMin {
				e := expOnNegativeValues(SatRoundDoublingHighMul(d*mask, mult))
				y[i] = softmaxOut(scale, e, bitsOver)
			} else {
				y[i] = -128
			}
		}
	}
}

func softmaxScale(sum int32) (scale, bitsOver int32) {
	headroom := int32(bits.LeadingZeros32(uint32(sum)))
	scale = oneOverOnePlusX(sum<<uint(headroom) - math.MinInt32)
	return scale, softmaxAccumBits - headroom + 31 - 8
}

func softmaxOut(scale, e, bitsOver int32) int8 {
	return sat8(DivByPowerOfTwo(SatRoundDoublingHighMul(scale, e), bitsOver) - 128)
}

func mulPowerOf2(v int32, exp uint) int32 {
	threshold := int32(1)<<(31-exp) - 1
	if v > threshold {
		return math.MaxInt32
	}
	if v < -threshold {
		return math.MinInt32
	}
	return v << exp
}

func oneOverOnePlusX(v int32) int32 {
	sum := int64(v) + math.MaxInt32
	if sum >= 0 {
		sum++
	} else {
		sum--
	}
	half := int32(sum / 2)
	x := 1515870810 + SatRoundDoublingHighMul(half, -1010580540)
	for i := 0; i < 3; i++ {
		x += mulPowerOf2(SatRoundDoublingHighMul(x, 1<<29-SatRoundDoublingHighMul(half, x)), 2)
	}
	return mulPowerOf2(x, 1)
}

var expBarrelShifter = [...]int32{1672461947, 1302514674, 790015084, 290630308, 39332535, 720401, 242}

func expOnNegativeValues(v int32) int32 {
	if v == 0 {
		return math.MaxInt32
	}
	const quarter = 1 << 24
	modMinusQuarter := v&(quarter-1) - quarter
	rem := modMinusQuarter - v
	x := modMinusQuarter<<5 + 1<<28
	x2 := SatRoundDoublingHighMul(x, x)
	x3 := SatRoundDoublingHighMul(x2, x)
	x4 := DivByPowerOfTwo(SatRoundDoublingHighMul(x2, x2), 2)
	poly := DivByPowerOfTwo(SatRoundDoublingHighMul(x4+x3, 715827883)+x2, 1)
	r := 1895147668 + SatRoundDoublingHighMul(1895147668, x+poly)
	for i, k := range expBarrelShifter {
		if rem&(1<<(24+i)) != 0 {
			r = SatRoundDoublingHighMul(r, k)
		}
	}
	return r
}
