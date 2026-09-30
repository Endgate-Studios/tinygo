//go:build esp32s3

package nn

import "encoding/binary"

// This file is a port of the ESP32-S3 convolution code of esp-nn, without the
// dual core split.

// panelBytes is ESP_NN_S3_PANEL_BYTES for the default 32 KB data cache.
const panelBytes = 32 * 1024 / 2

func convScratchSize(in, f, out *Dims, p *ConvParams) int {
	fd := *f
	if fd.Channels == 0 {
		fd.Channels = in.Channels
	}
	return convScratchNeed(in, &fd, out, p) + 16
}

// convScratchNeed is esp_nn_get_conv_scratch_size_esp32s3.
func convScratchNeed(in, f, out *Dims, p *ConvParams) int {
	fch := f.Channels
	if fch != 0 && fch < in.Channels && in.Channels%fch == 0 {
		groups := in.Channels / fch
		if groups > 1 && out.Channels%groups == 0 {
			inG, outG := *in, *out
			inG.Channels, outG.Channels = fch, out.Channels/groups
			inner := convScratchNeed(&inG, f, &outG, p)
			return inner + in.Width*in.Height*fch + out.Width*out.Height*outG.Channels + 64
		}
	}
	inW, inH, inCh := in.Width, in.Height, in.Channels
	fW, fH, outCh := f.Width, f.Height, out.Channels
	padW, padH := p.Padding.Width, p.Padding.Height
	strideW, strideH := p.Stride.Width, p.Stride.Height
	if conv3x3CanUse(fW, fH, inCh, outCh) {
		return conv3x3ScratchSize(inCh, outCh, out.Width, out.Height)
	}
	const alignBuf = 64
	if fW == 1 && fH == 1 && padW == 0 && padH == 0 && strideW == 1 && strideH == 1 {
		transpose := 2 * 8 * ((inCh + 7) &^ 7)
		if inW*inH < 8 {
			transpose = 0
		}
		panel := 24*inCh + 16*1024 + alignBuf
		if inCh*outCh <= 24*1024 {
			panel = 0
		}
		return max(transpose+alignBuf, 16*inCh+8*inCh+8*outCh+alignBuf, panel)
	}
	rowSize := fW * inCh
	windowLen := fW * fH * inCh
	if rowSize < 16 && windowLen >= 16 {
		wla := (windowLen + 15) &^ 15
		return outCh*4 + 16 + outCh*wla + 16 + wla + alignBuf
	}
	padRight := max(0, (out.Width-1)*strideW+fW-padW-inW)
	padBottom := max(0, (out.Height-1)*strideH+fH-padH-inH)
	inputScratch := 0
	if padW != 0 || padH != 0 || padRight != 0 || padBottom != 0 {
		inputScratch = (inW + padW + padRight) * (inH + padH + padBottom) * inCh
	}
	filterScratch := max((rowSize+15)/16*16*fH*outCh, fW*fH*inCh*outCh)
	return inputScratch + filterScratch + alignBuf + outCh*4
}

