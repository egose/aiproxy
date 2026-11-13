package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/tlsprofile"
)

const fixtureRuntimeJSON = `{"version":"1.4.4","revision":"1111111111111111111111111111111111111111","platform":"linux","arch":"x64"}`

func TestMain(tests *testing.M) {
	name := strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe")
	if strings.HasPrefix(name, "bun-fixture-") {
		os.Exit(fixtureBunMain(strings.TrimPrefix(name, "bun-fixture-")))
	}
	if name == "asdf" && len(os.Args) > 1 && os.Args[1] == "which" {
		os.Exit(fixtureAsdfMain())
	}
	os.Exit(tests.Run())
}

func toolFixture(t testing.TB) []byte {
	t.Helper()
	data, err := os.ReadFile("../../internal/tlsprofile/testdata/bun-1.4.4-fetch.clienthello.hex")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := hex.DecodeString(strings.Join(strings.Fields(string(data)), ""))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func toolFixtureProfile(t testing.TB) tlsprofile.Profile {
	t.Helper()
	data, err := tlsprofile.ExportClientHello(toolFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	profile, err := tlsprofile.ParseCustom(data)
	if err != nil {
		t.Fatal(err)
	}
	return profile
}

func fixtureExecutable(t testing.TB, mode string) string {
	t.Helper()
	directory := t.TempDir()
	path := filepath.Join(directory, "bun-fixture-"+mode)
	if runtime.GOOS == "windows" {
		path += ".exe"
	}
	linkTestExecutable(t, path)
	if err := os.WriteFile(filepath.Join(directory, "hello.hex"), []byte(hex.EncodeToString(toolFixture(t))), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func linkTestExecutable(t testing.TB, path string) {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if os.Link(executable, path) == nil {
		return
	}
	source, err := os.Open(executable)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	target, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0700)
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := io.Copy(target, source)
	closeErr := target.Close()
	if writeErr != nil || closeErr != nil {
		t.Fatal("could not create the hermetic child executable")
	}
}

func fixtureBunMain(mode string) int {
	if len(os.Args) != 7 || !strings.HasPrefix(os.Args[1], "--config=") || os.Args[2] != "--no-env-file" || os.Args[3] != "--no-install" || os.Args[4] != "run" {
		return 11
	}
	directory, err := os.Getwd()
	if err != nil || directory != os.Getenv("HOME") || directory != os.Getenv("XDG_CONFIG_HOME") || directory != os.Getenv("TMPDIR") {
		return 12
	}
	for _, entry := range os.Environ() {
		key := strings.SplitN(entry, "=", 2)[0]
		switch key {
		case "HOME", "XDG_CONFIG_HOME", "TMPDIR", "TMP", "TEMP", "NO_PROXY", "no_proxy", "LC_ALL", "LANG", "SYSTEMROOT":
		default:
			return 13
		}
	}
	if os.Getenv("NO_PROXY") != "*" || os.Getenv("no_proxy") != "*" {
		return 14
	}
	config := strings.TrimPrefix(os.Args[1], "--config=")
	data, err := os.ReadFile(config)
	if err != nil || len(data) != 0 || filepath.Dir(config) != directory || filepath.Dir(os.Args[5]) != directory {
		return 15
	}
	script, err := os.ReadFile(os.Args[5])
	if err != nil || string(script) != bunFetchScript {
		return 16
	}
	target, err := url.Parse(os.Args[6])
	if err != nil || target.Scheme != "https" || target.Hostname() != "127.0.0.1" || target.Path != "/" || target.User != nil || target.RawQuery != "" {
		return 17
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(os.Args[0]), fmt.Sprintf("invocation-%d", os.Getpid())), nil, 0600); err != nil {
		return 18
	}
	if mode == "stdout-overflow" || mode == "stderr-overflow" {
		writer := io.Writer(os.Stdout)
		if mode == "stderr-overflow" {
			writer = os.Stderr
		}
		writer.Write(bytes.Repeat([]byte{'x'}, maxChildOutputBytes+1))
		return 0
	}
	if mode == "exit" {
		fmt.Fprintln(os.Stderr, "private-sentinel child output")
		return 19
	}
	if mode == "no-connect" {
		time.Sleep(10 * time.Second)
		return 0
	}
	if mode == "bad-metadata" {
		fmt.Fprintln(os.Stdout, `{"version":"private-sentinel","revision":"1111111","platform":"linux","arch":"x64"}`)
	} else if mode == "runtime-changes" {
		entries, _ := os.ReadDir(filepath.Dir(os.Args[0]))
		count := 0
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), "invocation-") {
				count++
			}
		}
		info := fixtureRuntimeJSON
		if count > 1 {
			info = strings.Replace(info, "1.4.4", "1.4.5", 1)
		}
		fmt.Fprintln(os.Stdout, info)
	} else {
		fmt.Fprintln(os.Stdout, fixtureRuntimeJSON)
	}
	if mode == "extra-output" {
		fmt.Fprintln(os.Stdout, "private-sentinel extra output")
	}
	connection, err := net.DialTimeout("tcp4", target.Host, time.Second)
	if err != nil {
		return 20
	}
	defer connection.Close()
	if mode == "no-hello" {
		time.Sleep(10 * time.Second)
		return 0
	}
	encoded, err := os.ReadFile(filepath.Join(filepath.Dir(os.Args[0]), "hello.hex"))
	if err != nil {
		return 21
	}
	raw, err := hex.DecodeString(string(encoded))
	if err != nil {
		return 22
	}
	if mode == "truncate" {
		connection.Write(raw[:20])
		return 0
	}
	if mode == "oversized" {
		connection.Write([]byte{22, 3, 1, 0x48, 1})
		return 0
	}
	if mode == "record-mismatch" {
		raw[2] = 3
	}
	if mode == "record-fragments" {
		var fragmented []byte
		message := raw[5:]
		for _, length := range []int{1, 3, 100, len(message) - 104} {
			fragmented = append(fragmented, 22, 3, 1, byte(length>>8), byte(length))
			fragmented = append(fragmented, message[:length]...)
			message = message[length:]
		}
		raw = fragmented
	}
	chunk := len(raw)
	if mode == "tcp-fragments" {
		chunk = 1
	}
	for len(raw) != 0 {
		length := min(len(raw), chunk)
		if _, err := connection.Write(raw[:length]); err != nil {
			return 23
		}
		raw = raw[length:]
	}
	if mode == "hang-after-hello" {
		time.Sleep(10 * time.Second)
		return 0
	}
	connection.SetReadDeadline(time.Now().Add(time.Second))
	var reply [1]byte
	if count, err := connection.Read(reply[:]); count != 0 || err == nil {
		return 24
	}
	return 0
}

