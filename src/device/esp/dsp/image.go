package dsp

// Dot2DFloat32 returns the sum of a*(b+offset) over the region, as esp-dsp
// dspi_dotprod_off_f32. Use offset 0 for dspi_dotprod_f32.
func Dot2DFloat32(a []float32, aStride int, b []float32, bStride int, width, height int, offset float32) float32 {
	checkRegion(len(a), aStride, len(b), bStride, width, height)
	var sum float32
	for r := 0; r < height; r++ {
		ra := a[r*aStride : r*aStride+width]
		rb := b[r*bStride : r*bStride+width]
		sum += dotFloat32(ra, rb)
		if offset != 0 {
			var s float32
			for _, v := range ra {
				s += v
			}
			sum += offset * s
		}
	}
	return sum
}

// Conv2DFloat32 filters the width by height image src with the kw by kh
// kernel, as esp-dsp dspi_conv_f32. dst has the size of src. Values outside of
// src count as zero, and the kernel center is at ((kw-1)/2, (kh-1)/2).
func Conv2DFloat32(dst []float32, dstStride int, src []float32, srcStride int, width, height int, kernel []float32, kStride, kw, kh int) {
	checkRegion(len(src), srcStride, len(dst), dstStride, width, height)
	checkRegion(len(kernel), kStride, len(kernel), kStride, kw, kh)
	rx, ry := (kw-1)/2, (kh-1)/2
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			var acc float32
			for m := 0; m < kh; m++ {
				sy := y + m - ry
				if sy < 0 || sy >= height {
					continue
				}
				for n := 0; n < kw; n++ {
					sx := x + n - rx
					if sx < 0 || sx >= width {
						continue
					}
					acc += src[sy*srcStride+sx] * kernel[m*kStride+n]
				}
			}
			dst[y*dstStride+x] = acc
		}
	}
}

// The Dot2D functions return the dot product of two image regions of width by
// height values, as esp-dsp dspi_dotprod does. aStride and bStride are the
// row lengths of the images. offset is added to each value of b, as in
// dspi_dotprod_off. Use offset 0 for a plain dot product. The integer versions
// return the full sum. For the esp-dsp result, add 1<<(shift-1) and shift right.

// Dot2DInt16 returns the sum of a*(b+offset) over the region.
func Dot2DInt16(a []int16, aStride int, b []int16, bStride int, width, height int, offset int32) int64 {
	checkRegion(len(a), aStride, len(b), bStride, width, height)
	var sum int64
	for r := 0; r < height; r++ {
		ra := a[r*aStride : r*aStride+width]
		rb := b[r*bStride : r*bStride+width]
		sum += dotInt16(ra, rb)
		if offset != 0 {
			var s int64
			for _, v := range ra {
				s += int64(v)
			}
			sum += int64(offset) * s
		}
	}
	return sum
}

// Dot2DInt8 returns the sum of a*(b+offset) over the region.
func Dot2DInt8(a []int8, aStride int, b []int8, bStride int, width, height int, offset int32) int64 {
	checkRegion(len(a), aStride, len(b), bStride, width, height)
	var sum int64
	for r := 0; r < height; r++ {
		ra := a[r*aStride : r*aStride+width]
		rb := b[r*bStride : r*bStride+width]
		sum += dotInt8(ra, rb)
		if offset != 0 {
			var s int64
			for _, v := range ra {
				s += int64(v)
			}
			sum += int64(offset) * s
		}
	}
	return sum
}

// Dot2DUint16 returns the sum of a*(b+offset) over the region.
func Dot2DUint16(a []uint16, aStride int, b []uint16, bStride int, width, height int, offset int32) int64 {
	checkRegion(len(a), aStride, len(b), bStride, width, height)
	var sum int64
	for r := 0; r < height; r++ {
		ra := a[r*aStride : r*aStride+width]
		rb := b[r*bStride : r*bStride+width]
		sum += int64(dotUint16(ra, rb))
		if offset != 0 {
			var s int64
			for _, v := range ra {
				s += int64(v)
			}
			sum += int64(offset) * s
		}
	}
	return sum
}

// Dot2DUint8 returns the sum of a*(b+offset) over the region.
func Dot2DUint8(a []uint8, aStride int, b []uint8, bStride int, width, height int, offset int32) int64 {
	checkRegion(len(a), aStride, len(b), bStride, width, height)
	var sum int64
	for r := 0; r < height; r++ {
		ra := a[r*aStride : r*aStride+width]
		rb := b[r*bStride : r*bStride+width]
		sum += int64(dotUint8(ra, rb))
		if offset != 0 {
			var s int64
			for _, v := range ra {
				s += int64(v)
			}
			sum += int64(offset) * s
		}
	}
	return sum
}

