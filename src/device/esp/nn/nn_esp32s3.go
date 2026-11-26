//go:build esp32s3

package nn

import "unsafe"

// This file is a port of the ESP32-S3 dispatch code of esp-nn. The kernels
// are in src/device/esp/esp32s3_nn.S, see targets/esp32s3.json.

//export tinygo_nn_relu6_s8
func asmRelu6(data *int8, size int32)

//export tinygo_nn_add_elementwise_s8
func asmAdd(in1, in2 *int8, in1Offset, in2Offset, in1Mult, in2Mult, in1Shift, in2Shift, leftShift int32,
	out *int8, outOffset, outMult, outShift, actMin, actMax, size int32)

//export tinygo_nn_mul_elementwise_s8
func asmMul(in1, in2 *int8, in1Offset, in2Offset int32, out *int8, outOffset, outMult, outShift, actMin, actMax, size int32)

//export tinygo_nn_mul_broadcast_channel_s8
func asmMulBroadcast(in, perChannel *int8, in1Offset, in2Offset int32, out *int8, outOffset, outMult, outShift,
	actMin, actMax, spatial, channels int32)

//export tinygo_nn_max_pool_s8
func asmMaxPool(in *int8, inW, inH int32, out *int8, outW, outH, strideW, strideH, filterW, filterH, padW, padH,
	actMin, actMax, channels int32)

//export tinygo_nn_avg_pool_s8
func asmAvgPool(in *int8, inW, inH int32, out *int8, outW, outH, strideW, strideH, filterW, filterH, padW, padH,
	actMin, actMax, channels int32)

//export tinygo_nn_fc_s16
func asmFCS16(in *int8, inOffset, rowLen int32, filter *int8, filterOffset int32, bias *int32, out *int8,
	outChannels, outOffset, outShift, outMult, actMin, actMax int32)

//export tinygo_nn_fc_per_ch_s16
func asmFCPerChannelS16(in *int8, inOffset, rowLen int32, filter *int8, filterOffset int32, bias *int32, out *int8,
	outChannels, outOffset int32, outShift, outMult *int32, actMin, actMax int32)

//export tinygo_nn_dot_s8_aligned
func asmDotAligned(a, b *int8, n int32) int32

//export tinygo_nn_dot_s8_unaligned
func asmDotUnaligned(a, b *int8, n16 int32) int32

//export tinygo_nn_conv_s8_mult8_1x1
func asmConvMult8x1(in *int8, inW, inH, inCh, inOffset int32, filter *int8, bias *int32, out *int8,
	outW, outH, outCh, outOffset int32, outShift, outMult *int32, actMin, actMax int32, buf unsafe.Pointer)

//export tinygo_nn_conv_s8_filter_aligned_input_padded
func asmConvPadded(in *int8, inW, inH, inCh, inOffset, strideW, strideH int32, filter *int8, filterW, filterH int32,
	bias *int32, out *int8, outW, outH, outCh, outOffset int32, outShift, outMult *int32, actMin, actMax int32,
	scratch unsafe.Pointer)

//export tinygo_nn_depthwise_conv_s16_mult8_3x3
func asmDwS16Mult8x3(in *int16, inW, inH, ch, padW, padH, strideW, strideH, chMult int32, filter *int16,
	bias *int32, out *int8, outW, outH, outOffset int32, outShift, outMult *int32, actMin, actMax int32)

//export tinygo_nn_depthwise_conv_s8_mult1_3x3_padded
func asmDwS8Mult1x3Padded(in *int8, inW, inH, ch, inOffset, strideW, strideH int32, filter *int8, bias *int32,
	out *int8, outW, outH, outOffset int32, outShift, outMult *int32, actMin, actMax int32)

//export tinygo_nn_depthwise_conv_s16_mult8
func asmDwS16Mult8(in *int16, inW, inH, ch, padW, padH, strideW, strideH, chMult int32, filter *int16,
	filterW, filterH int32, bias *int32, out *int8, outW, outH, outOffset int32, outShift, outMult *int32,
	actMin, actMax int32)

//export tinygo_nn_depthwise_conv_s16_mult4
func asmDwS16Mult4(in *int16, inW, inH, ch, padW, padH, strideW, strideH, chMult int32, filter *int16,
	filterW, filterH int32, bias *int32, out *int8, outW, outH, outOffset int32, outShift, outMult *int32,
	actMin, actMax int32)

//export tinygo_nn_depthwise_conv_s16_mult1_3x3
func asmDwS16Mult1x3(in *int16, inW, inH, ch, padW, padH, strideW, strideH int32, filter *int16, bias *int32,
	out *int8, outW, outH, outOffset int32, outShift, outMult *int32, actMin, actMax int32)