func fixtureAsdfMain() int {
	if len(os.Args) != 3 || os.Args[2] != "bun" {
		return 31
	}
	for _, entry := range os.Environ() {
		key := strings.SplitN(entry, "=", 2)[0]
		switch key {
		case "PATH", "HOME", "LC_ALL", "LANG", "ASDF_DATA_DIR", "SYSTEMROOT":
		default:
			return 32
		}
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(os.Args[0]), "resolved.txt"))
	if err != nil {
		return 33
	}
	os.Stdout.Write(data)
	return 0
}

func invocationCount(t testing.TB, executable string) int {
	t.Helper()
	entries, err := os.ReadDir(filepath.Dir(executable))
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "invocation-") {
			count++
		}
	}
	return count
}

func TestBunCaptureIsolatedEnvironmentAndFreshProcesses(t *testing.T) {
	for _, key := range []string{"UPSTREAM_API_KEY", "AIPROXY_CONFIG", "AWS_SECRET_ACCESS_KEY", "HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy", "ALL_PROXY", "all_proxy", "NODE_OPTIONS", "BUN_OPTIONS", "BUN_INSTALL", "BUN_FEATURE_FLAG", "NODE_TLS_REJECT_UNAUTHORIZED", "BASH_ENV", "LD_PRELOAD"} {
		t.Setenv(key, "private-sentinel")
	}
	parentHome := t.TempDir()
	t.Setenv("HOME", parentHome)
	t.Setenv("XDG_CONFIG_HOME", parentHome)
	t.Setenv("NO_PROXY", "parent.invalid")
	t.Setenv("no_proxy", "parent.invalid")
	if err := os.WriteFile(filepath.Join(parentHome, ".env"), []byte("UPSTREAM_API_KEY=private-sentinel"), 0600); err != nil {
		t.Fatal(err)
	}
	executable := fixtureExecutable(t, "success")
	runner := bunCapture{executable: executable, limits: defaultCaptureLimits()}
	for sample := 0; sample < 3; sample++ {
		raw, info, err := runner.capture(context.Background())
		if err != nil || !bytes.Equal(raw, toolFixture(t)) || info.Version != "1.4.4" {
			t.Fatalf("isolated capture failed: %v", err)
		}
	}
	if invocationCount(t, executable) != 3 {
		t.Fatal("samples reused a child process")
	}
	if os.Getenv("UPSTREAM_API_KEY") != "private-sentinel" || os.Getenv("NO_PROXY") != "parent.invalid" || os.Getenv("HOME") != parentHome {
		t.Fatal("capture changed the parent environment")
	}
}

