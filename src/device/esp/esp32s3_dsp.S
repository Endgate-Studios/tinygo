// ESP32-S3 PIE and FPU kernels for package device/esp/dsp. The Go code in
// src/device/esp/dsp/dsp_esp32s3.go checks the arguments before each call.
//
// PIE instructions, see ESP32-S3 Technical Reference Manual chapter 1.
// EE.VLD.128 ignores the low 4 address bits, so pointers are 16 byte aligned
// unless the name says unaligned. Every function has its own section, so the
// linker drops the ones that a program does not use. Functions start at 16
// bytes and nop instructions keep the first instruction of each loop body in
// one 16 byte instruction fetch, as the GNU assembler does.

	.section .text.tinygo_dsp_dot_s16,"ax",@progbits
	.global tinygo_dsp_dot_s16
	.type tinygo_dsp_dot_s16,@function
	.p2align 4
tinygo_dsp_dot_s16:
	entry a1, 32
	ee.zero.accx
	srli a4, a4, 3
	loopnez a4, .Ltinygo_dsp_dot_s16_end
	ee.vld.128.ip q0, a2, 16
	ee.vld.128.ip q1, a3, 16
	ee.vmulas.s16.accx q0, q1
.Ltinygo_dsp_dot_s16_end:
	rur.accx_0 a2
	rur.accx_1 a3
	sext a3, a3, 7
	retw.n
	.size tinygo_dsp_dot_s16, . - tinygo_dsp_dot_s16

	.section .text.tinygo_dsp_dot_s16_unaligned,"ax",@progbits
	.global tinygo_dsp_dot_s16_unaligned
	.type tinygo_dsp_dot_s16_unaligned,@function
	.p2align 4
tinygo_dsp_dot_s16_unaligned:
	entry a1, 32
	ee.zero.accx
	ee.ld.128.usar.ip q0, a2, 16
	srli a4, a4, 3
	nop.n
	loopnez a4, .Ltinygo_dsp_dot_s16_unaligned_end
	ee.ld.128.usar.ip q1, a2, 16
	ee.src.q q2, q0, q1
	ee.vld.128.ip q3, a3, 16
	ee.vmulas.s16.accx q2, q3
	ee.orq q0, q1, q1
.Ltinygo_dsp_dot_s16_unaligned_end:
	rur.accx_0 a2
	rur.accx_1 a3
	sext a3, a3, 7
	retw.n
	.size tinygo_dsp_dot_s16_unaligned, . - tinygo_dsp_dot_s16_unaligned

	.section .text.tinygo_dsp_dot_s8,"ax",@progbits
	.global tinygo_dsp_dot_s8
	.type tinygo_dsp_dot_s8,@function
	.p2align 4
tinygo_dsp_dot_s8:
	entry a1, 32
	ee.zero.accx
	srli a4, a4, 4
	loopnez a4, .Ltinygo_dsp_dot_s8_end
	ee.vld.128.ip q0, a2, 16
	ee.vld.128.ip q1, a3, 16
	ee.vmulas.s8.accx q0, q1
.Ltinygo_dsp_dot_s8_end:
	rur.accx_0 a2
	rur.accx_1 a3
	sext a3, a3, 7
	retw.n
	.size tinygo_dsp_dot_s8, . - tinygo_dsp_dot_s8

	.section .text.tinygo_dsp_dot_s8_unaligned,"ax",@progbits
	.global tinygo_dsp_dot_s8_unaligned
	.type tinygo_dsp_dot_s8_unaligned,@function
	.p2align 4
tinygo_dsp_dot_s8_unaligned:
	entry a1, 32
	ee.zero.accx
	ee.ld.128.usar.ip q0, a2, 16
	srli a4, a4, 4
	nop.n
	loopnez a4, .Ltinygo_dsp_dot_s8_unaligned_end
	ee.ld.128.usar.ip q1, a2, 16
	ee.src.q q2, q0, q1
	ee.vld.128.ip q3, a3, 16
	ee.vmulas.s8.accx q2, q3
	ee.orq q0, q1, q1
.Ltinygo_dsp_dot_s8_unaligned_end:
	rur.accx_0 a2
	rur.accx_1 a3
	sext a3, a3, 7
	retw.n
	.size tinygo_dsp_dot_s8_unaligned, . - tinygo_dsp_dot_s8_unaligned

	.section .text.tinygo_dsp_dot_u16,"ax",@progbits
	.global tinygo_dsp_dot_u16
	.type tinygo_dsp_dot_u16,@function
	.p2align 4
tinygo_dsp_dot_u16:
	entry a1, 32
	ee.zero.accx
	srli a4, a4, 3
	loopnez a4, .Ltinygo_dsp_dot_u16_end
	ee.vld.128.ip q0, a2, 16
	ee.vld.128.ip q1, a3, 16
	ee.vmulas.u16.accx q0, q1
.Ltinygo_dsp_dot_u16_end:
	rur.accx_0 a2
	rur.accx_1 a3
	extui a3, a3, 0, 8
	retw.n
	.size tinygo_dsp_dot_u16, . - tinygo_dsp_dot_u16

	.section .text.tinygo_dsp_dot_u8,"ax",@progbits
	.global tinygo_dsp_dot_u8
	.type tinygo_dsp_dot_u8,@function
	.p2align 4
tinygo_dsp_dot_u8:
	entry a1, 32
	ee.zero.accx
	srli a4, a4, 4
	loopnez a4, .Ltinygo_dsp_dot_u8_end
	ee.vld.128.ip q0, a2, 16
	ee.vld.128.ip q1, a3, 16
	ee.vmulas.u8.accx q0, q1
