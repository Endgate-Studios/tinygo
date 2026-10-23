// Copyright 2022 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file replaces GOROOT's syscall/rlimit.go in Linux shared libraries.
// It has no init, because a library must not change the limits of its host.

//go:build unix

package syscall

import (
	"sync/atomic"
)

// origRlimitNofile, if non-nil, is the original soft RLIMIT_NOFILE.
var origRlimitNofile atomic.Pointer[Rlimit]

func Setrlimit(resource int, rlim *Rlimit) error {
	if resource == RLIMIT_NOFILE {
		// Store nil in origRlimitNofile to tell StartProcess
		// to not adjust the rlimit in the child process.
		origRlimitNofile.Store(nil)
	}
	return setrlimit(resource, rlim)
}