//export tinygo_nn_depthwise_conv_s16_mult1
func asmDwS16Mult1(in *int16, inW, inH, ch, padW, padH, strideW, strideH int32, filter *int16,
	filterW, filterH int32, bias *int32, out *int8, outW, outH, outOffset int32, outShift, outMult *int32,
	actMin, actMax int32)

// dwKx1Params is dw_kx1_params_t of esp-nn.
type dwKx1Params struct {
	out       *int32
	outMult   *int32
	outShift  *int32
	outOffset int32
	actMin    int32
	actMax    int32
}

//export tinygo_nn_dw_s8_kx1
func asmDwKx1(in *int8, tapStride, taps int32, filter *int8, bias *int32, p *dwKx1Params)

//export tinygo_nn_s8_to_s16
func asmS8ToS16(src *int8, dst *int16, size int32)

//export tinygo_nn_aligned_s8_to_s16_with_offset
func asmS8ToS16Offset(src *int8, dst *int16, size, offset int32)

//export tinygo_nn_s8_to_s16_with_offset_row
func asmS8ToS16OffsetRow(src *int8, dst *int16, size, offset int32)

//export tinygo_nn_sum_blocks16_s8
func asmSumBlocks16(p *int8, blocks int32) int32

//export tinygo_nn_max_s8_blocks
func asmMaxBlocks(p *int8, n int32, out *int8)

//export tinygo_nn_conv_s8_1x1_mac
func asmConv1x1Mac(buf *int16, filter *int8, groups int32, qacc *int8)

func addr[T any](s []T) uintptr {
	return uintptr(unsafe.Pointer(unsafe.SliceData(s)))
}

func ptr[T any](s []T) *T {
	if len(s) == 0 {
		return nil
	}
	return &s[0]
}

// align16 returns b from the first 16 byte boundary.
func align16(b []byte) []byte {
	return b[int(-addr(b)&15):]
}

func bytesAt(b []byte, off int) unsafe.Pointer {
	return unsafe.Pointer(unsafe.SliceData(b[off:]))
}

func int8s(b []byte) []int8 {
	return unsafe.Slice((*int8)(unsafe.Pointer(unsafe.SliceData(b))), len(b))
}

func int16s(b []byte) []int16 {
	return unsafe.Slice((*int16)(unsafe.Pointer(unsafe.SliceData(b))), len(b)/2)
}

func int32s(b []byte) []int32 {
	return unsafe.Slice((*int32)(unsafe.Pointer(unsafe.SliceData(b))), len(b)/4)
}

func fill(s []int8, v int8) {
	for i := range s {
		s[i] = v
	}
}

func relu6Int8(data []int8) {
	for len(data) > 0 {
		n := min(len(data), 0xfff0)
		asmRelu6(&data[0], int32(n))
		data = data[n:]
	}
}

func hardSwishScratchSize() int { return 512 }

func hardSwishInt8(out, in []int8, p *HardSwishParams, scratch []byte) {
	if len(scratch) < 512 {
		hardSwishInt8Go(out, in, p)
		return
	}
	lutIn, lut := int8s(scratch[:256]), int8s(scratch[256:512])
	for i := range lutIn {
		lutIn[i] = int8(i)
	}
	hardSwishInt8Go(lut, lutIn, p)
	for i, v := range in {
		out[i] = lut[uint8(v)]
	}
}

// vectorOutOK reports if the add and mul kernels can write out. Their vector
// path runs for 16 byte aligned inputs and uses 8 byte stores to out.
func vectorOutOK(out, in1, in2 []int8) bool {
	return (addr(in1)|addr(in2))&15 != 0 || addr(out)&7 == 0
}

func addInt8(out, in1, in2 []int8, p *AddParams) {
	if len(out) == 0 {
		return
	}
	if !vectorOutOK(out, in1, in2) {
		addInt8Go(out, in1, in2, p)
		return
	}
	if len(out) >= 8192 && dualReady() && addDual(out, in1, in2, p) {
		return
	}
	addAsm(out, in1, in2, p)
}

// addDual gives the first half to the other core. It has its own frame,
// because the variables of a job closure go to the heap at function entry.
//
//go:noinline
func addDual(out, in1, in2 []int8, p *AddParams) bool {
	half := len(out) / 2 &^ 15
	if !dualRun(0, func([]byte) { addAsm(out[:half], in1, in2, p) }) {
		return false
	}
	addAsm(out[half:], in1[half:], in2[half:], p)
	dualWait()
	return true
}

