//go:build !linux

package main

import (
	"os"
	"os/exec"
)

func daemonLifecycleSupported() bool {
	return false
}

func configureDaemonCommand(child *exec.Cmd) {}

func acquireDaemonLock(path string) (*os.File, error) {
	return nil, errDaemonLifecycleUnsupported
}

func releaseDaemonLock(f *os.File) {}

func terminateDaemonProcess(proc *os.Process) error {
	return errDaemonLifecycleUnsupported
}

func killDaemonProcess(proc *os.Process) error {
	return errDaemonLifecycleUnsupported
}

func processAlive(pid int) bool {
	return false
}

func processExe(pid int) (string, error) {
	return "", errDaemonLifecycleUnsupported
}

func processStartTime(pid int) (string, error) {
	return "", errDaemonLifecycleUnsupported
}
