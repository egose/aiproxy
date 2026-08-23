//go:build linux

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "serve" {
		cmd := newRootCommand()
		cmd.SetArgs(os.Args[1:])
		if err := cmd.Execute(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

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
	cfgPath := filepath.Join(dir, "config.hcl")
	if err := os.WriteFile(cfgPath, []byte("listener \"http\" \"public\" { address = \":0\" }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	statePath, _, _, canonicalConfig, err := resolveDaemonFiles(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(statePath), 0o700); err != nil {
		t.Fatal(err)
	}
	writeSelfDaemonState(t, statePath, canonicalConfig)
	var out bytes.Buffer
	if err := statusServer(cfgPath, &out); err != nil {
		t.Fatalf("status err = %v", err)
	}
	if !strings.Contains(out.String(), "running") {
		t.Fatalf("stdout = %q, want mention 'running'", out.String())
	}
}

func TestStatusServerRefusesMismatchedLivePID(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	cfgPath := filepath.Join(dir, "config.hcl")
	if err := os.WriteFile(cfgPath, []byte("listener \"http\" \"public\" { address = \":0\" }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	statePath, _, _, canonicalConfig, err := resolveDaemonFiles(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(statePath), 0o700); err != nil {
		t.Fatal(err)
	}
	state := selfDaemonState(t, canonicalConfig)
	state.Exe = filepath.Join(dir, "not-aiproxy")
	writeDaemonState(t, statePath, state)

	var out bytes.Buffer
	if err := statusServer(cfgPath, &out); err == nil {
		t.Fatal("expected mismatched state to be refused")
	}
	if !strings.Contains(out.String(), "no server running") {
		t.Fatalf("stdout = %q, want no server running", out.String())
	}
	if _, err := os.Stat(statePath); err != nil {
		t.Fatalf("mismatched live state should remain for owner, stat err = %v", err)
	}
}

func TestStopServerRefusesMismatchedLivePID(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	cfgPath := filepath.Join(dir, "config.hcl")
	if err := os.WriteFile(cfgPath, []byte("listener \"http\" \"public\" { address = \":0\" }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	statePath, _, _, canonicalConfig, err := resolveDaemonFiles(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(statePath), 0o700); err != nil {
		t.Fatal(err)
	}
	state := selfDaemonState(t, canonicalConfig)
	state.StartTime = "1"
	writeDaemonState(t, statePath, state)

	var out bytes.Buffer
	if err := stopServer(cfgPath, &out); err == nil || !strings.Contains(err.Error(), "no server running") {
		t.Fatalf("stop err = %v, want no server running", err)
	}
	if out.String() != "" {
		t.Fatalf("stdout = %q, want empty", out.String())
	}
}

func TestMalformedDaemonStateReportsNotRunningWithoutDeleting(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	cfgPath := filepath.Join(dir, "config.hcl")
	if err := os.WriteFile(cfgPath, []byte("listener \"http\" \"public\" { address = \":0\" }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	statePath, _, _, _, err := resolveDaemonFiles(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(statePath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, []byte("{\"pid\":"), 0o600); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := statusServer(cfgPath, &out); err == nil {
		t.Fatal("expected malformed state to report not running")
	}
	if _, err := os.Stat(statePath); err != nil {
		t.Fatalf("malformed state should not be deleted, stat err = %v", err)
	}
}

func TestStaleDaemonStateReportsNotRunningAndRemovesState(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	cfgPath := filepath.Join(dir, "config.hcl")
	if err := os.WriteFile(cfgPath, []byte("listener \"http\" \"public\" { address = \":0\" }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	statePath, _, _, canonicalConfig, err := resolveDaemonFiles(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(statePath), 0o700); err != nil {
		t.Fatal(err)
	}
	writeDaemonState(t, statePath, daemonState{Version: daemonStateVersion, PID: 2147483647, Exe: "/missing", StartTime: "1", Config: canonicalConfig, Created: time.Now().Unix()})

	var out bytes.Buffer
	if err := statusServer(cfgPath, &out); err == nil {
		t.Fatal("expected stale state to report not running")
	}
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Fatalf("stale state should be removed, err = %v", err)
	}
}

func TestSpawnDaemonInvalidConfigDoesNotLeaveState(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	cfgPath := filepath.Join(dir, "bad.hcl")
	if err := os.WriteFile(cfgPath, []byte("not hcl"), 0o600); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	err := spawnDaemon(testCobraCommand(&out), cfgPath)
	if err == nil {
		t.Fatal("expected invalid config startup to fail")
	}
	statePath, _, _, _, err := resolveDaemonFiles(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Fatalf("state file should not remain after failed startup, err = %v", err)
	}
}

func TestSpawnDaemonOccupiedListenerDoesNotLeaveState(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	cfgPath := filepath.Join(dir, "config.hcl")
	writeLifecycleConfig(t, cfgPath, ln.Addr().String())

	var out bytes.Buffer
	err = spawnDaemon(testCobraCommand(&out), cfgPath)
	if err == nil {
		t.Fatal("expected occupied listener startup to fail")
	}
	statePath, _, _, _, err := resolveDaemonFiles(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Fatalf("state file should not remain after bind failure, err = %v", err)
	}
}

func TestConcurrentSpawnDaemonProducesOneServer(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	cfgPath := filepath.Join(dir, "config.hcl")
	writeLifecycleConfig(t, cfgPath, "127.0.0.1:0")

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			var out bytes.Buffer
			errs[i] = spawnDaemon(testCobraCommand(&out), cfgPath)
		}(i)
	}
	wg.Wait()
	successes := 0
	for _, err := range errs {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("successful starts = %d, want 1; errs = %v", successes, errs)
	}

	var statusOut bytes.Buffer
	if err := statusServer(cfgPath, &statusOut); err != nil {
		t.Fatalf("status after concurrent start err = %v", err)
	}
	var stopOut bytes.Buffer
	if err := stopServer(cfgPath, &stopOut); err != nil {
		t.Fatalf("stop after concurrent start err = %v", err)
	}
}

func writeSelfDaemonState(t *testing.T, path, canonicalConfig string) {
	t.Helper()
	writeDaemonState(t, path, selfDaemonState(t, canonicalConfig))
}

func selfDaemonState(t *testing.T, canonicalConfig string) daemonState {
	t.Helper()
	exe, err := processExe(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	startTime, err := processStartTime(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	return daemonState{Version: daemonStateVersion, PID: os.Getpid(), Exe: exe, StartTime: startTime, Config: canonicalConfig, Created: time.Now().Unix()}
}

func writeDaemonState(t *testing.T, path string, state daemonState) {
	t.Helper()
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeLifecycleConfig(t *testing.T, path, address string) {
	t.Helper()
	config := fmt.Sprintf(`listener "http" "public" {
  address = %q
}

auth "main" {
  mode = "none"
}

provider "openai" "openai" {
  api_key = "test-key"

  model "gpt-4o-mini" {}
}
`, address)
	if err := os.WriteFile(path, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
}

func testCobraCommand(out *bytes.Buffer) *cobra.Command {
	cmd := &cobra.Command{}
	cmd.SetOut(out)
	return cmd
}
