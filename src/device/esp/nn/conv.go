package nn

// ConvParams are the values of ConvInt8, as esp-nn conv_params_t. Dilation
// is not used, as in esp-nn.
type ConvParams struct {
	InOffset  int32
	OutOffset int32
	Stride    Size2D
	Padding   Size2D
	Dilation  Size2D
	Activation
}

// ConvScratchSize returns the scratch size in bytes for ConvInt8.
func ConvScratchSize(inDims, filterDims, outDims Dims, p *ConvParams) int {
	return convScratchSize(&inDims, &filterDims, &outDims, p)
}

// ConvInt8 is the 2D convolution of an NHWC input with filters in OHWI
// order, as esp-nn esp_nn_conv_s8. If filterDims.Channels is less than
// inDims.Channels, it is a grouped convolution. The bias can be nil.
func ConvInt8(inDims Dims, in []int8, filterDims Dims, filter []int8, bias []int32,
	outDims Dims, out []int8, p *ConvParams, q Quant, scratch []byte) {
	fch := filterDims.Channels
	if fch == 0 {
		fch = inDims.Channels
	}
	checkLen(inDims.Width*inDims.Height*inDims.Channels, len(in))
	checkLen(outDims.Channels*filterDims.Width*filterDims.Height*fch, len(filter))
	checkLen(outDims.Width*outDims.Height*outDims.Channels, len(out))
	checkLen(outDims.Channels, len(q.Shift), len(q.Mult))
	if bias != nil {
		checkLen(outDims.Channels, len(bias))
	}
	convInt8(&inDims, in, &filterDims, filter, bias, &outDims, out, p, q, scratch)
}

func convInt8Go(inDims *Dims, in []int8, filterDims *Dims, filter []int8, bias []int32,
	outDims *Dims, out []int8, p *ConvParams, q Quant) {
	inW, inH, inCh := inDims.Width, inDims.Height, inDims.Channels
	fW, fH := filterDims.Width, filterDims.Height
	outCh := outDims.Channels
	fch := filterDims.Channels
	if fch == 0 {
		fch = inCh
	}
	perGroup := outCh / (inCh / fch)
	i := 0
	for outY := 0; outY < outDims.Height; outY++ {
		baseY := p.Stride.Height*outY - p.Padding.Height
		for outX := 0; outX < outDims.Width; outX++ {
			baseX := p.Stride.Width*outX - p.Padding.Width
			yStart, yEnd := max(0, -baseY), min(fH, inH-baseY)
			xStart, xEnd := max(0, -baseX), min(fW, inW-baseX)
			for oc := 0; oc < outCh; oc++ {
				chStart := oc / perGroup * fch
				var acc int32
				for fy := yStart; fy < yEnd; fy++ {
					for fx := xStart; fx < xEnd; fx++ {
						ib := ((baseY+fy)*inW+baseX+fx)*inCh + chStart
						fb := oc*fch*fH*fW + (fy*fW+fx)*fch
						for c := 0; c < fch; c++ {
							acc += (int32(in[ib+c]) + p.InOffset) * int32(filter[fb+c])
						}
					}
				}
				if bias != nil {
					acc += bias[oc]
				}
				acc = MultiplyByQuantizedMult(acc, q.Mult[oc], q.Shift[oc]) + p.OutOffset
				out[i] = int8(p.clamp(acc))
				i++
			}
		}
	}
}

