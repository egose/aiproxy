package filestore

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

type File struct {
	Path string
	Data []byte
	Mode os.FileMode
}

type Options struct {
	DirMode os.FileMode
	Secret  bool
}

type hookSet struct {
	afterWrite   func(string) error
	afterSync    func(string) error
	beforeRename func(string, string) error
}

type writer struct {
	hooks hookSet
}

func WriteFile(path string, data []byte, mode os.FileMode, opts Options) error {
	return writer{}.WriteFile(path, data, mode, opts)
}

func ReplaceFiles(files []File, opts Options) error {
	return writer{}.ReplaceFiles(files, opts)
}

func (w writer) WriteFile(path string, data []byte, mode os.FileMode, opts Options) error {
	prepared, err := w.prepare(path, data, mode, opts)
	if err != nil {
		return err
	}
	defer prepared.cleanup()
	if err := safeDestination(path, opts.Secret); err != nil {
		return err
	}
	if err := w.rename(prepared.temp, path); err != nil {
		return fmt.Errorf("rename %s: %w", path, err)
	}
	prepared.temp = ""
	return syncDir(filepath.Dir(path))
}

func (w writer) ReplaceFiles(files []File, opts Options) error {
	prepared := make([]preparedFile, 0, len(files))
	for _, file := range files {
		path := file.Path
		p, err := w.prepare(path, file.Data, file.Mode, opts)
		if err != nil {
			cleanupPrepared(prepared)
			return err
		}
		prepared = append(prepared, p)
	}
	defer cleanupPrepared(prepared)

	committed := make([]preparedFile, 0, len(prepared))
	for i := range prepared {
		p := &prepared[i]
		if err := safeDestination(p.path, opts.Secret); err != nil {
			rollback(committed, opts, w)
			return err
		}
		backup := p.temp + ".old"
		hadOld := false
		if _, err := os.Lstat(p.path); err == nil {
			if err := w.rename(p.path, backup); err != nil {
				rollback(committed, opts, w)
				return fmt.Errorf("backup %s: %w", p.path, err)
			}
			hadOld = true
		} else if !errors.Is(err, os.ErrNotExist) {
			rollback(committed, opts, w)
			return fmt.Errorf("stat %s: %w", p.path, err)
		}
		p.backup = backup
		p.hadOld = hadOld
		if err := w.rename(p.temp, p.path); err != nil {
			if hadOld {
				_ = os.Rename(backup, p.path)
			}
			rollback(committed, opts, w)
			return fmt.Errorf("rename %s: %w", p.path, err)
		}
		p.temp = ""
		committed = append(committed, *p)
	}

	seen := map[string]struct{}{}
	for _, p := range prepared {
		dir := filepath.Dir(p.path)
		if _, ok := seen[dir]; ok {
			continue
		}
		seen[dir] = struct{}{}
		if err := syncDir(dir); err != nil {
			return err
		}
	}
	return nil
}

type preparedFile struct {
	path   string
	temp   string
	backup string
	hadOld bool
}

func (w writer) prepare(path string, data []byte, mode os.FileMode, opts Options) (preparedFile, error) {
	if opts.DirMode == 0 {
		opts.DirMode = 0o755
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, opts.DirMode); err != nil {
		return preparedFile{}, fmt.Errorf("create parent directory for %s: %w", path, err)
	}
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return preparedFile{}, fmt.Errorf("create temp file for %s: %w", path, err)
	}
	temp := f.Name()
	ok := false
	defer func() {
		if !ok {
			_ = f.Close()
			_ = os.Remove(temp)
		}
	}()
	if _, err := f.Write(data); err != nil {
		return preparedFile{}, fmt.Errorf("write temp file for %s: %w", path, err)
	}
	if w.hooks.afterWrite != nil {
		if err := w.hooks.afterWrite(path); err != nil {
			return preparedFile{}, err
		}
	}
	if err := f.Chmod(mode); err != nil {
		return preparedFile{}, fmt.Errorf("chmod temp file for %s: %w", path, err)
	}
	if err := f.Sync(); err != nil {
		return preparedFile{}, fmt.Errorf("sync temp file for %s: %w", path, err)
	}
	if w.hooks.afterSync != nil {
		if err := w.hooks.afterSync(path); err != nil {
			return preparedFile{}, err
		}
	}
	if err := f.Close(); err != nil {
		return preparedFile{}, fmt.Errorf("close temp file for %s: %w", path, err)
	}
	ok = true
	return preparedFile{path: path, temp: temp}, nil
}

func (w writer) rename(oldPath, newPath string) error {
	if w.hooks.beforeRename != nil {
		if err := w.hooks.beforeRename(oldPath, newPath); err != nil {
			return err
		}
	}
	return os.Rename(oldPath, newPath)
}

func safeDestination(path string, secret bool) error {
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("stat %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refuse to replace symlink %s", path)
	}
	if secret && !info.Mode().IsRegular() {
		return fmt.Errorf("refuse to replace non-regular file %s", path)
	}
	return nil
}

func syncDir(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open directory %s: %w", path, err)
	}
	defer dir.Close()
	if err := dir.Sync(); err != nil && !errors.Is(err, syscall.EINVAL) && !errors.Is(err, syscall.ENOTSUP) {
		return fmt.Errorf("sync directory %s: %w", path, err)
	}
	return nil
}

func cleanupPrepared(prepared []preparedFile) {
	for _, p := range prepared {
		p.cleanup()
	}
}

func (p preparedFile) cleanup() {
	if p.temp != "" {
		_ = os.Remove(p.temp)
	}
	if p.backup != "" {
		_ = os.Remove(p.backup)
	}
}

func rollback(committed []preparedFile, opts Options, w writer) {
	for i := len(committed) - 1; i >= 0; i-- {
		p := committed[i]
		_ = os.Remove(p.path)
		if p.hadOld {
			_ = w.rename(p.backup, p.path)
		}
	}
}
