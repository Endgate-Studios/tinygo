//go:build !esp32s3

package aes

func (c *Cipher) encrypt(dst, src *[BlockSize]byte) { c.encryptSoft(dst, src) }
func (c *Cipher) decrypt(dst, src *[BlockSize]byte) { c.decryptSoft(dst, src) }
