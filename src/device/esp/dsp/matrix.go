package dsp

// MatMulInt16 sets dst to the product of a and b, shifted right by shift and
// saturated to the int16 range. a is m by n, b is n by k and dst is m by k, all
// in row major order.
func MatMulInt16(dst, a, b []int16, m, n, k int, shift uint) {
	if m < 0 || n < 0 || k < 0 || len(a) != m*n || len(b) != n*k || len(dst) != m*k {
		panic("dsp: length mismatch")
	}
	if shift > 63 {
		panic("dsp: shift out of range")
	}
	matMulInt16(dst, a, b, m, n, k, shift)
}

func matMulInt16Go(dst, a, b []int16, m, n, k int, shift uint) {
	for i := 0; i < m; i++ {
		for j := 0; j < k; j++ {
			var sum int64
			for s := 0; s < n; s++ {
				sum += int64(a[i*n+s]) * int64(b[s*k+j])
			}
			v := sum >> shift
			if v > 32767 {
				v = 32767
			} else if v < -32768 {
				v = -32768
			}
			dst[i*k+j] = int16(v)
		}
	}
}

// MatAddFloat32 sets dst to a+b for matrices of rows by cols values, as
// esp-dsp dspm_add_f32. The strides are the row lengths.
func MatAddFloat32(dst, a, b []float32, rows, cols, dstStride, aStride, bStride int) {
	matOp(dst, a, b, rows, cols, dstStride, aStride, bStride, func(x, y float32) float32 { return x + y })
}

// MatSubFloat32 sets dst to a-b, as esp-dsp dspm_sub_f32.
func MatSubFloat32(dst, a, b []float32, rows, cols, dstStride, aStride, bStride int) {
	matOp(dst, a, b, rows, cols, dstStride, aStride, bStride, func(x, y float32) float32 { return x - y })
}

// MatAddConstFloat32 sets dst to src+c, as esp-dsp dspm_addc_f32.
func MatAddConstFloat32(dst, src []float32, c float32, rows, cols, dstStride, srcStride int) {
	matOp(dst, src, src, rows, cols, dstStride, srcStride, srcStride, func(x, _ float32) float32 { return x + c })
}

// MatMulConstFloat32 sets dst to src*c, as esp-dsp dspm_mulc_f32.
func MatMulConstFloat32(dst, src []float32, c float32, rows, cols, dstStride, srcStride int) {
	matOp(dst, src, src, rows, cols, dstStride, srcStride, srcStride, func(x, _ float32) float32 { return x * c })
}

func matOp(dst, a, b []float32, rows, cols, dstStride, aStride, bStride int, op func(x, y float32) float32) {
	checkRegion(len(a), aStride, len(dst), dstStride, cols, rows)
	checkRegion(len(b), bStride, len(dst), dstStride, cols, rows)
	for r := 0; r < rows; r++ {
		d := dst[r*dstStride : r*dstStride+cols]
		ra := a[r*aStride : r*aStride+cols]
		rb := b[r*bStride : r*bStride+cols]
		for i := range d {
			d[i] = op(ra[i], rb[i])
		}
	}
}

// MatMul3x3x1Float32 sets c to the 3x3 matrix a times the 3x1 vector b, as
// esp-dsp dspm_mult_3x3x1_f32.
func MatMul3x3x1Float32(c *[3]float32, a *[9]float32, b *[3]float32) {
	for i := 0; i < 3; i++ {
		c[i] = a[i*3]*b[0] + a[i*3+1]*b[1] + a[i*3+2]*b[2]
	}
}

// MatMul3x3x3Float32 sets c to the 3x3 matrices a times b, as esp-dsp
// dspm_mult_3x3x3_f32.
func MatMul3x3x3Float32(c, a, b *[9]float32) {
	var r [9]float32
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			r[i*3+j] = a[i*3]*b[j] + a[i*3+1]*b[3+j] + a[i*3+2]*b[6+j]
		}
	}
	*c = r
}

// MatMul4x4x1Float32 sets c to the 4x4 matrix a times the 4x1 vector b, as
// esp-dsp dspm_mult_4x4x1_f32.
func MatMul4x4x1Float32(c *[4]float32, a *[16]float32, b *[4]float32) {
	for i := 0; i < 4; i++ {
		c[i] = a[i*4]*b[0] + a[i*4+1]*b[1] + a[i*4+2]*b[2] + a[i*4+3]*b[3]
	}
}

// MatMul4x4x4Float32 sets c to the 4x4 matrices a times b, as esp-dsp
// dspm_mult_4x4x4_f32. It uses the PIE matrix kernel on the ESP32-S3.
func MatMul4x4x4Float32(c, a, b *[16]float32) {
	MatMulFloat32(c[:], a[:], b[:], 4, 4, 4)
}

// MatVecInt8 sets dst[i] to the dot product of row i of the m by n matrix a
// and the vector b, as esp-dsp dspm_mult_mxn_1xm_int8.
func MatVecInt8(dst []int32, a, b []int8, m, n int) {
	if m < 0 || n < 0 || len(dst) != m || len(a) != m*n || len(b) != n {
		panic("dsp: length mismatch")
	}
	for i := range dst {
		dst[i] = int32(dotInt8(a[i*n:i*n+n], b))
	}
}

// MatMulFloat32 sets dst to the product of a and b. a is m by n, b is n by k
// and dst is m by k, all in row major order.
func MatMulFloat32(dst, a, b []float32, m, n, k int) {
	MatMulFloat32Ex(dst, a, b, m, n, k, n, k, k)
}

// MatMulFloat32Ex is MatMulFloat32 for sub matrices, as esp-dsp dspm_mult_ex
// does. aStride, bStride and dstStride are the row lengths of the full matrices.
func MatMulFloat32Ex(dst, a, b []float32, m, n, k, aStride, bStride, dstStride int) {
	if m < 0 || n < 0 || k < 0 || aStride < n || bStride < k || dstStride < k {
		panic("dsp: bad matrix size")
	}
	if m > 0 && ((m-1)*aStride+n > len(a) || (m-1)*dstStride+k > len(dst)) {
		panic("dsp: bad matrix size")
	}
	if n > 0 && (n-1)*bStride+k > len(b) {
		panic("dsp: bad matrix size")
	}
	matMulFloat32(dst, a, b, m, n, k, aStride, bStride, dstStride)
}

func matMulFloat32Go(dst, a, b []float32, m, n, k, aStride, bStride, dstStride int) {
	for i := 0; i < m; i++ {
		row := dst[i*dstStride : i*dstStride+k]
		for j := range row {
			row[j] = 0
		}
		for s := 0; s < n; s++ {
			v := a[i*aStride+s]
			brow := b[s*bStride : s*bStride+k]
			for j := range row {
				row[j] += v * brow[j]
			}
		}
	}
}