.Ltinygo_dsp_dot_u8_end:
	rur.accx_0 a2
	rur.accx_1 a3
	extui a3, a3, 0, 8
	retw.n
	.size tinygo_dsp_dot_u8, . - tinygo_dsp_dot_u8

	.section .text.tinygo_dsp_add_s16,"ax",@progbits
	.global tinygo_dsp_add_s16
	.type tinygo_dsp_add_s16,@function
	.p2align 4
tinygo_dsp_add_s16:
	entry a1, 32
	srli a5, a5, 3
	loopnez a5, .Ltinygo_dsp_add_s16_end
	ee.vld.128.ip q0, a3, 16
	ee.vld.128.ip q1, a4, 16
	ee.vadds.s16 q2, q0, q1
	ee.vst.128.ip q2, a2, 16
.Ltinygo_dsp_add_s16_end:
	retw.n
	.size tinygo_dsp_add_s16, . - tinygo_dsp_add_s16

	.section .text.tinygo_dsp_sub_s16,"ax",@progbits
	.global tinygo_dsp_sub_s16
	.type tinygo_dsp_sub_s16,@function
	.p2align 4
tinygo_dsp_sub_s16:
	entry a1, 32
	srli a5, a5, 3
	loopnez a5, .Ltinygo_dsp_sub_s16_end
	ee.vld.128.ip q0, a3, 16
	ee.vld.128.ip q1, a4, 16
	ee.vsubs.s16 q2, q0, q1
	ee.vst.128.ip q2, a2, 16
.Ltinygo_dsp_sub_s16_end:
	retw.n
	.size tinygo_dsp_sub_s16, . - tinygo_dsp_sub_s16

	.section .text.tinygo_dsp_add_s8,"ax",@progbits
	.global tinygo_dsp_add_s8
	.type tinygo_dsp_add_s8,@function
	.p2align 4
tinygo_dsp_add_s8:
	entry a1, 32
	srli a5, a5, 4
	loopnez a5, .Ltinygo_dsp_add_s8_end
	ee.vld.128.ip q0, a3, 16
	ee.vld.128.ip q1, a4, 16
	ee.vadds.s8 q2, q0, q1
	ee.vst.128.ip q2, a2, 16
.Ltinygo_dsp_add_s8_end:
	retw.n
	.size tinygo_dsp_add_s8, . - tinygo_dsp_add_s8

	.section .text.tinygo_dsp_sub_s8,"ax",@progbits
	.global tinygo_dsp_sub_s8
	.type tinygo_dsp_sub_s8,@function
	.p2align 4
tinygo_dsp_sub_s8:
	entry a1, 32
	srli a5, a5, 4
	loopnez a5, .Ltinygo_dsp_sub_s8_end
	ee.vld.128.ip q0, a3, 16
	ee.vld.128.ip q1, a4, 16
	ee.vsubs.s8 q2, q0, q1
	ee.vst.128.ip q2, a2, 16
.Ltinygo_dsp_sub_s8_end:
	retw.n
	.size tinygo_dsp_sub_s8, . - tinygo_dsp_sub_s8

	.section .text.tinygo_dsp_mul_s16,"ax",@progbits
	.global tinygo_dsp_mul_s16
	.type tinygo_dsp_mul_s16,@function
	.p2align 4
tinygo_dsp_mul_s16:
	entry a1, 32
	wsr.sar a6
	srli a5, a5, 3
	loopnez a5, .Ltinygo_dsp_mul_s16_end
	ee.vld.128.ip q0, a3, 16
	ee.vld.128.ip q1, a4, 16
	ee.vmul.s16 q2, q0, q1
	ee.vst.128.ip q2, a2, 16
.Ltinygo_dsp_mul_s16_end:
	retw.n
	.size tinygo_dsp_mul_s16, . - tinygo_dsp_mul_s16

	.section .text.tinygo_dsp_mul_s8,"ax",@progbits
	.global tinygo_dsp_mul_s8
	.type tinygo_dsp_mul_s8,@function
	.p2align 4
tinygo_dsp_mul_s8:
	entry a1, 32
	wsr.sar a6
	srli a5, a5, 4
	loopnez a5, .Ltinygo_dsp_mul_s8_end
	ee.vld.128.ip q0, a3, 16
	ee.vld.128.ip q1, a4, 16
	ee.vmul.s8 q2, q0, q1
	ee.vst.128.ip q2, a2, 16
.Ltinygo_dsp_mul_s8_end:
	retw.n
	.size tinygo_dsp_mul_s8, . - tinygo_dsp_mul_s8

	.section .text.tinygo_dsp_copy,"ax",@progbits
	.global tinygo_dsp_copy
	.type tinygo_dsp_copy,@function
	.p2align 4
tinygo_dsp_copy:
	entry a1, 32
	srli a4, a4, 4
	loopnez a4, .Ltinygo_dsp_copy_end
	ee.vld.128.ip q0, a3, 16
	ee.vst.128.ip q0, a2, 16
.Ltinygo_dsp_copy_end:
	retw.n
	.size tinygo_dsp_copy, . - tinygo_dsp_copy

	.section .text.tinygo_dsp_fill,"ax",@progbits
	.global tinygo_dsp_fill
	.type tinygo_dsp_fill,@function
	.p2align 4
tinygo_dsp_fill:
	entry a1, 32
	ee.vldbc.8.ip q0, a3, 0
	srli a4, a4, 4
	loopnez a4, .Ltinygo_dsp_fill_end
	ee.vst.128.ip q0, a2, 16
.Ltinygo_dsp_fill_end:
	retw.n
	.size tinygo_dsp_fill, . - tinygo_dsp_fill

	.section .text.tinygo_dsp_matmul_s16_block,"ax",@progbits
	.global tinygo_dsp_matmul_s16_block
	.type tinygo_dsp_matmul_s16_block,@function
	.p2align 4
