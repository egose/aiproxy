package copilotlogin

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/egose/aiproxy/internal/filestore"
)

func writeSecretFile(dest string, body []byte) error {
	if err := filestore.WriteFile(dest, body, 0o600, filestore.Options{DirMode: 0o700, Secret: true}); err != nil {
		return fmt.Errorf("persist credential: %w", sanitizePathError(dest, err))
	}
	return nil
}

func sanitizePathError(dest string, err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	_ = dest
	if len(msg) > 300 {
		return fmt.Errorf("%s", msg[:300])
	}
	return err
}

func EnsureParentDir(path string) error {
	dir := filepath.Dir(path)
	if dir == "" || dir == "." {
		return nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create credential directory: %w", err)
	}
	return nil
}