func convInt8(in *Dims, input []int8, f *Dims, filter []int8, bias []int32, out *Dims, output []int8,
	p *ConvParams, q Quant, scratch []byte) {
	fd := *f
	if fd.Channels == 0 {
		fd.Channels = in.Channels
	}
	total := convScratchNeed(in, &fd, out, p)
	if len(output) == 0 || len(scratch) < total+16 || (addr(input)|addr(filter)|addr(output))&15 != 0 {
		convInt8Go(in, input, &fd, filter, bias, out, output, p, q)
		return
	}
	if in.Channels != fd.Channels {
		groups := in.Channels / fd.Channels
		if groups > 1 && out.Channels%groups == 0 && in.Channels%fd.Channels == 0 &&
			convGroupedRepack(in, input, &fd, filter, bias, out, output, p, q, scratch, total) {
			return
		}
		convGoSplit(in, input, &fd, filter, bias, out, output, p, q)
		return
	}
	scratch = align16(scratch)
	ch, outCh := in.Channels, out.Channels
	fW, fH := fd.Width, fd.Height
	padW, padH := p.Padding.Width, p.Padding.Height
	strideW, strideH := p.Stride.Width, p.Stride.Height
	if fW == 1 && fH == 1 && padW == 0 && padH == 0 && strideW == 1 && strideH == 1 {
		if ch%8 == 0 {
			if ch*outCh > 24*1024 {
				spatial := in.Width * in.Height
				if outCh >= 32 && dualReady() &&
					conv1x1PanelDual(input, spatial, ch, filter, bias, output, outCh, p, q, scratch) {
					return
				}
				conv1x1Panel(input, spatial, ch, filter, bias, output, outCh, outCh, p, q, scratch)
				return
			}
			conv1x1BatchedTail(input, in.Width, in.Height, ch, filter, bias, output, outCh, p, q, scratch)
		} else {
			conv1x1(input, in.Width*in.Height, ch, filter, bias, output, outCh, p, q, scratch)
		}
		return
	}
	if conv3x3CanUse(fW, fH, ch, outCh) {
		conv3x3(in, input, filter, bias, out, output, p, q, scratch)
		return
	}
	if rowSize, windowLen := fW*ch, fW*fH*ch; rowSize < 16 && windowLen >= 16 {
		convIm2col(in, input, &fd, filter, bias, out, output, p, q, scratch)
		return
	}
	convFilterAligned(in, input, &fd, filter, bias, out, output, p, q, scratch)
}

// The dual core functions below have their own frames, because the variables
// of a job closure go to the heap at function entry.

// conv1x1PanelDual runs the first half of the output channels of
// conv1x1Panel on the other core.
//
//go:noinline
func conv1x1PanelDual(input []int8, spatial, ch int, filter []int8, bias []int32, output []int8, outCh int,
	p *ConvParams, q Quant, scratch []byte) bool {
	oc0 := (outCh/2 + 7) &^ 7
	if oc0 >= outCh || !dualRun(24*ch+16*1024+64, func(s []byte) {
		conv1x1Panel(input, spatial, ch, filter, bias, output, oc0, outCh, p, q, s)
	}) {
		return false
	}
	conv1x1Panel(input, spatial, ch, filter[oc0*ch:], offsetBias(bias, oc0), output[oc0:], outCh-oc0, outCh, p,
		q.from(oc0), scratch)
	dualWait()
	return true
}

// convGoSplit is esp_nn_conv_s8_ansi_mt_split. The other core computes the
// top half of the output rows.
func convGoSplit(in *Dims, input []int8, f *Dims, filter []int8, bias []int32, out *Dims, output []int8,
	p *ConvParams, q Quant) {
	if out.Height >= 4 && out.Height/2*p.Stride.Height >= p.Padding.Height && dualReady() &&
		convGoSplitDual(in, input, f, filter, bias, out, output, p, q) {
		return
	}
	convInt8Go(in, input, f, filter, bias, out, output, p, q)
}

//go:noinline
func convGoSplitDual(in *Dims, input []int8, f *Dims, filter []int8, bias []int32, out *Dims, output []int8,
	p *ConvParams, q Quant) bool {
	h0 := out.Height / 2
	rowOff := h0*p.Stride.Height - p.Padding.Height
	top := *out
	top.Height = h0
	if !dualRun(0, func([]byte) { convInt8Go(in, input, f, filter, bias, &top, output, p, q) }) {
		return false
	}
	in1, out1, p1 := *in, *out, *p
	in1.Height -= rowOff
	out1.Height -= h0
	p1.Padding.Height = 0
	convInt8Go(&in1, input[rowOff*in.Width*in.Channels:], f, filter, bias, &out1,
		output[h0*out.Width*out.Channels:], &p1, q)
	dualWait()
	return true
}

func offsetBias(bias []int32, n int) []int32 {
	if bias == nil {
		return nil
	}
	return bias[n:]
}