// ConvUint8 is the 2D convolution for uint8 data with one output multiplier,
// as esp-nn esp_nn_conv_u8. The bias can be nil.
func ConvUint8(inDims Dims, in []uint8, inOffset int32, filterDims Dims, filter []uint8,
	filterOffset int32, bias []int32, outDims Dims, out []uint8, stride, padding Size2D,
	outOffset, outShift, outMult int32, act Activation) {
	inW, inH, inCh := inDims.Width, inDims.Height, inDims.Channels
	fW, fH := filterDims.Width, filterDims.Height
	outW, outCh := outDims.Width, outDims.Channels
	checkLen(inW*inH*inCh, len(in))
	checkLen(outCh*fW*fH*inCh, len(filter))
	checkLen(outW*outDims.Height*outCh, len(out))
	if bias != nil {
		checkLen(outCh, len(bias))
	}
	for outY := 0; outY < outDims.Height; outY++ {
		baseY := outY*stride.Height - padding.Height
		for outX := 0; outX < outW; outX++ {
			baseX := outX*stride.Width - padding.Width
			yStart, yEnd := max(0, -baseY), min(fH, inH-baseY)
			xStart, xEnd := max(0, -baseX), min(fW, inW-baseX)
			for oc := 0; oc < outCh; oc++ {
				var acc int32
				for fy := yStart; fy < yEnd; fy++ {
					for fx := xStart; fx < xEnd; fx++ {
						ib := ((baseY+fy)*inW + baseX + fx) * inCh
						fb := ((oc*fH+fy)*fW + fx) * inCh
						for c := 0; c < inCh; c++ {
							acc += (int32(in[ib+c]) + inOffset) * (int32(filter[fb+c]) + filterOffset)
						}
					}
				}
				if bias != nil {
					acc += bias[oc]
				}
				acc = MultiplyByQuantizedMult(acc, outMult, outShift) + outOffset
				out[(outY*outW+outX)*outCh+oc] = uint8(act.clamp(acc))
			}
		}
	}
}

// DepthwiseConvParams are the values of DepthwiseConvInt8, as esp-nn
// dw_conv_params_t. Dilation is not used, as in esp-nn.
type DepthwiseConvParams struct {
	InOffset  int32
	OutOffset int32
	ChMult    int
	Stride    Size2D
	Padding   Size2D
	Dilation  Size2D
	Activation
}

// DepthwiseConvScratchSize returns the scratch size in bytes for
// DepthwiseConvInt8.
func DepthwiseConvScratchSize(inDims, filterDims, outDims Dims, p *DepthwiseConvParams) int {
	return depthwiseConvScratchSize(&inDims, &filterDims, &outDims, p)
}

// DepthwiseConvInt8 is the depthwise 2D convolution of an NHWC input with a
// filter in 1HWO order, as esp-nn esp_nn_depthwise_conv_s8. The bias can be
// nil.
func DepthwiseConvInt8(inDims Dims, in []int8, filterDims Dims, filter []int8, bias []int32,
	outDims Dims, out []int8, p *DepthwiseConvParams, q Quant, scratch []byte) {
	outCh := inDims.Channels * p.ChMult
	checkLen(inDims.Width*inDims.Height*inDims.Channels, len(in))
	checkLen(filterDims.Width*filterDims.Height*outCh, len(filter))
	checkLen(outDims.Width*outDims.Height*outCh, len(out))
	checkLen(outCh, len(q.Shift), len(q.Mult))
	if bias != nil {
		checkLen(outCh, len(bias))
	}
	depthwiseConvInt8(&inDims, in, &filterDims, filter, bias, &outDims, out, p, q, scratch)
}

func depthwiseConvInt8Go(inDims *Dims, in []int8, filterDims *Dims, filter []int8, bias []int32,
	outDims *Dims, out []int8, p *DepthwiseConvParams, q Quant) {
	inW, inH, ch := inDims.Width, inDims.Height, inDims.Channels
	fW, fH := filterDims.Width, filterDims.Height
	chMult := p.ChMult
	i := 0
	for outY := 0; outY < outDims.Height; outY++ {
		baseY := outY*p.Stride.Height - p.Padding.Height
		for outX := 0; outX < outDims.Width; outX++ {
			baseX := outX*p.Stride.Width - p.Padding.Width
			yStart, yEnd := max(0, -baseY), min(fH, inH-baseY)
			xStart, xEnd := max(0, -baseX), min(fW, inW-baseX)
			for c := 0; c < ch; c++ {
				for m := 0; m < chMult; m++ {
					oc := c*chMult + m
					var acc int32
					for fy := yStart; fy < yEnd; fy++ {
						for fx := xStart; fx < xEnd; fx++ {
							iv := int32(in[((baseY+fy)*inW+baseX+fx)*ch+c]) + p.InOffset
							acc += iv * int32(filter[(fy*fW+fx)*ch*chMult+oc])
						}
					}
					if bias != nil {
						acc += bias[oc]
					}
					acc = MultiplyByQuantizedMult(acc, q.Mult[oc], q.Shift[oc]) + p.OutOffset
					out[i] = int8(p.clamp(acc))
					i++
				}
			}
		}
	}
}
