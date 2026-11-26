//go:build esp32s3

package nn

// This file is a port of the ESP32-S3 depthwise convolution code of esp-nn,
// without the dual core split.

func dwKx1Case(in, f, out *Dims, p *DepthwiseConvParams) bool {
	return p.ChMult == 1 && in.Width == 1 && f.Width == 1 && out.Width == 1 && p.Stride.Width == 1 &&
		p.Padding.Width == 0 && in.Channels%16 == 0
}

func depthwiseScratchNeed(in, f, out *Dims, p *DepthwiseConvParams) int {
	inW, inH, ch := in.Width, in.Height, in.Channels
	fW, fH := f.Width, f.Height
	chMult := p.ChMult
	outW, outH := out.Width, out.Height
	padW, padH := p.Padding.Width, p.Padding.Height
	strideW, strideH := p.Stride.Width, p.Stride.Height
	if dwKx1Case(in, f, out, p) {
		// esp-nn uses a stack buffer for the 16 results, it is in the scratch here.
		return ch*4 + 2*fH*ch + 64 + 80
	}
	filterSize := fW * fH * ch * chMult
	switch {
	case chMult == 1 && ch%8 == 0:
		if fW == 3 && fH == 3 {
			if ch%16 == 0 {
				padWidth := padW + max(0, (outW-1)*strideW+fW-padW-inW)
				padHeight := padH + max(0, (outH-1)*strideH+fH-padH-inH)
				if padWidth == 0 && padHeight == 0 {
					return filterSize + 16
				}
				full := (inW + padWidth) * (inH + padHeight) * ch
				if full <= 40*1024 {
					return filterSize + full + 16
				}
				rowBytes := (inW + padWidth) * ch
				rows := (40*1024 - filterSize - 32) / rowBytes
				rows = min(max(rows, fH), inH+padHeight)
				return filterSize + rows*rowBytes + 32
			}
			if ch >= 12 {
				newCh := (ch + 15) &^ 15
				totalPadW := padW*2 + max(0, outW*strideW+2-inW)
				totalPadH := padH*2 + max(0, outH*strideH+2-inH)
				return 9*newCh + (inW+totalPadW)*(inH+totalPadH)*newCh + outW*outH*newCh + 3*newCh*4 + 64
			}
			return 2*(filterSize+inW*inH*ch) + 32
		}
		total := 2 * (filterSize + inW*inH*ch)
		if total <= 48*1024 {
			return total + 32
		}
		return 2*filterSize + 2*inW*fH*ch + 32
	case chMult == 1 && ch > 3:
		padded := (ch + 7) &^ 7
		inputStart := (fW*fH*padded*2 + 15) &^ 15
		outStart := (inputStart + inW*inH*padded*2 + 15) &^ 15
		biasStart := (outStart + outW*outH*padded + 15) &^ 15
		return biasStart + 3*padded*4 + 16
	case chMult%4 == 0:
		return 2*(filterSize+inW*inH*ch) + 32
	}
	return 32
}

func depthwiseConvScratchSize(in, f, out *Dims, p *DepthwiseConvParams) int {
	return depthwiseScratchNeed(in, f, out, p) + 16
}