tinygo_dsp_matmul_s16_block:
	entry a1, 32
	ee.zero.qacc
	loopnez a5, .Ltinygo_dsp_matmul_s16_block_end
	ee.vldbc.16.ip q1, a3, 2
	ee.vld.128.xp q0, a4, a6
	ee.vmulas.s16.qacc q0, q1
.Ltinygo_dsp_matmul_s16_block_end:
	ee.srcmb.s16.qacc q2, a7, 0
	ee.vst.128.ip q2, a2, 0
	retw.n
	.size tinygo_dsp_matmul_s16_block, . - tinygo_dsp_matmul_s16_block

// The functions below are ports of the ESP32-S3 kernels of esp-dsp,
// https://github.com/espressif/esp-dsp. Their license:
//
// Copyright 2018-2019 Espressif Systems (Shanghai) PTE LTD
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

	.section .text.tinygo_dsp_fft2r_sc16,"ax",@progbits
	.global tinygo_dsp_fft2r_sc16
	.type tinygo_dsp_fft2r_sc16,@function
	.p2align 4
tinygo_dsp_fft2r_sc16:
	entry a1, 64
	mov.n a9, a1
	movi a8, -1
	extui a8, a8, 0, 15
	addi.n a6, a3, 1
	movi.n a10, 16
	wsr.sar a10
	movgez a6, a3, a3
	s16i a8, a1, 0
	ee.vldbc.16.ip q6, a9, 0
	srai a6, a6, 1
	bgeui a6, 3, 1f
	movi.n a5, 1
	j .Ltg_fft_last
1:
	movi.n a5, 1
.Ltg_fft_stage:
	mov.n a13, a4
	mov.n a9, a2
	beqz.n a5, .Ltg_fft_next
	srli a3, a6, 2
	movi.n a14, 0
	slli a15, a6, 2
	add.n a8, a15, a9
.Ltg_fft_group:
	ee.vldbc.32.ip q5, a13, 4
	nop.n
	loopnez a3, .Ltg_fft_group_end
	ee.vld.128.ip q0, a8, 0
	ee.vld.128.ip q2, a9, 0
	ee.cmul.s16 q1, q5, q0, 2
	ee.vmul.s16 q2, q2, q6
	ee.cmul.s16 q1, q5, q0, 3
	ee.vsubs.s16 q3, q2, q1
	ee.vadds.s16.st.incp q3, a8, q3, q2, q1
	ee.vst.128.ip q3, a9, 16
.Ltg_fft_group_end:
	addi.n a14, a14, 1
	add.n a9, a9, a15
	add.n a8, a15, a9
	bne a14, a5, .Ltg_fft_group
.Ltg_fft_next:
	slli a5, a5, 1
	srli a6, a6, 1
	bgeui a6, 3, .Ltg_fft_stage
	srli a10, a5, 1
	bnez a10, 2f
	addi a15, a1, 16
	ee.vst.128.ip q1, a15, 0
	j .Ltg_fft_last
2:
	mov.n a9, a4
	mv.qr q4, q1
	mov.n a8, a2
	mv.qr q1, q0
	mv.qr q0, q2
	loopnez a10, .Ltg_fft_two_end
	ee.vld.l.64.ip q0, a8, 8
	ee.vldbc.32.ip q2, a9, 4
	ee.vld.l.64.ip q1, a8, 8
	ee.vldbc.32.ip q3, a9, 4
	ee.vld.h.64.ip q0, a8, 8
	ee.vunzip.32 q2, q3
	ee.vld.h.64.ip q1, a8, -24
	ee.vmul.s16 q0, q0, q6
	ee.cmul.s16 q4, q2, q1, 2
	ee.cmul.s16 q4, q2, q1, 3
	ee.vadds.s16 q2, q0, q4
	ee.vsubs.s16 q3, q0, q4
	ee.vst.l.64.ip q2, a8, 8
	ee.vst.l.64.ip q3, a8, 8
	ee.vst.h.64.ip q2, a8, 8
	ee.vst.h.64.ip q3, a8, 8
.Ltg_fft_two_end:
	addi a15, a1, 16
	ee.vst.128.ip q4, a15, 0
.Ltg_fft_last:
	addi a15, a1, 16
	ee.vld.128.ip q3, a15, 0
	slli a10, a5, 1
	srli a10, a10, 2
	nop.n
	loopnez a10, .Ltg_fft_last_end
	ee.vld.128.ip q0, a2, 16
	ee.vld.128.ip q1, a2, -16
	ee.vld.128.ip q2, a4, 16
	ee.vunzip.32 q0, q1
	ee.cmul.s16 q3, q2, q1, 2
	ee.vmul.s16 q0, q0, q6
	ee.cmul.s16 q3, q2, q1, 3
	ee.vsubs.s16 q1, q0, q3
	ee.vadds.s16 q0, q0, q3
	ee.vzip.32 q0, q1
	ee.vst.128.ip q0, a2, 16
	ee.vst.128.ip q1, a2, 16
.Ltg_fft_last_end:
	retw.n
	.size tinygo_dsp_fft2r_sc16, . - tinygo_dsp_fft2r_sc16

	.section .text.tinygo_dsp_dotprod_f32,"ax",@progbits
	.global tinygo_dsp_dotprod_f32
	.type tinygo_dsp_dotprod_f32,@function
	.p2align 4
tinygo_dsp_dotprod_f32:
	entry a1, 16
	movi.n a9, 0
	wfr f0, a9
	wfr f1, a9
	wfr f2, a9
	wfr f3, a9
	srli a6, a5, 2
	loopnez a6, .Ltg_dotprod_f32_loop_mac_end_m_ae32
	ee.ldf.128.ip f11, f10, f9, f8, a2, 16
	ee.ldf.128.ip f7, f6, f5, f4, a3, 16
	madd.s f0, f4, f8
	madd.s f1, f5, f9
	madd.s f2, f6, f10
	madd.s f3, f7, f11
