package nn

import (
	"hash/fnv"
	"testing"
)

// The golden hashes are from the esp-nn ansi C code with the same inputs.
var golden = map[string]uint32{
	"conv":            0xa742e87d,
	"depthwise":       0xd781818a,
	"fully connected": 0xe06e19fd,
	"elementwise":     0xc81c6d6e,
	"activation":      0x945aa9cc,
	"pool and mean":   0x12f04373,
}

type lcg uint32

func (r *lcg) next() uint32 {
	*r = *r*1664525 + 1013904223
	return uint32(*r)
}

func (r *lcg) rng(lo, hi int) int { return lo + int(r.next()>>8)%(hi-lo+1) }

func (r *lcg) int8s(n int) []int8 {
	s := AlignedInt8(n)
	for i := range s {
		s[i] = int8(r.next() >> 24)
	}
	return s
}

func (r *lcg) int32s(n, lo, hi int) []int32 {
	s := make([]int32, n)
	for i := range s {
		s[i] = int32(r.rng(lo, hi))
	}
	return s
}

func (r *lcg) mult() int32 { return int32(1<<30 + r.next()>>2) }

func (r *lcg) act() Activation {
	a, b := int32(r.rng(-128, 127)), int32(r.rng(-128, 127))
	return Activation{Min: min(a, b), Max: max(a, b)}
}

func (r *lcg) quant(n int) Quant {
	q := Quant{Shift: r.int32s(n, -10, 1), Mult: make([]int32, n)}
	for i := range q.Mult {
		q.Mult[i] = r.mult()
	}
	return q
}

type goldenCase struct {
	name string
	run  func(r *lcg) []int8
}

