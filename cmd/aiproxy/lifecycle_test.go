package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestStopServerErrorsWhenNotRunning(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	var out bytes.Buffer
	err := stopServer(filepath.Join(dir, "config.hcl"), &out)
	if err == nil {
		t.Fatal("expected error when no server is running")
	}
	if !strings.Contains(err.Error(), "no server running") {
		t.Fatalf("err = %v, want 'no server running'", err)
	}
	if strings.Contains(out.String(), "stopped") {
		t.Fatalf("stdout should not contain 'stopped', got: %s", out.String())
	}
}

func TestStatusServerReportsNotRunning(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	var out bytes.Buffer
	err := statusServer(filepath.Join(dir, "config.hcl"), &out)
	if err == nil {
		t.Fatal("expected error when no server is running")
	}
	if !strings.Contains(out.String(), "no server running") {
		t.Fatalf("stdout = %q, want mention 'no server running'", out.String())
	}
}

func TestStatusServerReportsRunning(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	pidPath, _ := resolveDaemonPaths()
	if err := os.MkdirAll(filepath.Dir(pidPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(os.Getpid())+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := statusServer(filepath.Join(dir, "config.hcl"), &out); err != nil {
		t.Fatalf("status err = %v", err)
	}
	if !strings.Contains(out.String(), "running") {
		t.Fatalf("stdout = %q, want mention 'running'", out.String())
	}
}
