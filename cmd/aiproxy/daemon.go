package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/egose/aiproxy/internal/filestore"
	"github.com/spf13/cobra"
)

const (
	daemonStateVersion = 1
	daemonReadyEnv     = "AIPROXY_DAEMON_READY_FD"
	daemonReadyMessage = "ready\n"
)

type daemonState struct {
	Version   int    `json:"version"`
	PID       int    `json:"pid"`
	Exe       string `json:"exe"`
	StartTime string `json:"start_time"`
	Config    string `json:"config"`
	Created   int64  `json:"created"`
}

func resolveDaemonPaths() (pidPath, logPath string) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "aiproxy", "aiproxy.pid"),
			filepath.Join(xdg, "aiproxy", "aiproxy.log")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join("aiproxy", "aiproxy.pid"), filepath.Join("aiproxy", "aiproxy.log")
	}
	return filepath.Join(home, ".config", "aiproxy", "aiproxy.pid"),
		filepath.Join(home, ".config", "aiproxy", "aiproxy.log")
}

func spawnDaemon(cmd *cobra.Command, cfgPath string) error {
	statePath, lockPath, logPath, canonicalConfig, err := resolveDaemonFiles(cfgPath)
	if err != nil {
		return err
	}

	lockFile, err := acquireDaemonLock(lockPath)
	if err != nil {
		return err
	}
	defer releaseDaemonLock(lockFile)

	if state, ok := readVerifiedDaemonState(statePath); ok {
		return fmt.Errorf("server already running (pid %d); use `aiproxy stop --config %s` first", state.PID, canonicalConfig)
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(logPath), 0o700); err != nil {
		return fmt.Errorf("create log dir %s: %w", filepath.Dir(logPath), err)
	}
	if err := os.MkdirAll(filepath.Dir(statePath), 0o700); err != nil {
		return fmt.Errorf("create state dir %s: %w", filepath.Dir(statePath), err)
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open log file %s: %w", logPath, err)
	}
	defer logFile.Close()

	readyRead, readyWrite, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("create readiness pipe: %w", err)
	}
	defer readyRead.Close()

	child := exec.Command(exe, "serve", "--config", cfgPath)
	child.Stdin = nil
	child.Stdout = logFile
	child.Stderr = logFile
	child.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	child.ExtraFiles = []*os.File{readyWrite}
	child.Env = append(os.Environ(), daemonReadyEnv+"=3")
	if err := child.Start(); err != nil {
		_ = readyWrite.Close()
		return fmt.Errorf("start daemon: %w", err)
	}
	_ = readyWrite.Close()

	pid := child.Process.Pid
	if err := waitForDaemonReady(child, readyRead); err != nil {
		_ = child.Process.Signal(syscall.SIGTERM)
		_, _ = child.Process.Wait()
		_ = os.Remove(statePath)
		return err
	}

	startTime, err := processStartTime(pid)
	if err != nil {
		_ = child.Process.Signal(syscall.SIGTERM)
		_, _ = child.Process.Wait()
		_ = os.Remove(statePath)
		return fmt.Errorf("verify daemon identity: %w", err)
	}
	state := daemonState{Version: daemonStateVersion, PID: pid, Exe: exe, StartTime: startTime, Config: canonicalConfig, Created: time.Now().Unix()}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		_ = child.Process.Signal(syscall.SIGTERM)
		_, _ = child.Process.Wait()
		_ = os.Remove(statePath)
		return fmt.Errorf("encode daemon state: %w", err)
	}
	data = append(data, '\n')
	if err := filestore.WriteFile(statePath, data, 0o600, filestore.Options{DirMode: 0o700, Secret: true}); err != nil {
		_ = child.Process.Signal(syscall.SIGTERM)
		_, _ = child.Process.Wait()
		_ = os.Remove(statePath)
		return fmt.Errorf("write daemon state %s: %w", statePath, err)
	}
	if err := child.Process.Release(); err != nil {
		_ = child.Process.Signal(syscall.SIGTERM)
		_, _ = child.Process.Wait()
		_ = os.Remove(statePath)
		return fmt.Errorf("release daemon: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "started aiproxy in background (pid %d)\nlog: %s\nstate: %s\n", pid, logPath, statePath)
	return nil
}

func resolveDaemonFiles(cfgPath string) (statePath, lockPath, logPath, canonicalConfig string, err error) {
	_, logPath = resolveDaemonPaths()
	canonicalConfig, err = canonicalConfigPath(cfgPath)
	if err != nil {
		return "", "", "", "", err
	}
	baseDir := filepath.Dir(logPath)
	sum := sha256.Sum256([]byte(canonicalConfig))
	name := hex.EncodeToString(sum[:])[:16]
	statePath = filepath.Join(baseDir, "daemon-"+name+".json")
	lockPath = filepath.Join(baseDir, "daemon-"+name+".lock")
	return statePath, lockPath, logPath, canonicalConfig, nil
}

func canonicalConfigPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve config path: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved, nil
	}
	return abs, nil
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

