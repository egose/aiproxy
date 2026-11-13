package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestRegularReadDoesNotAcquireControllingTTY(t *testing.T) {
	master, err := unix.Open("/dev/ptmx", unix.O_RDWR|unix.O_NOCTTY|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal("cannot create the owned PTY regression fixture")
	}
	defer unix.Close(master)
	if err := unix.IoctlSetPointerInt(master, unix.TIOCSPTLCK, 0); err != nil {
		t.Fatal(err)
	}
	number, err := unix.IoctlGetInt(master, unix.TIOCGPTN)
	if err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, executable, "-test.run=^TestRegularReadPTYChild$", "--", fmt.Sprintf("/dev/pts/%d", number))
	command.Env = []string{"TLS_CLIENTHELLO_PTY_HELPER=1"}
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	output := &boundedChildBuffer{cancel: cancel}
	command.Stdout, command.Stderr = output, output
	command.WaitDelay = 100 * time.Millisecond
	if err := command.Run(); err != nil {
		t.Fatalf("fresh-session PTY child failed: %v\n%s", err, output.bytes())
	}
}

func TestRegularReadPTYChild(t *testing.T) {
	if os.Getenv("TLS_CLIENTHELLO_PTY_HELPER") != "1" {
		t.Skip("fresh-session subprocess helper")
	}
	if session, err := unix.Getsid(0); err != nil || session != os.Getpid() {
		t.Fatal("PTY regression child is not its own fresh session leader")
	}
	assertNoControllingTTY(t)
	if len(os.Args) < 2 {
		t.Fatal("missing owned PTY slave")
	}
	if _, err := readSpec(os.Args[len(os.Args)-1]); err == nil {
		t.Fatal("PTY slave was accepted as a regular specification")
	}
	assertNoControllingTTY(t)
}

func assertNoControllingTTY(t *testing.T) {
	t.Helper()
	descriptor, err := unix.Open("/dev/tty", unix.O_RDONLY|unix.O_NOCTTY|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err == nil {
		unix.Close(descriptor)
		t.Fatal("rejected PTY became the fresh session's controlling terminal")
	}
	if err != unix.ENXIO {
		t.Fatalf("unexpected controlling-terminal probe result: %v", err)
	}
}

func TestOutputAnchorsAndStagingCloseAllDescriptors(t *testing.T) {
	directory := t.TempDir()
	profilePath, metadataPath := filepath.Join(directory, "profile.json"), filepath.Join(directory, "metadata.json")
	outputs, err := prepareOutputs(profilePath, metadataPath, false)
	if err != nil {
		t.Fatal(err)
	}
	assertNoOutputFixtureDescriptors(t, directory)
	outputs[0].data, outputs[1].data = []byte("profile"), []byte("metadata")
	if err := publishOutputs(outputs, false); err != nil {
		t.Fatal(err)
	}
	assertNoOutputFixtureDescriptors(t, directory)
	if _, err := prepareOutputs(profilePath, metadataPath, false); err == nil {
		t.Fatal("existing files were accepted without force")
	}
	assertNoOutputFixtureDescriptors(t, directory)
	if err := publishOutputs(outputs, false); err == nil {
		t.Fatal("existing files were overwritten without force")
	}
	assertNoOutputFixtureDescriptors(t, directory)
}

func assertNoOutputFixtureDescriptors(t *testing.T, directory string) {
	t.Helper()
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Skip("descriptor census requires Linux procfs")
	}
	for _, entry := range entries {
		target, err := os.Readlink(filepath.Join("/proc/self/fd", entry.Name()))
		if err == nil && (target == directory || strings.HasPrefix(target, directory+string(os.PathSeparator))) {
			t.Fatal("output helper returned with an open directory or staging descriptor")
		}
	}
}