func depthwiseConvInt8(in *Dims, input []int8, f *Dims, filter []int8, bias []int32, out *Dims, output []int8,
	p *DepthwiseConvParams, q Quant, scratch []byte) {
	need := depthwiseScratchNeed(in, f, out, p)
	if len(output) == 0 || len(scratch) < need+16 || (addr(input)|addr(filter)|addr(output))&15 != 0 {
		depthwiseConvInt8Go(in, input, f, filter, bias, out, output, p, q)
		return
	}
	scratch = align16(scratch)[:need]
	inW, inH, ch := in.Width, in.Height, in.Channels
	fW, fH := f.Width, f.Height
	outW, outH := out.Width, out.Height
	padW, padH := p.Padding.Width, p.Padding.Height
	strideW, strideH := p.Stride.Width, p.Stride.Height
	chMult := p.ChMult
	shift, mult := &q.Shift[0], &q.Mult[0]
	filterSize := fW * fH * ch * chMult
	alignLen := 16 - filterSize&15
	inputSize := inW * inH * ch
	// The int16 copies of the filter and the input, for the s16 kernels.
	views := func() (filter16, input16 []int16) {
		s16 := int16s(scratch)
		return s16[:filterSize], s16[filterSize+alignLen:]
	}

	if dwKx1Case(in, f, out, p) {
		dwKx1(input, inH, ch, filter, fH, bias, output, outH, p, q, scratch)
		return
	}
	switch {
	case chMult == 1 && ch%8 == 0:
		if fW == 3 && fH == 3 {
			if ch%16 == 0 {
				filterAligned := int8s(scratch[:filterSize])
				copy(filterAligned, filter)
				padded := int8s(scratch[filterSize+alignLen:])
				padRight := max(0, (outW-1)*strideW+fW-padW-inW)
				padBottom := max(0, (outH-1)*strideH+fH-padH-inH)
				fullW, fullH := inW+padW+padRight, inH+padH+padBottom
				if padW|padH|padRight|padBottom == 0 {
					dw3x3Split(input, inW, inH, ch, filterAligned, bias, output, outW, outH, p, q)
					return
				}
				if fullW*fullH*ch <= 40*1024 {
					padAsymmetric(input, padded, inW, inH, ch, int8(-p.InOffset), padW, padH, padRight, padBottom)
					dw3x3Split(padded, fullW, fullH, ch, filterAligned, bias, output, outW, outH, p, q)
					return
				}
				tile := padded[:need-filterSize-alignLen-16]
				if outH >= 4 && dualReady() && dw3x3StripsDual(input, inW, inH, ch, padW, padH, padRight,
					filterAligned, bias, output, outW, outH, p, q, tile) {
					return
				}
				dw3x3Strips(input, inW, inH, ch, padW, padH, padRight, filterAligned, bias, output, outW, 0, outH,
					p, q, tile)
				return
			}
			if ch >= 12 {
				dw3x3Wide(input, inW, inH, ch, filter, bias, output, outW, outH, p, q, scratch)
				return
			}
			filter16, input16 := views()
			asmS8ToS16(&filter[0], &filter16[0], int32(filterSize))
			s8ToS16Offset(input, input16[:inputSize], p.InOffset)
			asmDwS16Mult1x3(&input16[0], int32(inW), int32(inH), int32(ch), int32(padW), int32(padH), int32(strideW),
				int32(strideH), &filter16[0], ptr(bias), &output[0], int32(outW), int32(outH), p.OutOffset, shift, mult,
				p.Min, p.Max)
			return
		}
		filter16, input16 := views()
		asmS8ToS16(&filter[0], &filter16[0], int32(filterSize))
		if 2*(filterSize+inputSize) <= 48*1024 {
			s8ToS16Offset(input, input16[:inputSize], p.InOffset)
			asmDwS16Mult1(&input16[0], int32(inW), int32(inH), int32(ch), int32(padW), int32(padH), int32(strideW),
				int32(strideH), &filter16[0], int32(fW), int32(fH), ptr(bias), &output[0], int32(outW), int32(outH),
				p.OutOffset, shift, mult, p.Min, p.Max)
			return
		}
		rowLen := inW * ch
		for outRow := 0; outRow < outH; outRow++ {
			start := outRow*strideH - padH
			dst := input16
			for r := start; r < start+fH; r++ {
				row := dst[:rowLen]
				switch {
				case r < 0 || r >= inH:
					clear(row)
				case (addr(input[r*rowLen:])|addr(row))&15 == 0:
					asmS8ToS16Offset(&input[r*rowLen], &row[0], int32(rowLen), p.InOffset)
				case r > 0 && r+1 < inH && addr(row)&15 == 0:
					asmS8ToS16OffsetRow(&input[r*rowLen], &row[0], int32(rowLen), p.InOffset)
				default:
					for i, v := range input[r*rowLen : (r+1)*rowLen] {
						row[i] = int16(int32(v) + p.InOffset)
					}
				}
				dst = dst[rowLen:]
			}
			asmDwS16Mult1(&input16[0], int32(inW), int32(fH), int32(ch), int32(padW), 0, int32(strideW), 1,
				&filter16[0], int32(fW), int32(fH), ptr(bias), &output[outRow*outW*ch], int32(outW), 1, p.OutOffset,
				shift, mult, p.Min, p.Max)
		}
	case chMult == 1 && ch > 3:
		dwPadChannels(input, inW, inH, ch, filter, fW, fH, bias, output, outW, outH, p, q, scratch)
	case chMult%8 == 0:
		filter16, input16 := views()
		asmS8ToS16(&filter[0], &filter16[0], int32(filterSize))
		s8ToS16Offset(input, input16[:inputSize], p.InOffset)
		if fW == 3 && fH == 3 {
			if outH >= 8 && outH/2*strideH >= padH && dualReady() &&
				dwMult8x3Dual(input16, inW, inH, ch, filter16, bias, output, outW, outH, p, q) {
				return
			}
			dwMult8x3(input16, inW, inH, ch, padH, filter16, bias, output, outW, outH, p, q)
			return
		}
		asmDwS16Mult8(&input16[0], int32(inW), int32(inH), int32(ch), int32(padW), int32(padH), int32(strideW),
			int32(strideH), int32(chMult), &filter16[0], int32(fW), int32(fH), ptr(bias), &output[0], int32(outW),
			int32(outH), p.OutOffset, shift, mult, p.Min, p.Max)
	case chMult%4 == 0:
		filter16, input16 := views()
		asmS8ToS16(&filter[0], &filter16[0], int32(filterSize))
		s8ToS16Offset(input, input16[:inputSize], p.InOffset)
		asmDwS16Mult4(&input16[0], int32(inW), int32(inH), int32(ch), int32(padW), int32(padH), int32(strideW),
			int32(strideH), int32(chMult), &filter16[0], int32(fW), int32(fH), ptr(bias), &output[0], int32(outW),
			int32(outH), p.OutOffset, shift, mult, p.Min, p.Max)
	default:
		depthwiseConvInt8Go(in, input, f, filter, bias, out, output, p, q)
	}
}

