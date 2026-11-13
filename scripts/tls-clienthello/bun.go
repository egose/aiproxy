package main

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/egose/aiproxy/internal/tlsprofile"
)

const maxChildOutputBytes = 64 << 10

//go:embed fetch.js
var bunFetchScript string

var (
	errChildOutputLimit = errors.New("child output limit exceeded")
	bunVersionPattern   = regexp.MustCompile(`^[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}(?:-[0-9A-Za-z.-]{1,64})?$`)
	bunRevisionPattern  = regexp.MustCompile(`^[0-9a-f]{7,64}$`)
)

type captureLimits struct {
	accept time.Duration
	read   time.Duration
	child  time.Duration
}

func defaultCaptureLimits() captureLimits {
	return captureLimits{accept: 3 * time.Second, read: 3 * time.Second, child: 5 * time.Second}
}

type bunRuntime struct {
	Version  string `json:"version"`
	Revision string `json:"revision"`
	Platform string `json:"platform"`
	Arch     string `json:"arch"`
}

type bunCapture struct {
	executable string
	limits     captureLimits
}

func (runner bunCapture) capture(parent context.Context) ([]byte, bunRuntime, error) {
	ctx, cancel := context.WithTimeout(parent, runner.limits.child)
	defer cancel()
	if err := captureContextError(ctx); err != nil {
		return nil, bunRuntime{}, err
	}
	directory, err := os.MkdirTemp("", "aiproxy-clienthello-")
	if err != nil {
		return nil, bunRuntime{}, fmt.Errorf("cannot create an isolated Bun working directory")
	}
	defer os.RemoveAll(directory)
	configPath, scriptPath := filepath.Join(directory, "bunfig.toml"), filepath.Join(directory, "fetch.js")
	if err := os.WriteFile(configPath, nil, 0600); err != nil {
		return nil, bunRuntime{}, fmt.Errorf("cannot create the empty Bun config")
	}
	if err := os.WriteFile(scriptPath, []byte(bunFetchScript), 0600); err != nil {
		return nil, bunRuntime{}, fmt.Errorf("cannot create the isolated fetch script")
	}
	listener, err := listenLoopback(ctx, runner.limits.accept)
	if err != nil {
		return nil, bunRuntime{}, err
	}
	defer listener.Close()
	stopListener := context.AfterFunc(ctx, func() { listener.Close() })
	defer stopListener()
	target := "https://" + listener.Addr().String() + "/"
	arguments := []string{"--config=" + configPath, "--no-env-file", "--no-install", "run", scriptPath, target}
	child, err := startChild(ctx, runner.executable, arguments, directory, captureChildEnv(directory), "Bun", func() { listener.Close() })
	if err != nil {
		return nil, bunRuntime{}, err
	}
	defer child.close()
	connection, err := listener.AcceptTCP()
	if err != nil {
		operationErr := captureContextError(ctx)
		cancel()
		if operationErr != nil {
			return nil, bunRuntime{}, operationErr
		}
		if result := child.result(); result != nil && !errors.Is(result, context.Canceled) {
			return nil, bunRuntime{}, result
		}
		if parent.Err() != nil {
			return nil, bunRuntime{}, parent.Err()
		}
		if netError, ok := err.(net.Error); ok && netError.Timeout() {
			return nil, bunRuntime{}, fmt.Errorf("Bun did not connect within the loopback accept deadline")
		}
		return nil, bunRuntime{}, fmt.Errorf("Bun exited without a loopback ClientHello; select an installed binary with --bun")
	}
	defer connection.Close()
	stopConnection := context.AfterFunc(ctx, func() { connection.Close() })
	defer stopConnection()
	if err := connection.SetReadDeadline(captureDeadline(ctx, runner.limits.read)); err != nil {
		operationErr := captureContextError(ctx)
		cancel()
		if operationErr != nil {
			return nil, bunRuntime{}, operationErr
		}
		return nil, bunRuntime{}, fmt.Errorf("cannot set the ClientHello read deadline")
	}
	raw, err := tlsprofile.ReadClientHello(connection)
	connection.Close()
	listener.Close()
	if err != nil {
		operationErr := captureContextError(ctx)
		cancel()
		if operationErr != nil {
			return nil, bunRuntime{}, operationErr
		}
		if netError := new(net.OpError); errors.As(err, &netError) && netError.Timeout() {
			return nil, bunRuntime{}, fmt.Errorf("ClientHello read deadline exceeded")
		}
		return nil, bunRuntime{}, fmt.Errorf("cannot read a complete bounded ClientHello: %w", err)
	}
	if err := child.result(); err != nil {
		return nil, bunRuntime{}, err
	}
	info, err := parseBunRuntime(child.stdout.bytes())
	if err != nil {
		return nil, bunRuntime{}, err
	}
	return raw, info, nil
}