.Ltg_dotprod_f32_loop_mac_end_m_ae32:
	add.s f0, f0, f1
	add.s f0, f0, f2
	add.s f0, f0, f3
	ssi f0, a4, 0
	movi.n a2, 0
	retw.n
	.size tinygo_dsp_dotprod_f32, . - tinygo_dsp_dotprod_f32

	.section .text.tinygo_dsp_fft2r_fc32,"ax",@progbits
	.global tinygo_dsp_fft2r_fc32
	.type tinygo_dsp_fft2r_fc32,@function
	.p2align 4
tinygo_dsp_fft2r_fc32:
	entry a1, 16
	srli a6, a3, 1
	movi.n a7, 1
.Ltg_fft2r_fc32_fft2r_l1:
	movi.n a8, 0
	movi.n a11,0
.Ltg_fft2r_fc32_fft2r_l2:
	addx8 a10, a8, a4
	ee.ldf.64.ip f1, f0, a10, 0
	add.n a12, a11, a6
	addx8 a14, a12, a2
	loopnez a6, .Ltg_fft2r_fc32_fft2r_l3
	ee.ldf.64.ip f5, f4, a14, 0
	mul.s f6, f0, f4
	mul.s f7, f0, f5
	addx8 a13, a11, a2
	ee.ldf.64.ip f3, f2, a13, 0
	madd.s f6, f1, f5
	msub.s f7, f1, f4
	addi a11, a11, 1
	add.n a12, a11, a6
	sub.s f8, f2, f6
	sub.s f9, f3, f7
	add.s f10, f2, f6
	add.s f11, f3, f7
	ee.stf.64.ip f9, f8, a14, 0
	addx8 a14, a12, a2
	ee.stf.64.ip f11, f10, a13, 0
.Ltg_fft2r_fc32_fft2r_l3:
	add.n a11, a11, a6
	addi.n a8, a8, 1
	bne a8, a7, .Ltg_fft2r_fc32_fft2r_l2
	slli a7, a7, 1
	srli a6, a6, 1
	bnez a6, .Ltg_fft2r_fc32_fft2r_l1
	movi.n a2, 0
	retw.n
	.size tinygo_dsp_fft2r_fc32, . - tinygo_dsp_fft2r_fc32

	.section .text.tinygo_dsp_fft4r_fc32,"ax",@progbits
	.global tinygo_dsp_fft4r_fc32
	.type tinygo_dsp_fft4r_fc32,@function
	.p2align 4
tinygo_dsp_fft4r_fc32:
	entry a1, 16
	addi.n a6, a3, -1
	and a6, a3, a6
	nsau a6, a3
	movi.n a7, 31
	xor a6, a6, a7
	srli a7, a6, 1
	addi.n a6, a6, -1
	ssr a6
	srl a6, a5
	movi.n a5, 2
.Ltg_fft4r_fc32_stage:
	srli a3, a3, 2
	movi.n a8, 0
.Ltg_fft4r_fc32_group:
	mov.n a9, a4
	mov.n a10, a4
	mov.n a11, a4
	mul16u a12, a8, a3
	slli a12, a12, 1
	addx8 a12, a12, a2
	addx8 a13, a3, a12
	addx8 a14, a3, a13
	addx8 a15, a3, a14
	nop
	loopnez a3, .Ltg_fft4r_fc32_bf4_loop_end
	ee.ldf.64.ip f1, f0, a12, 0
	ee.ldf.64.ip f3, f2, a14, 0
	add.s f5, f1, f3
	sub.s f7, f1, f3
	add.s f4, f0, f2
	sub.s f6, f0, f2
	ee.ldf.64.ip f1, f0, a13, 0
	ee.ldf.64.ip f3, f2, a15, 0
	add.s f9, f1, f3
	sub.s f11, f1, f3
	lsi f12, a9, 0
	lsi f13, a10, 0
	lsi f14, a11, 0
	add.s f8, f0, f2
	sub.s f10, f0, f2
	sub.s f1, f5, f9
	add.s f5, f5, f9
	add.s f2, f6, f11
	sub.s f6, f6, f11
	sub.s f0, f4, f8
	add.s f4, f4, f8
	sub.s f3, f7, f10
	add.s f7, f7, f10
	mul.s f10, f6, f14
	ee.stf.64.ip f5, f4, a12, 8
	mul.s f4, f2, f12
	mul.s f11, f7, f14
	mul.s f5, f3, f12
	mul.s f8, f0, f13
	mul.s f9, f1, f13
	lsi f12, a9, 4
	lsi f13, a10, 4
	lsi f14, a11, 4
	msub.s f5, f2, f12
	madd.s f4, f3, f12
	msub.s f9, f0, f13
	madd.s f8, f1, f13
	msub.s f11, f6, f14
	madd.s f10, f7, f14
	addx4 a9, a6, a9
	addx8 a10, a6, a10
	addx4 a11, a6, a11
	addx8 a11, a6, a11
	ee.stf.64.ip f5, f4, a13, 8
	ee.stf.64.ip f9, f8, a14, 8
	ee.stf.64.ip f11, f10, a15, 8
.Ltg_fft4r_fc32_bf4_loop_end:
	addi.n a8, a8, 2
	bgeu a8, a5, .Ltg_fft4r_fc32_stage_next
	j .Ltg_fft4r_fc32_group