// s8ToS16Offset is esp_nn_aligned_s8_to_s16_with_offset_esp32s3. The kernel
// needs a 16 byte aligned source, else the Go loop gives the same values.
func s8ToS16Offset(src []int8, dst []int16, offset int32) {
	if addr(src)&15 == 0 {
		asmS8ToS16Offset(&src[0], &dst[0], int32(len(dst)), offset)
		return
	}
	for i := range dst {
		dst[i] = int16(int32(src[i]) + offset)
	}
}

// dwKx1 is esp_nn_depthwise_conv_s8_mult1_kx1.
func dwKx1(input []int8, inH, ch int, filter []int8, fH int, bias []int32, output []int8, outH int,
	p *DepthwiseConvParams, q Quant, scratch []byte) {
	biasAdj := int32s(scratch[:ch*4])
	edge := int8s(scratch[ch*4 : ch*4+fH*ch])
	filtAligned := int8s(scratch[ch*4+fH*ch : ch*4+2*fH*ch])
	acc := int32s(align16(scratch[ch*4+2*fH*ch:])[:64])
	for c := 0; c < ch; c++ {
		var sum int32
		for k := 0; k < fH; k++ {
			sum += int32(filter[k*ch+c])
		}
		biasAdj[c] = p.InOffset * sum
		if bias != nil {
			biasAdj[c] += bias[c]
		}
	}
	if addr(filter)&15 != 0 {
		copy(filtAligned, filter[:fH*ch])
		filter = filtAligned
	}
	unaligned := addr(input)&15 != 0
	pad := int8(-p.InOffset)
	pp := dwKx1Params{out: &acc[0], outOffset: p.OutOffset, actMin: p.Min, actMax: p.Max}
	for y := 0; y < outH; y++ {
		baseY := y*p.Stride.Height - p.Padding.Height
		var rows []int8
		if unaligned || baseY < 0 || baseY+fH > inH {
			for k := 0; k < fH; k++ {
				iy := baseY + k
				if iy < 0 || iy >= inH {
					fill(edge[k*ch:(k+1)*ch], pad)
				} else {
					copy(edge[k*ch:(k+1)*ch], input[iy*ch:])
				}
			}
			rows = edge
		} else {
			rows = input[baseY*ch:]
		}
		for c := 0; c < ch; c += 16 {
			pp.outMult, pp.outShift = &q.Mult[c], &q.Shift[c]
			asmDwKx1(&rows[c], int32(ch), int32(fH), &filter[c], &biasAdj[c], &pp)
			for i := 0; i < 16; i++ {
				output[y*ch+c+i] = int8(acc[i])
			}
		}
	}
}

