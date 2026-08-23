//go:build !windows

package app

import (
	"os"
	"syscall"
)

func reloadSignals() []os.Signal {
	return []os.Signal{syscall.SIGHUP}
}
