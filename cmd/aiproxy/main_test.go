package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultConfigPathUsesXDGConfigHome(t *testing.T) {
	xdgRoot := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdgRoot)
	t.Setenv("HOME", t.TempDir())

	got := defaultConfigPath()
	want := filepath.Join(xdgRoot, "aiproxy", "config.hcl")
	if got != want {
		t.Fatalf("defaultConfigPath() = %q, want %q", got, want)
	}
}

func TestDefaultConfigPathFallsBackToHomeConfigDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", home)

	got := defaultConfigPath()
	want := filepath.Join(home, ".config", "aiproxy", "config.hcl")
	if got != want {
		t.Fatalf("defaultConfigPath() = %q, want %q", got, want)
	}
}

func TestServeCommandDefaultsConfigFlag(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", home)

	cmd := newServeCommand()
	got, err := cmd.Flags().GetString("config")
	if err != nil {
		t.Fatalf("GetString(config): %v", err)
	}
	want := filepath.Join(home, ".config", "aiproxy", "config.hcl")
	if got != want {
		t.Fatalf("serve default config flag = %q, want %q", got, want)
	}
}

func TestValidateCommandDefaultsConfigFlag(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", home)

	cmd := newValidateCommand()
	got, err := cmd.Flags().GetString("config")
	if err != nil {
		t.Fatalf("GetString(config): %v", err)
	}
	want := filepath.Join(home, ".config", "aiproxy", "config.hcl")
	if got != want {
		t.Fatalf("validate default config flag = %q, want %q", got, want)
	}
}

func TestRootHelpIncludesDefaultPaths(t *testing.T) {
	cmd := newRootCommand()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--help"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute(): %v", err)
	}

	help := buf.String()
	checks := []string{
		"$XDG_CONFIG_HOME/aiproxy/config.hcl",
		"~/.config/aiproxy/config.hcl",
		"$XDG_CONFIG_HOME/aiproxy/keys.json",
		"~/.config/aiproxy/keys.json",
	}
	for _, check := range checks {
		if !strings.Contains(help, check) {
			t.Fatalf("help output missing %q:\n%s", check, help)
		}
	}
}

func TestPathsCommandUsesXDGConfigHome(t *testing.T) {
	xdgRoot := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdgRoot)
	t.Setenv("HOME", t.TempDir())

	cmd := newRootCommand()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"paths"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute(): %v", err)
	}

	want := "config: " + filepath.Join(xdgRoot, "aiproxy", "config.hcl") + "\n" +
		"secrets: " + filepath.Join(xdgRoot, "aiproxy", "keys.json") + "\n"
	if buf.String() != want {
		t.Fatalf("paths output = %q, want %q", buf.String(), want)
	}
}

func TestPathsCommandFallsBackToHomeConfigDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", home)

	cmd := newRootCommand()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"paths"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute(): %v", err)
	}

	want := "config: " + filepath.Join(home, ".config", "aiproxy", "config.hcl") + "\n" +
		"secrets: " + filepath.Join(home, ".config", "aiproxy", "keys.json") + "\n"
	if buf.String() != want {
		t.Fatalf("paths output = %q, want %q", buf.String(), want)
	}
}

func TestExamplesCommandIncludesCommandsAndConfig(t *testing.T) {
	cmd := newRootCommand()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"examples"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute(): %v", err)
	}

	out := buf.String()
	checks := []string{
		"aiproxy serve",
		"aiproxy validate",
		"aiproxy serve --config /etc/aiproxy/config.hcl",
		"listener \"http\" \"public\"",
		"provider \"openai\" \"openai\"",
		"env(\"OPENAI_API_KEY\")",
		"Alias failover example:",
		"alias \"chat_default\"",
		"provider \"openai-compatible\" \"backup\"",
		"api_key_ref override example:",
		"path = \"/etc/aiproxy/keys.json\"",
		"\"openai\": \"sk-...\"",
		"\"localai\": \"secret\"",
		"Docker example:",
		"docker run --rm \\",
		"systemd example:",
		"ExecStart=/usr/local/bin/aiproxy serve --config /etc/aiproxy/config.hcl",
	}
	for _, check := range checks {
		if !strings.Contains(out, check) {
			t.Fatalf("examples output missing %q:\n%s", check, out)
		}
	}
}