// dw3x3Split is dw3x3_run_split of esp-nn. The input has its padding.
func dw3x3Split(input []int8, inW, inH, ch int, filter []int8, bias []int32, output []int8, outW, outH int,
	p *DepthwiseConvParams, q Quant) {
	if outH >= 4 && dualReady() && dw3x3SplitDual(input, inW, inH, ch, filter, bias, output, outW, outH, p, q) {
		return
	}
	dw3x3Run(input, inW, inH, ch, filter, bias, output, outW, outH, p, q)
}

func dw3x3Run(input []int8, inW, inH, ch int, filter []int8, bias []int32, output []int8, outW, outH int,
	p *DepthwiseConvParams, q Quant) {
	asmDwS8Mult1x3Padded(&input[0], int32(inW), int32(inH), int32(ch), p.InOffset, int32(p.Stride.Width),
		int32(p.Stride.Height), &filter[0], ptr(bias), &output[0], int32(outW), int32(outH), p.OutOffset,
		&q.Shift[0], &q.Mult[0], p.Min, p.Max)
}

// The dual core functions below have their own frames, because the variables
// of a job closure go to the heap at function entry.

//go:noinline
func dw3x3SplitDual(input []int8, inW, inH, ch int, filter []int8, bias []int32, output []int8, outW, outH int,
	p *DepthwiseConvParams, q Quant) bool {
	sh := p.Stride.Height
	h0 := outH / 2
	if !dualRun(0, func([]byte) { dw3x3Run(input, inW, (h0-1)*sh+3, ch, filter, bias, output, outW, h0, p, q) }) {
		return false
	}
	dw3x3Run(input[h0*sh*inW*ch:], inW, inH-h0*sh, ch, filter, bias, output[h0*outW*ch:], outW, outH-h0, p, q)
	dualWait()
	return true
}

//go:noinline
func dw3x3StripsDual(input []int8, inW, inH, ch, padW, padH, padRight int, filter []int8, bias []int32,
	output []int8, outW, outH int, p *DepthwiseConvParams, q Quant, tile []int8) bool {
	want := 3*(inW+padW+padRight)*ch + 16
	if want < 24*1024 && len(tile) > want {
		want = min(len(tile), 24*1024)
	}
	if !dualRun(want, func(s []byte) {
		dw3x3Strips(input, inW, inH, ch, padW, padH, padRight, filter, bias, output, outW, 0, outH/2, p, q, int8s(s))
	}) {
		return false
	}
	dw3x3Strips(input, inW, inH, ch, padW, padH, padRight, filter, bias, output, outW, outH/2, outH, p, q, tile)
	dualWait()
	return true
}