func (q Quant) from(n int) Quant {
	return Quant{Shift: q.Shift[n:], Mult: q.Mult[n:]}
}

// convGroupedRepack is esp_nn_conv_s8_grouped_repack.
func convGroupedRepack(in *Dims, input []int8, f *Dims, filter []int8, bias []int32, out *Dims, output []int8,
	p *ConvParams, q Quant, scratch []byte, total int) bool {
	fch, inCh, outCh := f.Channels, in.Channels, out.Channels
	groups := inCh / fch
	if f.Width*fch < 16 {
		return false
	}
	perGroup := outCh / groups
	pixelsIn, pixelsOut := in.Width*in.Height, out.Width*out.Height
	inSlice, outSlice := pixelsIn*fch, pixelsOut*perGroup
	off := total - inSlice - outSlice - 16
	off -= int((addr(scratch) + uintptr(off)) & 15)
	inBuf := int8s(scratch[off : off+inSlice])
	outBuf := int8s(scratch[off+inSlice : off+inSlice+outSlice])
	inG, outG := *in, *out
	inG.Channels, outG.Channels = fch, perGroup
	block := perGroup * fch * f.Width * f.Height
	for g := 0; g < groups; g++ {
		for i := 0; i < pixelsIn; i++ {
			copy(inBuf[i*fch:(i+1)*fch], input[i*inCh+g*fch:])
		}
		qG := Quant{Shift: q.Shift[g*perGroup:], Mult: q.Mult[g*perGroup:]}
		var bG []int32
		if bias != nil {
			bG = bias[g*perGroup:]
		}
		convInt8(&inG, inBuf, f, filter[g*block:], bG, &outG, outBuf, p, qG, scratch[:off])
		for i := 0; i < pixelsOut; i++ {
			copy(output[i*outCh+g*perGroup:i*outCh+(g+1)*perGroup], outBuf[i*perGroup:])
		}
	}
	return true
}

// conv1x1BatchedTail is esp_nn_conv_s8_mult8_1x1_batched_tail.
func conv1x1BatchedTail(input []int8, inW, inH, inCh int, filter []int8, bias []int32, output []int8, outCh int,
	p *ConvParams, q Quant, work []byte) {
	spatial := inW * inH
	tail := spatial & 7
	if tail < 2 {
		asmConvMult8x1(&input[0], int32(inW), int32(inH), int32(inCh), p.InOffset, &filter[0], ptr(bias), &output[0],
			int32(inW), int32(inH), int32(outCh), p.OutOffset, &q.Shift[0], &q.Mult[0], p.Min, p.Max, bytesAt(work, 0))
		return
	}
	tailIn := int8s(work[16*inCh : 24*inCh])
	tailOut := int8s(work[24*inCh : 24*inCh+8*outCh])
	vec := spatial - tail
	if vec != 0 {
		asmConvMult8x1(&input[0], int32(vec), 1, int32(inCh), p.InOffset, &filter[0], ptr(bias), &output[0],
			int32(vec), 1, int32(outCh), p.OutOffset, &q.Shift[0], &q.Mult[0], p.Min, p.Max, bytesAt(work, 0))
	}
	copy(tailIn, input[vec*inCh:spatial*inCh])
	fill(tailIn[tail*inCh:], int8(-p.InOffset))
	asmConvMult8x1(&tailIn[0], 8, 1, int32(inCh), p.InOffset, &filter[0], ptr(bias), &tailOut[0],
		8, 1, int32(outCh), p.OutOffset, &q.Shift[0], &q.Mult[0], p.Min, p.Max, bytesAt(work, 0))
	copy(output[vec*outCh:spatial*outCh], tailOut)
}