.Ltg_fft4r_fc32_stage_next:
	slli a5, a5, 2
	slli a6, a6, 2
	addi.n a7, a7, -1
	bnez a7, .Ltg_fft4r_fc32_stage
	movi.n a2, 0
	retw.n
	.size tinygo_dsp_fft4r_fc32, . - tinygo_dsp_fft4r_fc32

	.section .text.tinygo_dsp_bit_rev_lookup_fc32,"ax",@progbits
	.global tinygo_dsp_bit_rev_lookup_fc32
	.type tinygo_dsp_bit_rev_lookup_fc32,@function
	.p2align 4
tinygo_dsp_bit_rev_lookup_fc32:
	entry a1, 16
	loopnez a3, .Ltg_bit_rev_lookup_fc32___loop_end_radix2_reorder_lookup_table
	l16ui a5, a4, 0
	l16ui a6, a4, 2
	addi a4, a4, 4
	add.n a5, a5, a2
	add.n a6, a6, a2
	ee.ldf.64.ip f0, f2, a5, 0
	ee.ldf.64.ip f1, f3, a6, 0
	ee.stf.64.ip f0, f2, a6, 0
	ee.stf.64.ip f1, f3, a5, 0
.Ltg_bit_rev_lookup_fc32___loop_end_radix2_reorder_lookup_table:
	movi.n a2, 0
	retw.n
	.size tinygo_dsp_bit_rev_lookup_fc32, . - tinygo_dsp_bit_rev_lookup_fc32

	.section .text.tinygo_dsp_fir_f32,"ax",@progbits
	.global tinygo_dsp_fir_f32
	.type tinygo_dsp_fir_f32,@function
	.p2align 4
tinygo_dsp_fir_f32:
	entry a1, 16
	l32i a7, a2, 12
	l32i a6, a2, 8
	l32i a10, a2, 0
	l32i a11, a2, 4
	addx4 a11, a7, a11
	l32i a6, a2, 8
	movi.n a9, 0
	movi.n a12, 3
	movi.n a12, -16
	movi.n a13, 15
.Ltg_fir_f32_fir_loop_len:
	lsip f15, a3, 4
	ssip f15, a11, 4
	addi a7, a7, 1
	blt a7, a6, .Ltg_fir_f32_do_not_reset_a11
	l32i a11, a2, 4
	movi a7, 0
.Ltg_fir_f32_do_not_reset_a11:
	and a15, a11, a12
	l32i a10, a2, 0
	const.s f4, 0
	const.s f5, 0
	const.s f6, 0
	const.s f7, 0
	and a8, a11, a13
	beqz a8, .Ltg_fir_f32_offset_0
	addi a8, a8, -4
	beqz a8, .Ltg_fir_f32_offset_1
	addi a8, a8, -4
	beqz a8, .Ltg_fir_f32_offset_2
	addi a8, a8, -4
	beqz a8, .Ltg_fir_f32_offset_3
.Ltg_fir_f32_offset_0:
	sub a14, a6, a7
	srli a14, a14, 2
	loopnez a14, .Ltg_fir_f32_first_fir_loop_0
	ee.ldf.128.ip f3, f2, f1, f0, a10, 16
	ee.ldf.128.ip f11, f10, f9, f8, a15, 16
	madd.s f4, f0, f8
	madd.s f5, f1, f9
	madd.s f6, f2, f10
	madd.s f7, f3, f11
.Ltg_fir_f32_first_fir_loop_0:
	l32i a15, a2, 4
	srli a14, a7, 2
	nop.n
	loopnez a14, .Ltg_fir_f32_second_fir_loop_0
	ee.ldf.128.ip f3, f2, f1, f0, a10, 16
	ee.ldf.128.ip f11, f10, f9, f8, a15, 16
	madd.s f4, f0, f8
	madd.s f5, f1, f9
	madd.s f6, f2, f10
	madd.s f7, f3, f11
.Ltg_fir_f32_second_fir_loop_0:
	j .Ltg_fir_f32_store_fir_result
.Ltg_fir_f32_offset_1:
	sub a14, a6, a7
	addi a14, a14, 3
	srli a14, a14, 2
	ee.ldf.128.ip f11, f10, f9, f12, a15, 16
	loopnez a14, .Ltg_fir_f32_first_fir_loop_1
	ee.ldf.128.ip f3, f2, f1, f0, a10, 16
	madd.s f4, f0, f9
	madd.s f5, f1, f10
	madd.s f6, f2, f11
	ee.ldf.128.ip f11, f10, f9, f8, a15, 16
	madd.s f7, f3, f8
.Ltg_fir_f32_first_fir_loop_1:
	l32i a15, a2, 4
	ee.ldf.128.ip f11, f10, f9, f8, a15, 16
	srli a14, a7, 2
	loopnez a14, .Ltg_fir_f32_second_fir_loop_1
	madd.s f4, f3, f8
	ee.ldf.128.ip f3, f2, f1, f0, a10, 16
	madd.s f5, f0, f9
	madd.s f6, f1, f10
	madd.s f7, f2, f11
	ee.ldf.128.ip f11, f10, f9, f8, a15, 16
.Ltg_fir_f32_second_fir_loop_1:
	madd.s f4, f3, f12
	j .Ltg_fir_f32_store_fir_result
.Ltg_fir_f32_offset_2:
	sub a14, a6, a7
	addi a14, a14, 3
	srli a14, a14, 2
	ee.ldf.128.ip f11, f10, f13, f12, a15, 16
	loopnez a14, .Ltg_fir_f32_first_fir_loop_2
	ee.ldf.128.ip f3, f2, f1, f0, a10, 16
	madd.s f4, f0, f10
	madd.s f5, f1, f11
	ee.ldf.128.ip f11, f10, f9, f8, a15, 16
	madd.s f6, f2, f8
	madd.s f7, f3, f9