func waitForDaemonReady(child *exec.Cmd, ready *os.File) error {
	type result struct {
		data []byte
		err  error
	}
	readCh := make(chan result, 1)
	go func() {
		data, err := io.ReadAll(ready)
		readCh <- result{data: data, err: err}
	}()
	timer := time.NewTimer(15 * time.Second)
	defer timer.Stop()
	select {
	case res := <-readCh:
		if res.err != nil {
			return fmt.Errorf("read daemon readiness: %w", res.err)
		}
		if string(res.data) == daemonReadyMessage {
			return nil
		}
		if err := child.Wait(); err != nil {
			return fmt.Errorf("daemon exited before startup completed: %w", err)
		}
		return errors.New("daemon exited before startup completed")
	case <-timer.C:
		return errors.New("daemon startup timed out")
	}
}

func notifyDaemonReady() error {
	fd := os.Getenv(daemonReadyEnv)
	if fd == "" {
		return nil
	}
	n, err := strconv.Atoi(fd)
	if err != nil {
		return fmt.Errorf("invalid readiness fd: %w", err)
	}
	f := os.NewFile(uintptr(n), "daemon-ready")
	if f == nil {
		return errors.New("open readiness fd")
	}
	defer f.Close()
	_, err = io.WriteString(f, daemonReadyMessage)
	return err
}

// readLivePID returns the PID stored in pidPath if the process is still
// alive, otherwise 0. Errors (missing file, stale entry) are swallowed.
func readLivePID(pidPath string) (int, error) {
	data, err := os.ReadFile(pidPath)
	if err != nil {
		return 0, err
	}
	pid, err := strconv.Atoi(string(firstLine(data)))
	if err != nil {
		return 0, err
	}
	if !processAlive(pid) {
		_ = os.Remove(pidPath)
		return 0, nil
	}
	return pid, nil
}

func firstLine(data []byte) string {
	for i, c := range data {
		if c == '\n' || c == '\r' {
			return string(data[:i])
		}
	}
	return string(data)
}

// processAlive returns true if the process exists. We use signal 0, which is
// the standard no-op probe.
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
		// EPERM means the process exists but we cannot signal it.
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

func readVerifiedDaemonState(path string) (daemonState, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return daemonState{}, false
	}
	var state daemonState
	if err := json.Unmarshal(data, &state); err != nil || state.Version != daemonStateVersion || state.PID <= 0 || state.Exe == "" || state.StartTime == "" {
		return daemonState{}, false
	}
	if !processAlive(state.PID) {
		_ = os.Remove(path)
		return daemonState{}, false
	}
	exe, err := processExe(state.PID)
	if err != nil || exe != state.Exe {
		return daemonState{}, false
	}
	startTime, err := processStartTime(state.PID)
	if err != nil || startTime != state.StartTime {
		return daemonState{}, false
	}
	return state, true
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

func stopServer(cfgPath string, out io.Writer) error {
	statePath, lockPath, _, _, err := resolveDaemonFiles(cfgPath)
	if err != nil {
		return err
	}
	lockFile, err := acquireDaemonLock(lockPath)
	if err != nil {
		return err
	}
	defer releaseDaemonLock(lockFile)
	state, ok := readVerifiedDaemonState(statePath)
	if !ok {
		return errors.New("no server running")
	}
	pid := state.PID
	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("find pid %d: %w", pid, err)
	}
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("signal pid %d: %w", pid, err)
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if !processAlive(pid) {
			_ = os.Remove(statePath)
			fmt.Fprintf(out, "stopped aiproxy (pid %d)\n", pid)
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	if err := proc.Signal(syscall.SIGKILL); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return fmt.Errorf("kill pid %d: %w", pid, err)
	}
	_ = os.Remove(statePath)
	fmt.Fprintf(out, "killed aiproxy (pid %d)\n", pid)
	return nil
}

func statusServer(cfgPath string, out io.Writer) error {
	statePath, lockPath, _, _, err := resolveDaemonFiles(cfgPath)
	if err != nil {
		return err
	}
	lockFile, err := acquireDaemonLock(lockPath)
	if err != nil {
		return err
	}
	defer releaseDaemonLock(lockFile)
	state, ok := readVerifiedDaemonState(statePath)
	if !ok {
		fmt.Fprintln(out, "no server running")
		return errors.New("not running")
	}
	fmt.Fprintf(out, "running (pid %d)\n", state.PID)
	return nil
}

// restartServer stops the running daemon (if any) and starts a fresh one.
func restartServer(cmd *cobra.Command, cfgPath string) error {
	_ = stopServer(cfgPath, cmd.OutOrStdout())
	return spawnDaemon(cmd, cfgPath)
}

func newStopCommand() *cobra.Command {
	var cfgPath string
	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop a backgrounded aiproxy server",
		RunE: func(cmd *cobra.Command, args []string) error {
			return stopServer(cfgPath, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVarP(&cfgPath, "config", "c", defaultConfigPath(), "path to config file")
	return cmd
}

func newStatusCommand() *cobra.Command {
	var cfgPath string
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Report whether a backgrounded aiproxy server is running",
		RunE: func(cmd *cobra.Command, args []string) error {
			return statusServer(cfgPath, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVarP(&cfgPath, "config", "c", defaultConfigPath(), "path to config file")
	return cmd
}

func newRestartCommand() *cobra.Command {
	var cfgPath string
	cmd := &cobra.Command{
		Use:   "restart",
		Short: "Restart a backgrounded aiproxy server (stop if running, then start)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return restartServer(cmd, cfgPath)
		},
	}
	cmd.Flags().StringVarP(&cfgPath, "config", "c", defaultConfigPath(), "path to config file")
	return cmd
}
