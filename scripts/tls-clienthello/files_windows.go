package main

import (
	"fmt"
	"os"
	"syscall"
)

func openRegularRead(path string) (*os.File, error) {
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.FILE_FLAG_OVERLAPPED, 0)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		file.Close()
		return nil, fmt.Errorf("not a regular file")
	}
	return file, nil
}
