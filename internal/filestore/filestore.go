package filestore

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	beforeRemove func(string) error
	afterDirSync func(string) error
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
	defer func() {
		if prepared.temp != "" {
			_ = w.remove(prepared.temp)
		}
	}()
	if err := safeDestination(path, opts.Secret); err != nil {
		return err
	}
	if err := w.rename(prepared.temp, path); err != nil {
		return fmt.Errorf("rename %s: %w", path, err)
	}
	prepared.temp = ""
	return syncDir(filepath.Dir(path))
}

type RecoveryFailure struct {
	Path     string
	Op       string
	Err      error
	Retained string
}

type ReplaceError struct {
	Op         string
	Path       string
	Err        error
	Recoveries []RecoveryFailure
	Retained   []string
}

func (e *ReplaceError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s: %v", e.Op, e.Path, e.Err)
	for _, r := range e.Recoveries {
		fmt.Fprintf(&b, "; %s %s: %v", r.Op, r.Path, r.Err)
	}
	if len(e.Retained) > 0 {
		fmt.Fprintf(&b, "; retained recovery paths: %s", strings.Join(e.Retained, ", "))
	}
	return b.String()
}

func (e *ReplaceError) Unwrap() []error {
	errs := make([]error, 0, len(e.Recoveries)+1)
	if e.Err != nil {
		errs = append(errs, e.Err)
	}
	for _, r := range e.Recoveries {
		if r.Err != nil {
			errs = append(errs, r.Err)
		}
	}
	return errs
}

func (w writer) ReplaceFiles(files []File, opts Options) error {
	prepared := make([]preparedFile, 0, len(files))
	for _, file := range files {
		path := file.Path
		p, err := w.prepare(path, file.Data, file.Mode, opts)
		if err != nil {
			for _, q := range prepared {
				if q.temp != "" {
					_ = w.remove(q.temp)
				}
			}
			return err
		}
		prepared = append(prepared, p)
	}

	committed := make([]preparedFile, 0, len(prepared))
	for i := range prepared {
		p := &prepared[i]
		if err := safeDestination(p.path, opts.Secret); err != nil {
			recoveries, retained := w.restoreCommitted(committed)
			w.discardTemps(prepared[i:])
			return &ReplaceError{Op: "verify", Path: p.path, Err: err, Recoveries: recoveries, Retained: retained}
		}
		backup := p.temp + ".old"
		hadOld := false
		if _, err := os.Lstat(p.path); err == nil {
			if err := w.rename(p.path, backup); err != nil {
				recoveries, retained := w.restoreCommitted(committed)
				w.discardTemps(prepared[i:])
				return &ReplaceError{Op: "backup", Path: p.path, Err: err, Recoveries: recoveries, Retained: retained}
			}
			hadOld = true
		} else if !errors.Is(err, os.ErrNotExist) {
			recoveries, retained := w.restoreCommitted(committed)
			w.discardTemps(prepared[i:])
			return &ReplaceError{Op: "stat", Path: p.path, Err: err, Recoveries: recoveries, Retained: retained}
		}
		p.backup = backup
		p.hadOld = hadOld
		if err := w.rename(p.temp, p.path); err != nil {
			var recoveries []RecoveryFailure
			var retained []string
			if hadOld {
				if rerr := w.rename(p.backup, p.path); rerr != nil {
					recoveries = append(recoveries, RecoveryFailure{Path: p.path, Op: "restore", Err: rerr, Retained: p.backup})
					retained = append(retained, p.backup, p.temp)
				} else {
					p.backup = ""
					_ = w.remove(p.temp)
					p.temp = ""
				}
			} else {
				retained = append(retained, p.temp)
				p.temp = ""
			}
			cRec, cRet := w.restoreCommitted(committed)
			recoveries = append(recoveries, cRec...)
			retained = append(retained, cRet...)
			w.discardTemps(prepared[i+1:])
			return &ReplaceError{Op: "rename", Path: p.path, Err: err, Recoveries: recoveries, Retained: retained}
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
		if err := w.syncDir(dir); err != nil {
			var retained []string
			for _, q := range committed {
				if q.hadOld {
					retained = append(retained, q.backup)
				}
			}
			return &ReplaceError{Op: "sync", Path: dir, Err: err, Retained: retained}
		}
	}
	if err := w.removeBackups(committed); err != nil {
		return err
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

func (w writer) remove(path string) error {
	if w.hooks.beforeRemove != nil {
		if err := w.hooks.beforeRemove(path); err != nil {
			return err
		}
	}
	return os.Remove(path)
}

func (w writer) restoreCommitted(committed []preparedFile) ([]RecoveryFailure, []string) {
	var recoveries []RecoveryFailure
	var retained []string
	for i := len(committed) - 1; i >= 0; i-- {
		p := committed[i]
		if !p.hadOld {
			if err := w.remove(p.path); err != nil {
				recoveries = append(recoveries, RecoveryFailure{Path: p.path, Op: "remove-new", Err: err, Retained: p.path})
				retained = append(retained, p.path)
			}
			continue
		}
		if err := w.rename(p.backup, p.path); err != nil {
			recoveries = append(recoveries, RecoveryFailure{Path: p.path, Op: "restore", Err: err, Retained: p.backup})
			retained = append(retained, p.backup)
		}
	}
	return recoveries, retained
}

func (w writer) discardTemps(files []preparedFile) {
	for _, p := range files {
		if p.temp != "" {
			_ = w.remove(p.temp)
		}
	}
}

func (w writer) removeBackups(committed []preparedFile) error {
	var recoveries []RecoveryFailure
	var retained []string
	var firstErr error
	var firstPath string
	for _, p := range committed {
		if !p.hadOld || p.backup == "" {
			continue
		}
		if err := w.remove(p.backup); err != nil {
			if firstErr == nil {
				firstErr = err
				firstPath = p.backup
			}
			recoveries = append(recoveries, RecoveryFailure{Path: p.path, Op: "cleanup", Err: err, Retained: p.backup})
			retained = append(retained, p.backup)
		}
	}
	if firstErr != nil {
		return &ReplaceError{Op: "cleanup", Path: firstPath, Err: firstErr, Recoveries: recoveries, Retained: retained}
	}
	return nil
}

func (w writer) syncDir(path string) error {
	if err := syncDir(path); err != nil {
		return err
	}
	if w.hooks.afterDirSync != nil {
		if err := w.hooks.afterDirSync(path); err != nil {
			return err
		}
	}
	return nil
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