var goldenCases = []goldenCase{
	{"conv", func(r *lcg) []int8 {
		var out []int8
		for _, c := range [][8]int{{7, 6, 16, 3, 3, 16, 1, 1}, {9, 9, 3, 3, 3, 8, 2, 1}, {6, 5, 8, 1, 1, 12, 1, 0},
			{8, 8, 32, 3, 3, 24, 1, 1}, {10, 7, 12, 5, 3, 7, 2, 2}} {
			inD := Dims{c[0], c[1], c[2]}
			fD := Dims{c[3], c[4], 0}
			p := ConvParams{InOffset: int32(r.rng(-127, 128)), OutOffset: int32(r.rng(-128, 127)),
				Stride: Size2D{c[6], c[6]}, Padding: Size2D{c[7], c[7]}, Activation: r.act()}
			oD := Dims{(c[0]+2*c[7]-c[3])/c[6] + 1, (c[1]+2*c[7]-c[4])/c[6] + 1, c[5]}
			in, f := r.int8s(c[0]*c[1]*c[2]), r.int8s(c[5]*c[3]*c[4]*c[2])
			b, q := r.int32s(c[5], -1<<16, 1<<16), r.quant(c[5])
			o := AlignedInt8(oD.Width * oD.Height * c[5])
			ConvInt8(inD, in, fD, f, b, oD, o, &p, q, make([]byte, ConvScratchSize(inD, fD, oD, &p)))
			out = append(out, o...)
		}
		return out
	}},
	{"depthwise", func(r *lcg) []int8 {
		var out []int8
		for _, c := range [][8]int{{8, 8, 16, 3, 3, 1, 1, 1}, {9, 7, 5, 3, 3, 1, 2, 1}, {6, 6, 3, 3, 3, 8, 1, 1},
			{1, 12, 16, 1, 5, 1, 1, 0}, {7, 5, 4, 5, 3, 4, 1, 2}} {
			inD := Dims{c[0], c[1], c[2]}
			fD := Dims{c[3], c[4], 0}
			p := DepthwiseConvParams{InOffset: int32(r.rng(-127, 128)), OutOffset: int32(r.rng(-128, 127)), ChMult: c[5],
				Stride: Size2D{c[6], c[6]}, Padding: Size2D{min(c[7], c[3]/2), min(c[7], c[4]/2)}, Activation: r.act()}
			outCh := c[2] * c[5]
			oD := Dims{(c[0]+2*p.Padding.Width-c[3])/c[6] + 1, (c[1]+2*p.Padding.Height-c[4])/c[6] + 1, outCh}
			in, f := r.int8s(c[0]*c[1]*c[2]), r.int8s(c[3]*c[4]*outCh)
			b, q := r.int32s(outCh, -1<<16, 1<<16), r.quant(outCh)
			o := AlignedInt8(oD.Width * oD.Height * outCh)
			DepthwiseConvInt8(inD, in, fD, f, b, oD, o, &p, q, make([]byte, DepthwiseConvScratchSize(inD, fD, oD, &p)))
			out = append(out, o...)
		}
		return out
	}},
	{"fully connected", func(r *lcg) []int8 {
		var out []int8
		for _, rowLen := range []int{5, 64, 200} {
			outCh := 9
			in, f := r.int8s(rowLen*2), r.int8s(rowLen*outCh)
			b := r.int32s(outCh, -1<<16, 1<<16)
			p := FullyConnectedParams{InOffset: int32(r.rng(-127, 128)), FilterOffset: int32(r.rng(-3, 3)),
				OutOffset: int32(r.rng(-128, 127)), Activation: r.act()}
			o := AlignedInt8(outCh)
			FullyConnectedInt8(o, in[:rowLen], f, b, &p, int32(r.rng(-10, 1)), r.mult())
			out = append(out, o...)
			q := r.quant(outCh)
			o = AlignedInt8(outCh * 2)
			FullyConnectedPerChannelBatchInt8(o, in, f, b, &p, q, 2)
			out = append(out, o...)
		}
		return out
	}},
	{"elementwise", func(r *lcg) []int8 {
		n := 300
		a, b := r.int8s(n), r.int8s(n)
		out := AlignedInt8(4 * n)
		ap := AddParams{In1Offset: int32(r.rng(-127, 128)), In2Offset: int32(r.rng(-127, 128)), In1Mult: r.mult(),
			In2Mult: r.mult(), In1Shift: int32(r.rng(-8, 0)), In2Shift: int32(r.rng(-8, 0)), LeftShift: 20,
			OutOffset: int32(r.rng(-128, 127)), OutMult: r.mult(), OutShift: int32(r.rng(-25, -15)), Activation: r.act()}
		AddInt8(out[:n], a, b, &ap)
		mp := MulParams{In1Offset: int32(r.rng(-127, 128)), In2Offset: int32(r.rng(-127, 128)),
			OutOffset: int32(r.rng(-128, 127)), OutMult: r.mult(), OutShift: int32(r.rng(-16, -6)), Activation: r.act()}
		MulInt8(out[n:2*n], a, b, &mp)
		MulBroadcastChannelInt8(out[2*n:3*n], a, b[:12], &mp)
		copy(out[3*n:], a)
		Relu6Int8(out[3*n:])
		return out
	}},
	{"activation", func(r *lcg) []int8 {
		in := r.int8s(256)
		out := AlignedInt8(3 * 256)
		hs := HardSwishParams{InZeroPoint: int16(r.rng(-128, 127)), OutMultFxp: int16(r.rng(-32768, 32767)),
			ReluishMultFxp: int16(r.rng(-32768, 32767)), ReluishMultExp: int32(r.rng(-5, 5)),
			OutMultExp: int32(r.rng(-10, 0)), OutZeroPoint: int16(r.rng(-128, 127))}
		HardSwishInt8(out[:256], in, &hs, make([]byte, HardSwishScratchSize()))
		SoftmaxInt8(out[256:512], in, 64, r.mult(), int32(r.rng(0, 8)), int32(r.rng(-250, -20)),
			make([]byte, SoftmaxScratchSize(64, 4)))
		var lut [256]int8
		LogisticTable(&lut, int32(r.rng(-128, 127)), 0.05)
		LogisticInt8(out[512:], in, &lut)
		return out
	}},
	{"pool and mean", func(r *lcg) []int8 {
		inD := Dims{9, 7, 8}
		in := r.int8s(9 * 7 * 8)
		p := PoolParams{Filter: Size2D{3, 2}, Stride: Size2D{2, 1}, Padding: Size2D{1, 0}, Activation: r.act()}
		oD := Dims{Width: 5, Height: 6}
		out := AlignedInt8(2*5*6*8 + 8)
		MaxPoolInt8(oD, out[:240], inD, in, &p)
		AvgPoolInt8(oD, out[240:480], inD, in, &p)
		MeanInt8(out[480:], in, 7, 9, int32(r.rng(-128, 127)), int32(r.rng(-128, 127)), r.mult(), int32(r.rng(-8, 0)),
			make([]byte, MeanScratchSize(7, 9, 8)))
		return out
	}},
}

func TestGolden(t *testing.T) {
	r := lcg(1)
	for _, c := range goldenCases {
		h := fnv.New32a()
		for _, v := range c.run(&r) {
			h.Write([]byte{byte(v)})
		}
		if got := h.Sum32(); got != golden[c.name] {
			t.Errorf("%s: hash %#08x, want %#08x", c.name, got, golden[c.name])
		}
	}
}

func TestQuantization(t *testing.T) {
	// The values are from esp_nn_multiply_by_quantized_mult.
	for _, c := range []struct{ x, mult, shift, want int32 }{
		{100, 1073741824, 0, 50},
		{-100, 1073741824, 0, -50},
		{3, 1073741824, 0, 2},
		{-3, 1073741824, 0, -1},
		{1000, 1073741824, -3, 63},
		{1000, 1073741824, 2, 2000},
		{-2147483648, -2147483648, 0, 2147483647},
		{123456, 1518500250, -5, 2728},
		{-99999, 2100000000, 1, -195576},
	} {
		if got := MultiplyByQuantizedMult(c.x, c.mult, c.shift); got != c.want {
			t.Errorf("MultiplyByQuantizedMult(%d, %d, %d) = %d, want %d", c.x, c.mult, c.shift, got, c.want)
		}
	}
}
