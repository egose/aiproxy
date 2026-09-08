package filestore

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func seedFile(t *testing.T, path, contents string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), mode); err != nil {
		t.Fatalf("WriteFile(seed): %v", err)
	}
}

func replaceErr(t *testing.T, err error) *ReplaceError {
	t.Helper()
	if err == nil {
		t.Fatalf("ReplaceFiles() succeeded, want failure")
	}
	var rerr *ReplaceError
	if !errors.As(err, &rerr) {
		t.Fatalf("ReplaceFiles() error type = %T, want *ReplaceError", err)
	}
	return rerr
}

func assertNoBackupsOrTemps(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(): %v", err)
	}
	for _, e := range entries {
		name := e.Name()
		if strings.HasSuffix(name, ".old") {
			t.Fatalf("retained backup %s after success", name)
		}
		if strings.Contains(name, ".tmp-") {
			t.Fatalf("retained temp %s after success", name)
		}
	}
}

func TestReplaceFilesFailingFileRestoreFailureRetainsBothCopies(t *testing.T) {
	dir := t.TempDir()
	secrets := filepath.Join(dir, "keys.json")
	config := filepath.Join(dir, "config.hcl")
	seedFile(t, secrets, "old secrets\n", 0o600)
	seedFile(t, config, "old config\n", 0o600)
	publishErr := errors.New("publish boom")
	restoreErr := errors.New("restore boom")
	w := writer{hooks: hookSet{beforeRename: func(oldPath, newPath string) error {
		if newPath != config {
			return nil
		}
		if strings.HasSuffix(oldPath, ".old") {
			return restoreErr
		}
		return publishErr
	}}}
	err := w.ReplaceFiles([]File{
		{Path: secrets, Data: []byte("new secrets\n"), Mode: 0o600},
		{Path: config, Data: []byte("new config\n"), Mode: 0o600},
	}, Options{DirMode: 0o700, Secret: true})
	rerr := replaceErr(t, err)
	if !errors.Is(err, publishErr) {
		t.Fatalf("primary error = %v, want publish boom", err)
	}
	if !errors.Is(err, restoreErr) {
		t.Fatalf("recovery error missing restore boom: %v", err)
	}
	if rerr.Op != "rename" || rerr.Path != config {
		t.Fatalf("Op/Path = %s %s, want rename %s", rerr.Op, rerr.Path, config)
	}
	if len(rerr.Retained) != 2 {
		t.Fatalf("Retained = %#v, want 2 paths", rerr.Retained)
	}
	assertFile(t, secrets, "old secrets\n")
	if _, statErr := os.Lstat(config); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("config stat = %v, want not exist", statErr)
	}
	oldFound := false
	newFound := false
	for _, p := range rerr.Retained {
		data, readErr := os.ReadFile(p)
		if readErr != nil {
			t.Fatalf("ReadFile(retained %s): %v", p, readErr)
		}
		switch string(data) {
		case "old config\n":
			oldFound = true
		case "new config\n":
			newFound = true
		default:
			t.Fatalf("retained %s has unexpected contents %q", p, data)
		}
	}
	if !oldFound || !newFound {
		t.Fatalf("retained copies old=%v new=%v, want both", oldFound, newFound)
	}
	msg := rerr.Error()
	if !strings.Contains(msg, "retained recovery paths") {
		t.Fatalf("error missing retained paths: %q", msg)
	}
	if strings.Contains(msg, "old secrets") || strings.Contains(msg, "new secrets") || strings.Contains(msg, "old config\n") {
		t.Fatalf("error exposes contents: %q", msg)
	}
	info, statErr := os.Stat(secrets)
	if statErr != nil {
		t.Fatalf("Stat(secrets): %v", statErr)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("secrets mode = %o, want 600", got)
	}
}