// conv1x1Panel is esp_nn_conv_s8_mult8_1x1_oc_panel.
func conv1x1Panel(input []int8, spatial, inCh int, filter []int8, bias []int32, output []int8, outCh, outStride int,
	p *ConvParams, q Quant, scratch []byte) {
	tile := panelBytes / inCh &^ 7
	tile = max(tile, 16)
	tile = min(tile, outCh)
	tile = max(tile, 8)
	tile = min(tile, 1024, outCh)
	work := align16(scratch)
	if tile == outCh && outStride == outCh {
		conv1x1BatchedTail(input, spatial, 1, inCh, filter, bias, output, outCh, p, q, work)
		return
	}
	staging := int8s(work[24*inCh+8*tile+32:])
	chunk := max(8*1024/tile&^7, 8)
	for oc := 0; oc < outCh; oc += tile {
		n := min(tile, outCh-oc)
		var b []int32
		if bias != nil {
			b = bias[oc:]
		}
		qo := Quant{Shift: q.Shift[oc:], Mult: q.Mult[oc:]}
		for pos := 0; pos < spatial; pos += chunk {
			pn := min(chunk, spatial-pos)
			conv1x1BatchedTail(input[pos*inCh:], pn, 1, inCh, filter[oc*inCh:], b, staging, n, p, qo, work)
			for i := 0; i < pn; i++ {
				copy(output[(pos+i)*outStride+oc:(pos+i)*outStride+oc+n], staging[i*n:])
			}
		}
	}
}

// conv1x1 is esp_nn_conv_s8_1x1, for channel counts that are not a multiple
// of 8.
func conv1x1(input []int8, size, inCh int, filter []int8, bias []int32, output []int8, outCh int,
	p *ConvParams, q Quant, scratch []byte) {
	ch8 := inCh / 8
	tbuf := int16s(scratch[:ch8*128])
	raw := scratch[ch8*128 : ch8*128+64]
	var acc [8]int32
	pos := 0
	for ; pos+7 < size; pos += 8 {
		in := input[pos*inCh:]
		for g := 0; g < ch8; g++ {
			for c := 0; c < 8; c++ {
				for k := 0; k < 8; k++ {
					tbuf[g*64+c*8+k] = int16(int32(in[k*inCh+g*8+c]) + p.InOffset)
				}
			}
		}
		for oc := 0; oc < outCh; oc++ {
			filt := filter[oc*inCh : (oc+1)*inCh]
			asmConv1x1Mac(ptr(tbuf), &filt[0], int32(ch8), (*int8)(bytesAt(raw, 0)))
			for k := 0; k < 4; k++ {
				acc[k] = int32(binary.LittleEndian.Uint32(raw[5*k:]))
				acc[4+k] = int32(binary.LittleEndian.Uint32(raw[32+5*k:]))
			}
			for c := ch8 * 8; c < inCh; c++ {
				f := int32(filt[c])
				for k := range acc {
					acc[k] += (int32(in[k*inCh+c]) + p.InOffset) * f
				}
			}
			for k, a := range acc {
				if bias != nil {
					a += bias[oc]
				}
				a = MultiplyByQuantizedMult(a, q.Mult[oc], q.Shift[oc]) + p.OutOffset
				output[(pos+k)*outCh+oc] = int8(p.clamp(a))
			}
		}
	}
	for ; pos < size; pos++ {
		in := input[pos*inCh : (pos+1)*inCh]
		for oc := 0; oc < outCh; oc++ {
			filt := filter[oc*inCh:]
			var a int32
			for c, v := range in {
				a += (int32(v) + p.InOffset) * int32(filt[c])
			}
			if bias != nil {
				a += bias[oc]
			}
			a = MultiplyByQuantizedMult(a, q.Mult[oc], q.Shift[oc]) + p.OutOffset
			output[pos*outCh+oc] = int8(p.clamp(a))
		}
	}
}

func conv3x3CanUse(fW, fH, inCh, outCh int) bool {
	return fW == 3 && fH == 3 && inCh >= 16 && inCh%16 == 0 && outCh >= 16
}

func conv3x3ScratchSize(inCh, outCh, outW, outH int) int {
	wla := (9*inCh + 15) &^ 15
	if outW*outH >= 16 {
		return 16*wla + 24*wla + 16*1024 + 64 + 32
	}
	return wla + outCh*wla + outCh*4 + 32
}

