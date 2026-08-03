package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

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

// spawnDaemon forks the current aiproxy serve as a detached background
// process, recording its PID in a pidfile and redirecting stdio to a log file.
func spawnDaemon(cmd *cobra.Command, cfgPath string) error {
	pidPath, logPath := resolveDaemonPaths()

	if pid, _ := readLivePID(pidPath); pid != 0 {
		return fmt.Errorf("server already running (pid %d); use `aiproxy stop` first", pid)
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(logPath), 0o700); err != nil {
		return fmt.Errorf("create log dir %s: %w", filepath.Dir(logPath), err)
	}
	if err := os.MkdirAll(filepath.Dir(pidPath), 0o700); err != nil {
		return fmt.Errorf("create pid dir %s: %w", filepath.Dir(pidPath), err)
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open log file %s: %w", logPath, err)
	}

	child := exec.Command(exe, "serve", "--config", cfgPath)
	child.Stdin = nil
	child.Stdout = logFile
	child.Stderr = logFile
	child.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := child.Start(); err != nil {
		logFile.Close()
		return fmt.Errorf("start daemon: %w", err)
	}

	pid := child.Process.Pid
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(pid)+"\n"), 0o600); err != nil {
		_ = child.Process.Signal(syscall.SIGTERM)
		logFile.Close()
		return fmt.Errorf("write pidfile %s: %w", pidPath, err)
	}

	if err := child.Process.Release(); err != nil {
		logFile.Close()
		return fmt.Errorf("release daemon: %w", err)
	}

	// Close our handle on the log file. The child has its own inherited handle
	// via Setsid, so writing continues. We only needed it for redirection.
	_ = logFile.Close()

	fmt.Fprintf(cmd.OutOrStdout(), "started aiproxy in background (pid %d)\nlog: %s\npidfile: %s\n", pid, logPath, pidPath)
	return nil
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
	return true
}

// stopServer reads the pidfile and sends SIGTERM, escalating to SIGKILL after
// a timeout. Returns nil on success.
func stopServer(cfgPath string, out io.Writer) error {
	pidPath, _ := resolveDaemonPaths()
	pid, err := readLivePID(pidPath)
	if err != nil || pid == 0 {
		return errors.New("no server running")
	}
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
			_ = os.Remove(pidPath)
			fmt.Fprintf(out, "stopped aiproxy (pid %d)\n", pid)
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	if err := proc.Signal(syscall.SIGKILL); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return fmt.Errorf("kill pid %d: %w", pid, err)
	}
	_ = os.Remove(pidPath)
	fmt.Fprintf(out, "killed aiproxy (pid %d)\n", pid)
	return nil
}

// statusServer reports whether the daemon is running.
func statusServer(cfgPath string, out io.Writer) error {
	pidPath, _ := resolveDaemonPaths()
	pid, _ := readLivePID(pidPath)
	if pid == 0 {
		fmt.Fprintln(out, "no server running")
		return errors.New("not running")
	}
	fmt.Fprintf(out, "running (pid %d)\n", pid)
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
