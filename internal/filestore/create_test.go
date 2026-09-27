package filestore

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestCreateFileConcurrentPublication(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	const publishers = 16
	var ready sync.WaitGroup
	ready.Add(publishers)
	w := writer{hooks: hookSet{afterSync: func(string) error {
		ready.Done()
		ready.Wait()
		return nil
	}}}
	type result struct {
		data string
		err  error
	}
	results := make(chan result, publishers)
	for i := 0; i < publishers; i++ {
		go func() {
			data := fmt.Sprintf("synthetic-secret-%d\n", i) + strings.Repeat("x", 64*1024)
			results <- result{data, w.CreateFile(path, []byte(data), 0o600, Options{Secret: true})}
		}()
	}
	winners := 0
	for i := 0; i < publishers; i++ {
		r := <-results
		if r.err == nil {
			winners++
			assertFile(t, path, r.data)
		} else if !errors.Is(r.err, os.ErrExist) {
			t.Errorf("losing publisher error = %v, want ErrExist", r.err)
		}
	}
	if winners != 1 {
		t.Fatalf("successful publishers = %d, want 1", winners)
	}
	assertMode(t, path, 0o600)
	assertNoStagedFiles(t, path)
}

func TestCreateFileLateDestinationDoesNotClobber(t *testing.T) {
	for _, kind := range []string{"regular", "live-link", "dangling-link", "directory"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "config.json")
			referent := filepath.Join(dir, "referent")
			w := writer{hooks: hookSet{beforeLink: func(staged, target string) error {
				assertFile(t, staged, "new secret")
				assertMode(t, staged, 0o600)
				switch kind {
				case "regular":
					return os.WriteFile(target, []byte("other publisher"), 0o644)
				case "live-link":
					if err := os.WriteFile(referent, []byte("other publisher"), 0o644); err != nil {
						return err
					}
					return os.Symlink(referent, target)
				case "dangling-link":
					return os.Symlink(referent, target)
				default:
					return os.Mkdir(target, 0o700)
				}
			}}}
			err := w.CreateFile(path, []byte("new secret"), 0o600, Options{Secret: true})
			if !errors.Is(err, os.ErrExist) {
				t.Fatalf("error = %v, want ErrExist", err)
			}
			switch kind {
			case "regular":
				assertFile(t, path, "other publisher")
			case "live-link", "dangling-link":
				link, err := os.Readlink(path)
				if err != nil || link != referent {
					t.Fatalf("symlink changed: %q, %v", link, err)
				}
				if kind == "live-link" {
					assertFile(t, referent, "other publisher")
				} else if _, err := os.Lstat(referent); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("dangling referent created: %v", err)
				}
			case "directory":
				info, err := os.Lstat(path)
				if err != nil || !info.IsDir() {
					t.Fatalf("directory changed: %v", err)
				}
			}
			assertNoStagedFiles(t, path)
		})
	}
}

func TestSingleFilePublicationFailures(t *testing.T) {
	injected := errors.New("injected publication failure")
	for _, exclusive := range []bool{false, true} {
		for _, existing := range []bool{false, true} {
			for _, phase := range []string{"write", "sync", "publish"} {
				t.Run(fmt.Sprintf("exclusive=%t/existing=%t/%s", exclusive, existing, phase), func(t *testing.T) {
					path := filepath.Join(t.TempDir(), "config.json")
					if existing {
						if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
							t.Fatal(err)
						}
						if err := os.Chmod(path, 0o644); err != nil {
							t.Fatal(err)
						}
					}
					failPublish := func(staged, target string) error {
						assertFile(t, staged, "new secret")
						assertMode(t, staged, 0o600)
						if existing {
							assertFile(t, target, "old")
						}
						return injected
					}
					hooks := map[string]hookSet{
						"write":   {afterWrite: func(string) error { return injected }},
						"sync":    {afterSync: func(string) error { return injected }},
						"publish": {beforeRename: failPublish, beforeLink: failPublish},
					}
					w := writer{hooks: hooks[phase]}
					err := w.writeFile(path, []byte("new secret"), 0o600, Options{Secret: true}, exclusive)
					if !errors.Is(err, injected) {
						t.Fatalf("error = %v, want injected", err)
					}
					if existing {
						assertFile(t, path, "old")
						assertMode(t, path, 0o644)
					} else if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
						t.Fatalf("failed write published: %v", err)
					}
					assertNoStagedFiles(t, path)
				})
			}
		}
	}
}

func TestCreateFileCleanupFailureReportsPublication(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	injected := errors.New("injected cleanup failure")
	w := writer{hooks: hookSet{beforeRemove: func(string) error { return injected }}}
	err := w.CreateFile(path, []byte("new secret"), 0o600, Options{Secret: true})
	if !errors.Is(err, injected) || !strings.Contains(err.Error(), "published") {
		t.Fatalf("error = %v, want published with cleanup failure", err)
	}
	assertFile(t, path, "new secret")
	assertMode(t, path, 0o600)
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".config.json.tmp-*"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("retained staging files = %v, %v", matches, err)
	}
	assertFile(t, matches[0], "new secret")
	assertMode(t, matches[0], 0o600)
}

func TestSingleFileDirectorySyncFailureReportsPublication(t *testing.T) {
	for _, exclusive := range []bool{false, true} {
		t.Run(fmt.Sprint(exclusive), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			injected := errors.New("injected directory sync failure")
			w := writer{hooks: hookSet{afterDirSync: func(string) error { return injected }}}
			err := w.writeFile(path, []byte("complete secret"), 0o600, Options{Secret: true}, exclusive)
			if !errors.Is(err, injected) || !strings.Contains(err.Error(), "published") {
				t.Fatalf("err=%v", err)
			}
			assertFile(t, path, "complete secret")
			assertMode(t, path, 0o600)
			assertNoStagedFiles(t, path)
		})
	}
}

func assertMode(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode() != mode {
		t.Fatalf("mode = %v, want %v", info.Mode(), mode)
	}
}

func assertNoStagedFiles(t *testing.T, path string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*"))
	if err != nil || len(matches) != 0 {
		t.Fatalf("staging files remain: %v, %v", matches, err)
	}
}
