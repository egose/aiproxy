//go:build unix

package filestore

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

func Lock(path string) (func(), error) {
	mu := registryMutexFor(path)
	mu.Lock()
	lockPath := lockFilePath(path)
	f, err := openLockFile(lockPath, filepath.Dir(lockPath), 0o700)
	if err != nil {
		mu.Unlock()
		return nil, fmt.Errorf("open lock %s: %w", lockPath, err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		mu.Unlock()
		return nil, fmt.Errorf("lock %s: %w", path, err)
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
		mu.Unlock()
	}, nil
}

var _ = os.O_CREATE