// buildWindow is esp_nn_3x3_build_window.
func buildWindow(dst, input []int8, inY, inX, inW, inH, inCh int, pad int8) {
	rowStride := inW * inCh
	for fy := 0; fy < 3; fy++ {
		y := inY + fy
		row := dst[:3*inCh]
		switch {
		case y < 0 || y >= inH:
			fill(row, pad)
		case inX >= 0 && inX+3 <= inW:
			copy(row, input[y*rowStride+inX*inCh:])
		default:
			for fx := 0; fx < 3; fx++ {
				x := inX + fx
				if x < 0 || x >= inW {
					fill(row[fx*inCh:(fx+1)*inCh], pad)
				} else {
					copy(row[fx*inCh:(fx+1)*inCh], input[y*rowStride+x*inCh:])
				}
			}
		}
		dst = dst[3*inCh:]
	}
}

// conv3x3 is esp_nn_conv_s8_3x3_opt.
func conv3x3(in *Dims, input, filter []int8, bias []int32, out *Dims, output []int8, p *ConvParams, q Quant, scratch []byte) {
	inCh, outCh := in.Channels, out.Channels
	windowLen := 9 * inCh
	wla := (windowLen + 15) &^ 15
	pad := int8(-p.InOffset)
	if total := out.Width * out.Height; total >= 16 {
		if total >= 32 && dualReady() && conv3x3StagedDual(in, input, filter, bias, out, output, p, q, scratch) {
			return
		}
		conv3x3Staged(in, input, filter, bias, out, output, p, q, 0, total, scratch)
		return
	}
	im2col := int8s(scratch[:wla])
	filterAligned := int8s(scratch[wla : wla+outCh*wla])
	corr := int32s(scratch[wla+outCh*wla : wla+outCh*wla+outCh*4])
	copyFilter := addr(filter)&15 != 0 || wla != windowLen
	filterBase := filter
	if copyFilter {
		filterBase = filterAligned
	}
	for oc := 0; oc < outCh; oc++ {
		f := filter[oc*windowLen : (oc+1)*windowLen]
		if copyFilter {
			slot := filterAligned[oc*wla : (oc+1)*wla]
			copy(slot, f)
			clear(slot[windowLen:])
		}
		var c int32
		if bias != nil {
			c = bias[oc]
		}
		if p.InOffset != 0 {
			c += filterSum(f) * p.InOffset
		}
		corr[oc] = c
	}
	clear(im2col[windowLen:])
	i := 0
	for y := 0; y < out.Height; y++ {
		for x := 0; x < out.Width; x++ {
			buildWindow(im2col, input, y*p.Stride.Height-p.Padding.Height, x*p.Stride.Width-p.Padding.Width,
				in.Width, in.Height, inCh, pad)
			for oc := 0; oc < outCh; oc++ {
				a := asmDotAligned(&im2col[0], &filterBase[oc*wla], int32(wla)) + corr[oc]
				a = MultiplyByQuantizedMult(a, q.Mult[oc], q.Shift[oc]) + p.OutOffset
				output[i] = int8(p.clamp(a))
				i++
			}
		}
	}
}

// conv3x3Staged is esp_nn_3x3_staged_range, for the output positions from
// begin to end.
func conv3x3Staged(in *Dims, input, filter []int8, bias []int32, out *Dims, output []int8, p *ConvParams, q Quant,
	begin, end int, scratch []byte) {
	inCh, outCh := in.Channels, out.Channels
	wla := (9*inCh + 15) &^ 15
	pad := int8(-p.InOffset)
	staging := int8s(scratch[:16*wla])
	panel := scratch[16*wla:]
	for pos0 := begin; pos0 < end; pos0 += 16 {
		n := min(16, end-pos0)
		for k := 0; k < n; k++ {
			y, x := (pos0+k)/out.Width, (pos0+k)%out.Width
			buildWindow(staging[k*wla:], input, y*p.Stride.Height-p.Padding.Height,
				x*p.Stride.Width-p.Padding.Width, in.Width, in.Height, inCh, pad)
		}
		conv1x1Panel(staging, n, wla, filter, bias, output[pos0*outCh:], outCh, outCh, p, q, panel)
	}
}