func TestBunCaptureTCPAndRecordFragmentation(t *testing.T) {
	for _, mode := range []string{"tcp-fragments", "record-fragments"} {
		t.Run(mode, func(t *testing.T) {
			runner := bunCapture{executable: fixtureExecutable(t, mode), limits: defaultCaptureLimits()}
			raw, _, err := runner.capture(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if _, err := tlsprofile.ExportClientHello(raw); err != nil {
				t.Fatalf("fragmented capture did not export: %v", err)
			}
			if mode == "record-fragments" {
				summary, _ := tlsprofile.SummarizeClientHello(raw)
				if len(summary.RecordVersions) != 4 {
					t.Fatal("TLS record boundaries were flattened")
				}
			}
		})
	}
}

func TestBunCaptureFailuresAreBoundedAndPrivate(t *testing.T) {
	for _, mode := range []string{"truncate", "oversized", "no-connect", "no-hello", "hang-after-hello", "stdout-overflow", "stderr-overflow", "exit", "bad-metadata", "extra-output"} {
		t.Run(mode, func(t *testing.T) {
			executable := fixtureExecutable(t, mode)
			runner := bunCapture{executable: executable, limits: captureLimits{accept: 100 * time.Millisecond, read: 100 * time.Millisecond, child: 300 * time.Millisecond}}
			start := time.Now()
			if raw, _, err := runner.capture(context.Background()); err == nil || raw != nil || strings.Contains(err.Error(), "private-sentinel") || strings.Contains(err.Error(), filepath.Dir(executable)) {
				t.Fatalf("capture failed unsafely: %v", err)
			}
			if time.Since(start) > 2*time.Second {
				t.Fatal("capture did not reap its child within the bounded deadline")
			}
		})
	}
}

func TestBunCaptureCancellation(t *testing.T) {
	executable := fixtureExecutable(t, "no-hello")
	runner := bunCapture{executable: executable, limits: defaultCaptureLimits()}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := runner.capture(ctx); !errors.Is(err, context.Canceled) || invocationCount(t, executable) != 0 {
		t.Fatal("pre-canceled capture started a child")
	}
	ctx, cancel = context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, _, err := runner.capture(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("mid-capture cancellation was lost: %v", err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("cancellation did not close/reap capture resources promptly")
	}
}

type delayedDeadlineContext struct {
	context.Context
	deadline time.Time
}

func (ctx delayedDeadlineContext) Deadline() (time.Time, bool) {
	return ctx.deadline, true
}

func TestCaptureContextDeadlineOrdering(t *testing.T) {
	ctx := delayedDeadlineContext{Context: context.Background(), deadline: time.Now().Add(-time.Second)}
	if ctx.Err() != nil || !errors.Is(captureContextError(ctx), context.DeadlineExceeded) {
		t.Fatal("elapsed socket/context deadlines depend on timer goroutine ordering")
	}
	ctx.deadline = time.Now().Add(time.Minute)
	if captureContextError(ctx) != nil || captureContextError(context.Background()) != nil {
		t.Fatal("future or unbounded contexts were prematurely expired")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if !errors.Is(captureContextError(canceled), context.Canceled) {
		t.Fatal("explicit cancellation cause was replaced by a deadline")
	}
}

func TestBunRuntimeMetadataStrictAndSecretFree(t *testing.T) {
	if info, err := parseBunRuntime([]byte(fixtureRuntimeJSON)); err != nil || info.Version != "1.4.4" {
		t.Fatal("valid runtime metadata was rejected")
	}
	for _, data := range []string{
		"", "null", "[]", "{}", fixtureRuntimeJSON + " {}", fixtureRuntimeJSON + " private-sentinel",
		`{"version":"1.4.4","version":"1.4.4","revision":"1111111","platform":"linux","arch":"x64"}`,
		`{"version":"1.4.4","revision":"1111111","platform":"linux","arch":"x64","executable":"private-sentinel"}`,
		strings.Replace(fixtureRuntimeJSON, "1.4.4", "private-sentinel", 1),
		strings.Replace(fixtureRuntimeJSON, "linux", "private-sentinel", 1),
		strings.Replace(fixtureRuntimeJSON, "x64", "private-sentinel", 1),
		strings.Repeat(" ", maxChildOutputBytes+1),
	} {
		if _, err := parseBunRuntime([]byte(data)); err == nil || strings.Contains(err.Error(), "private-sentinel") {
			t.Fatalf("metadata failure leaked data or accepted malformed input: %v", err)
		}
	}
}

func TestResolveBunMissingRuntimeAndUnsupportedWrappers(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("PATH", directory)
	if _, err := resolveBun(context.Background(), "missing-private-sentinel"); err == nil || !strings.Contains(err.Error(), "--bun") || strings.Contains(err.Error(), "private-sentinel") {
		t.Fatalf("missing runtime was not actionable and private: %v", err)
	}
	for _, contents := range []string{"", "#!/bin/sh\necho private-sentinel\n", "#!/usr/bin/env bash\n# asdf-plugin: bun 1.4.4\nexec asdf exec \"bun\" \"$@\"\n"} {
		path := filepath.Join(directory, "bun")
		if err := os.WriteFile(path, []byte(contents), 0700); err != nil {
			t.Fatal(err)
		}
		if _, err := resolveBun(context.Background(), "bun"); err == nil || strings.Contains(err.Error(), "private-sentinel") {
			t.Fatalf("unsafe/missing wrapper was not rejected: %v", err)
		}
	}
	for _, selected := range []string{"", "private-sentinel\n", strings.Repeat("x", 4097)} {
		if _, err := resolveBun(context.Background(), selected); err == nil || strings.Contains(err.Error(), "private-sentinel") {
			t.Fatal("invalid executable path was not rejected privately")
		}
	}
}

func TestResolveBunAsdfShimWithoutSecretInheritance(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix asdf shim fixture")
	}
	directory := t.TempDir()
	direct := fixtureExecutable(t, "success")
	shim := "#!/usr/bin/env bash\n# asdf-plugin: bun 1.4.4\nexec asdf exec \"bun\" \"$@\"\n"
	if err := os.WriteFile(filepath.Join(directory, "bun"), []byte(shim), 0700); err != nil {
		t.Fatal(err)
	}
	linkTestExecutable(t, filepath.Join(directory, "asdf"))
	t.Setenv("PATH", directory)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("ASDF_DATA_DIR", t.TempDir())
	for _, key := range []string{"HTTPS_PROXY", "UPSTREAM_API_KEY", "AIPROXY_CONFIG", "NODE_OPTIONS", "BUN_OPTIONS", "BASH_ENV", "LD_PRELOAD"} {
		t.Setenv(key, "private-sentinel")
	}
	resolutionFile := filepath.Join(directory, "resolved.txt")
	if err := os.WriteFile(resolutionFile, []byte(direct+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	resolved, err := resolveBun(context.Background(), "bun")
	if err != nil || resolved != direct {
		t.Fatalf("known asdf shim was not safely resolved: %v", err)
	}
	for _, output := range []string{"relative-private-sentinel", direct + "\n/another/private-sentinel", filepath.Join(directory, "bun")} {
		if err := os.WriteFile(resolutionFile, []byte(output), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := resolveBun(context.Background(), "bun"); err == nil || strings.Contains(err.Error(), "private-sentinel") {
			t.Fatalf("unsafe shim resolution did not fail privately: %v", err)
		}
	}
}

func TestChildOutputBufferBounds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	buffer := boundedChildBuffer{cancel: cancel}
	if count, err := buffer.Write(make([]byte, maxChildOutputBytes)); count != maxChildOutputBytes || err != nil || buffer.exceeded() {
		t.Fatal("exact output bound was rejected")
	}
	if _, err := buffer.Write([]byte{1}); !errors.Is(err, errChildOutputLimit) || !buffer.exceeded() || ctx.Err() != context.Canceled || len(buffer.bytes()) != maxChildOutputBytes {
		t.Fatal("output overflow did not stop the child or remained unbounded")
	}
	copy := buffer.bytes()
	copy[0] = 1
	if buffer.bytes()[0] != 0 {
		t.Fatal("child output exposed mutable storage")
	}
}

func TestMaterializedCaptureAndComparison(t *testing.T) {
	profile := toolFixtureProfile(t)
	for sample := 0; sample < 3; sample++ {
		raw, err := captureProfile(context.Background(), profile, defaultCaptureLimits())
		if err != nil {
			t.Fatal(err)
		}
		normalized, err := tlsprofile.NormalizeClientHello(raw)
		if err != nil || !bytes.Equal(normalized, toolFixture(t)) {
			t.Fatal("fresh on-wire materialization did not match the measured fixture")
		}
	}
	result, err := compareProfile(context.Background(), [][]byte{toolFixture(t), toolFixture(t)}, profile, defaultCaptureLimits())
	if err != nil || !result.Matched || result.BunSamples != 2 || result.UTLSSamples != 2 || result.Structure.RecordBytes != 1500 {
		t.Fatalf("full comparison failed: %+v, %v", result, err)
	}
	changed := toolFixture(t)
	changed[2] = 3
	result, err = compareProfile(context.Background(), [][]byte{toolFixture(t), changed}, profile, defaultCaptureLimits())
	if err != nil || result.Matched || len(result.Differences) != 1 || !strings.Contains(result.Differences[0], "offset 2") {
		t.Fatal("record-version variation was normalized away")
	}
	if _, err := compareProfile(context.Background(), [][]byte{toolFixture(t)}, profile, defaultCaptureLimits()); err == nil {
		t.Fatal("comparison accepted only one sample")
	}
	if _, err := captureProfile(context.Background(), tlsprofile.Native(), defaultCaptureLimits()); err == nil {
		t.Fatal("native profile unexpectedly used uTLS")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := captureProfile(ctx, profile, defaultCaptureLimits()); !errors.Is(err, context.Canceled) {
		t.Fatal("generated capture ignored cancellation")
	}
}

func TestRecordComparisonReportsNoCapturedValues(t *testing.T) {
	if recordDifference([]byte{1}, []byte{1}) != "" || !strings.Contains(recordDifference([]byte{1}, []byte{1, 2}), "lengths") ||
		!strings.Contains(recordDifference([]byte{1}, []byte{2}), "offset 0") {
		t.Fatal("record comparison did not preserve byte/length distinctions")
	}
	fixture := toolFixture(t)
	if binary.BigEndian.Uint16(fixture[3:5]) != 1495 {
		t.Fatal("fixture TLS framing changed")
	}
	var info bunRuntime
	if json.Unmarshal([]byte(fixtureRuntimeJSON), &info) != nil {
		t.Fatal("invalid runtime test fixture")
	}
	metadata := newCaptureMetadata(info, toolFixtureProfile(t), 3, comparisonResult{Matched: true})
	encoded, _ := json.Marshal(metadata)
	if bytes.Contains(encoded, []byte("private-sentinel")) || bytes.Contains(encoded, []byte("/home/")) || bytes.Contains(encoded, []byte("/tmp/")) {
		t.Fatal("metadata included process/machine context")
	}
}
