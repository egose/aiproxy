package copilotlogin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestCredentialRoundTrip(t *testing.T) {
	dir := t.TempDir()
	secrets := filepath.Join(dir, "keys.json")
	cred, err := NewCredential("Ov23test", "gho_secret", time.Unix(1700000000, 0))
	if err != nil {
		t.Fatalf("NewCredential(): %v", err)
	}
	if cred.RefreshToken != cred.AccessToken || cred.ExpiresAt != 0 || cred.Domain != CredentialDomain {
		t.Fatalf("cred = %+v", cred)
	}
	if err := Save(secrets, "main", cred); err != nil {
		t.Fatalf("Save(): %v", err)
	}
	got, err := Load(secrets, "main")
	if err != nil {
		t.Fatalf("Load(): %v", err)
	}
	if got != cred {
		t.Fatalf("got %+v want %+v", got, cred)
	}
}

func TestSidecarDoesNotTouchFlatMap(t *testing.T) {
	dir := t.TempDir()
	secrets := filepath.Join(dir, "keys.json")
	flat := "{\n  \"openai\": \"sk-test\"\n}\n"
	if err := os.WriteFile(secrets, []byte(flat), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	cred, _ := NewCredential("Ov23test", "gho_secret", time.Now())
	if err := Save(secrets, "copilot-main", cred); err != nil {
		t.Fatalf("Save(): %v", err)
	}
	data, err := os.ReadFile(secrets)
	if err != nil {
		t.Fatalf("ReadFile(): %v", err)
	}
	if string(data) != flat {
		t.Fatalf("flat map modified: %q", data)
	}
	var m map[string]string
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("flat map no longer parses as map[string]string: %v", err)
	}
	sidecar, err := SidecarPath(secrets, "copilot-main")
	if err != nil {
		t.Fatalf("SidecarPath(): %v", err)
	}
	if sidecar == secrets {
		t.Fatalf("sidecar must differ from flat map")
	}
	raw, err := os.ReadFile(sidecar)
	if err != nil {
		t.Fatalf("ReadFile(sidecar): %v", err)
	}
	var stored Credential
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatalf("sidecar not structured: %v", err)
	}
	if stored.AccessToken != "gho_secret" {
		t.Fatalf("stored = %+v", stored)
	}
}

func TestCredentialFilePerms(t *testing.T) {
	dir := t.TempDir()
	secrets := filepath.Join(dir, "sub", "keys.json")
	cred, _ := NewCredential("Ov23test", "gho_secret", time.Now())
	if err := Save(secrets, "main", cred); err != nil {
		t.Fatalf("Save(): %v", err)
	}
	sidecar, _ := SidecarPath(secrets, "main")
	info, err := os.Stat(sidecar)
	if err != nil {
		t.Fatalf("Stat(): %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("mode = %o, want 600", got)
	}
	dirInfo, err := os.Stat(filepath.Dir(sidecar))
	if err != nil {
		t.Fatalf("Stat(dir): %v", err)
	}
	if got := dirInfo.Mode().Perm() & 0o777; got != 0o700 {
		t.Fatalf("dir mode = %o, want 700", got)
	}
}

func TestUnsafeCredentialNamesRejected(t *testing.T) {
	dir := t.TempDir()
	secrets := filepath.Join(dir, "keys.json")
	cred, _ := NewCredential("Ov23test", "gho_secret", time.Now())
	for _, name := range []string{"", "Bad Name", "a/b", "../evil", "/abs", "UPPER", ".hidden-ok-1", strings.Repeat("a", 200)} {
		if name == ".hidden-ok-1" {
			continue
		}
		if err := Save(secrets, name, cred); err == nil {
			t.Fatalf("Save(%q) succeeded", name)
		}
		if _, err := SidecarPath(secrets, name); err == nil {
			t.Fatalf("SidecarPath(%q) succeeded", name)
		}
	}
}

func TestSymlinkDestRefused(t *testing.T) {
	dir := t.TempDir()
	secrets := filepath.Join(dir, "keys.json")
	cred, _ := NewCredential("Ov23test", "gho_secret", time.Now())
	sidecar, err := SidecarPath(secrets, "main")
	if err != nil {
		t.Fatalf("SidecarPath(): %v", err)
	}
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte("target"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(sidecar), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Symlink(target, sidecar); err != nil {
		t.Fatalf("Symlink(): %v", err)
	}
	if err := Save(secrets, "main", cred); err == nil {
		t.Fatalf("Save(symlink) succeeded")
	}
	data, _ := os.ReadFile(target)
	if string(data) != "target" {
		t.Fatalf("symlink target overwritten")
	}
}

func TestConcurrentIndependentCredentials(t *testing.T) {
	dir := t.TempDir()
	secrets := filepath.Join(dir, "keys.json")
	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			name := "cred-" + string(rune('a'+i))
			cred, _ := NewCredential("Ov23test", "gho_secret", time.Now())
			if err := Save(secrets, name, cred); err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("Save(): %v", err)
	}
	for i := 0; i < 8; i++ {
		name := "cred-" + string(rune('a'+i))
		if _, err := Load(secrets, name); err != nil {
			t.Fatalf("Load(%q): %v", name, err)
		}
	}
}

func TestCancelPreservesExisting(t *testing.T) {
	dir := t.TempDir()
	secrets := filepath.Join(dir, "keys.json")
	old, _ := NewCredential("Ov23old", "gho_old", time.Now())
	if err := Save(secrets, "main", old); err != nil {
		t.Fatalf("Save(): %v", err)
	}
	before, _ := os.ReadFile(mustSidecar(t, secrets, "main"))
	if err := Save(secrets, "", old); err == nil {
		t.Fatalf("invalid save succeeded")
	}
	after, _ := os.ReadFile(mustSidecar(t, secrets, "main"))
	if string(before) != string(after) {
		t.Fatalf("existing credential modified on failure")
	}
	got, err := Load(secrets, "main")
	if err != nil || got.AccessToken != "gho_old" {
		t.Fatalf("got %+v err %v", got, err)
	}
}

func TestPersistenceFailureSurfaces(t *testing.T) {
	cred, _ := NewCredential("Ov23test", "gho_secret", time.Now())
	if err := Save("", "main", cred); err == nil {
		t.Fatalf("Save(empty path) succeeded")
	}
	if err := Save("/dev/null/impossible/keys.json\x00", "main", cred); err == nil {
		t.Fatalf("Save(bad path) succeeded")
	}
}

func mustSidecar(t *testing.T, secrets, name string) string {
	t.Helper()
	p, err := SidecarPath(secrets, name)
	if err != nil {
		t.Fatalf("SidecarPath(): %v", err)
	}
	return p
}