//go:noinline
func conv3x3StagedDual(in *Dims, input, filter []int8, bias []int32, out *Dims, output []int8, p *ConvParams,
	q Quant, scratch []byte) bool {
	wla := (9*in.Channels + 15) &^ 15
	total := out.Width * out.Height
	split := total / 2 / 16 * 16
	if !dualRun(16*wla+24*wla+16*1024+64, func(s []byte) {
		conv3x3Staged(in, input, filter, bias, out, output, p, q, 0, split, s)
	}) {
		return false
	}
	conv3x3Staged(in, input, filter, bias, out, output, p, q, split, total, scratch)
	dualWait()
	return true
}

// convIm2col is esp_nn_conv_s8_im2col_s3.
func convIm2col(in *Dims, input []int8, f *Dims, filter []int8, bias []int32, out *Dims, output []int8,
	p *ConvParams, q Quant, scratch []byte) {
	inW, inH, inCh := in.Width, in.Height, in.Channels
	fW, fH := f.Width, f.Height
	outW, outH, outCh := out.Width, out.Height, out.Channels
	padW, padH := p.Padding.Width, p.Padding.Height
	strideW, strideH := p.Stride.Width, p.Stride.Height
	windowLen := fW * fH * inCh
	wla := (windowLen + 15) &^ 15
	pad := int8(-p.InOffset)
	sums := int32s(scratch[:outCh*4])
	fOff := (outCh*4 + 15) &^ 15
	aligned := int8s(scratch[fOff : fOff+outCh*wla])
	cOff := fOff + outCh*wla
	buf := int8s(scratch[cOff : cOff+wla])
	for oc := 0; oc < outCh; oc++ {
		fw := filter[oc*windowLen : (oc+1)*windowLen]
		var s int32
		for _, v := range fw {
			s += int32(v)
		}
		sums[oc] = s * p.InOffset
		slot := aligned[oc*wla : (oc+1)*wla]
		copy(slot, fw)
		clear(slot[windowLen:])
	}
	clear(buf[windowLen:])
	rowBytes := fW * inCh
	safeY0 := min((padH+strideH-1)/strideH, outH)
	safeY1 := max(min((inH-fH+padH)/strideH+1, outH), safeY0)
	safeX0 := min((padW+strideW-1)/strideW, outW)
	safeX1 := max(min((inW-fW+padW)/strideW+1, outW), safeX0)
	i := 0
	for y := 0; y < outH; y++ {
		baseY := y*strideH - padH
		safeY := y >= safeY0 && y < safeY1
		for x := 0; x < outW; x++ {
			baseX := x*strideW - padW
			b := buf
			if safeY && x >= safeX0 && x < safeX1 {
				for fy := 0; fy < fH; fy++ {
					copy(b[:rowBytes], input[((baseY+fy)*inW+baseX)*inCh:])
					b = b[rowBytes:]
				}
			} else {
				for fy := 0; fy < fH; fy++ {
					iy := baseY + fy
					if iy >= 0 && iy < inH {
						for fx := 0; fx < fW; fx++ {
							ix := baseX + fx
							if ix >= 0 && ix < inW {
								copy(b[:inCh], input[(iy*inW+ix)*inCh:])
							} else {
								fill(b[:inCh], pad)
							}
							b = b[inCh:]
						}
					} else {
						fill(b[:rowBytes], pad)
						b = b[rowBytes:]
					}
				}
			}
			for oc := 0; oc < outCh; oc++ {
				a := asmDotAligned(&buf[0], &aligned[oc*wla], int32(wla)) + sums[oc]
				if bias != nil {
					a += bias[oc]
				}
				a = MultiplyByQuantizedMult(a, q.Mult[oc], q.Shift[oc]) + p.OutOffset
				output[i] = int8(p.clamp(a))
				i++
			}
		}
	}
}

