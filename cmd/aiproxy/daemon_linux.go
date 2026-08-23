//go:build linux

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

func daemonLifecycleSupported() bool {
	return true
}

func configureDaemonCommand(child *exec.Cmd) {
	child.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

func acquireDaemonLock(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create lock dir %s: %w", filepath.Dir(path), err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open lifecycle lock %s: %w", path, err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if errno, ok := err.(syscall.Errno); ok && (errno == syscall.EWOULDBLOCK || errno == syscall.EAGAIN) {
			return nil, errors.New("another daemon lifecycle operation is in progress")
		}
		return nil, fmt.Errorf("lock lifecycle state: %w", err)
	}
	return f, nil
}

func releaseDaemonLock(f *os.File) {
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	_ = f.Close()
}

func terminateDaemonProcess(proc *os.Process) error {
	return proc.Signal(syscall.SIGTERM)
}

func killDaemonProcess(proc *os.Process) error {
	return proc.Signal(syscall.SIGKILL)
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		if errors.Is(err, os.ErrProcessDone) {
			return false
		}
		if errno, ok := err.(syscall.Errno); ok && errno == syscall.ESRCH {
			return false
		}
		if errno, ok := err.(syscall.Errno); ok && errno == syscall.EPERM {
			return true
		}
		return false
	}
	return !processZombie(pid)
}

func processZombie(pid int) bool {
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return false
	}
	s := string(data)
	end := strings.LastIndex(s, ")")
	if end < 0 || end+2 > len(s) {
		return false
	}
	fields := strings.Fields(s[end+2:])
	return len(fields) > 0 && fields[0] == "Z"
}

func processExe(pid int) (string, error) {
	exe, err := os.Readlink(filepath.Join("/proc", strconv.Itoa(pid), "exe"))
	if err != nil {
		return "", err
	}
	exe = strings.TrimSuffix(exe, " (deleted)")
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		return resolved, nil
	}
	return exe, nil
}

func processStartTime(pid int) (string, error) {
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return "", err
	}
	s := string(data)
	end := strings.LastIndex(s, ")")
	if end < 0 || end+2 > len(s) {
		return "", errors.New("malformed process stat")
	}
	fields := strings.Fields(s[end+2:])
	if len(fields) <= 19 {
		return "", errors.New("malformed process stat")
	}
	return fields[19], nil
}
