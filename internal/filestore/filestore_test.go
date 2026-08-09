package filestore

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestWriteFileTightensModeAndRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "token")
	if err := os.WriteFile(path, []byte("old\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(seed): %v", err)
	}
	if err := WriteFile(path, []byte("new\n"), 0o600, Options{DirMode: 0o700, Secret: true}); err != nil {
		t.Fatalf("WriteFile(): %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(): %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("mode = %o, want 600", got)
	}

	target := filepath.Join(dir, "target")
	link := filepath.Join(dir, "link")
	if err := os.WriteFile(target, []byte("target\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(target): %v", err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("Symlink(): %v", err)
	}
	if err := WriteFile(link, []byte("bad\n"), 0o600, Options{DirMode: 0o700, Secret: true}); err == nil {
		t.Fatalf("WriteFile(symlink) succeeded")
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile(target): %v", err)
	}
	if string(data) != "target\n" {
		t.Fatalf("target overwritten: %q", data)
	}
}

func TestReplaceFilesRollsBackRenameFailure(t *testing.T) {
	dir := t.TempDir()
	secrets := filepath.Join(dir, "keys.json")
	config := filepath.Join(dir, "config.hcl")
	if err := os.WriteFile(secrets, []byte("old secrets\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(secrets): %v", err)
	}
	if err := os.WriteFile(config, []byte("old config\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(config): %v", err)
	}
	injected := errors.New("injected rename failure")
	w := writer{hooks: hookSet{beforeRename: func(_, newPath string) error {
		if newPath == config {
			return injected
		}
		return nil
	}}}
	err := w.ReplaceFiles([]File{
		{Path: secrets, Data: []byte("new secrets\n"), Mode: 0o600},
		{Path: config, Data: []byte("new config\n"), Mode: 0o600},
	}, Options{DirMode: 0o700, Secret: true})
	if !errors.Is(err, injected) {
		t.Fatalf("ReplaceFiles() error = %v, want injected", err)
	}
	assertFile(t, secrets, "old secrets\n")
	assertFile(t, config, "old config\n")
}

func TestWriteFileInjectedFailuresKeepOldContents(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secret")
	if err := os.WriteFile(path, []byte("old\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(seed): %v", err)
	}
	injected := errors.New("injected")
	for name, hooks := range map[string]hookSet{
		"write":  {afterWrite: func(string) error { return injected }},
		"sync":   {afterSync: func(string) error { return injected }},
		"rename": {beforeRename: func(_, _ string) error { return injected }},
	} {
		t.Run(name, func(t *testing.T) {
			w := writer{hooks: hooks}
			err := w.WriteFile(path, []byte("new\n"), 0o600, Options{DirMode: 0o700, Secret: true})
			if !errors.Is(err, injected) {
				t.Fatalf("WriteFile() error = %v, want injected", err)
			}
			assertFile(t, path, "old\n")
		})
	}
}

func TestConcurrentWritersDoNotExposePartialContents(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	old := repeated('a', 64*1024)
	newValue := repeated('b', 64*1024)
	if err := WriteFile(path, []byte(old), 0o600, Options{DirMode: 0o700, Secret: true}); err != nil {
		t.Fatalf("WriteFile(seed): %v", err)
	}
	stop := make(chan struct{})
	errCh := make(chan string, 1)
	var wg sync.WaitGroup
	writers := 8
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				if err := WriteFile(path, []byte(newValue), 0o600, Options{DirMode: 0o700, Secret: true}); err != nil {
					select {
					case errCh <- err.Error():
					default:
					}
					return
				}
			}
		}()
	}
	for i := 0; i < 200; i++ {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile(): %v", err)
		}
		text := string(data)
		if text != old && text != newValue {
			t.Fatalf("observed partial contents of length %d", len(data))
		}
	}
	close(stop)
	wg.Wait()
	select {
	case err := <-errCh:
		t.Fatalf("writer error: %s", err)
	default:
	}
}

func assertFile(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	if string(data) != want {
		t.Fatalf("%s = %q, want %q", path, data, want)
	}
}

func repeated(b byte, n int) string {
	data := make([]byte, n)
	for i := range data {
		data[i] = b
	}
	return string(data)
}
