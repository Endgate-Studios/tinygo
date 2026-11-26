//go:build linux && (amd64 || arm64)

// Package sysroots reads the trusted CA certificates of the system, for
// tls.Config.Roots on Linux. It does not allocate.
package sysroots

import (
	"unsafe"

	"tinygo/net/tls"
)

// Error is an error of sysroots.
type Error string

func (e Error) Error() string { return string(e) }

const (
	ErrNotFound Error = "sysroots: no CA bundle found"
	ErrTooLarge Error = "sysroots: the CA bundle is too large"
	ErrRead     Error = "sysroots: cannot read the CA bundle"
	ErrNone     Error = "sysroots: the CA bundle has no usable certificate"
)

// Files are the CA bundles of Linux distributions, in the order of Go
// (crypto/x509/root_linux.go).
var Files = [...]string{
	"/etc/ssl/certs/ca-certificates.crt",                // Debian, Ubuntu, Gentoo, Arch
	"/etc/pki/tls/certs/ca-bundle.crt",                  // Fedora, RHEL 6
	"/etc/ssl/ca-bundle.pem",                            // OpenSUSE
	"/etc/pki/tls/cacert.pem",                           // OpenELEC
	"/etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem", // CentOS, RHEL 7
	"/etc/ssl/cert.pem",                                 // Alpine
}

// Sizes of a Store. A distribution bundle has about 150 roots in 220 KB.
const (
	MaxRoots = 256
	maxPEM   = 512 << 10
	maxDER   = 384 << 10
	maxPath  = 256
)

// Store holds the roots and the DER bytes that they refer to. Put it in
// memory that you own, for example a global.
type Store struct {
	pem   [maxPEM]byte
	der   [maxDER]byte
	roots [MaxRoots]tls.PublicKey
	path  [maxPath]byte
}

// Load reads the first bundle of Files that exists.
func (s *Store) Load() ([]tls.PublicKey, error) {
	for _, f := range Files {
		roots, err := s.LoadFile(f)
		if err != ErrNotFound {
			return roots, err
		}
	}
	return nil, ErrNotFound
}

// LoadFile reads the PEM bundle at path. Certificates with a key type that
// tls does not support are skipped.
func (s *Store) LoadFile(path string) ([]tls.PublicKey, error) {
	if len(path) >= len(s.path) {
		return nil, ErrNotFound
	}
	n := copy(s.path[:], path)
	s.path[n] = 0
	fd := libc_open(&s.path[0], oRDONLY|oCLOEXEC)
	if fd < 0 {
		return nil, ErrNotFound
	}
	total := 0
	for {
		if total == len(s.pem) {
			libc_close(fd)
			return nil, ErrTooLarge
		}
		r := libc_read(fd, unsafe.Pointer(&s.pem[total]), uintptr(len(s.pem)-total))
		if r < 0 {
			libc_close(fd)
			return nil, ErrRead
		}
		if r == 0 {
			break
		}
		total += r
	}
	libc_close(fd)
	return s.parse(s.pem[:total])
}

func (s *Store) parse(pem []byte) ([]tls.PublicKey, error) {
	n, used := 0, 0
	for len(pem) > 0 && n < len(s.roots) {
		der, rest, ok := nextCertificate(pem, s.der[used:])
		if !ok {
			break
		}
		pem = rest
		if len(der) == 0 {
			continue
		}
		key, err := tls.AnchorFromCertificate(der)
		if err != nil {
			continue
		}
		s.roots[n] = key
		n++
		used += len(der)
	}
	if n == 0 {
		return nil, ErrNone
	}
	return s.roots[:n], nil
}

// nextCertificate decodes the next CERTIFICATE block of pem into dst. It
// gives an empty der for a block of another type or a bad block.
func nextCertificate(pem, dst []byte) (der, rest []byte, ok bool) {
	const begin, end, label = "-----BEGIN ", "-----END ", "CERTIFICATE-----"
	i := index(pem, begin)
	if i < 0 {
		return nil, nil, false
	}
	pem = pem[i+len(begin):]
	eol := index(pem, "\n")
	if eol < 0 {
		return nil, nil, false
	}
	name := pem[:eol]
	for len(name) > 0 && (name[len(name)-1] == '\r' || name[len(name)-1] == ' ') {
		name = name[:len(name)-1]
	}
	body := pem[eol+1:]
	j := index(body, end)
	if j < 0 {
		return nil, nil, false
	}
	rest = body[j+len(end):]
	if len(name) != len(label) || index(name, label) != 0 {
		return nil, rest, true
	}
	n, ok := decodeBase64(dst, body[:j])
	if !ok {
		return nil, rest, true
	}
	return dst[:n], rest, true
}

func index(b []byte, s string) int {
	for i := 0; i+len(s) <= len(b); i++ {
		match := true
		for k := 0; k < len(s); k++ {
			if b[i+k] != s[k] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

// decodeBase64 decodes padded standard base64 (RFC 4648 section 4) and
// skips white space.
func decodeBase64(dst, src []byte) (int, bool) {
	n, acc, bits := 0, uint32(0), 0
	for _, c := range src {
		var v byte
		switch {
		case c >= 'A' && c <= 'Z':
			v = c - 'A'
		case c >= 'a' && c <= 'z':
			v = c - 'a' + 26
		case c >= '0' && c <= '9':
			v = c - '0' + 52
		case c == '+':
			v = 62
		case c == '/':
			v = 63
		case c == '=' || c == '\n' || c == '\r' || c == ' ' || c == '\t':
			continue
		default:
			return 0, false
		}
		acc = acc<<6 | uint32(v)
		if bits += 6; bits >= 8 {
			bits -= 8
			if n >= len(dst) {
				return 0, false
			}
			dst[n] = byte(acc >> bits)
			n++
		}
	}
	return n, true
}

// The flags are the same in glibc and musl on amd64 and arm64.
const (
	oRDONLY  = 0
	oCLOEXEC = 0o2000000
)

// int open(const char *path, int flags);
//
//export open
//go:noescape
func libc_open(path *byte, flags int32) int32

// ssize_t read(int fd, void *buf, size_t n);
//
//export read
//go:noescape
func libc_read(fd int32, buf unsafe.Pointer, n uintptr) int

// int close(int fd);
//
//export close
func libc_close(fd int32) int32