.Ltg_fir_f32_first_fir_loop_2:
	l32i a15, a2, 4
	ee.ldf.128.ip f11, f10, f9, f8, a15, 16
	srli a14, a7, 2
	loopnez a14, .Ltg_fir_f32_second_fir_loop_2
	madd.s f4, f2, f8
	madd.s f5, f3, f9
	ee.ldf.128.ip f3, f2, f1, f0, a10, 16
	madd.s f6, f0, f10
	madd.s f7, f1, f11
	ee.ldf.128.ip f11, f10, f9, f8, a15, 16
.Ltg_fir_f32_second_fir_loop_2:
	madd.s f4, f2, f12
	madd.s f5, f3, f13
	j .Ltg_fir_f32_store_fir_result
.Ltg_fir_f32_offset_3:
	sub a14, a6, a7
	addi a14, a14, 3
	srli a14, a14, 2
	ee.ldf.128.ip f11, f14, f13, f12, a15, 16
	loopnez a14, .Ltg_fir_f32_first_fir_loop_3
	ee.ldf.128.ip f3, f2, f1, f0, a10, 16
	madd.s f4, f0, f11
	ee.ldf.128.ip f11, f10, f9, f8, a15, 16
	madd.s f5, f1, f8
	madd.s f6, f2, f9
	madd.s f7, f3, f10
.Ltg_fir_f32_first_fir_loop_3:
	l32i a15, a2, 4
	ee.ldf.128.ip f11, f10, f9, f8, a15, 16
	srli a14, a7, 2
	loopnez a14, .Ltg_fir_f32_second_fir_loop_3
	madd.s f4, f1, f8
	madd.s f5, f2, f9
	madd.s f6, f3, f10
	ee.ldf.128.ip f3, f2, f1, f0, a10, 16
	madd.s f7, f0, f11
	ee.ldf.128.ip f11, f10, f9, f8, a15, 16
.Ltg_fir_f32_second_fir_loop_3:
	madd.s f4, f1, f12
	madd.s f5, f2, f13
	madd.s f4, f3, f14
.Ltg_fir_f32_store_fir_result:
	add.s f4, f4, f5
	add.s f6, f6, f7
	add.s f4, f4, f6
	ssip f4, a4, 4
	addi a5, a5, -1
	bnez a5, .Ltg_fir_f32_fir_loop_len
	s32i a7, a2, 12
	movi.n a2, 0
	retw.n
	.size tinygo_dsp_fir_f32, . - tinygo_dsp_fir_f32

	.section .text.tinygo_dsp_fird_f32,"ax",@progbits
	.global tinygo_dsp_fird_f32
	.type tinygo_dsp_fird_f32,@function
	.p2align 4
tinygo_dsp_fird_f32:
	entry a1, 16
	l32i a7, a2, 12
	l32i a6, a2, 8
	l32i a11, a2, 4
	addx4 a11, a7, a11
	mov.n a9, a5
	movi.n a12, -16
	movi.n a13, 15
.Ltg_fird_f32_fird_loop_len:
	l32i a14, a2, 16
	loopnez a14, .Ltg_fird_f32_fird_load_data
	lsip f15, a3, 4
	ssip f15, a11, 4
	addi a7, a7, 1
	blt a7, a6, .Ltg_fird_f32_do_not_reset_a11
	l32i a11, a2, 4
	movi a7, 0
.Ltg_fird_f32_do_not_reset_a11:
	and a15, a11, a12
.Ltg_fird_f32_fird_load_data:
	l32i a10, a2, 0
	const.s f4, 0
	const.s f5, 0
	const.s f6, 0
	const.s f7, 0
	and a8, a11, a13
	beqz a8, .Ltg_fird_f32_offset_0
	addi a8, a8, -4
	beqz a8, .Ltg_fird_f32_offset_1
	addi a8, a8, -4
	beqz a8, .Ltg_fird_f32_offset_2
	addi a8, a8, -4
	beqz a8, .Ltg_fird_f32_offset_3
.Ltg_fird_f32_offset_0:
	sub a14, a6, a7
	srli a14, a14, 2
	loopnez a14, .Ltg_fird_f32_first_fir_loop_0
	ee.ldf.128.ip f3, f2, f1, f0, a10, 16
	ee.ldf.128.ip f11, f10, f9, f8, a15, 16
	madd.s f4, f0, f8
	madd.s f5, f1, f9
	madd.s f6, f2, f10
	madd.s f7, f3, f11
.Ltg_fird_f32_first_fir_loop_0:
	l32i a15, a2, 4
	srli a14, a7, 2
	nop
	loopnez a14, .Ltg_fird_f32_second_fir_loop_0
	ee.ldf.128.ip f3, f2, f1, f0, a10, 16
	ee.ldf.128.ip f11, f10, f9, f8, a15, 16
	madd.s f4, f0, f8
	madd.s f5, f1, f9
	madd.s f6, f2, f10
	madd.s f7, f3, f11
.Ltg_fird_f32_second_fir_loop_0:
	j .Ltg_fird_f32_store_fir_result
.Ltg_fird_f32_offset_1:
	sub a14, a6, a7
	addi a14, a14, 3
	srli a14, a14, 2
	const.s f3, 0
	ee.ldf.128.ip f11, f10, f9, f12, a15, 16
	loopnez a14, .Ltg_fird_f32_first_fir_loop_1
	madd.s f4, f3, f8
	ee.ldf.128.ip f3, f2, f1, f0, a10, 16
	madd.s f5, f0, f9
	madd.s f6, f1, f10
	madd.s f7, f2, f11
	ee.ldf.128.ip f11, f10, f9, f8, a15, 16
.Ltg_fird_f32_first_fir_loop_1:
	l32i a15, a2, 4
	srli a14, a7, 2
	loopnez a14, .Ltg_fird_f32_second_fir_loop_1
	ee.ldf.128.ip f11, f10, f9, f8, a15, 16
	madd.s f4, f3, f8
	ee.ldf.128.ip f3, f2, f1, f0, a10, 16
	madd.s f5, f0, f9
	madd.s f6, f1, f10
	madd.s f7, f2, f11