func addAsm(out, in1, in2 []int8, p *AddParams) {
	asmAdd(&in1[0], &in2[0], p.In1Offset, p.In2Offset, p.In1Mult, p.In2Mult, p.In1Shift, p.In2Shift,
		p.LeftShift, &out[0], p.OutOffset, p.OutMult, p.OutShift, p.Min, p.Max, int32(len(out)))
}

func mulInt8(out, in1, in2 []int8, p *MulParams) {
	if len(out) == 0 {
		return
	}
	if !vectorOutOK(out, in1, in2) {
		mulInt8Go(out, in1, in2, p)
		return
	}
	if len(out) >= 8192 && dualReady() && mulDual(out, in1, in2, p) {
		return
	}
	mulAsm(out, in1, in2, p)
}

// mulDual gives the first half to the other core. It has its own frame,
// because the variables of a job closure go to the heap at function entry.
//
//go:noinline
func mulDual(out, in1, in2 []int8, p *MulParams) bool {
	half := len(out) / 2 &^ 15
	if !dualRun(0, func([]byte) { mulAsm(out[:half], in1, in2, p) }) {
		return false
	}
	mulAsm(out[half:], in1[half:], in2[half:], p)
	dualWait()
	return true
}

func mulAsm(out, in1, in2 []int8, p *MulParams) {
	asmMul(&in1[0], &in2[0], p.In1Offset, p.In2Offset, &out[0], p.OutOffset, p.OutMult, p.OutShift,
		p.Min, p.Max, int32(len(out)))
}

func mulBroadcastChannelInt8(out, in, perChannel []int8, p *MulParams) {
	if len(out) == 0 {
		return
	}
	c := len(perChannel)
	asmMulBroadcast(&in[0], &perChannel[0], p.In1Offset, p.In2Offset, &out[0], p.OutOffset, p.OutMult, p.OutShift,
		p.Min, p.Max, int32(len(out)/c), int32(c))
}

// poolAsm reports if the pool kernels can run. They need a channel count that
// is a multiple of 4 and they load 8 or 4 channels at an aligned address.
func poolAsm(out, in []int8, ch int) bool {
	a := uintptr(4)
	if ch%8 == 0 {
		a = 8
	}
	return len(out) > 0 && ch%4 == 0 && (addr(in)|addr(out))%a == 0
}

func maxPoolInt8(outDims *Dims, out []int8, inDims *Dims, in []int8, p *PoolParams) {
	if !poolAsm(out, in, inDims.Channels) {
		maxPoolInt8Go(outDims, out, inDims, in, p)
		return
	}
	asmMaxPool(&in[0], int32(inDims.Width), int32(inDims.Height), &out[0], int32(outDims.Width), int32(outDims.Height),
		int32(p.Stride.Width), int32(p.Stride.Height), int32(p.Filter.Width), int32(p.Filter.Height),
		int32(p.Padding.Width), int32(p.Padding.Height), p.Min, p.Max, int32(inDims.Channels))
}

func avgPoolInt8(outDims *Dims, out []int8, inDims *Dims, in []int8, p *PoolParams) {
	ch := inDims.Channels
	if len(out) == 0 {
		return
	}
	if poolAsm(out, in, ch) {
		asmAvgPool(&in[0], int32(inDims.Width), int32(inDims.Height), &out[0], int32(outDims.Width),
			int32(outDims.Height), int32(p.Stride.Width), int32(p.Stride.Height), int32(p.Filter.Width),
			int32(p.Filter.Height), int32(p.Padding.Width), int32(p.Padding.Height), p.Min, p.Max, int32(ch))
		return
	}
	const chunk = 64
	var acc [chunk]int16
	inW, inH := inDims.Width, inDims.Height
	for outY := 0; outY < outDims.Height; outY++ {
		baseY := outY*p.Stride.Height - p.Padding.Height
		for outX := 0; outX < outDims.Width; outX++ {
			baseX := outX*p.Stride.Width - p.Padding.Width
			yStart, yEnd := max(0, -baseY), min(p.Filter.Height, inH-baseY)
			xStart, xEnd := max(0, -baseX), min(p.Filter.Width, inW-baseX)
			count := int32((yEnd - yStart) * (xEnd - xStart))
			half := count / 2
			outIdx := (outY*outDims.Width + outX) * ch
			for c0 := 0; c0 < ch; c0 += chunk {
				n := min(chunk, ch-c0)
				clear(acc[:n])
				for fy := yStart; fy < yEnd; fy++ {
					for fx := xStart; fx < xEnd; fx++ {
						row := in[((baseY+fy)*inW+baseX+fx)*ch+c0:]
						for c := 0; c < n; c++ {
							acc[c] += int16(row[c])
						}
					}
				}
				for c := 0; c < n; c++ {
					r := int32(acc[c])
					if r > 0 {
						r = (r + half) / count
					} else {
						r = (r - half) / count
					}
					out[outIdx+c0+c] = int8(p.clamp(r))
				}
			}
		}
	}
}

