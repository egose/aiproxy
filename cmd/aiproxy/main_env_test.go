package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestEnvTruthyVariants(t *testing.T) {
	cases := map[string]bool{
		"":       false,
		"0":      false,
		"false":  false,
		"no":     false,
		"1":      true,
		"true":   true,
		"TRUE":   true,
		" True ": true,
		"yes":    true,
		"YES":    true,
		"on":     true,
		"ON":     true,
	}
	for value, want := range cases {
		t.Setenv(defaultServeEnv, value)
		t.Setenv(daemonEnv, value)
		if got := envTruthy(defaultServeEnv); got != want {
			t.Errorf("envTruthy(%s=%q) = %v, want %v", defaultServeEnv, value, got, want)
		}
		if got := envTruthy(daemonEnv); got != want {
			t.Errorf("envTruthy(%s=%q) = %v, want %v", daemonEnv, value, got, want)
		}
	}
}

func TestDaemonRequestedHonorsEnv(t *testing.T) {
	t.Setenv(daemonEnv, "1")
	cmd := newServeCommand()
	if !daemonRequested(cmd) {
		t.Fatalf("daemonRequested = false with %s=1", daemonEnv)
	}
}

func TestDaemonRequestedDefaultsOff(t *testing.T) {
	t.Setenv(daemonEnv, "")
	cmd := newServeCommand()
	if daemonRequested(cmd) {
		t.Fatalf("daemonRequested = true with %s unset", daemonEnv)
	}
}

func TestDaemonFlagOverridesEnv(t *testing.T) {
	t.Setenv(daemonEnv, "1")
	cmd := newServeCommand()
	cmd.SetArgs([]string{"--daemon=false"})
	if err := cmd.ParseFlags([]string{"--daemon=false"}); err != nil {
		t.Fatalf("ParseFlags(): %v", err)
	}
	if daemonRequested(cmd) {
		t.Fatalf("daemonRequested = true despite explicit --daemon=false")
	}
}

func TestRootNoArgsShowsHelpByDefault(t *testing.T) {
	t.Setenv(defaultServeEnv, "")
	cmd := newRootCommand()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute(): %v", err)
	}
	if !strings.Contains(buf.String(), "Proxies multiple AI providers") {
		t.Fatalf("bare output missing help text:\n%s", buf.String())
	}
}

func TestRootNoArgsServesWhenEnvSet(t *testing.T) {
	t.Setenv(defaultServeEnv, "1")
	t.Setenv("AIPROXY_CONFIG", "invalid hcl >>>")
	cmd := newRootCommand()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "Invalid block definition") {
		t.Fatalf("Execute() = %v, want config parse error proving serve delegation", err)
	}
}

func TestRootDaemonEnvRequiresConfigFile(t *testing.T) {
	t.Setenv(defaultServeEnv, "1")
	t.Setenv(daemonEnv, "1")
	t.Setenv("AIPROXY_CONFIG", `listener "http" "public" { address = ":0" }`)
	cmd := newRootCommand()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "daemon mode requires a config file") {
		t.Fatalf("Execute() = %v, want daemon file requirement error", err)
	}
}

func TestServeDaemonEnvRequiresConfigFile(t *testing.T) {
	t.Setenv(daemonEnv, "1")
	t.Setenv("AIPROXY_CONFIG", `listener "http" "public" { address = ":0" }`)
	cmd := newRootCommand()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"serve"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "daemon mode requires a config file") {
		t.Fatalf("serve Execute() = %v, want daemon file requirement error", err)
	}
}

func TestServeDaemonFlagFalseOverridesEnv(t *testing.T) {
	t.Setenv(daemonEnv, "1")
	t.Setenv("AIPROXY_CONFIG", "invalid hcl >>>")
	cmd := newRootCommand()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"serve", "--daemon=false"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "Invalid block definition") {
		t.Fatalf("serve Execute() = %v, want foreground parse error proving flag override", err)
	}
}