.Ltg_fird_f32_second_fir_loop_1:
	madd.s f4, f3, f12
	j .Ltg_fird_f32_store_fir_result
.Ltg_fird_f32_offset_2:
	sub a14, a6, a7
	addi a14, a14, 3
	srli a14, a14, 2
	const.s f2, 0
	const.s f3, 0
	ee.ldf.128.ip f11, f10, f13, f12, a15, 16
	loopnez a14, .Ltg_fird_f32_first_fir_loop_2
	madd.s f4, f2, f8
	madd.s f5, f3, f9
	ee.ldf.128.ip f3, f2, f1, f0, a10, 16
	madd.s f6, f0, f10
	madd.s f7, f1, f11
	ee.ldf.128.ip f11, f10, f9, f8, a15, 16
.Ltg_fird_f32_first_fir_loop_2:
	l32i a15, a2, 4
	srli a14, a7, 2
	loopnez a14, .Ltg_fird_f32_second_fir_loop_2
	ee.ldf.128.ip f11, f10, f9, f8, a15, 16
	madd.s f4, f2, f8
	madd.s f5, f3, f9
	ee.ldf.128.ip f3, f2, f1, f0, a10, 16
	madd.s f6, f0, f10
	madd.s f7, f1, f11
.Ltg_fird_f32_second_fir_loop_2:
	madd.s f4, f2, f12
	madd.s f5, f3, f13
	j .Ltg_fird_f32_store_fir_result
.Ltg_fird_f32_offset_3:
	sub a14, a6, a7
	addi a14, a14, 3
	srli a14, a14, 2
	const.s f1, 0
	const.s f2, 0
	const.s f3, 0
	ee.ldf.128.ip f11, f14, f13, f12, a15, 16
	loopnez a14, .Ltg_fird_f32_first_fir_loop_3
	madd.s f4, f1, f8
	madd.s f5, f2, f9
	madd.s f6, f3, f10
	ee.ldf.128.ip f3, f2, f1, f0, a10, 16
	madd.s f7, f0, f11
	ee.ldf.128.ip f11, f10, f9, f8, a15, 16
.Ltg_fird_f32_first_fir_loop_3:
	l32i a15, a2, 4
	srli a14, a7, 2
	loopnez a14, .Ltg_fird_f32_second_fir_loop_3
	ee.ldf.128.ip f11, f10, f9, f8, a15, 16
	madd.s f4, f1, f8
	madd.s f5, f2, f9
	madd.s f6, f3, f10
	ee.ldf.128.ip f3, f2, f1, f0, a10, 16
	madd.s f7, f0, f11
.Ltg_fird_f32_second_fir_loop_3:
	madd.s f4, f1, f12
	madd.s f5, f2, f13
	madd.s f6, f3, f14
.Ltg_fird_f32_store_fir_result:
	add.s f4, f4, f5
	add.s f6, f6, f7
	add.s f4, f4, f6
	ssip f4, a4, 4
	addi a5, a5, -1
	bnez a5, .Ltg_fird_f32_fird_loop_len
	s32i a7, a2, 12
	mov.n a2, a9
	retw.n
	.size tinygo_dsp_fird_f32, . - tinygo_dsp_fird_f32

	.section .text.tinygo_dsp_biquad_f32,"ax",@progbits
	.global tinygo_dsp_biquad_f32
	.type tinygo_dsp_biquad_f32,@function
	.p2align 4
tinygo_dsp_biquad_f32:
	entry a1, 16
	lsi f0, a5, 0
	lsi f1, a5, 4
	lsi f2, a5, 8
	lsi f3, a5, 12
	lsi f4, a5, 16
	neg.s f5, f3
	neg.s f6, f4
	lsi f7, a6, 0
	lsi f8, a6, 4
	addi a3, a3, -4
	lsi f9, a2, 0
	loopnez a4, .Ltg_biquad_f32_loop_bq_end_m_aes3
	madd.s f9, f7, f5
	addi a3, a3, 4
	mul.s f10, f1, f7
	madd.s f9, f8, f6
	madd.s f10, f9, f0
	addi a2, a2, 4
	madd.s f10, f2, f8
	mov.s f8, f7
	mov.s f7, f9
	lsi f9, a2, 0
	ssi f10, a3, 0
.Ltg_biquad_f32_loop_bq_end_m_aes3:
	ssi f7, a6, 0
	ssi f8, a6, 4
	movi.n a2, 0
	retw.n
	.size tinygo_dsp_biquad_f32, . - tinygo_dsp_biquad_f32

	.section .text.tinygo_dsp_mult_f32,"ax",@progbits
	.global tinygo_dsp_mult_f32
	.type tinygo_dsp_mult_f32,@function
	.p2align 4
tinygo_dsp_mult_f32:
	entry a1, 16
	movi.n a14, 0
	slli a12, a7, 2
	slli a10, a7, 2
	srli a11, a6, 2
	addi.n a11, a11, -1
	movi.n a15, 0
	mov a13, a3
	mov a7, a4
.Ltg_mult_f32_loop_x_aes3:
	movi.n a9, 0
	mov a8, a2