func parseBunRuntime(data []byte) (bunRuntime, error) {
	if len(data) > maxChildOutputBytes {
		return bunRuntime{}, fmt.Errorf("Bun runtime metadata exceeds its output limit")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	start, err := decoder.Token()
	if err != nil || start != json.Delim('{') {
		return bunRuntime{}, fmt.Errorf("Bun did not emit valid runtime metadata; pass --bun pointing to the installed binary")
	}
	fields := make(map[string]string)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return bunRuntime{}, fmt.Errorf("Bun runtime metadata is malformed")
		}
		key, ok := token.(string)
		if !ok || key != "version" && key != "revision" && key != "platform" && key != "arch" {
			return bunRuntime{}, fmt.Errorf("Bun runtime metadata contains an unsupported field")
		}
		if _, exists := fields[key]; exists {
			return bunRuntime{}, fmt.Errorf("Bun runtime metadata contains a duplicate field")
		}
		var value string
		if err := decoder.Decode(&value); err != nil {
			return bunRuntime{}, fmt.Errorf("Bun runtime metadata contains an invalid field")
		}
		fields[key] = value
	}
	if end, err := decoder.Token(); err != nil || end != json.Delim('}') {
		return bunRuntime{}, fmt.Errorf("Bun runtime metadata is malformed")
	}
	if decoder.Decode(new(any)) != io.EOF {
		return bunRuntime{}, fmt.Errorf("Bun emitted unexpected output; no child output is included in metadata")
	}
	info := bunRuntime{Version: fields["version"], Revision: fields["revision"], Platform: fields["platform"], Arch: fields["arch"]}
	if !bunVersionPattern.MatchString(info.Version) || !bunRevisionPattern.MatchString(info.Revision) {
		return bunRuntime{}, fmt.Errorf("Bun version or revision metadata is unsupported")
	}
	if info.Platform != "linux" && info.Platform != "darwin" && info.Platform != "win32" {
		return bunRuntime{}, fmt.Errorf("Bun platform metadata is unsupported")
	}
	if info.Arch != "x64" && info.Arch != "arm64" {
		return bunRuntime{}, fmt.Errorf("Bun architecture metadata is unsupported")
	}
	return info, nil
}

func captureChildEnv(directory string) []string {
	environment := []string{
		"HOME=" + directory, "XDG_CONFIG_HOME=" + directory, "TMPDIR=" + directory, "TMP=" + directory, "TEMP=" + directory,
		"NO_PROXY=*", "no_proxy=*", "LC_ALL=C", "LANG=C",
	}
	if runtime.GOOS == "windows" {
		if root := os.Getenv("SYSTEMROOT"); root != "" {
			environment = append(environment, "SYSTEMROOT="+root)
		}
	}
	return environment
}

func resolveBun(ctx context.Context, selected string) (string, error) {
	if !validLocalPath(selected) {
		return "", fmt.Errorf("--bun must be an executable name or a valid local path")
	}
	path, err := exec.LookPath(selected)
	if err != nil {
		return "", fmt.Errorf("Bun was not found; install it separately or pass --bun pointing to an installed binary")
	}
	path, prefix, err := inspectExecutable(path)
	if err != nil {
		return "", err
	}
	if !bytes.HasPrefix(prefix, []byte("#!")) {
		return path, nil
	}
	if !bytes.Contains(prefix, []byte("# asdf-plugin: bun ")) || !bytes.Contains(prefix, []byte(`exec asdf exec "bun" "$@"`)) {
		return "", fmt.Errorf("selected Bun is an unsupported wrapper; pass --bun pointing to the actual installed binary")
	}
	asdf, err := exec.LookPath("asdf")
	if err != nil {
		return "", fmt.Errorf("Bun is an asdf shim but asdf is unavailable; pass --bun pointing to the installed binary")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot resolve the asdf Bun shim; pass --bun pointing to the installed binary")
	}
	environment := []string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "LC_ALL=C", "LANG=C"}
	if directory := os.Getenv("ASDF_DATA_DIR"); directory != "" {
		environment = append(environment, "ASDF_DATA_DIR="+directory)
	}
	resolveCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	child, err := startChild(resolveCtx, asdf, []string{"which", "bun"}, "", environment, "asdf Bun resolver", nil)
	if err != nil {
		return "", fmt.Errorf("cannot resolve the asdf Bun shim; pass --bun with the result of asdf which bun")
	}
	defer child.close()
	if err := child.result(); err != nil {
		return "", fmt.Errorf("asdf which bun failed in a secret-free environment; pass --bun pointing to the installed binary: %w", err)
	}
	resolved := strings.TrimSpace(string(child.stdout.bytes()))
	if !validLocalPath(resolved) || !filepath.IsAbs(resolved) {
		return "", fmt.Errorf("asdf did not resolve a single absolute executable path; pass --bun pointing to the installed binary")
	}
	path, prefix, err = inspectExecutable(resolved)
	if err != nil || bytes.HasPrefix(prefix, []byte("#!")) {
		return "", fmt.Errorf("asdf did not resolve a direct Bun binary; pass --bun pointing to the installed binary")
	}
	return path, nil
}