func TestReplaceFilesEarlierCommittedRestoreFailure(t *testing.T) {
	dir := t.TempDir()
	secrets := filepath.Join(dir, "keys.json")
	config := filepath.Join(dir, "config.hcl")
	seedFile(t, secrets, "old secrets\n", 0o600)
	seedFile(t, config, "old config\n", 0o600)
	publishErr := errors.New("config publish boom")
	restoreErr := errors.New("secrets restore boom")
	w := writer{hooks: hookSet{beforeRename: func(oldPath, newPath string) error {
		if strings.HasSuffix(oldPath, ".old") && newPath == secrets {
			return restoreErr
		}
		if newPath == config && !strings.HasSuffix(oldPath, ".old") {
			return publishErr
		}
		return nil
	}}}
	err := w.ReplaceFiles([]File{
		{Path: secrets, Data: []byte("new secrets\n"), Mode: 0o600},
		{Path: config, Data: []byte("new config\n"), Mode: 0o600},
	}, Options{DirMode: 0o700, Secret: true})
	rerr := replaceErr(t, err)
	if !errors.Is(err, publishErr) || !errors.Is(err, restoreErr) {
		t.Fatalf("error = %v, want primary+recovery", err)
	}
	if len(rerr.Recoveries) != 1 || rerr.Recoveries[0].Path != secrets || rerr.Recoveries[0].Op != "restore" {
		t.Fatalf("Recoveries = %#v, want single secrets restore", rerr.Recoveries)
	}
	assertFile(t, secrets, "new secrets\n")
	assertFile(t, config, "old config\n")
	backupData := false
	for _, p := range rerr.Retained {
		data, readErr := os.ReadFile(p)
		if readErr != nil {
			t.Fatalf("ReadFile(retained): %v", readErr)
		}
		if string(data) == "old secrets\n" {
			backupData = true
		}
	}
	if !backupData {
		t.Fatalf("Retained = %#v, want old secrets backup", rerr.Retained)
	}
}

func TestReplaceFilesSuccessfulRollbackRemovesStaging(t *testing.T) {
	dir := t.TempDir()
	secrets := filepath.Join(dir, "keys.json")
	config := filepath.Join(dir, "config.hcl")
	seedFile(t, secrets, "old secrets\n", 0o600)
	seedFile(t, config, "old config\n", 0o600)
	injected := errors.New("injected rename failure")
	w := writer{hooks: hookSet{beforeRename: func(oldPath, newPath string) error {
		if newPath == config && !strings.HasSuffix(oldPath, ".old") {
			return injected
		}
		return nil
	}}}
	err := w.ReplaceFiles([]File{
		{Path: secrets, Data: []byte("new secrets\n"), Mode: 0o600},
		{Path: config, Data: []byte("new config\n"), Mode: 0o600},
	}, Options{DirMode: 0o700, Secret: true})
	if !errors.Is(err, injected) {
		t.Fatalf("error = %v, want injected", err)
	}
	var rerr *ReplaceError
	if !errors.As(err, &rerr) {
		t.Fatalf("error type = %T, want *ReplaceError", err)
	}
	if len(rerr.Recoveries) != 0 {
		t.Fatalf("Recoveries = %#v, want none", rerr.Recoveries)
	}
	assertFile(t, secrets, "old secrets\n")
	assertFile(t, config, "old config\n")
	assertNoBackupsOrTemps(t, dir)
}

func TestReplaceFilesAbsentOriginalsRollback(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "a.hcl")
	second := filepath.Join(dir, "b.hcl")
	injected := errors.New("second publish boom")
	w := writer{hooks: hookSet{beforeRename: func(_, newPath string) error {
		if newPath == second {
			return injected
		}
		return nil
	}}}
	err := w.ReplaceFiles([]File{
		{Path: first, Data: []byte("new a\n"), Mode: 0o600},
		{Path: second, Data: []byte("new b\n"), Mode: 0o600},
	}, Options{DirMode: 0o700, Secret: true})
	rerr := replaceErr(t, err)
	if !errors.Is(err, injected) {
		t.Fatalf("error = %v, want injected", err)
	}
	if _, statErr := os.Lstat(first); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("first stat = %v, want absent after remove-new", statErr)
	}
	if _, statErr := os.Lstat(second); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("second stat = %v, want absent", statErr)
	}
	if len(rerr.Retained) != 1 {
		t.Fatalf("Retained = %#v, want single temp", rerr.Retained)
	}
	assertFile(t, rerr.Retained[0], "new b\n")
	_ = os.Remove(rerr.Retained[0])
}