// padAsymmetric is esp_nn_aligned_s8_pad_asymmetric.
func padAsymmetric(src, dst []int8, inW, inH, ch int, pad int8, left, top, right, bottom int) {
	row := (left + inW + right) * ch
	fill(dst[:row*top], pad)
	dst = dst[row*top:]
	for i := 0; i < inH; i++ {
		fill(dst[:left*ch], pad)
		dst = dst[left*ch:]
		copy(dst[:inW*ch], src[i*inW*ch:])
		dst = dst[inW*ch:]
		fill(dst[:right*ch], pad)
		dst = dst[right*ch:]
	}
	fill(dst[:row*bottom], pad)
}

// convFilterAligned is the general path of esp_nn_conv_s8_esp32s3.
func convFilterAligned(in *Dims, input []int8, f *Dims, filter []int8, bias []int32, out *Dims, output []int8,
	p *ConvParams, q Quant, scratch []byte) {
	inW, inH, ch := in.Width, in.Height, in.Channels
	fW, fH := f.Width, f.Height
	outW, outH, outCh := out.Width, out.Height, out.Channels
	padW, padH := p.Padding.Width, p.Padding.Height
	strideW, strideH := p.Stride.Width, p.Stride.Height
	rowSize := fW * ch
	filterSize := fW * fH * ch * outCh
	alignPad := 16 - rowSize&15
	filterAligned := filter
	inputPadded := input
	off := 0
	newW, newH := inW, inH
	if alignPad != 16 {
		newRow := rowSize + alignPad
		dst := int8s(scratch[:newRow*fH*outCh])
		for r := 0; r < fH*outCh; r++ {
			copy(dst[r*newRow:], filter[r*rowSize:(r+1)*rowSize])
			clear(dst[r*newRow+rowSize : (r+1)*newRow])
		}
		filterAligned = dst
		off = newRow * fH * outCh
	} else if addr(filter)&15 != 0 {
		filterAligned = int8s(scratch[:filterSize])
		copy(filterAligned, filter)
		off = filterSize
	}
	padRight := max(0, (outW-1)*strideW+fW-padW-inW)
	padBottom := max(0, (outH-1)*strideH+fH-padH-inH)
	if padW != 0 || padH != 0 || padRight > 0 || padBottom > 0 {
		newW, newH = inW+padW+padRight, inH+padH+padBottom
		inputPadded = int8s(scratch[off : off+newW*newH*ch])
		padAsymmetric(input, inputPadded, inW, inH, ch, int8(-p.InOffset), padW, padH, padRight, padBottom)
		off += newW * newH * ch
	}
	off = (off + 3) &^ 3
	rest := bytesAt(scratch, off)
	if p.InOffset != 0 && filterSize > 16384 {
		corr := int32s(scratch[off : off+outCh*4])
		n := fW * fH * ch
		for oc := 0; oc < outCh; oc++ {
			corr[oc] = filterSum(filter[oc*n:(oc+1)*n]) * p.InOffset
			if bias != nil {
				corr[oc] += bias[oc]
			}
		}
		asmConvPadded(&inputPadded[0], int32(newW), int32(newH), int32(ch), 0, int32(strideW), int32(strideH),
			&filterAligned[0], int32(fW), int32(fH), &corr[0], &output[0], int32(outW), int32(outH), int32(outCh),
			p.OutOffset, &q.Shift[0], &q.Mult[0], p.Min, p.Max, rest)
		return
	}
	asmConvPadded(&inputPadded[0], int32(newW), int32(newH), int32(ch), p.InOffset, int32(strideW), int32(strideH),
		&filterAligned[0], int32(fW), int32(fH), ptr(bias), &output[0], int32(outW), int32(outH), int32(outCh),
		p.OutOffset, &q.Shift[0], &q.Mult[0], p.Min, p.Max, rest)
}