func TestExamplesConfigCommandPrintsConfigExamples(t *testing.T) {
	cmd := newRootCommand()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"examples", "config"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute(): %v", err)
	}

	out := buf.String()
	checks := []string{
		"Common commands:",
		"Minimal config:",
		"Alias failover example:",
		"api_key_ref override example:",
	}
	for _, check := range checks {
		if !strings.Contains(out, check) {
			t.Fatalf("config examples output missing %q:\n%s", check, out)
		}
	}
	if strings.Contains(out, "Docker example:") || strings.Contains(out, "systemd example:") {
		t.Fatalf("config examples output unexpectedly included deployment sections:\n%s", out)
	}
}

func TestExamplesAllCommandPrintsCombinedExamples(t *testing.T) {
	cmd := newRootCommand()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"examples", "all"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute(): %v", err)
	}

	out := buf.String()
	checks := []string{
		"Common commands:",
		"Docker example:",
		"systemd example:",
	}
	for _, check := range checks {
		if !strings.Contains(out, check) {
			t.Fatalf("all examples output missing %q:\n%s", check, out)
		}
	}
}

func TestExamplesAuthCommandPrintsAuthExample(t *testing.T) {
	cmd := newRootCommand()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"examples", "auth"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute(): %v", err)
	}

	out := buf.String()
	checks := []string{
		"Auth example:",
		"rate_limit {",
		"allowed_models = [\"alias/chat_default\", \"openai/gpt-4o-mini\"]",
	}
	for _, check := range checks {
		if !strings.Contains(out, check) {
			t.Fatalf("auth examples output missing %q:\n%s", check, out)
		}
	}
	if strings.Contains(out, "Docker example:") || strings.Contains(out, "Alias failover example:") {
		t.Fatalf("auth examples output unexpectedly included other sections:\n%s", out)
	}
}

func TestExamplesAliasCommandPrintsAliasExample(t *testing.T) {
	cmd := newRootCommand()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"examples", "alias"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute(): %v", err)
	}

	out := buf.String()
	checks := []string{
		"Auth example:",
		"Alias failover example:",
		"alias \"chat_default\"",
		"provider \"openai-compatible\" \"backup\"",
	}
	for _, check := range checks {
		if !strings.Contains(out, check) {
			t.Fatalf("alias examples output missing %q:\n%s", check, out)
		}
	}
	if strings.Contains(out, "Docker example:") || strings.Contains(out, "systemd example:") || strings.Contains(out, "Minimal config:") {
		t.Fatalf("alias examples output unexpectedly included other sections:\n%s", out)
	}
}

func TestExamplesDockerCommandPrintsDockerExample(t *testing.T) {
	cmd := newRootCommand()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"examples", "docker"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute(): %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Docker example:") || !strings.Contains(out, "docker run --rm \\") {
		t.Fatalf("docker examples output missing expected content:\n%s", out)
	}
	if strings.Contains(out, "systemd example:") || strings.Contains(out, "Minimal config:") {
		t.Fatalf("docker examples output unexpectedly included other sections:\n%s", out)
	}
}

func TestExamplesSystemdCommandPrintsSystemdExample(t *testing.T) {
	cmd := newRootCommand()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"examples", "systemd"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute(): %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "systemd example:") || !strings.Contains(out, "ExecStart=/usr/local/bin/aiproxy serve --config /etc/aiproxy/config.hcl") {
		t.Fatalf("systemd examples output missing expected content:\n%s", out)
	}
	if strings.Contains(out, "Docker example:") || strings.Contains(out, "Minimal config:") {
		t.Fatalf("systemd examples output unexpectedly included other sections:\n%s", out)
	}
}