// Image is a 2D region as esp-dsp image2d_t. Data[0] is the first value, StepX
// and StepY are the steps between used columns and rows, Stride is the row
// length and SizeX and SizeY are the size for ConvImageFloat32.
type Image[T int8 | uint8 | int16 | uint16 | float32] struct {
	Data         []T
	StepX, StepY int
	Stride       int
	SizeX, SizeY int
}

func (m Image[T]) at(x, y int) T {
	return m.Data[y*m.StepY*m.Stride+x*m.StepX]
}

func (m Image[T]) check(countX, countY int) {
	if m.StepX < 1 || m.StepY < 1 || countX < 0 || countY < 0 || m.StepX*countX > m.Stride {
		panic("dsp: bad image")
	}
	if countX > 0 && countY > 0 && (countY-1)*m.StepY*m.Stride+(countX-1)*m.StepX >= len(m.Data) {
		panic("dsp: bad image")
	}
}

func dotImage[T int8 | uint8 | int16 | uint16](in, filter Image[T], countX, countY, shift int, offset T) int64 {
	in.check(countX, countY)
	filter.check(countX, countY)
	var acc int64
	for y := 0; y < countY; y++ {
		for x := 0; x < countX; x++ {
			acc += int64(in.at(x, y)) * (int64(filter.at(x, y)) + int64(offset))
		}
	}
	if shift > 0 {
		acc += 1 << uint(shift-1)
		acc >>= uint(shift)
	}
	return acc
}

// DotImageInt16 is esp-dsp dspi_dotprod_off_s16: the sum of in*(filter+offset)
// over countX by countY values, rounded and shifted right by shift. Use offset 0
// for dspi_dotprod_s16.
func DotImageInt16(in, filter Image[int16], countX, countY, shift int, offset int16) int16 {
	return int16(dotImage(in, filter, countX, countY, shift, offset))
}

// DotImageUint16 is esp-dsp dspi_dotprod_off_u16 and dspi_dotprod_u16.
func DotImageUint16(in, filter Image[uint16], countX, countY, shift int, offset uint16) uint16 {
	return uint16(dotImage(in, filter, countX, countY, shift, offset))
}

// DotImageInt8 is esp-dsp dspi_dotprod_off_s8 and dspi_dotprod_s8.
func DotImageInt8(in, filter Image[int8], countX, countY, shift int, offset int8) int8 {
	return int8(dotImage(in, filter, countX, countY, shift, offset))
}

// DotImageUint8 is esp-dsp dspi_dotprod_off_u8 and dspi_dotprod_u8.
func DotImageUint8(in, filter Image[uint8], countX, countY, shift int, offset uint8) uint8 {
	return uint8(dotImage(in, filter, countX, countY, shift, offset))
}

// DotImageFloat32 is esp-dsp dspi_dotprod_off_f32 and dspi_dotprod_f32.
func DotImageFloat32(in, filter Image[float32], countX, countY int, offset float32) float32 {
	in.check(countX, countY)
	filter.check(countX, countY)
	var acc float32
	for y := 0; y < countY; y++ {
		for x := 0; x < countX; x++ {
			acc += in.at(x, y) * (filter.at(x, y) + offset)
		}
	}
	return acc
}

// ConvImageFloat32 is esp-dsp dspi_conv_f32. It filters the SizeX by SizeY
// image in with filter into out, which has the same size. Values outside of in
// count as zero, and the filter center is at ((SizeX-1)/2, (SizeY-1)/2).
func ConvImageFloat32(out, in, filter Image[float32]) {
	in.check(in.SizeX, in.SizeY)
	out.check(in.SizeX, in.SizeY)
	filter.check(filter.SizeX, filter.SizeY)
	rx, ry := (filter.SizeX-1)/2, (filter.SizeY-1)/2
	for y := 0; y < in.SizeY; y++ {
		for x := 0; x < in.SizeX; x++ {
			var acc float32
			for m := 0; m < filter.SizeY; m++ {
				sy := y + m - ry
				if sy < 0 || sy >= in.SizeY {
					continue
				}
				for n := 0; n < filter.SizeX; n++ {
					sx := x + n - rx
					if sx < 0 || sx >= in.SizeX {
						continue
					}
					acc += in.at(sx, sy) * filter.at(n, m)
				}
			}
			out.Data[y*out.StepY*out.Stride+x*out.StepX] = acc
		}
	}
}