// dwMult8x3 runs the s16 3x3 kernel for channel multipliers of 8 or more.
func dwMult8x3(input16 []int16, inW, inH, ch, padH int, filter16 []int16, bias []int32, output []int8, outW, outH int,
	p *DepthwiseConvParams, q Quant) {
	asmDwS16Mult8x3(&input16[0], int32(inW), int32(inH), int32(ch), int32(p.Padding.Width), int32(padH),
		int32(p.Stride.Width), int32(p.Stride.Height), int32(p.ChMult), &filter16[0], ptr(bias), &output[0],
		int32(outW), int32(outH), p.OutOffset, &q.Shift[0], &q.Mult[0], p.Min, p.Max)
}

// dwMult8x3Dual is dw_s16m8_3x3_run_split of esp-nn.
//
//go:noinline
func dwMult8x3Dual(input16 []int16, inW, inH, ch int, filter16 []int16, bias []int32, output []int8, outW, outH int,
	p *DepthwiseConvParams, q Quant) bool {
	h0 := outH / 2
	rowOff := h0*p.Stride.Height - p.Padding.Height
	if !dualRun(0, func([]byte) {
		dwMult8x3(input16, inW, inH, ch, p.Padding.Height, filter16, bias, output, outW, h0, p, q)
	}) {
		return false
	}
	dwMult8x3(input16[rowOff*inW*ch:], inW, inH-rowOff, ch, 0, filter16, bias, output[h0*outW*ch*p.ChMult:], outW,
		outH-h0, p, q)
	dualWait()
	return true
}

// dw3x3Strips is dw3x3_strip_rows of esp-nn, for the output rows y0 to y1.
func dw3x3Strips(input []int8, inW, inH, ch, padW, padH, padRight int, filter []int8, bias []int32, output []int8,
	outW, y0, y1 int, p *DepthwiseConvParams, q Quant, tile []int8) {
	paddedW := inW + padW + padRight
	pad := int8(-p.InOffset)
	rowBytes := paddedW * ch
	stripRows := max(len(tile)/rowBytes, 3)
	perStrip := (stripRows-3)/p.Stride.Height + 1
	for y := y0; y < y1; {
		n := min(perStrip, y1-y)
		rows := (n-1)*p.Stride.Height + 3
		t := tile
		for r := 0; r < rows; r++ {
			src := y*p.Stride.Height + r - padH
			if src < 0 || src >= inH {
				fill(t[:rowBytes], pad)
			} else {
				fill(t[:padW*ch], pad)
				copy(t[padW*ch:], input[src*inW*ch:(src+1)*inW*ch])
				fill(t[(padW+inW)*ch:rowBytes], pad)
			}
			t = t[rowBytes:]
		}
		asmDwS8Mult1x3Padded(&tile[0], int32(paddedW), int32(rows), int32(ch), p.InOffset, int32(p.Stride.Width),
			int32(p.Stride.Height), &filter[0], ptr(bias), &output[y*outW*ch], int32(outW), int32(n), p.OutOffset,
			&q.Shift[0], &q.Mult[0], p.Min, p.Max)
		y += n
	}
}

