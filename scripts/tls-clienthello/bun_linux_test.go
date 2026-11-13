package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestTimedOutAndCanceledBunChildrenAreReaped(t *testing.T) {
	for _, mode := range []string{"no-connect", "no-hello", "hang-after-hello"} {
		t.Run(mode, func(t *testing.T) {
			executable := fixtureExecutable(t, mode)
			runner := bunCapture{executable: executable, limits: captureLimits{accept: 150 * time.Millisecond, read: 150 * time.Millisecond, child: 400 * time.Millisecond}}
			if _, _, err := runner.capture(context.Background()); err == nil {
				t.Fatal("hanging child was accepted")
			}
			assertFixtureChildrenReaped(t, executable)
		})
	}
	executable := fixtureExecutable(t, "no-hello")
	runner := bunCapture{executable: executable, limits: defaultCaptureLimits()}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	if _, _, err := runner.capture(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("capture did not honor cancellation: %v", err)
	}
	assertFixtureChildrenReaped(t, executable)
}

func TestBunCancellationClosesListenerAndSocketDescriptors(t *testing.T) {
	for _, mode := range []string{"no-connect", "no-hello", "hang-after-hello"} {
		t.Run(mode, func(t *testing.T) {
			executable := fixtureExecutable(t, mode)
			before := fixtureSocketDescriptors(t)
			runner := bunCapture{executable: executable, limits: defaultCaptureLimits()}
			ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
			defer cancel()
			if _, _, err := runner.capture(ctx); !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("capture did not honor cancellation: %v", err)
			}
			for socket := range fixtureSocketDescriptors(t) {
				if !before[socket] {
					t.Fatal("capture returned with an open listener or accepted socket descriptor")
				}
			}
			assertFixtureChildrenReaped(t, executable)
		})
	}
}

func TestMaterializedCaptureClosesSocketDescriptors(t *testing.T) {
	profile := toolFixtureProfile(t)
	for sample := 0; sample < 5; sample++ {
		before := fixtureSocketDescriptors(t)
		if _, err := captureProfile(context.Background(), profile, defaultCaptureLimits()); err != nil {
			t.Fatal(err)
		}
		for socket := range fixtureSocketDescriptors(t) {
			if !before[socket] {
				t.Fatal("materialized capture returned before its client/listener sockets closed")
			}
		}
	}
}

func fixtureSocketDescriptors(t *testing.T) map[string]bool {
	t.Helper()
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Skip("socket descriptor census requires Linux procfs")
	}
	sockets := make(map[string]bool)
	for _, entry := range entries {
		target, err := os.Readlink(filepath.Join("/proc/self/fd", entry.Name()))
		if err == nil && strings.HasPrefix(target, "socket:[") {
			sockets[target] = true
		}
	}
	return sockets
}

func assertFixtureChildrenReaped(t *testing.T, executable string) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Dir(executable))
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "invocation-") {
			continue
		}
		count++
		pid, err := strconv.Atoi(strings.TrimPrefix(entry.Name(), "invocation-"))
		if err != nil {
			t.Fatal("invalid hermetic child identity")
		}
		if err := syscall.Kill(pid, 0); !errors.Is(err, syscall.ESRCH) {
			t.Fatal("capture returned with a live or unreaped child")
		}
	}
	if count != 1 {
		t.Fatal("reaping test did not observe its own fresh child")
	}
}
