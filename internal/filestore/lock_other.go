//go:build !unix && !windows

package filestore

func Lock(path string) (func(), error) {
	mu := registryMutexFor(path)
	mu.Lock()
	return func() { mu.Unlock() }, nil
}
