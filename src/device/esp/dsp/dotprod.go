package dsp

// DotInt16 returns the sum of a[i]*b[i]. It panics if the lengths differ.
func DotInt16(a, b []int16) int64 {
	if len(a) != len(b) {
		panic("dsp: length mismatch")
	}
	return dotInt16(a, b)
}

// DotInt8 returns the sum of a[i]*b[i]. It panics if the lengths differ.
func DotInt8(a, b []int8) int64 {
	if len(a) != len(b) {
		panic("dsp: length mismatch")
	}
	return dotInt8(a, b)
}

func dotInt16Go(a, b []int16) int64 {
	var sum int64
	for i := range a {
		sum += int64(a[i]) * int64(b[i])
	}
	return sum
}

func dotInt8Go(a, b []int8) int64 {
	var sum int64
	for i := range a {
		sum += int64(a[i]) * int64(b[i])
	}
	return sum
}

// DotStrideFloat32 returns the sum of a[i*aStep]*b[i*bStep] for i below n, as
// esp-dsp dsps_dotprode_f32.
func DotStrideFloat32(a []float32, aStep int, b []float32, bStep int, n int) float32 {
	if n < 0 || aStep < 1 || bStep < 1 || (n > 0 && ((n-1)*aStep >= len(a) || (n-1)*bStep >= len(b))) {
		panic("dsp: length mismatch")
	}
	var sum float32
	for i := 0; i < n; i++ {
		sum += a[i*aStep] * b[i*bStep]
	}
	return sum
}

// DotFloat32 returns the sum of a[i]*b[i].
func DotFloat32(a, b []float32) float32 {
	checkLen2(len(a), len(b))
	return dotFloat32(a, b)
}

func dotFloat32Go(a, b []float32) float32 {
	var sum float32
	for i := range a {
		sum += a[i] * b[i]
	}
	return sum
}

// DotUint16 returns the sum of a[i]*b[i].
func DotUint16(a, b []uint16) uint64 {
	checkLen2(len(a), len(b))
	return dotUint16(a, b)
}

// DotUint8 returns the sum of a[i]*b[i].
func DotUint8(a, b []uint8) uint64 {
	checkLen2(len(a), len(b))
	return dotUint8(a, b)
}

func dotUint16Go(a, b []uint16) uint64 {
	var sum uint64
	for i := range a {
		sum += uint64(a[i]) * uint64(b[i])
	}
	return sum
}

func dotUint8Go(a, b []uint8) uint64 {
	var sum uint64
	for i := range a {
		sum += uint64(a[i]) * uint64(b[i])
	}
	return sum
}

// DotInt16Q15 is esp-dsp dsps_dotprod_s16. It returns the dot product, rounded
// and shifted right by 15-shift, truncated to int16.
func DotInt16Q15(a, b []int16, shift int) int16 {
	checkLen2(len(a), len(b))
	if shift < 0 || shift > 15 {
		panic("dsp: shift out of range")
	}
	acc := int64(0x7fff>>uint(shift)) + dotInt16(a, b)
	return int16(acc >> uint(15-shift))
}
