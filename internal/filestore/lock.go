package filestore

import (
	"os"
	"path/filepath"
	"sync"
)

var (
	lockRegistryMu sync.Mutex
	lockRegistry   = map[string]*sync.Mutex{}
)

func registryMutexFor(path string) *sync.Mutex {
	key := path
	if abs, err := filepath.Abs(path); err == nil {
		key = abs
	}
	lockRegistryMu.Lock()
	defer lockRegistryMu.Unlock()
	mu, ok := lockRegistry[key]
	if !ok {
		mu = &sync.Mutex{}
		lockRegistry[key] = mu
	}
	return mu
}

func lockFilePath(path string) string {
	return path + ".lock"
}

func openLockFile(lockPath, dir string, dirMode os.FileMode) (*os.File, error) {
	if dirMode == 0 {
		dirMode = 0o700
	}
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return nil, err
	}
	return os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
}