func inspectExecutable(path string) (string, []byte, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", nil, fmt.Errorf("cannot resolve the selected Bun executable")
	}
	absolute, err = filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", nil, fmt.Errorf("selected Bun executable cannot be accessed")
	}
	file, err := openRegularRead(absolute)
	if err != nil {
		return "", nil, fmt.Errorf("selected Bun must be an accessible regular executable file")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || runtime.GOOS != "windows" && info.Mode()&0111 == 0 {
		return "", nil, fmt.Errorf("selected Bun file is not executable")
	}
	prefix, err := io.ReadAll(io.LimitReader(file, 4096))
	if err != nil || len(prefix) == 0 {
		return "", nil, fmt.Errorf("selected Bun executable cannot be inspected")
	}
	return absolute, prefix, nil
}

func validLocalPath(path string) bool {
	if path == "" || len(path) > 4096 || !utf8.ValidString(path) {
		return false
	}
	for _, value := range path {
		if unicode.IsControl(value) {
			return false
		}
	}
	return true
}

type boundedChildBuffer struct {
	mutex    sync.Mutex
	data     []byte
	overflow bool
	cancel   context.CancelFunc
}

func (buffer *boundedChildBuffer) Write(data []byte) (int, error) {
	buffer.mutex.Lock()
	defer buffer.mutex.Unlock()
	remaining := maxChildOutputBytes - len(buffer.data)
	if len(data) > remaining {
		buffer.data = append(buffer.data, data[:remaining]...)
		buffer.overflow = true
		buffer.cancel()
		return remaining, errChildOutputLimit
	}
	buffer.data = append(buffer.data, data...)
	return len(data), nil
}

func (buffer *boundedChildBuffer) bytes() []byte {
	buffer.mutex.Lock()
	defer buffer.mutex.Unlock()
	return append([]byte(nil), buffer.data...)
}

func (buffer *boundedChildBuffer) exceeded() bool {
	buffer.mutex.Lock()
	defer buffer.mutex.Unlock()
	return buffer.overflow
}

type childProcess struct {
	ctx    context.Context
	cancel context.CancelFunc
	stdout *boundedChildBuffer
	stderr *boundedChildBuffer
	done   chan struct{}
	err    error
	label  string
}

func startChild(parent context.Context, executable string, arguments []string, directory string, environment []string, label string, onExit func()) (*childProcess, error) {
	ctx, cancel := context.WithCancel(parent)
	child := &childProcess{
		ctx: ctx, cancel: cancel, stdout: &boundedChildBuffer{cancel: cancel}, stderr: &boundedChildBuffer{cancel: cancel},
		done: make(chan struct{}), label: label,
	}
	command := exec.CommandContext(ctx, executable, arguments...)
	command.Dir, command.Env = directory, environment
	command.Stdout, command.Stderr = child.stdout, child.stderr
	command.WaitDelay = 100 * time.Millisecond
	if err := command.Start(); err != nil {
		operationErr := captureContextError(ctx)
		cancel()
		if operationErr != nil {
			return nil, fmt.Errorf("%s process canceled or timed out: %w", label, operationErr)
		}
		return nil, fmt.Errorf("cannot start %s; select an installed executable without a wrapper", label)
	}
	go func() {
		child.err = command.Wait()
		if onExit != nil {
			onExit()
		}
		close(child.done)
	}()
	return child, nil
}

func (child *childProcess) result() error {
	<-child.done
	if child.stdout.exceeded() || child.stderr.exceeded() {
		return fmt.Errorf("%s stdout or stderr exceeded the 65536-byte limit", child.label)
	}
	if err := captureContextError(child.ctx); err != nil {
		return fmt.Errorf("%s process canceled or timed out: %w", child.label, err)
	}
	if child.err != nil {
		return fmt.Errorf("%s process failed; child output is suppressed", child.label)
	}
	return nil
}

func (child *childProcess) close() {
	child.cancel()
	<-child.done
}

func listenLoopback(ctx context.Context, timeout time.Duration) (*net.TCPListener, error) {
	listener, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		return nil, fmt.Errorf("cannot create the loopback capture listener")
	}
	if err := listener.SetDeadline(captureDeadline(ctx, timeout)); err != nil {
		listener.Close()
		return nil, fmt.Errorf("cannot set the loopback accept deadline")
	}
	return listener, nil
}

func captureDeadline(ctx context.Context, timeout time.Duration) time.Time {
	deadline := time.Now().Add(timeout)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		return contextDeadline
	}
	return deadline
}

func captureContextError(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if deadline, ok := ctx.Deadline(); ok && !time.Now().Before(deadline) {
		return context.DeadlineExceeded
	}
	return nil
}
