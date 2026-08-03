package main

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestFirstLineStripsTrailingNewline(t *testing.T) {
	if got := firstLine([]byte("123\n")); got != "123" {
		t.Fatalf("firstLine = %q, want 123", got)
	}
	if got := firstLine([]byte("456\r\nabc")); got != "456" {
		t.Fatalf("firstLine (CR) = %q, want 456", got)
	}
	if got := firstLine([]byte("789")); got != "789" {
		t.Fatalf("firstLine (no nl) = %q, want 789", got)
	}
}

func TestProcessAliveForSelf(t *testing.T) {
	if !processAlive(os.Getpid()) {
		t.Fatal("processAlive should report self as alive")
	}
	if processAlive(0) {
		t.Fatal("pid 0 should never be alive")
	}
	if processAlive(-1) {
		t.Fatal("negative pid should never be alive")
	}
}

func TestReadLivePIDWithMissingFile(t *testing.T) {
	dir := t.TempDir()
	pidPath := filepath.Join(dir, "missing.pid")
	if pid, err := readLivePID(pidPath); err == nil || pid != 0 {
		t.Fatalf("missing pid file: pid=%d err=%v", pid, err)
	}
}

func TestReadLivePIDWithStaleEntryRemovesFile(t *testing.T) {
	dir := t.TempDir()
	pidPath := filepath.Join(dir, "stale.pid")
	// 2147483647 is INT_MAX — almost certainly no such process.
	if err := os.WriteFile(pidPath, []byte("2147483647\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	pid, err := readLivePID(pidPath)
	if err != nil {
		t.Fatalf("readLivePID err = %v", err)
	}
	if pid != 0 {
		t.Fatalf("stale pid should resolve to 0, got %d", pid)
	}
	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Fatalf("stale pidfile should be removed, err = %v", err)
	}
}

func TestReadLivePIDWithLiveEntry(t *testing.T) {
	dir := t.TempDir()
	pidPath := filepath.Join(dir, "live.pid")
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(os.Getpid())+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	pid, err := readLivePID(pidPath)
	if err != nil {
		t.Fatalf("readLivePID err = %v", err)
	}
	if pid != os.Getpid() {
		t.Fatalf("readLivePID = %d, want %d", pid, os.Getpid())
	}
}

func TestReadLivePIDWithNonNumericEntry(t *testing.T) {
	dir := t.TempDir()
	pidPath := filepath.Join(dir, "garbage.pid")
	if err := os.WriteFile(pidPath, []byte("not-a-number\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readLivePID(pidPath); err == nil {
		t.Fatal("expected error for non-numeric pidfile")
	}
}

func TestResolveDaemonPathsHonorsXDG(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	pid, log := resolveDaemonPaths()
	wantPid := filepath.Join(xdg, "aiproxy", "aiproxy.pid")
	wantLog := filepath.Join(xdg, "aiproxy", "aiproxy.log")
	if pid != wantPid {
		t.Fatalf("pidPath = %q, want %q", pid, wantPid)
	}
	if log != wantLog {
		t.Fatalf("logPath = %q, want %q", log, wantLog)
	}
}

func TestResolveDaemonPathsFallsBackToHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", home)
	pid, log := resolveDaemonPaths()
	wantPid := filepath.Join(home, ".config", "aiproxy", "aiproxy.pid")
	wantLog := filepath.Join(home, ".config", "aiproxy", "aiproxy.log")
	if pid != wantPid {
		t.Fatalf("pidPath = %q, want %q", pid, wantPid)
	}
	if log != wantLog {
		t.Fatalf("logPath = %q, want %q", log, wantLog)
	}
}

func TestNormalizeBaseURL(t *testing.T) {
	cases := map[string]string{
		":8080":          "http://127.0.0.1:8080",
		"0.0.0.0:9090":   "http://127.0.0.1:9090",
		"127.0.0.1:8080": "http://127.0.0.1:8080",
		"http://x:1":     "http://x:1",
		"https://y:2":    "https://y:2",
		"":               "http://127.0.0.1:8080",
	}
	for in, want := range cases {
		if got := normalizeBaseURL(in); got != want {
			t.Errorf("normalizeBaseURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsConnectionRefused(t *testing.T) {
	if isConnectionRefused(nil) {
		t.Fatal("nil err should not be refused")
	}
	if !isConnectionRefused(errFake("dial tcp: connect: connection refused")) {
		t.Fatal("should match 'connection refused'")
	}
	if !isConnectionRefused(errFake("lookup host: no such host")) {
		t.Fatal("should match 'no such host'")
	}
	if isConnectionRefused(errFake("some other error")) {
		t.Fatal("should not match unrelated error")
	}
}

type errFake string

func (e errFake) Error() string { return string(e) }