// filterSum is esp_nn_filter_sum_s8_esp32s3.
func filterSum(p []int8) int32 {
	var sum int32
	i := 0
	for i < len(p) && addr(p[i:])&15 != 0 {
		sum += int32(p[i])
		i++
	}
	if blocks := (len(p) - i) >> 4; blocks > 0 {
		sum += asmSumBlocks16(&p[i], int32(blocks))
		i += blocks << 4
	}
	for ; i < len(p); i++ {
		sum += int32(p[i])
	}
	return sum
}

// fcDotPathWins is fc_dot_path_wins of esp-nn.
func fcDotPathWins(rowLen int, inOffset int32) bool {
	tail := rowLen & 15
	if inOffset == 0 {
		return rowLen >= 64+tail*8
	}
	return rowLen >= 192+tail*16
}

func fcDot(in, row []int8, inAligned bool) int32 {
	n16 := len(in) >> 4
	var acc int32
	if n16 > 0 {
		if inAligned {
			acc = asmDotUnaligned(&in[0], &row[0], int32(n16))
		} else {
			acc = asmDotUnaligned(&row[0], &in[0], int32(n16))
		}
	}
	for i := n16 << 4; i < len(in); i++ {
		acc += int32(in[i]) * int32(row[i])
	}
	return acc
}

func fullyConnectedInt8(out, in, filter []int8, bias []int32, p *FullyConnectedParams, shift, mult int32) {
	rowLen := len(in)
	if len(out) == 0 || rowLen == 0 {
		fullyConnectedInt8Go(out, in, filter, bias, p, shift, mult)
		return
	}
	inAligned := addr(in)&15 == 0
	filterAligned := addr(filter)&15 == 0 && rowLen&15 == 0
	if p.FilterOffset != 0 || !fcDotPathWins(rowLen, p.InOffset) || (!inAligned && !filterAligned) {
		if addr(in)&7 != 0 {
			fullyConnectedInt8Go(out, in, filter, bias, p, shift, mult)
			return
		}
		asmFCS16(&in[0], p.InOffset, int32(rowLen), &filter[0], p.FilterOffset, ptr(bias), &out[0],
			int32(len(out)), p.OutOffset, shift, mult, p.Min, p.Max)
		return
	}
	for o := range out {
		row := filter[o*rowLen : (o+1)*rowLen]
		var corr int32
		if p.InOffset != 0 {
			corr = filterSum(row) * p.InOffset
		}
		if bias != nil {
			corr += bias[o]
		}
		acc := fcDot(in, row, inAligned) + corr
		acc = MultiplyByQuantizedMult(acc, mult, shift) + p.OutOffset
		out[o] = int8(p.clamp(acc))
	}
}

func fullyConnectedPerChannelInt8(out, in, filter []int8, bias []int32, p *FullyConnectedParams, q Quant) {
	rowLen := len(in)
	if len(out) == 0 || rowLen == 0 {
		fullyConnectedPerChannelInt8Go(out, in, filter, bias, p, q)
		return
	}
	inAligned := addr(in)&15 == 0
	filterAligned := addr(filter)&15 == 0 && rowLen&15 == 0
	if p.FilterOffset != 0 || !fcDotPathWins(rowLen, p.InOffset) || (!inAligned && !filterAligned) {
		if addr(in)&7 != 0 {
			fullyConnectedPerChannelInt8Go(out, in, filter, bias, p, q)
			return
		}
		asmFCPerChannelS16(&in[0], p.InOffset, int32(rowLen), &filter[0], p.FilterOffset, ptr(bias), &out[0],
			int32(len(out)), p.OutOffset, &q.Shift[0], &q.Mult[0], p.Min, p.Max)
		return
	}
	for o := range out {
		row := filter[o*rowLen : (o+1)*rowLen]
		var corr int32
		if p.InOffset != 0 {
			corr = filterSum(row) * p.InOffset
		}
		if bias != nil {
			corr += bias[o]
		}
		acc := fcDot(in, row, inAligned) + corr
		acc = MultiplyByQuantizedMult(acc, q.Mult[o], q.Shift[o]) + p.OutOffset
		out[o] = int8(p.clamp(acc))
	}
}

