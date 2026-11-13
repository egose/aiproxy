//go:build !aix && !android && !darwin && !dragonfly && !freebsd && !illumos && !ios && !linux && !netbsd && !openbsd && !solaris && !windows

package main

import (
	"fmt"
	"os"
)

func openRegularRead(path string) (*os.File, error) {
	return nil, fmt.Errorf("regular-file input is unsupported on this platform")
}
