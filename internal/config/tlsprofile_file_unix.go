//go:build unix

package config

import (
	"os"
	"syscall"
)

func openTLSProfileFile(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOCTTY, 0)
}