func fullyConnectedPerChannelBatchInt8(out, in, filter []int8, bias []int32, p *FullyConnectedParams, q Quant, batches int) {
	rowLen, outCh := len(in)/batches, len(out)/batches
	inAligned := addr(in)&15 == 0 && rowLen&15 == 0
	filterAligned := addr(filter)&15 == 0 && rowLen&15 == 0
	gate := p.InOffset
	if batches > 1 {
		gate = 0
	}
	if rowLen == 0 || outCh == 0 || p.FilterOffset != 0 || !fcDotPathWins(rowLen, gate) || (!inAligned && !filterAligned) {
		for b := 0; b < batches; b++ {
			fullyConnectedPerChannelInt8(out[b*outCh:(b+1)*outCh], in[b*rowLen:(b+1)*rowLen], filter, bias, p, q)
		}
		return
	}
	for o := 0; o < outCh; o++ {
		row := filter[o*rowLen : (o+1)*rowLen]
		var corr int32
		if p.InOffset != 0 {
			corr = filterSum(row) * p.InOffset
		}
		if bias != nil {
			corr += bias[o]
		}
		for b := 0; b < batches; b++ {
			acc := fcDot(in[b*rowLen:(b+1)*rowLen], row, inAligned) + corr
			acc = MultiplyByQuantizedMult(acc, q.Mult[o], q.Shift[o]) + p.OutOffset
			out[b*outCh+o] = int8(p.clamp(acc))
		}
	}
}

func softmaxScratchSize(width, height int) int { return 2*256*4 + 16 + 32 }

func softmaxInt8(out, in []int8, width int, mult, shift, diffMin int32, scratch []byte) {
	if len(scratch) < softmaxScratchSize(width, 0) {
		softmaxInt8Go(out, in, width, mult, shift, diffMin)
		return
	}
	base := align16(scratch)
	lut := int32s(base[:2*256*4])
	expLUT, sumLUT := lut[:256], lut[256:]
	tmp := int8s(base[2*256*4 : 2*256*4+16])
	mask := int32(1) << uint(shift)
	for d := int32(0); d < 256; d++ {
		if -d >= diffMin {
			expLUT[d] = expOnNegativeValues(SatRoundDoublingHighMul(-d*mask, mult))
			sumLUT[d] = DivByPowerOfTwo(expLUT[d], softmaxAccumBits)
		} else {
			expLUT[d], sumLUT[d] = 0, 0
		}
	}
	for row := 0; row+width <= len(in); row += width {
		x, y := in[row:row+width], out[row:row+width]
		m := maxInt8(x, tmp)
		var sum int32
		for _, v := range x {
			sum += sumLUT[int32(m)-int32(v)]
		}
		scale, bitsOver := softmaxScale(sum)
		for i, v := range x {
			if d := int32(v) - int32(m); d >= diffMin {
				y[i] = softmaxOut(scale, expLUT[-d], bitsOver)
			} else {
				y[i] = -128
			}
		}
	}
}

// maxInt8 is find_max_s8 of the esp-nn softmax.
func maxInt8(data, tmp []int8) int8 {
	m := int8(-128)
	i := 0
	if len(data) >= 32 && addr(data)&15 == 0 {
		n := len(data) &^ 15
		asmMaxBlocks(&data[0], int32(n), &tmp[0])
		for _, v := range tmp[:16] {
			m = max(m, v)
		}
		i = n
	}
	for _, v := range data[i:] {
		m = max(m, v)
	}
	return m
}

func meanScratchSize(height, width, channels int) int { return channels*4 + 16 }

func meanInt8(out, in []int8, inZeroPoint, outZeroPoint, mult, shift int32, scratch []byte) {
	ch := len(out)
	if len(scratch) < ch*4+16 {
		meanInt8Go(out, in, inZeroPoint, outZeroPoint, mult, shift)
		return
	}
	n := len(in) / ch
	corr := int32(n) * inZeroPoint
	scratch = align16(scratch)
	if n <= 256 {
		acc := int16s(scratch[:ch*2])
		clear(acc)
		for i := 0; i < len(in); i += ch {
			for c, v := range in[i : i+ch] {
				acc[c] += int16(v)
			}
		}
		for c := range out {
			v := MultiplyByQuantizedMult(int32(acc[c])-corr, mult, shift) + outZeroPoint
			out[c] = sat8(v)
		}
		return
	}
	sums := int32s(scratch[:ch*4])
	clear(sums)
	for i := 0; i < len(in); i += ch {
		for c, v := range in[i : i+ch] {
			sums[c] += int32(v)
		}
	}
	for c := range out {
		v := MultiplyByQuantizedMult(sums[c]-corr, mult, shift) + outZeroPoint
		out[c] = sat8(v)
	}
}
