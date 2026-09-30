package nn

// Relu6Int8 limits each value of data to the range 0 to 6, as esp-nn
// esp_nn_relu6_s8.
func Relu6Int8(data []int8) {
	relu6Int8(data)
}

func relu6Int8Go(data []int8) {
	for i, v := range data {
		if v < 0 {
			v = 0
		}
		if v > 6 {
			v = 6
		}
		data[i] = v
	}
}

// HardSwishParams are the quantization values of HardSwishInt8, as in the
// TensorFlow Lite HardSwishParams.
type HardSwishParams struct {
	InZeroPoint    int16
	OutMultFxp     int16
	ReluishMultFxp int16
	ReluishMultExp int32
	OutMultExp     int32
	OutZeroPoint   int16
}

// HardSwishScratchSize returns the scratch size in bytes for HardSwishInt8.
func HardSwishScratchSize() int {
	return hardSwishScratchSize()
}

// HardSwishInt8 sets out[i] to x*relu6(x+3)/6 for x=in[i], as esp-nn
// esp_nn_hard_swish_s8.
func HardSwishInt8(out, in []int8, p *HardSwishParams, scratch []byte) {
	checkLen(len(in), len(out))
	hardSwishInt8(out[:len(in)], in, p, scratch)
}

func hardSwishInt8Go(out, in []int8, p *HardSwishParams) {
	for i, x := range in {
		hires := (int16(x) - p.InZeroPoint) * 128
		onOut := satRoundDblHighMul16(hires, p.OutMultFxp)
		r := hires
		if p.ReluishMultExp > 0 {
			r = satLeftShift16(r, p.ReluishMultExp-1)
		}
		r = satRoundDblHighMul16(r, p.ReluishMultFxp)
		if p.ReluishMultExp > 0 {
			r = satLeftShift16(r, 1)
		}
		if p.ReluishMultExp < 0 {
			r = roundingDivPot16(r, -p.ReluishMultExp)
		}
		r = int16((int32(r) + 1<<15) >> 1)
		v := roundingDivPot16(satDblHighMul16(r, onOut), -p.OutMultExp) + p.OutZeroPoint
		out[i] = sat8(int32(v))
	}
}

func satLeftShift16(v int16, shift int32) int16 {
	r := int32(v) << uint(shift)
	if r > 32767 {
		return 32767
	}
	if r < -32768 {
		return -32768
	}
	return int16(r)
}

func satRoundDblHighMul16(a, b int16) int16 {
	if a == b && a == -32768 {
		return 32767
	}
	ab := int32(a) * int32(b)
	nudge := int32(1 << 14)
	if ab < 0 {
		nudge = 1 - nudge
	}
	return int16((ab + nudge) / (1 << 15))
}

func satDblHighMul16(a, b int16) int16 {
	if a == b && a == -32768 {
		return 32767
	}
	return int16(int32(a) * int32(b) / (1 << 15))
}

func roundingDivPot16(v int16, exp int32) int16 {
	mask := int16(uint16(1)<<uint(exp) - 1)
	threshold := mask >> 1
	if v < 0 {
		threshold++
	}
	r := v >> uint(exp)
	if v&mask > threshold {
		r++
	}
	return r
}