.Ltg_mult_f32_loop_y_aes3:
	add a13, a3, a14
	ee.ldf.128.ip f11, f10, f9, f8, a8, 16
	ee.ldf.128.xp f7, f6, f5, f4, a13, a12
	mul.s f0, f4, f8
	mul.s f1, f5, f8
	mul.s f2, f6, f8
	mul.s f3, f7, f8
	ee.ldf.128.xp f7, f6, f5, f4, a13, a12
	madd.s f0, f4, f9
	madd.s f1, f5, f9
	madd.s f2, f6, f9
	madd.s f3, f7, f9
	ee.ldf.128.xp f7, f6, f5, f4, a13, a12
	madd.s f0, f4, f10
	madd.s f1, f5, f10
	madd.s f2, f6, f10
	madd.s f3, f7, f10
	ee.ldf.128.xp f7, f6, f5, f4, a13, a12
	madd.s f0, f4, f11
	madd.s f1, f5, f11
	madd.s f2, f6, f11
	madd.s f3, f7, f11
	loopnez a11, .Ltg_mult_f32_loop_end_m_aes3
	ee.ldf.128.ip f11, f10, f9, f8, a8, 16
	ee.ldf.128.xp f7, f6, f5, f4, a13, a12
	madd.s f0, f4, f8
	madd.s f1, f5, f8
	madd.s f2, f6, f8
	madd.s f3, f7, f8
	ee.ldf.128.xp f7, f6, f5, f4, a13, a12
	madd.s f0, f4, f9
	madd.s f1, f5, f9
	madd.s f2, f6, f9
	madd.s f3, f7, f9
	ee.ldf.128.xp f7, f6, f5, f4, a13, a12
	madd.s f0, f4, f10
	madd.s f1, f5, f10
	madd.s f2, f6, f10
	madd.s f3, f7, f10
	ee.ldf.128.xp f7, f6, f5, f4, a13, a12
	madd.s f0, f4, f11
	madd.s f1, f5, f11
	madd.s f2, f6, f11
	madd.s f3, f7, f11
.Ltg_mult_f32_loop_end_m_aes3:
	ee.stf.128.xp f3, f2, f1, f0, a4, a10
	addi a9, a9, 1
	bge a9, a5, 1f
	j .Ltg_mult_f32_loop_y_aes3
1:
	addi a7, a7, 16
	mov a4, a7
	addi a14, a14, 16
	addi a15, a15, 16
	bge a15, a12, 1f
	j .Ltg_mult_f32_loop_x_aes3
1:
	movi.n a2, 0
	retw.n
	.size tinygo_dsp_mult_f32, . - tinygo_dsp_mult_f32

	.section .text.tinygo_dsp_mult_ex_f32,"ax",@progbits
	.global tinygo_dsp_mult_ex_f32
	.type tinygo_dsp_mult_ex_f32,@function
	.p2align 4
tinygo_dsp_mult_ex_f32:
	entry a1, 16
	l32i.n a8, a1, 16
	l32i.n a9, a1, 20
	l32i.n a15, a1, 24
	movi.n a14, 0
	add a15, a15, a7
	slli a10, a15, 2
	mov a15, a9
	slli a15, a15, 2
	add a7, a7, a9
	slli a12, a7, 2
	srli a11, a6, 2
	addi.n a11, a11, -1
	slli a6, a8, 2
	mov a13, a3
	mov a7, a4
.Ltg_mult_ex_f32_loop_x_mult_ex:
	movi.n a9, 0
	mov a8, a2
.Ltg_mult_ex_f32_loop_y_mult_ex:
	add a13, a3, a14
	ee.ldf.128.ip f11, f10, f9, f8, a8, 16
	ee.ldf.128.xp f7, f6, f5, f4, a13, a12
	mul.s f0, f4, f8
	mul.s f1, f5, f8
	mul.s f2, f6, f8
	mul.s f3, f7, f8
	ee.ldf.128.xp f7, f6, f5, f4, a13, a12
	madd.s f0, f4, f9
	madd.s f1, f5, f9
	madd.s f2, f6, f9
	madd.s f3, f7, f9
	ee.ldf.128.xp f7, f6, f5, f4, a13, a12
	madd.s f0, f4, f10
	madd.s f1, f5, f10
	madd.s f2, f6, f10
	madd.s f3, f7, f10
	ee.ldf.128.xp f7, f6, f5, f4, a13, a12
	madd.s f0, f4, f11
	madd.s f1, f5, f11
	madd.s f2, f6, f11
	madd.s f3, f7, f11
	loopnez a11, .Ltg_mult_ex_f32_iner_loop_mult_ex
	ee.ldf.128.ip f11, f10, f9, f8, a8, 16
	ee.ldf.128.xp f7, f6, f5, f4, a13, a12
	madd.s f0, f4, f8
	madd.s f1, f5, f8
	madd.s f2, f6, f8
	madd.s f3, f7, f8
	ee.ldf.128.xp f7, f6, f5, f4, a13, a12
	madd.s f0, f4, f9
	madd.s f1, f5, f9
	madd.s f2, f6, f9
	madd.s f3, f7, f9
	ee.ldf.128.xp f7, f6, f5, f4, a13, a12
	madd.s f0, f4, f10
	madd.s f1, f5, f10
	madd.s f2, f6, f10
	madd.s f3, f7, f10
	ee.ldf.128.xp f7, f6, f5, f4, a13, a12
	madd.s f0, f4, f11
	madd.s f1, f5, f11
	madd.s f2, f6, f11
	madd.s f3, f7, f11
.Ltg_mult_ex_f32_iner_loop_mult_ex:
	ee.stf.128.xp f3, f2, f1, f0, a4, a10
	addi.n a9, a9, 1
	add a8, a8, a6
	bge a9, a5, 1f
	j .Ltg_mult_ex_f32_loop_y_mult_ex
1:
	addi a7, a7, 16
	mov a4, a7
	addi a14, a14, 16
	addi a15, a15, 16
	bge a15, a12, 1f
	j .Ltg_mult_ex_f32_loop_x_mult_ex
1:
	movi.n a2, 0
	retw.n
	.size tinygo_dsp_mult_ex_f32, . - tinygo_dsp_mult_ex_f32
