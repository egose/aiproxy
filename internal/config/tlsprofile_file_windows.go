package config

import (
	"os"
	"syscall"
)

func openTLSProfileFile(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|syscall.FILE_FLAG_OVERLAPPED, 0)
}
