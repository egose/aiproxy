//go:build linux

package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/spf13/cobra"
)

func TestRestartAbsentDoesNotSpawn(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	cfgPath := filepath.Join(dir, "config.hcl")
	writeLifecycleConfig(t, cfgPath, "127.0.0.1:0")

	var out bytes.Buffer
	err := restartServer(testCobraCommand(&out), cfgPath)
	if err == nil || !strings.Contains(err.Error(), "no server running") {
		t.Fatalf("restart err = %v, want no server running", err)
	}
	if strings.Contains(out.String(), "started") {
		t.Fatalf("restart must not spawn when absent, out = %q", out.String())
	}
	statePath, _, _, _, err := resolveDaemonFiles(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Fatalf("state file must not exist after absent restart, err = %v", err)
	}
}

func TestRestartMalformedDoesNotSpawn(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	cfgPath := filepath.Join(dir, "config.hcl")
	writeLifecycleConfig(t, cfgPath, "127.0.0.1:0")
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
	err = restartServer(testCobraCommand(&out), cfgPath)
	if err == nil || !strings.Contains(err.Error(), "no server running") {
		t.Fatalf("restart err = %v, want no server running", err)
	}
	if strings.Contains(out.String(), "started") {
		t.Fatalf("restart must not spawn on malformed state, out = %q", out.String())
	}
	if _, err := os.Stat(statePath); err != nil {
		t.Fatalf("malformed state must be preserved, stat err = %v", err)
	}
}

func TestRestartIdentityMismatchDoesNotSpawn(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	cfgPath := filepath.Join(dir, "config.hcl")
	writeLifecycleConfig(t, cfgPath, "127.0.0.1:0")
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
	err = restartServer(testCobraCommand(&out), cfgPath)
	if err == nil || !strings.Contains(err.Error(), "no server running") {
		t.Fatalf("restart err = %v, want no server running", err)
	}
	if strings.Contains(out.String(), "started") {
		t.Fatalf("restart must not spawn on identity mismatch, out = %q", out.String())
	}
	if _, err := os.Stat(statePath); err != nil {
		t.Fatalf("mismatched state must be preserved, stat err = %v", err)
	}
}

func TestRestartInjectedStopFailureSpawnsNothing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	cfgPath := filepath.Join(dir, "config.hcl")
	writeLifecycleConfig(t, cfgPath, "127.0.0.1:0")

	oldStop := restartStopImpl
	oldSpawn := restartSpawnImpl
	defer func() {
		restartStopImpl = oldStop
		restartSpawnImpl = oldSpawn
	}()
	stopErr := errors.New("injected stop failure")
	var spawnCalls atomic.Int32
	restartStopImpl = func(statePath string, out io.Writer) error {
		return stopErr
	}
	restartSpawnImpl = func(cmd *cobra.Command, cfgPath, statePath, logPath, canonicalConfig string) error {
		spawnCalls.Add(1)
		return nil
	}

	var out bytes.Buffer
	err := restartServer(testCobraCommand(&out), cfgPath)
	if !errors.Is(err, stopErr) {
		t.Fatalf("restart err = %v, want injected stop failure", err)
	}
	if got := spawnCalls.Load(); got != 0 {
		t.Fatalf("spawn calls = %d, want 0 after failed stop", got)
	}
	if strings.Contains(out.String(), "started") {
		t.Fatalf("restart must not print started after failed stop, out = %q", out.String())
	}
	statePath, _, _, _, err := resolveDaemonFiles(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Fatalf("state file must not exist after failed stop, err = %v", err)
	}
}