// dw3x3Wide is the 3x3 path of esp-nn for 12 or more channels that are not a
// multiple of 16.
func dw3x3Wide(input []int8, inW, inH, ch int, filter []int8, bias []int32, output []int8, outW, outH int,
	p *DepthwiseConvParams, q Quant, scratch []byte) {
	newCh := (ch + 15) &^ 15
	pad := int8(-p.InOffset)
	padW, padH := p.Padding.Width, p.Padding.Height
	filterPadded := int8s(scratch[:9*newCh])
	clear(filterPadded)
	for i := 0; i < 9; i++ {
		copy(filterPadded[i*newCh:], filter[i*ch:(i+1)*ch])
	}
	newW, newH := inW+2*padW, inH+2*padH
	newW += max(0, outW*p.Stride.Width+2-newW)
	newH += max(0, outH*p.Stride.Height+2-newH)
	off := 9*newCh + 16
	padded := int8s(scratch[off : off+newW*newH*newCh])
	fill(padded, pad)
	for y := 0; y < inH; y++ {
		for x := 0; x < inW; x++ {
			copy(padded[((y+padH)*newW+x+padW)*newCh:], input[(y*inW+x)*ch:(y*inW+x+1)*ch])
		}
	}
	off += newW * newH * newCh
	outPadded := int8s(scratch[off : off+outW*outH*newCh])
	off += outW * outH * newCh
	q32 := int32s(scratch[off : off+3*newCh*4])
	shiftPad, multPad, biasPad := q32[:newCh], q32[newCh:2*newCh], q32[2*newCh:]
	copy(shiftPad, q.Shift[:ch])
	copy(multPad, q.Mult[:ch])
	clear(shiftPad[ch:])
	clear(multPad[ch:])
	var b *int32
	if bias != nil {
		copy(biasPad, bias[:ch])
		clear(biasPad[ch:])
		b = &biasPad[0]
	}
	asmDwS8Mult1x3Padded(&padded[0], int32(newW), int32(newH), int32(newCh), p.InOffset, int32(p.Stride.Width),
		int32(p.Stride.Height), &filterPadded[0], b, &outPadded[0], int32(outW), int32(outH), p.OutOffset,
		&shiftPad[0], &multPad[0], p.Min, p.Max)
	for i := 0; i < outW*outH; i++ {
		copy(output[i*ch:(i+1)*ch], outPadded[i*newCh:])
	}
}

// dwPadChannels is the esp-nn path for more than 3 channels that are not a
// multiple of 8.
func dwPadChannels(input []int8, inW, inH, ch int, filter []int8, fW, fH int, bias []int32, output []int8,
	outW, outH int, p *DepthwiseConvParams, q Quant, scratch []byte) {
	padded := (ch + 7) &^ 7
	filterSize := fW * fH * padded
	inputStart := (filterSize*2 + 15) &^ 15
	outStart := (inputStart + inW*inH*padded*2 + 15) &^ 15
	biasStart := (outStart + outW*outH*padded + 15) &^ 15
	filter16 := int16s(scratch[:filterSize*2])
	input16 := int16s(scratch[inputStart : inputStart+inW*inH*padded*2])
	outPadded := int8s(scratch[outStart : outStart+outW*outH*padded])
	q32 := int32s(scratch[biasStart : biasStart+3*padded*4])
	biasPad, shiftPad, multPad := q32[:padded], q32[padded:2*padded], q32[2*padded:]
	clear(q32)
	if bias != nil {
		copy(biasPad, bias[:ch])
	}
	copy(shiftPad, q.Shift[:ch])
	copy(multPad, q.Mult[:ch])
	clear(filter16)
	for k := 0; k < fW*fH; k++ {
		for c := 0; c < ch; c++ {
			filter16[k*padded+c] = int16(filter[k*ch+c])
		}
	}
	clear(input16)
	for k := 0; k < inW*inH; k++ {
		for c := 0; c < ch; c++ {
			input16[k*padded+c] = int16(input[k*ch+c]) + int16(p.InOffset)
		}
	}
	asmDwS16Mult1(&input16[0], int32(inW), int32(inH), int32(padded), int32(p.Padding.Width), int32(p.Padding.Height),
		int32(p.Stride.Width), int32(p.Stride.Height), &filter16[0], int32(fW), int32(fH), &biasPad[0], &outPadded[0],
		int32(outW), int32(outH), p.OutOffset, &shiftPad[0], &multPad[0], p.Min, p.Max)
	for k := 0; k < outW*outH; k++ {
		copy(output[k*ch:(k+1)*ch], outPadded[k*padded:])
	}
}