func TestReplaceFilesAbsentOriginalRemoveFailureRetainsNew(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "a.hcl")
	second := filepath.Join(dir, "b.hcl")
	publishErr := errors.New("second publish boom")
	removeErr := errors.New("remove-new boom")
	w := writer{hooks: hookSet{
		beforeRename: func(_, newPath string) error {
			if newPath == second {
				return publishErr
			}
			return nil
		},
		beforeRemove: func(path string) error {
			if path == first {
				return removeErr
			}
			return nil
		},
	}}
	err := w.ReplaceFiles([]File{
		{Path: first, Data: []byte("new a\n"), Mode: 0o600},
		{Path: second, Data: []byte("new b\n"), Mode: 0o600},
	}, Options{DirMode: 0o700, Secret: true})
	rerr := replaceErr(t, err)
	if !errors.Is(err, publishErr) || !errors.Is(err, removeErr) {
		t.Fatalf("error = %v, want primary+remove-new", err)
	}
	assertFile(t, first, "new a\n")
	if len(rerr.Retained) != 2 {
		t.Fatalf("Retained = %#v, want first dest + second temp", rerr.Retained)
	}
}

func TestReplaceFilesSuccessCleansBackups(t *testing.T) {
	dir := t.TempDir()
	secrets := filepath.Join(dir, "keys.json")
	config := filepath.Join(dir, "config.hcl")
	seedFile(t, secrets, "old secrets\n", 0o600)
	seedFile(t, config, "old config\n", 0o600)
	if err := ReplaceFiles([]File{
		{Path: secrets, Data: []byte("new secrets\n"), Mode: 0o600},
		{Path: config, Data: []byte("new config\n"), Mode: 0o600},
	}, Options{DirMode: 0o700, Secret: true}); err != nil {
		t.Fatalf("ReplaceFiles(): %v", err)
	}
	assertFile(t, secrets, "new secrets\n")
	assertFile(t, config, "new config\n")
	assertNoBackupsOrTemps(t, dir)
	for _, p := range []string{secrets, config} {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatalf("Stat(): %v", err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Fatalf("%s mode = %o, want 600", p, got)
		}
	}
}

func TestReplaceFilesSyncFailureRetainsBackups(t *testing.T) {
	dir := t.TempDir()
	secrets := filepath.Join(dir, "keys.json")
	config := filepath.Join(dir, "config.hcl")
	seedFile(t, secrets, "old secrets\n", 0o600)
	seedFile(t, config, "old config\n", 0o600)
	injected := errors.New("injected dir sync failure")
	w := writer{hooks: hookSet{afterDirSync: func(string) error { return injected }}}
	err := w.ReplaceFiles([]File{
		{Path: secrets, Data: []byte("new secrets\n"), Mode: 0o600},
		{Path: config, Data: []byte("new config\n"), Mode: 0o600},
	}, Options{DirMode: 0o700, Secret: true})
	rerr := replaceErr(t, err)
	if !errors.Is(err, injected) {
		t.Fatalf("error = %v, want injected", err)
	}
	if rerr.Op != "sync" {
		t.Fatalf("Op = %s, want sync", rerr.Op)
	}
	assertFile(t, secrets, "new secrets\n")
	assertFile(t, config, "new config\n")
	if len(rerr.Retained) != 2 {
		t.Fatalf("Retained = %#v, want 2 backups", rerr.Retained)
	}
	for _, p := range rerr.Retained {
		if _, statErr := os.Lstat(p); statErr != nil {
			t.Fatalf("retained %s missing: %v", p, statErr)
		}
	}
}

func TestReplaceFilesCleanupFailureSurfacesRetained(t *testing.T) {
	dir := t.TempDir()
	secrets := filepath.Join(dir, "keys.json")
	seedFile(t, secrets, "old secrets\n", 0o600)
	injected := errors.New("cleanup boom")
	calls := 0
	w := writer{hooks: hookSet{beforeRemove: func(path string) error {
		if strings.HasSuffix(path, ".old") {
			calls++
			return injected
		}
		return nil
	}}}
	err := w.ReplaceFiles([]File{
		{Path: secrets, Data: []byte("new secrets\n"), Mode: 0o600},
	}, Options{DirMode: 0o700, Secret: true})
	rerr := replaceErr(t, err)
	if !errors.Is(err, injected) {
		t.Fatalf("error = %v, want cleanup boom", err)
	}
	if rerr.Op != "cleanup" {
		t.Fatalf("Op = %s, want cleanup", rerr.Op)
	}
	assertFile(t, secrets, "new secrets\n")
	if calls != 1 {
		t.Fatalf("cleanup calls = %d, want 1", calls)
	}
	if len(rerr.Retained) != 1 {
		t.Fatalf("Retained = %#v, want 1", rerr.Retained)
	}
}