func TestRestartHoldsOneLockAcrossStopAndSpawn(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	cfgPath := filepath.Join(dir, "config.hcl")
	writeLifecycleConfig(t, cfgPath, "127.0.0.1:0")
	_, lockPath, _, _, err := resolveDaemonFiles(cfgPath)
	if err != nil {
		t.Fatal(err)
	}

	oldStop := restartStopImpl
	oldSpawn := restartSpawnImpl
	defer func() {
		restartStopImpl = oldStop
		restartSpawnImpl = oldSpawn
	}()

	stopEntered := make(chan struct{})
	allowStopFinish := make(chan struct{})
	var stopFinished atomic.Bool
	var spawnSawStopDone atomic.Bool
	var stopHeldLock atomic.Bool
	var spawnHeldLock atomic.Bool

	restartStopImpl = func(statePath string, out io.Writer) error {
		if _, err := acquireDaemonLock(lockPath); err != nil && strings.Contains(err.Error(), "another daemon lifecycle operation is in progress") {
			stopHeldLock.Store(true)
		} else if err == nil {
			t.Error("stop phase must hold lifecycle lock")
		}
		close(stopEntered)
		<-allowStopFinish
		stopFinished.Store(true)
		return nil
	}
	restartSpawnImpl = func(cmd *cobra.Command, cfgPath, statePath, logPath, canonicalConfig string) error {
		if _, err := acquireDaemonLock(lockPath); err != nil && strings.Contains(err.Error(), "another daemon lifecycle operation is in progress") {
			spawnHeldLock.Store(true)
		} else if err == nil {
			t.Error("spawn phase must hold lifecycle lock")
		}
		spawnSawStopDone.Store(stopFinished.Load())
		return nil
	}

	var out bytes.Buffer
	errCh := make(chan error, 1)
	go func() {
		errCh <- restartServer(testCobraCommand(&out), cfgPath)
	}()

	<-stopEntered
	var concOut bytes.Buffer
	concErr := spawnDaemon(testCobraCommand(&concOut), cfgPath)
	if concErr == nil || !strings.Contains(concErr.Error(), "another daemon lifecycle operation is in progress") {
		close(allowStopFinish)
		<-errCh
		t.Fatalf("concurrent spawn err = %v, want in-progress lock error", concErr)
	}
	close(allowStopFinish)
	if err := <-errCh; err != nil {
		t.Fatalf("restart err = %v", err)
	}
	if !stopHeldLock.Load() {
		t.Fatal("stop phase did not hold lifecycle lock")
	}
	if !spawnHeldLock.Load() {
		t.Fatal("spawn phase did not hold lifecycle lock")
	}
	if !spawnSawStopDone.Load() {
		t.Fatal("spawn started before verified stop completed")
	}
}

func TestRestartFailedReplacementLeavesNoStateAndUnlocks(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	cfgPath := filepath.Join(dir, "bad.hcl")
	if err := os.WriteFile(cfgPath, []byte("not hcl"), 0o600); err != nil {
		t.Fatal(err)
	}

	oldStop := restartStopImpl
	defer func() {
		restartStopImpl = oldStop
	}()
	restartStopImpl = func(statePath string, out io.Writer) error {
		return nil
	}

	var out bytes.Buffer
	err := restartServer(testCobraCommand(&out), cfgPath)
	if err == nil {
		t.Fatal("expected failed replacement to report error")
	}
	statePath, lockPath, _, _, resolveErr := resolveDaemonFiles(cfgPath)
	if resolveErr != nil {
		t.Fatal(resolveErr)
	}
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Fatalf("state file must not remain after failed replacement, err = %v", err)
	}
	lockFile, err := acquireDaemonLock(lockPath)
	if err != nil {
		t.Fatalf("lifecycle lock must be released after failed replacement, err = %v", err)
	}
	releaseDaemonLock(lockFile)
	var statusOut bytes.Buffer
	if err := statusServer(cfgPath, &statusOut); err == nil {
		t.Fatal("status must report not running after failed replacement")
	}
}

func TestRestartSuccessWaitsForReadiness(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	cfgPath := filepath.Join(dir, "config.hcl")
	writeLifecycleConfig(t, cfgPath, "127.0.0.1:0")

	var spawnOut bytes.Buffer
	if err := spawnDaemon(testCobraCommand(&spawnOut), cfgPath); err != nil {
		t.Fatalf("spawn err = %v", err)
	}
	statePath, _, _, _, err := resolveDaemonFiles(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}

	var restartOut bytes.Buffer
	if err := restartServer(testCobraCommand(&restartOut), cfgPath); err != nil {
		var stopOut bytes.Buffer
		_ = stopServer(cfgPath, &stopOut)
		t.Fatalf("restart err = %v", err)
	}
	out := restartOut.String()
	if !strings.Contains(out, "started") {
		var stopOut bytes.Buffer
		_ = stopServer(cfgPath, &stopOut)
		t.Fatalf("restart out must contain started, got %q", out)
	}
	after, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) == string(after) {
		t.Logf("state unchanged after restart (pid reuse possible), verifying liveness instead")
	}
	var statusOut bytes.Buffer
	if err := statusServer(cfgPath, &statusOut); err != nil {
		t.Fatalf("status after restart err = %v (restart did not wait for readiness)", err)
	}
	if !strings.Contains(statusOut.String(), "running") {
		t.Fatalf("status out = %q, want running", statusOut.String())
	}
	var stopOut bytes.Buffer
	if err := stopServer(cfgPath, &stopOut); err != nil {
		t.Fatalf("stop after restart err = %v", err)
	}
}
